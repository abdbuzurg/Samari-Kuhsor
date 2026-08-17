package auth_test

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/auth"
	"github.com/qoim/samari/backend/internal/testsupport"
)

const testPassword = "правильный-пароль-42"

func testIP() *netip.Addr {
	a := netip.MustParseAddr("10.0.0.7")
	return &a
}

// newUser inserts an active user with a known password and returns its id.
func newUser(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var id uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ($1, 'Тест Пользователь', $2) RETURNING id`, email, hash).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func newService(t *testing.T) (*auth.Service, *pgxpool.Pool) {
	t.Helper()
	pool := testsupport.NewDB(t)
	return auth.NewService(pool, auth.DefaultConfig()), pool
}

func login(t *testing.T, s *auth.Service, email string) (string, auth.Identity) {
	t.Helper()
	raw, ident, err := s.Login(context.Background(), auth.LoginRequest{
		Email: email, Password: testPassword, IP: testIP(), UserAgent: "test-agent",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return raw, ident
}

func TestLoginIssuesASession(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	userID := newUser(t, pool, "warehouse@samari-kuhsor.tj")

	raw, ident, err := s.Login(context.Background(), auth.LoginRequest{
		Email: "warehouse@samari-kuhsor.tj", Password: testPassword,
		IP: testIP(), UserAgent: "test-agent",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if raw == "" {
		t.Fatal("login returned an empty token")
	}
	if ident.User.ID != userID {
		t.Errorf("identity is user %s, expected %s", ident.User.ID, userID)
	}

	// last_login_at is set and the failure counter is cleared.
	var failed int
	var lastLogin *time.Time
	err = pool.QueryRow(context.Background(),
		`SELECT failed_attempts, last_login_at FROM users WHERE id = $1`, userID).
		Scan(&failed, &lastLogin)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if failed != 0 {
		t.Errorf("failed_attempts = %d after a successful login, expected 0", failed)
	}
	if lastLogin == nil {
		t.Error("last_login_at was not recorded")
	}
}

// docs/02-SCHEMA.md:77 — store the hash, never the token. A database disclosure
// must not hand the attacker usable sessions.
func TestRawTokenIsNeverStored(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	newUser(t, pool, "director@samari-kuhsor.tj")
	raw, _ := login(t, s, "director@samari-kuhsor.tj")

	// Search every text column of every table for the raw token.
	rows, err := pool.Query(context.Background(), `
		SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND data_type IN ('text','character varying')`)
	if err != nil {
		t.Fatalf("list columns: %v", err)
	}
	type col struct{ table, column string }
	var cols []col
	for rows.Next() {
		var c col
		if err := rows.Scan(&c.table, &c.column); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		cols = append(cols, c)
	}
	rows.Close()

	for _, c := range cols {
		var n int
		q := `SELECT count(*) FROM ` + c.table + ` WHERE ` + c.column + ` = $1`
		if err := pool.QueryRow(context.Background(), q, raw).Scan(&n); err != nil {
			t.Fatalf("scan %s.%s: %v", c.table, c.column, err)
		}
		if n != 0 {
			t.Errorf("raw session token found in %s.%s — only its hash may be stored", c.table, c.column)
		}
	}

	// And the stored hash is genuinely a hash of it, not the token itself.
	var stored string
	if err := pool.QueryRow(context.Background(),
		`SELECT token_hash FROM sessions`).Scan(&stored); err != nil {
		t.Fatalf("read token_hash: %v", err)
	}
	if stored == raw {
		t.Fatal("sessions.token_hash holds the raw token")
	}
	if len(stored) != 64 { // sha256 hex
		t.Errorf("token_hash is %d chars, expected a 64-char sha256 hex digest", len(stored))
	}
}

func TestWrongPasswordIncrementsFailedAttempts(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	userID := newUser(t, pool, "ops@samari-kuhsor.tj")

	for i := 1; i <= 3; i++ {
		_, _, err := s.Login(context.Background(), auth.LoginRequest{
			Email: "ops@samari-kuhsor.tj", Password: "wrong", IP: testIP(),
		})
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: expected ErrInvalidCredentials, got %v", i, err)
		}
		var failed int
		if err := pool.QueryRow(context.Background(),
			`SELECT failed_attempts FROM users WHERE id = $1`, userID).Scan(&failed); err != nil {
			t.Fatalf("read counter: %v", err)
		}
		if failed != i {
			t.Fatalf("after %d failures the counter is %d", i, failed)
		}
	}
}

func TestAccountLocksAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	cfg := auth.DefaultConfig()
	cfg.MaxFailedAttempts = 3
	s := auth.NewService(pool, cfg)
	newUser(t, pool, "qc@samari-kuhsor.tj")

	for i := 0; i < 3; i++ {
		if _, _, err := s.Login(context.Background(), auth.LoginRequest{
			Email: "qc@samari-kuhsor.tj", Password: "wrong",
		}); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: got %v", i, err)
		}
	}

	// The point of a lock: the CORRECT password must now be refused too.
	// A lock that yields to the right password is decorative.
	_, _, err := s.Login(context.Background(), auth.LoginRequest{
		Email: "qc@samari-kuhsor.tj", Password: testPassword,
	})
	if !errors.Is(err, auth.ErrAccountLocked) {
		t.Fatalf("locked account accepted the correct password: %v", err)
	}
}

// The lock must release itself once locked_until passes.
//
// Deliberately not tested by sleeping out a short lockout: argon2 is designed to
// be slow, so a few failed attempts take longer than any lockout short enough to
// wait for, and the test would pass or fail depending on machine speed. Expiring
// the timestamp directly exercises the same branch — LockedUntil.After(now) — and
// is deterministic.
func TestLockExpires(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	cfg := auth.DefaultConfig()
	cfg.MaxFailedAttempts = 2
	cfg.LockoutDuration = time.Hour
	s := auth.NewService(pool, cfg)
	userID := newUser(t, pool, "shift@samari-kuhsor.tj")

	for i := 0; i < 2; i++ {
		_, _, _ = s.Login(context.Background(), auth.LoginRequest{
			Email: "shift@samari-kuhsor.tj", Password: "wrong",
		})
	}
	if _, _, err := s.Login(context.Background(), auth.LoginRequest{
		Email: "shift@samari-kuhsor.tj", Password: testPassword,
	}); !errors.Is(err, auth.ErrAccountLocked) {
		t.Fatalf("expected lock, got %v", err)
	}

	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET locked_until = now() - interval '1 second' WHERE id = $1`,
		userID); err != nil {
		t.Fatalf("expire lock: %v", err)
	}

	if _, _, err := s.Login(context.Background(), auth.LoginRequest{
		Email: "shift@samari-kuhsor.tj", Password: testPassword,
	}); err != nil {
		t.Fatalf("login after the lock expired: %v", err)
	}

	// A successful login clears the lock and the counter, so the next failure
	// starts from zero rather than immediately re-locking.
	var failed int
	var locked *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT failed_attempts, locked_until FROM users WHERE id = $1`, userID).
		Scan(&failed, &locked); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if failed != 0 || locked != nil {
		t.Errorf("after a successful login: failed_attempts=%d locked_until=%v, expected 0 and nil", failed, locked)
	}
}

func TestUnknownEmailAndInactiveAccount(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)

	_, _, err := s.Login(context.Background(), auth.LoginRequest{
		Email: "nobody@samari-kuhsor.tj", Password: testPassword,
	})
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("unknown email: got %v, expected ErrInvalidCredentials", err)
	}

	id := newUser(t, pool, "left@samari-kuhsor.tj")
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET is_active = false WHERE id = $1`, id); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, _, err := s.Login(context.Background(), auth.LoginRequest{
		Email: "left@samari-kuhsor.tj", Password: testPassword,
	}); !errors.Is(err, auth.ErrAccountInactive) {
		t.Errorf("inactive account: got %v, expected ErrAccountInactive", err)
	}
}

