package rbac_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/qoim/samari/backend/internal/rbac"
)

// docs/04-RBAC.md §7 requires unit tests for permission resolution: union across
// roles, manage implying read, approve NOT implying manage, and a user with no
// roles having no access.

func TestManageImpliesRead(t *testing.T) {
	t.Parallel()
	s := rbac.NewSet([]string{"inventory:manage"})

	if !s.Can(rbac.Inventory, rbac.Read) {
		t.Error("manage must satisfy a read requirement (docs/04-RBAC.md:116)")
	}
	if !s.Can(rbac.Inventory, rbac.Manage) {
		t.Error("manage must satisfy manage")
	}
	if s.Can(rbac.Inventory, rbac.Approve) {
		t.Error("manage must NOT imply approve")
	}
}

// The distinction the whole quality module rests on: signing a batch out of
// quarantine is a different authority from editing a QC record (docs/01-DECISIONS.md D9).
func TestApproveDoesNotImplyManageOrRead(t *testing.T) {
	t.Parallel()
	s := rbac.NewSet([]string{"quality:approve"})

	if !s.Can(rbac.Quality, rbac.Approve) {
		t.Error("approve must satisfy approve")
	}
	if s.Can(rbac.Quality, rbac.Manage) {
		t.Error("approve must NOT imply manage")
	}
	if s.Can(rbac.Quality, rbac.Read) {
		t.Error("approve must NOT imply read")
	}
}

func TestReadDoesNotImplyManage(t *testing.T) {
	t.Parallel()
	s := rbac.NewSet([]string{"items:read"})
	if s.Can(rbac.Items, rbac.Manage) {
		t.Fatal("read must never satisfy manage")
	}
}

func TestUnionAcrossRoles(t *testing.T) {
	t.Parallel()
	// As if resolved from three roles, with items:read granted by two of them.
	s := rbac.NewSet([]string{
		"inventory:manage", "items:read",
		"quality:approve", "items:read",
		"dashboard:read",
	})

	for _, c := range []struct {
		resource string
		action   rbac.Action
		want     bool
	}{
		{rbac.Inventory, rbac.Manage, true},
		{rbac.Inventory, rbac.Read, true}, // via manage
		{rbac.Items, rbac.Read, true},
		{rbac.Items, rbac.Manage, false},
		{rbac.Quality, rbac.Approve, true},
		{rbac.Quality, rbac.Read, false},
		{rbac.Dashboard, rbac.Read, true},
		{rbac.HR, rbac.Read, false},
	} {
		if got := s.Can(c.resource, c.action); got != c.want {
			t.Errorf("Can(%s, %s) = %v, want %v", c.resource, c.action, got, c.want)
		}
	}

	if n := strings.Count(strings.Join(s.Strings(), " "), "items:read"); n != 1 {
		t.Errorf("items:read appears %d times in the flat list; the union must deduplicate", n)
	}
}

func TestNoRolesMeansNoAccess(t *testing.T) {
	t.Parallel()
	s := rbac.NewSet(nil)
	if !s.IsEmpty() {
		t.Error("an empty set should report empty")
	}
	for _, r := range []string{rbac.Items, rbac.Quality, rbac.Admin, rbac.Audit} {
		for _, a := range []rbac.Action{rbac.Read, rbac.Manage, rbac.Approve} {
			if s.Can(r, a) {
				t.Errorf("a user with no roles can %s:%s", r, a)
			}
		}
	}
	if got := s.ReadableResources(); len(got) != 0 {
		t.Errorf("a user with no roles sees modules: %v", got)
	}
}

// A malformed row in role_permissions must not lock a user out of everything.
func TestMalformedPermissionsAreIgnoredNotFatal(t *testing.T) {
	t.Parallel()
	s := rbac.NewSet([]string{"items:read", "garbage", "items:destroy", "quality:approve"})
	if !s.Can(rbac.Items, rbac.Read) {
		t.Error("a valid permission was dropped because a sibling was malformed")
	}
	if !s.Can(rbac.Quality, rbac.Approve) {
		t.Error("a valid permission was dropped because a sibling was malformed")
	}
	if len(s.Strings()) != 2 {
		t.Errorf("malformed entries survived: %v", s.Strings())
	}
}

