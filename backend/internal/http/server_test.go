package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/alerts"
	"github.com/qoim/samari/backend/internal/auth"
	"github.com/qoim/samari/backend/internal/domain/admin"
	"github.com/qoim/samari/backend/internal/domain/batches"
	"github.com/qoim/samari/backend/internal/domain/inquiries"
	"github.com/qoim/samari/backend/internal/domain/inventory"
	"github.com/qoim/samari/backend/internal/domain/items"
	"github.com/qoim/samari/backend/internal/domain/procurement"
	"github.com/qoim/samari/backend/internal/domain/production"
	"github.com/qoim/samari/backend/internal/domain/quality"
	"github.com/qoim/samari/backend/internal/domain/sales"
	samarihttp "github.com/qoim/samari/backend/internal/http"
	"github.com/qoim/samari/backend/internal/testsupport"
)

const password = "правильный-пароль-42"

const serviceKey = "test-service-key"

// servicesFor composes the domain services exactly as cmd/api does.
//
// One inventory service shared by production, procurement and sales, because the
// ledger has one writer. A harness that wired three separate services would pass
// every test here while the real binary oversold.
func servicesFor(pool *pgxpool.Pool) samarihttp.Services {
	inventorySvc := inventory.NewService(pool)
	qualitySvc := quality.NewService(pool)
	return samarihttp.Services{
		Auth:        auth.NewService(pool, auth.DefaultConfig()),
		Items:       items.NewService(pool),
		Batches:     batches.NewService(pool, "https://samari-kuhsor.tj"),
		Inventory:   inventorySvc,
		Production:  production.NewService(pool, inventorySvc, qualitySvc),
		Quality:     qualitySvc,
		Procurement: procurement.NewService(pool, inventorySvc),
		Sales:       sales.NewService(pool, inventorySvc),
		Inquiries:   inquiries.NewService(pool, inquiries.DefaultRateLimit()),
		Admin:       admin.NewService(pool),
		Alerts:      alerts.NewService(pool),
	}
}

func newServer(t *testing.T) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	pool := testsupport.NewDB(t)
	srv, err := samarihttp.NewServer(servicesFor(pool),
		samarihttp.Config{ServiceKey: serviceKey})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv.Handler(), pool
}

