// Package auth owns identity: password hashing, sessions, and the login lifecycle.
//
// Session transport, restated from docs/07-IMPLEMENTATION-PLAN.md I8 because it is
// the part most easily got wrong:
//
//	browser --httpOnly cookie--> Next.js BFF --Bearer session token--> Go
//
// The BFF forwards the opaque token; Go hashes it, looks up the session and
// resolves permissions itself, once per request, uncached (docs/04-RBAC.md:148).
// The BFF never sends a user id — that would move identity resolution out of Go
// and make the permission system forgeable by anything reaching the API port.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
)

// Failure modes. These are distinguished internally so the audit trail and logs
// are precise; the HTTP layer collapses the credential-related ones into a single
// `unauthenticated` response so the API never reveals whether an address exists.
var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrAccountLocked      = errors.New("auth: account locked")
	ErrAccountInactive    = errors.New("auth: account inactive")
	ErrSessionUnknown     = errors.New("auth: session not found")
	ErrSessionExpired     = errors.New("auth: session expired")
	ErrSessionRevoked     = errors.New("auth: session revoked")
)

// Config carries the session and lockout policy.
type Config struct {
	// MaxFailedAttempts before the account locks (docs/03-API-CONTRACT.md:192).
	MaxFailedAttempts int
	// LockoutDuration is how long a locked account stays locked.
	LockoutDuration time.Duration
	// IdleTimeout slides on activity: a session unused for this long is dead.
	IdleTimeout time.Duration
	// AbsoluteExpiry caps total session lifetime regardless of activity, so a
	// stolen token cannot be kept alive indefinitely by using it.
	AbsoluteExpiry time.Duration
}

func DefaultConfig() Config {
	return Config{
		MaxFailedAttempts: 5,
		LockoutDuration:   15 * time.Minute,
		IdleTimeout:       8 * time.Hour, // a factory shift
		AbsoluteExpiry:    30 * 24 * time.Hour,
	}
}

type Service struct {
	pool *pgxpool.Pool
	cfg  Config
}

func NewService(pool *pgxpool.Pool, cfg Config) *Service {
	return &Service{pool: pool, cfg: cfg}
}

// Identity is the resolved caller: who they are and what they may do.
type Identity struct {
	User        db.User
	SessionID   uuid.UUID
	Roles       []db.Role
	Permissions []string // "resource:action", the flat list /auth/me returns
}

// LoginRequest carries the credentials plus the request metadata recorded on the
// session and in the audit trail.
type LoginRequest struct {
	Email     string
	Password  string
	IP        *netip.Addr
	UserAgent string
}

// Login verifies credentials and issues a session.
//
// Returns the RAW token. It is never stored: only its SHA-256 hash goes to the
// database (docs/02-SCHEMA.md:77), so a database disclosure does not yield usable
// sessions. This is the only moment the raw token exists server-side.
func (s *Service) Login(ctx context.Context, req LoginRequest) (raw string, ident Identity, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", Identity{}, fmt.Errorf("auth: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	q := db.New(tx)

	user, err := q.GetUserByEmail(ctx, strings.TrimSpace(req.Email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Still spend the cost of a hash comparison. Returning early on an
			// unknown address makes login timing a user-enumeration oracle.
			_, _ = VerifyPassword(dummyHash, req.Password)
			return "", Identity{}, ErrInvalidCredentials
		}
		return "", Identity{}, fmt.Errorf("auth: lookup user: %w", err)
	}

	// Lockout is checked before the password. A locked account must reject even a
	// correct password, or the lock is decorative.
	if user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now()) {
		return "", Identity{}, ErrAccountLocked
	}
	if !user.IsActive {
		return "", Identity{}, ErrAccountInactive
	}

	ok, verifyErr := VerifyPassword(user.PasswordHash, req.Password)
	if verifyErr != nil {
		// A corrupt stored hash is an operational fault, not a failed attempt: do
		// not punish the user's counter for it.
		return "", Identity{}, fmt.Errorf("auth: verify: %w", verifyErr)
	}
	if !ok {
		if _, err := q.RecordLoginFailure(ctx, db.RecordLoginFailureParams{
			ID:          user.ID,
			MaxAttempts: int32(s.cfg.MaxFailedAttempts),
			Lockout:     interval(s.cfg.LockoutDuration),
		}); err != nil {
			return "", Identity{}, fmt.Errorf("auth: record failure: %w", err)
		}
		if err := tx.Commit(ctx); err != nil { // the counter increment must persist
			return "", Identity{}, fmt.Errorf("auth: commit failure counter: %w", err)
		}
		return "", Identity{}, ErrInvalidCredentials
	}

	raw, hash, err := newSessionToken()
	if err != nil {
		return "", Identity{}, err
	}

	// Expiry is the nearer of idle timeout and absolute expiry. TouchSession later
	// slides the idle window but can never push past the absolute cap.
	expires := time.Now().Add(s.cfg.IdleTimeout)
	if abs := time.Now().Add(s.cfg.AbsoluteExpiry); abs.Before(expires) {
		expires = abs
	}

	session, err := q.CreateSession(ctx, db.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: timestamptz(expires),
		Ip:        req.IP,
		UserAgent: strPtr(req.UserAgent),
	})
	if err != nil {
		return "", Identity{}, fmt.Errorf("auth: create session: %w", err)
	}

	if err := q.RecordLoginSuccess(ctx, user.ID); err != nil {
		return "", Identity{}, fmt.Errorf("auth: record success: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(user.ID),
		Action:     audit.ActionLogin,
		Resource:   "auth",
		ResourceID: audit.Target(session.ID),
		IP:         req.IP,
		After:      map[string]any{"user_agent": req.UserAgent},
	}); err != nil {
		return "", Identity{}, err
	}

	ident, err = s.identity(ctx, q, user, session.ID)
	if err != nil {
		return "", Identity{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", Identity{}, fmt.Errorf("auth: commit: %w", err)
	}
	return raw, ident, nil
}

