// Package http wires the API: middleware, routing and handlers.
//
// Topology (docs/03-API-CONTRACT.md §1):
//
//	Browser --httpOnly cookie--> Next.js BFF --Bearer + service key--> Go --> Postgres
//
// The browser never reaches this package. In production the api container
// publishes no host port at all, so it is reachable only by the two Next.js
// containers on the compose network (docs/07-IMPLEMENTATION-PLAN.md I8/I18).
package http

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/qoim/samari/backend/internal/auth"
	"github.com/qoim/samari/backend/internal/domain/batches"
	"github.com/qoim/samari/backend/internal/domain/items"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Config is the server's runtime configuration.
type Config struct {
	// ServiceKey proves the caller is one of our BFFs. It is NOT an identity:
	// it is the same for every request and says nothing about the user. Defence
	// in depth behind the unpublished port, never the only lock.
	ServiceKey string
}

type Server struct {
	auth    *auth.Service
	items   *items.Service
	batches *batches.Service
	cfg     Config
	router  chi.Router
	reg     *rbac.Registry
}

// NewServer builds the API. It returns an error rather than a Server if any route
// was registered without declaring its permission — the process must refuse to
// serve rather than expose an ungoverned endpoint (docs/04-RBAC.md:123).
func NewServer(authSvc *auth.Service, itemsSvc *items.Service, batchesSvc *batches.Service, cfg Config) (*Server, error) {
	s := &Server{auth: authSvc, items: itemsSvc, batches: batchesSvc, cfg: cfg, reg: rbac.NewRegistry()}

	// Sort whitelists are validated at startup: a default outside its own
	// whitelist would put an unvetted column name into an ORDER BY.
	if err := items.SortSpec.Validate(); err != nil {
		return nil, fmt.Errorf("items sort spec: %w", err)
	}

	// rbac cannot import this package (that would be a cycle), so it is given the
	// envelope-shaped responders here. Without this its 401/403 would be plain
	// text and would break the contract for exactly the two statuses every client
	// must branch on.
	rbac.Unauthorized = func(w http.ResponseWriter, r *http.Request) {
		common.Fail(w, r, common.Unauthenticated())
	}
	rbac.Forbidden = func(w http.ResponseWriter, r *http.Request) {
		common.Fail(w, r, common.Forbidden())
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// Deliberately NOT middleware.RealIP: it is deprecated for IP spoofing because
	// it rewrites RemoteAddr from X-Forwarded-For / X-Real-IP whether or not the
	// infrastructure actually sets them. clientIP() reads the header explicitly,
	// and only because nothing but the BFF can reach this port (I8/I18).
	r.Use(requestLogger)
	r.Use(recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(s.requireServiceKey)
		api.Use(s.resolveIdentity)

		// chi.Walk reports full paths, so declarations are recorded under the
		// same mount prefix (docs/04-RBAC.md:123).
		v1 := s.reg.Scope("/api/v1")

		v1.Public(api, http.MethodPost, "/auth/login",
			"login must succeed before any permission can be resolved", s.handleLogin)
		v1.Public(api, http.MethodPost, "/auth/logout",
			"revoking your own session needs no module permission", s.handleLogout)
		v1.Public(api, http.MethodGet, "/auth/me",
			"returns the caller's own identity and permission list", s.handleMe)
		v1.Public(api, http.MethodGet, "/health",
			"liveness probe; returns no data", handleHealth)

		// Товары и цены. Reads need items:read; writes need items:manage, which
		// implies read (docs/04-RBAC.md §3).
		v1.Guarded(api, http.MethodGet, "/items",
			rbac.Items, rbac.Read, s.handleListItems)
		v1.Guarded(api, http.MethodGet, "/items/{id}",
			rbac.Items, rbac.Read, s.handleGetItem)
		v1.Guarded(api, http.MethodPost, "/items",
			rbac.Items, rbac.Manage, s.handleCreateItem)
		v1.Guarded(api, http.MethodPatch, "/items/{id}",
			rbac.Items, rbac.Manage, s.handleUpdateItem)
		v1.Guarded(api, http.MethodDelete, "/items/{id}",
			rbac.Items, rbac.Manage, s.handleDeleteItem)
		v1.Guarded(api, http.MethodPost, "/items/{id}/prices",
			rbac.Items, rbac.Manage, s.handleAddItemPrice)

		// Batches and QR issuance (D11). QR generation is needed BEFORE the plant
		// produces anything, because wrappers are ordered in advance.
		v1.Guarded(api, http.MethodPost, "/batches",
			rbac.Items, rbac.Manage, s.handleCreateBatch)
		v1.Guarded(api, http.MethodGet, "/batches/{id}",
			rbac.Items, rbac.Read, s.handleGetBatch)
		v1.Guarded(api, http.MethodPost, "/batches/{id}/qr",
			rbac.Items, rbac.Manage, s.handleIssueBatchQR)
		v1.Guarded(api, http.MethodGet, "/batches/{id}/qr.svg",
			rbac.Items, rbac.Read, s.handleBatchQRSVG)
		v1.Guarded(api, http.MethodGet, "/batches/qr-export",
			rbac.Items, rbac.Read, s.handleExportQR)
	})

	if err := rbac.Verify(r, s.reg); err != nil {
		return nil, err
	}

	s.router = r
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.router }