// A tombstoned user must not be able to log in — every read filters
// deleted_at IS NULL (CLAUDE.md §4.3).
func TestTombstonedUserCannotLogIn(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	id := newUser(t, pool, "gone@samari-kuhsor.tj")
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET deleted_at = now() WHERE id = $1`, id); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	if _, _, err := s.Login(context.Background(), auth.LoginRequest{
		Email: "gone@samari-kuhsor.tj", Password: testPassword,
	}); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("tombstoned user: got %v", err)
	}
}

func TestAuthenticateAcceptsAValidSession(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	userID := newUser(t, pool, "line1@samari-kuhsor.tj")
	raw, _ := login(t, s, "line1@samari-kuhsor.tj")

	ident, err := s.Authenticate(context.Background(), raw)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if ident.User.ID != userID {
		t.Errorf("resolved user %s, expected %s", ident.User.ID, userID)
	}
}

func TestAuthenticateRejectsUnknownExpiredAndRevoked(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	newUser(t, pool, "sessions@samari-kuhsor.tj")

	t.Run("unknown", func(t *testing.T) {
		if _, err := s.Authenticate(context.Background(), "not-a-real-token"); !errors.Is(err, auth.ErrSessionUnknown) {
			t.Errorf("got %v, expected ErrSessionUnknown", err)
		}
	})

	t.Run("expired", func(t *testing.T) {
		raw, ident := login(t, s, "sessions@samari-kuhsor.tj")
		if _, err := pool.Exec(context.Background(),
			`UPDATE sessions SET expires_at = now() - interval '1 second' WHERE id = $1`,
			ident.SessionID); err != nil {
			t.Fatalf("expire session: %v", err)
		}
		if _, err := s.Authenticate(context.Background(), raw); !errors.Is(err, auth.ErrSessionExpired) {
			t.Errorf("got %v, expected ErrSessionExpired", err)
		}
	})

	t.Run("revoked", func(t *testing.T) {
		raw, ident := login(t, s, "sessions@samari-kuhsor.tj")
		if err := s.Logout(context.Background(), ident, testIP()); err != nil {
			t.Fatalf("logout: %v", err)
		}
		if _, err := s.Authenticate(context.Background(), raw); !errors.Is(err, auth.ErrSessionRevoked) {
			t.Errorf("got %v, expected ErrSessionRevoked", err)
		}
	})
}

// Deactivating a user must take effect on their next request, not at their next
// login — otherwise a dismissed employee keeps working until their session lapses.
func TestAuthenticateRejectsDeactivatedUserMidSession(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	id := newUser(t, pool, "dismissed@samari-kuhsor.tj")
	raw, _ := login(t, s, "dismissed@samari-kuhsor.tj")

	if _, err := s.Authenticate(context.Background(), raw); err != nil {
		t.Fatalf("session should be valid before deactivation: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET is_active = false WHERE id = $1`, id); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := s.Authenticate(context.Background(), raw); !errors.Is(err, auth.ErrAccountInactive) {
		t.Errorf("got %v, expected ErrAccountInactive on the very next request", err)
	}
}