// Authenticate resolves a raw session token to an identity, sliding the idle
// window. Called once per request by the HTTP middleware.
func (s *Service) Authenticate(ctx context.Context, raw string) (Identity, error) {
	q := db.New(s.pool)

	row, err := q.GetSessionByTokenHash(ctx, hashToken(raw))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Identity{}, ErrSessionUnknown
		}
		return Identity{}, fmt.Errorf("auth: lookup session: %w", err)
	}

	// Revocation is checked before expiry: a revoked session that also happens to
	// have expired should still report as revoked.
	if row.Session.RevokedAt.Valid {
		return Identity{}, ErrSessionRevoked
	}
	if !row.Session.ExpiresAt.Time.After(time.Now()) {
		return Identity{}, ErrSessionExpired
	}
	if !row.User.IsActive {
		return Identity{}, ErrAccountInactive
	}

	if err := q.TouchSession(ctx, db.TouchSessionParams{
		ID:        row.Session.ID,
		ExpiresAt: timestamptz(time.Now().Add(s.cfg.IdleTimeout)),
	}); err != nil {
		return Identity{}, fmt.Errorf("auth: touch session: %w", err)
	}

	return s.identity(ctx, q, row.User, row.Session.ID)
}

// Logout revokes a single session.
func (s *Service) Logout(ctx context.Context, ident Identity, ip *netip.Addr) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	if err := q.RevokeSession(ctx, ident.SessionID); err != nil {
		return fmt.Errorf("auth: revoke: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(ident.User.ID),
		Action:     audit.ActionLogout,
		Resource:   "auth",
		ResourceID: audit.Target(ident.SessionID),
		IP:         ip,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ChangePassword sets a new password and revokes every session for the user.
//
// Logout-everywhere on password change is required by docs/03-API-CONTRACT.md:193
// and is the point of changing a password after a suspected compromise: leaving
// existing sessions alive would leave the attacker logged in.
func (s *Service) ChangePassword(ctx context.Context, actor uuid.UUID, target uuid.UUID, newPassword string) error {
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	if err := q.SetPasswordHash(ctx, db.SetPasswordHashParams{ID: target, PasswordHash: hash}); err != nil {
		return fmt.Errorf("auth: set password: %w", err)
	}
	if err := q.RevokeAllUserSessions(ctx, target); err != nil {
		return fmt.Errorf("auth: revoke sessions: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionUpdate,
		Resource:   "auth",
		ResourceID: audit.Target(target),
		// Never record the password or its hash, in either direction.
		After: map[string]any{"password_changed": true, "sessions_revoked": true},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// identity loads roles and the flat permission list for a user.
func (s *Service) identity(ctx context.Context, q *db.Queries, user db.User, sessionID uuid.UUID) (Identity, error) {
	roles, err := q.GetUserRoles(ctx, user.ID)
	if err != nil {
		return Identity{}, fmt.Errorf("auth: load roles: %w", err)
	}
	perms, err := q.GetUserPermissions(ctx, user.ID)
	if err != nil {
		return Identity{}, fmt.Errorf("auth: load permissions: %w", err)
	}

	flat := make([]string, 0, len(perms))
	for _, p := range perms {
		flat = append(flat, p.Resource+":"+p.Action)
	}
	return Identity{User: user, SessionID: sessionID, Roles: roles, Permissions: flat}, nil
}

// newSessionToken returns a fresh raw token and the hash to store.
func newSessionToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("auth: read token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashToken(raw), nil
}

// hashToken is a plain SHA-256, deliberately not argon2: the token is 256 bits of
// entropy, so there is nothing to brute-force, and lookup happens on every single
// request. A slow KDF here would tax every API call for no security gain.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// dummyHash is a real argon2id hash of an unguessable value, used to equalise
// timing when the email is unknown.
var dummyHash = func() string {
	h, err := HashPassword("dummy-password-for-constant-time-login")
	if err != nil {
		panic("auth: cannot build dummy hash: " + err.Error())
	}
	return h
}()