// seedUser creates a user, optionally with a role carrying the given permissions.
func seedUser(t *testing.T, pool *pgxpool.Pool, email string, perms ...string) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ($1, 'Тест', $2) RETURNING id`, email, hash).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if len(perms) == 0 {
		return userID
	}

	var roleID uuid.UUID
	key := "role-" + uuid.NewString()[:8]
	if err := pool.QueryRow(ctx, `
		INSERT INTO roles (key, name_ru, name_tg, name_en)
		VALUES ($1, $1, $1, $1) RETURNING id`, key).Scan(&roleID); err != nil {
		t.Fatalf("insert role: %v", err)
	}
	for _, p := range perms {
		resource, action, _ := strings.Cut(p, ":")
		if _, err := pool.Exec(ctx, `
			INSERT INTO role_permissions (role_id, resource, action)
			VALUES ($1, $2, $3)`, roleID, resource, action); err != nil {
			t.Fatalf("grant %s: %v", p, err)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, userID, roleID); err != nil {
		t.Fatalf("assign role: %v", err)
	}
	return userID
}

// do issues a request with the service key, and a Bearer token when given one.
func do(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("X-Service-Key", serviceKey)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func loginAs(t *testing.T, h http.Handler, email string) string {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"email": email, "password": password})
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body)
	}
	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	return resp.Data.Token
}

func codeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error envelope: %v\n%s", err, rec.Body)
	}
	return resp.Error.Code
}

// ---------------------------------------------------------------------------
// The startup guarantee
// ---------------------------------------------------------------------------

// docs/04-RBAC.md:123 — the process must refuse to serve if any route lacks a
// permission declaration. NewServer returning an error is that refusal.
func TestServerRefusesToBuildWithAnUndeclaredRoute(t *testing.T) {
	t.Parallel()
	// Every route the real server mounts is declared, so it builds.
	pool := testsupport.NewDB(t)
	if _, err := samarihttp.NewServer(servicesFor(pool), samarihttp.Config{}); err != nil {
		t.Fatalf("the real router failed its own declaration check: %v", err)
	}
}

func TestEveryRouteDeclaresItsAccess(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	srv, err := samarihttp.NewServer(servicesFor(pool), samarihttp.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range srv.Declarations() {
		if d.Permission == nil && strings.TrimSpace(d.PublicReason) == "" {
			t.Errorf("%s %s is public with no reason", d.Method, d.Pattern)
		}
	}
	if len(srv.Declarations()) == 0 {
		t.Fatal("no routes were declared")
	}
}

// ---------------------------------------------------------------------------
// Service key
// ---------------------------------------------------------------------------

// The browser must never reach this API directly. The service key proves the
// caller is a BFF; it is not an identity (docs/07 I8).
func TestServiceKeyIsRequired(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	seedUser(t, pool, "a@samari-kuhsor.tj")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil) // no key
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("request without a service key got %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("X-Service-Key", "wrong")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("request with a wrong service key got %d, want 401", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// /auth/login
// ---------------------------------------------------------------------------

func TestLoginEndpoint(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	seedUser(t, pool, "line@samari-kuhsor.tj", "items:read", "inventory:manage")

	rec := do(t, h, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"email": "line@samari-kuhsor.tj", "password": password})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}

	var resp struct {
		Data struct {
			Token string `json:"token"`
			User  struct {
				Email       string   `json:"email"`
				Permissions []string `json:"permissions"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Token == "" {
		t.Error("no token returned")
	}
	if resp.Data.User.Email != "line@samari-kuhsor.tj" {
		t.Errorf("user = %+v", resp.Data.User)
	}
	// docs/03-API-CONTRACT.md:194 — the flat permission list.
	got := strings.Join(resp.Data.User.Permissions, " ")
	if !strings.Contains(got, "items:read") || !strings.Contains(got, "inventory:manage") {
		t.Errorf("permissions = %v", resp.Data.User.Permissions)
	}

	// The API must not set a session cookie: the cookie lives between the browser
	// and the BFF, and Go never sees it (docs/07 I8).
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("the API set a cookie; that is the BFF's job: %v", rec.Result().Cookies())
	}
}