// Declarations exposes the full access surface for the startup log, so every boot
// prints exactly which routes are guarded and which are deliberately public.
func (s *Server) Declarations() []rbac.Declaration { return s.reg.Declarations() }

func handleHealth(w http.ResponseWriter, r *http.Request) {
	common.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// requireServiceKey rejects anything that is not one of our BFFs.
func (s *Server) requireServiceKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.ServiceKey == "" { // not configured: development only
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get("X-Service-Key")
		// Constant-time: a byte-by-byte comparison leaks the key one byte at a
		// time to anything that can measure response latency.
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.ServiceKey)) != 1 {
			common.Fail(w, r, common.Unauthenticated())
			return
		}
		next.ServeHTTP(w, r)
	})
}

// resolveIdentity turns a Bearer token into a request-scoped identity.
//
// It does NOT reject anonymous requests: /auth/login and the public website
// endpoints are legitimately unauthenticated. Rejection is rbac.Require's job,
// which is what makes 401-versus-403 precise.
func (s *Server) resolveIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		ident, err := s.auth.Authenticate(r.Context(), token)
		if err != nil {
			// A bad token is anonymous, not an error: the route decides whether
			// anonymous is acceptable. Anything unexpected is still surfaced.
			if isSessionRejection(err) {
				next.ServeHTTP(w, r)
				return
			}
			common.Fail(w, r, err)
			return
		}

		ctx := withIdentity(r.Context(), ident)
		ctx = rbac.WithIdentity(ctx, rbac.Identity{
			UserID:      ident.User.ID.String(),
			Permissions: rbac.NewSet(ident.Permissions),
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isSessionRejection(err error) bool {
	return errors.Is(err, auth.ErrSessionUnknown) ||
		errors.Is(err, auth.ErrSessionExpired) ||
		errors.Is(err, auth.ErrSessionRevoked) ||
		errors.Is(err, auth.ErrAccountInactive)
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	scheme, token, found := strings.Cut(h, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// Request-scoped identity, kept separate from rbac.Identity: rbac needs only the
// permission set, while handlers need the full user record.
type identityCtxKey struct{}

func withIdentity(ctx context.Context, ident auth.Identity) context.Context {
	return context.WithValue(ctx, identityCtxKey{}, ident)
}

func identityFrom(r *http.Request) (auth.Identity, bool) {
	ident, ok := r.Context().Value(identityCtxKey{}).(auth.Identity)
	return ident, ok
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}

// recoverer turns a panic into a 500 in the API's envelope. chi's own recoverer
// writes a bare status, which would be the one response in the system not shaped
// like every other.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil && rec != http.ErrAbortHandler {
				slog.ErrorContext(r.Context(), "panic recovered",
					"panic", rec, "path", r.URL.Path)
				common.Fail(w, r, common.New(common.CodeInternal))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