// docs/03-API-CONTRACT.md:194 — /auth/me returns the flat permission list, as the
// union across roles (docs/04-RBAC.md §1).
func TestIdentityResolvesUnionOfPermissions(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	userID := newUser(t, pool, "multi@samari-kuhsor.tj")
	ctx := context.Background()

	grant := func(roleKey string, perms ...[2]string) {
		var roleID uuid.UUID
		if err := pool.QueryRow(ctx, `
			INSERT INTO roles (key, name_ru, name_tg, name_en, is_system)
			VALUES ($1, $1, $1, $1, true) RETURNING id`, roleKey).Scan(&roleID); err != nil {
			t.Fatalf("insert role %s: %v", roleKey, err)
		}
		for _, p := range perms {
			if _, err := pool.Exec(ctx, `
				INSERT INTO role_permissions (role_id, resource, action)
				VALUES ($1, $2, $3)`, roleID, p[0], p[1]); err != nil {
				t.Fatalf("grant %s:%s: %v", p[0], p[1], err)
			}
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, userID, roleID); err != nil {
			t.Fatalf("assign role: %v", err)
		}
	}

	// Two roles, with items:read deliberately granted by both — the union must
	// deduplicate rather than list it twice.
	grant("warehouse", [2]string{"inventory", "manage"}, [2]string{"items", "read"})
	grant("quality", [2]string{"quality", "approve"}, [2]string{"items", "read"})

	_, ident := login(t, s, "multi@samari-kuhsor.tj")

	got := strings.Join(ident.Permissions, " ")
	for _, want := range []string{"inventory:manage", "items:read", "quality:approve"} {
		if !strings.Contains(got, want) {
			t.Errorf("permission %q missing from %v", want, ident.Permissions)
		}
	}
	if n := strings.Count(got, "items:read"); n != 1 {
		t.Errorf("items:read appears %d times; the union must deduplicate", n)
	}
	if len(ident.Roles) != 2 {
		t.Errorf("resolved %d roles, expected 2", len(ident.Roles))
	}
}

func TestUserWithNoRolesHasNoPermissions(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	newUser(t, pool, "norole@samari-kuhsor.tj")
	_, ident := login(t, s, "norole@samari-kuhsor.tj")
	if len(ident.Permissions) != 0 {
		t.Errorf("a user with no roles has permissions: %v", ident.Permissions)
	}
}