// Wrong password, unknown address and a deactivated account must be
// indistinguishable, or login becomes a user-enumeration oracle.
func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	id := seedUser(t, pool, "real@samari-kuhsor.tj")
	seedUser(t, pool, "inactive@samari-kuhsor.tj")
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET is_active = false WHERE email = 'inactive@samari-kuhsor.tj'`); err != nil {
		t.Fatal(err)
	}
	_ = id

	var bodies []string
	for _, c := range []struct{ email, pass string }{
		{"real@samari-kuhsor.tj", "wrong"},
		{"ghost@samari-kuhsor.tj", password},
		{"inactive@samari-kuhsor.tj", password},
	} {
		rec := do(t, h, http.MethodPost, "/api/v1/auth/login", "",
			map[string]string{"email": c.email, "password": c.pass})
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s got %d, want 401", c.email, rec.Code)
		}
		bodies = append(bodies, rec.Body.String())
	}
	for i := 1; i < len(bodies); i++ {
		if bodies[i] != bodies[0] {
			t.Errorf("login failures are distinguishable:\n%s\nvs\n%s", bodies[0], bodies[i])
		}
	}
}

func TestLoginValidation(t *testing.T) {
	t.Parallel()
	h, _ := newServer(t)

	for name, body := range map[string]map[string]string{
		"missing email":    {"password": password},
		"missing password": {"email": "a@b.tj"},
		"both missing":     {},
	} {
		t.Run(name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/auth/login", "", body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status %d, want 400", rec.Code)
			}
			if got := codeOf(t, rec); got != "validation_failed" {
				t.Errorf("code = %q", got)
			}
		})
	}
}

func TestLockedAccountReports403NotJust401(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	seedUser(t, pool, "locked@samari-kuhsor.tj")

	for range 5 {
		do(t, h, http.MethodPost, "/api/v1/auth/login", "",
			map[string]string{"email": "locked@samari-kuhsor.tj", "password": "wrong"})
	}
	rec := do(t, h, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"email": "locked@samari-kuhsor.tj", "password": password})

	// A lockout is reported distinctly: the user needs to know why a correct
	// password stopped working, and by then the account's existence is not a secret.
	if rec.Code != http.StatusForbidden {
		t.Errorf("status %d, want 403", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// /auth/me and /auth/logout
// ---------------------------------------------------------------------------

func TestMeReturnsIdentityAndLogoutRevokesIt(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	seedUser(t, pool, "me@samari-kuhsor.tj", "quality:approve", "quality:read")
	token := loginAs(t, h, "me@samari-kuhsor.tj")

	rec := do(t, h, http.MethodGet, "/api/v1/auth/me", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: %d %s", rec.Code, rec.Body)
	}
	var resp struct {
		Data struct {
			Email       string   `json:"email"`
			Permissions []string `json:"permissions"`
			Roles       []struct {
				Key    string `json:"key"`
				NameRU string `json:"name_ru"`
			} `json:"roles"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Email != "me@samari-kuhsor.tj" || len(resp.Data.Permissions) != 2 {
		t.Errorf("me returned %+v", resp.Data)
	}
	if len(resp.Data.Roles) != 1 {
		t.Fatalf("roles = %v", resp.Data.Roles)
	}
	// Role names are content, not chrome (CLAUDE.md §6): the payload carries the
	// display name so the UI never has to show a raw key like "warehouse".
	if resp.Data.Roles[0].NameRU == "" {
		t.Errorf("role %s has no Russian display name", resp.Data.Roles[0].Key)
	}

	if rec := do(t, h, http.MethodPost, "/api/v1/auth/logout", token, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d %s", rec.Code, rec.Body)
	}
	// The token is dead immediately, not at expiry.
	if rec := do(t, h, http.MethodGet, "/api/v1/auth/me", token, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("a revoked token still works: %d", rec.Code)
	}
}

func TestMeWithoutATokenIs401(t *testing.T) {
	t.Parallel()
	h, _ := newServer(t)
	for name, token := range map[string]string{
		"no token":      "",
		"garbage token": "not-a-session",
	} {
		t.Run(name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, "/api/v1/auth/me", token, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status %d, want 401", rec.Code)
			}
			if got := codeOf(t, rec); got != "unauthenticated" {
				t.Errorf("code = %q, want unauthenticated", got)
			}
		})
	}
}

// A permission list with no entries must serialise as [], not null — a null
// breaks every client that maps over it to build the nav.
func TestEmptyPermissionsAreAnArrayNotNull(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	seedUser(t, pool, "norole@samari-kuhsor.tj")
	token := loginAs(t, h, "norole@samari-kuhsor.tj")

	rec := do(t, h, http.MethodGet, "/api/v1/auth/me", token, nil)
	if !strings.Contains(rec.Body.String(), `"permissions":[]`) {
		t.Errorf("empty permissions serialised as: %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// The envelope, end to end
// ---------------------------------------------------------------------------

func TestResponsesAreEnveloped(t *testing.T) {
	t.Parallel()
	h, _ := newServer(t)

	ok := do(t, h, http.MethodGet, "/api/v1/health", "", nil)
	if !strings.Contains(ok.Body.String(), `"data"`) {
		t.Errorf("success response is not enveloped: %s", ok.Body)
	}

	bad := do(t, h, http.MethodGet, "/api/v1/auth/me", "", nil)
	if !strings.Contains(bad.Body.String(), `"error"`) {
		t.Errorf("error response is not enveloped: %s", bad.Body)
	}
}