func TestParsePermission(t *testing.T) {
	t.Parallel()
	p, err := rbac.ParsePermission("quality:approve")
	if err != nil {
		t.Fatalf("ParsePermission: %v", err)
	}
	if p.Resource != rbac.Quality || p.Action != rbac.Approve {
		t.Errorf("parsed %+v", p)
	}
	if p.String() != "quality:approve" {
		t.Errorf("round-trip produced %q", p.String())
	}
	for _, bad := range []string{"quality", "quality:", ":approve", "quality:destroy", ""} {
		if _, err := rbac.ParsePermission(bad); err == nil {
			t.Errorf("ParsePermission(%q) should have failed", bad)
		}
	}
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

func TestRequireReturns401Then403Then200(t *testing.T) {
	t.Parallel()

	handler := rbac.Require(rbac.Items, rbac.Manage)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	t.Run("unauthenticated is 401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/items", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rec.Code)
		}
	})

	t.Run("authenticated without the grant is 403, never 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/items", nil)
		req = req.WithContext(rbac.WithIdentity(req.Context(), rbac.Identity{
			UserID: "u1", Permissions: rbac.NewSet([]string{"items:read"}),
		}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("got %d, want 403 — a missing permission is never 404 and never a silent empty list", rec.Code)
		}
	})

	t.Run("authenticated with the grant is 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/items", nil)
		req = req.WithContext(rbac.WithIdentity(req.Context(), rbac.Identity{
			UserID: "u1", Permissions: rbac.NewSet([]string{"items:manage"}),
		}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// The startup check — docs/04-RBAC.md:123
// ---------------------------------------------------------------------------

func noop(w http.ResponseWriter, r *http.Request) {}

func TestVerifyPassesWhenEveryRouteIsDeclared(t *testing.T) {
	t.Parallel()
	reg := rbac.NewRegistry()
	r := chi.NewRouter()

	reg.Guarded(r, http.MethodGet, "/items", rbac.Items, rbac.Read, noop)
	reg.Guarded(r, http.MethodPost, "/items", rbac.Items, rbac.Manage, noop)
	reg.Guarded(r, http.MethodPost, "/batches/{id}/release", rbac.Quality, rbac.Approve, noop)
	reg.Public(r, http.MethodPost, "/auth/login", "login must work before permissions can be resolved", noop)

	if err := rbac.Verify(r, reg); err != nil {
		t.Fatalf("Verify rejected a fully declared router: %v", err)
	}
}

// The check exists to catch exactly this: a route mounted directly on chi,
// bypassing the registry, with no permission on it.
func TestVerifyFailsOnAnUndeclaredRoute(t *testing.T) {
	t.Parallel()
	reg := rbac.NewRegistry()
	r := chi.NewRouter()

	reg.Guarded(r, http.MethodGet, "/items", rbac.Items, rbac.Read, noop)
	r.Get("/secret-backdoor", noop) // forgotten

	err := rbac.Verify(r, reg)
	if err == nil {
		t.Fatal("Verify accepted a router with an undeclared route — the startup check does not work")
	}
	if !strings.Contains(err.Error(), "/secret-backdoor") {
		t.Errorf("the error should name the offending route, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Registry.Public") {
		t.Errorf("the error should say how to fix it, got: %v", err)
	}
}

// "Public" must be a decision someone wrote down, not the default you get by
// forgetting a middleware.
func TestPublicRequiresAReason(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("declaring a public route with no reason should panic at startup")
		}
	}()
	rbac.NewRegistry().Public(chi.NewRouter(), http.MethodGet, "/open", "   ", noop)
}

func TestGuardedRejectsUnknownResourceAndAction(t *testing.T) {
	t.Parallel()

	t.Run("unknown resource", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("an unknown resource should panic at startup, not 403 at runtime")
			}
		}()
		rbac.NewRegistry().Guarded(chi.NewRouter(), http.MethodGet, "/x", "widgets", rbac.Read, noop)
	})

	t.Run("unknown action", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("an unknown action should panic at startup")
			}
		}()
		rbac.NewRegistry().Guarded(chi.NewRouter(), http.MethodGet, "/x", rbac.Items, rbac.Action("destroy"), noop)
	})
}