// CLAUDE.md §4.5 — every mutation writes an audit row. Login and logout are the
// first two in the system.
func TestLoginAndLogoutAreAudited(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	userID := newUser(t, pool, "audited@samari-kuhsor.tj")

	_, ident := login(t, s, "audited@samari-kuhsor.tj")
	entry := testsupport.AssertAudited(t, pool, "auth", ident.SessionID, "login")
	if entry.ActorID.UUID != userID {
		t.Errorf("login audited against actor %s, expected %s", entry.ActorID.UUID, userID)
	}

	if err := s.Logout(context.Background(), ident, testIP()); err != nil {
		t.Fatalf("logout: %v", err)
	}
	testsupport.AssertAudited(t, pool, "auth", ident.SessionID, "logout")
}

// A failed login must not be recorded as a login. The counter increment commits,
// but the audit trail must not claim someone signed in.
func TestFailedLoginIsNotAuditedAsLogin(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	newUser(t, pool, "failed@samari-kuhsor.tj")

	_, _, _ = s.Login(context.Background(), auth.LoginRequest{
		Email: "failed@samari-kuhsor.tj", Password: "wrong",
	})
	if n := testsupport.CountAudit(t, pool); n != 0 {
		t.Errorf("a failed login wrote %d audit rows, expected 0", n)
	}
}

// docs/03-API-CONTRACT.md:193 — logout everywhere on password change. Leaving
// sessions alive would leave an attacker signed in after the victim reacts.
func TestChangePasswordRevokesEverySession(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	userID := newUser(t, pool, "compromised@samari-kuhsor.tj")

	rawA, _ := login(t, s, "compromised@samari-kuhsor.tj")
	rawB, _ := login(t, s, "compromised@samari-kuhsor.tj")

	if err := s.ChangePassword(context.Background(), userID, userID, "новый-пароль-99"); err != nil {
		t.Fatalf("change password: %v", err)
	}

	for name, raw := range map[string]string{"first": rawA, "second": rawB} {
		if _, err := s.Authenticate(context.Background(), raw); !errors.Is(err, auth.ErrSessionRevoked) {
			t.Errorf("%s session survived the password change: %v", name, err)
		}
	}

	if _, _, err := s.Login(context.Background(), auth.LoginRequest{
		Email: "compromised@samari-kuhsor.tj", Password: "новый-пароль-99",
	}); err != nil {
		t.Errorf("the new password does not work: %v", err)
	}
	if _, _, err := s.Login(context.Background(), auth.LoginRequest{
		Email: "compromised@samari-kuhsor.tj", Password: testPassword,
	}); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("the old password still works: %v", err)
	}
}

// The audit trail must never carry the password or its hash, in either direction.
func TestPasswordChangeAuditDoesNotLeakTheSecret(t *testing.T) {
	t.Parallel()
	s, pool := newService(t)
	userID := newUser(t, pool, "secret@samari-kuhsor.tj")
	const newPassword = "очень-секретный-пароль"

	if err := s.ChangePassword(context.Background(), userID, userID, newPassword); err != nil {
		t.Fatalf("change password: %v", err)
	}
	entry := testsupport.AssertAudited(t, pool, "auth", userID, "update")

	blob := string(entry.Before) + string(entry.After)
	if strings.Contains(blob, newPassword) {
		t.Error("the audit entry contains the new password in plaintext")
	}
	if strings.Contains(blob, "argon2") {
		t.Error("the audit entry contains a password hash")
	}
}

// Sliding the idle window on activity is what stops an active user being logged
// out mid-shift (docs/03-API-CONTRACT.md:193).
func TestAuthenticateSlidesTheIdleWindow(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	cfg := auth.DefaultConfig()
	cfg.IdleTimeout = time.Hour
	s := auth.NewService(pool, cfg)
	newUser(t, pool, "active@samari-kuhsor.tj")

	raw, ident := login(t, s, "active@samari-kuhsor.tj")

	// Pull the expiry in, then use the session: it must be pushed back out.
	if _, err := pool.Exec(context.Background(),
		`UPDATE sessions SET expires_at = now() + interval '1 minute' WHERE id = $1`,
		ident.SessionID); err != nil {
		t.Fatalf("set expiry: %v", err)
	}
	if _, err := s.Authenticate(context.Background(), raw); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	var expires time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT expires_at FROM sessions WHERE id = $1`, ident.SessionID).Scan(&expires); err != nil {
		t.Fatalf("read expiry: %v", err)
	}
	if time.Until(expires) < 30*time.Minute {
		t.Errorf("expiry is %v away; the idle window did not slide", time.Until(expires))
	}
}