// approve is only defined for five resources (docs/04-RBAC.md §3). Requiring it
// elsewhere would create a permission no role could sensibly hold, so it is a
// startup error rather than a route nobody can ever call.
func TestGuardedRejectsApproveWhereItIsNotDefined(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("items:approve should be refused — approve is not defined for items")
		}
	}()
	rbac.NewRegistry().Guarded(chi.NewRouter(), http.MethodPost, "/items/{id}/approve",
		rbac.Items, rbac.Approve, noop)
}

func TestDuplicateDeclarationIsRefused(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("declaring the same method+pattern twice should panic")
		}
	}()
	reg := rbac.NewRegistry()
	r := chi.NewRouter()
	reg.Guarded(r, http.MethodGet, "/items", rbac.Items, rbac.Read, noop)
	reg.Guarded(r, http.MethodGet, "/items", rbac.Items, rbac.Manage, noop)
}

func TestDeclarationsAreReportable(t *testing.T) {
	t.Parallel()
	reg := rbac.NewRegistry()
	r := chi.NewRouter()
	reg.Guarded(r, http.MethodPost, "/batches/{id}/release", rbac.Quality, rbac.Approve, noop)
	reg.Public(r, http.MethodPost, "/public/inquiries", "the website submits without a session", noop)

	ds := reg.Declarations()
	if len(ds) != 2 {
		t.Fatalf("got %d declarations, want 2", len(ds))
	}
	joined := ds[0].String() + "\n" + ds[1].String()
	if !strings.Contains(joined, "quality:approve") {
		t.Errorf("permission missing from the report:\n%s", joined)
	}
	if !strings.Contains(joined, "public — the website submits without a session") {
		t.Errorf("public reason missing from the report:\n%s", joined)
	}
}

func TestIdentityRoundTripsThroughContext(t *testing.T) {
	t.Parallel()
	if _, ok := rbac.IdentityFrom(context.Background()); ok {
		t.Error("a bare context should carry no identity")
	}
	ctx := rbac.WithIdentity(context.Background(), rbac.Identity{
		UserID: "u1", Permissions: rbac.NewSet([]string{"items:read"}),
	})
	ident, ok := rbac.IdentityFrom(ctx)
	if !ok || ident.UserID != "u1" || !ident.Permissions.CanRead(rbac.Items) {
		t.Errorf("identity did not round-trip: %+v ok=%v", ident, ok)
	}
}

func TestReadableResourcesDrivesTheNav(t *testing.T) {
	t.Parallel()
	// The seed "Склад" role from docs/04-RBAC.md §4.
	s := rbac.NewSet([]string{
		"dashboard:read", "items:read", "inventory:manage", "procurement:manage",
		"production:read", "quality:read", "logistics:read", "equipment:read", "documents:read",
	})
	got := strings.Join(s.ReadableResources(), " ")
	for _, want := range []string{"dashboard", "items", "inventory", "procurement", "quality"} {
		if !strings.Contains(got, want) {
			t.Errorf("%s should be visible to Склад", want)
		}
	}
	for _, notWant := range []string{"crm", "hr", "admin", "audit", "finance", "cms"} {
		if strings.Contains(got, notWant) {
			t.Errorf("%s must not be visible to Склад", notWant)
		}
	}
}
