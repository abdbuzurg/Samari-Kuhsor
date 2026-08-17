package http_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	samarihttp "github.com/qoim/samari/backend/internal/http"
	"github.com/qoim/samari/backend/internal/rbac"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// The permission matrix, applied to every route.
//
// docs/04-RBAC.md is enforced in Go middleware and nowhere else (CLAUDE.md §3):
// the frontend may hide a button, but hiding is not enforcement. These tests are
// what makes that claim true — they call every endpoint with the wrong permission
// and require a 403.
//
// They are deliberately table-driven off the route list rather than hand-written
// per handler, because the failure this guards against is a NEW route added
// without a guard, and a hand-written test only covers routes someone remembered.

// routeCase is one endpoint plus the permission it must demand.
type routeCase struct {
	method string
	path   string
	// needs is the permission that must be held. Empty means public.
	needs string
	body  string
}

// everyGuardedRoute mirrors the registrations in server.go.
//
// Kept as data so the "wrong permission is refused" test below covers all of them
// uniformly. TestRouteTableCoversEveryDeclaredRoute asserts this list has not
// fallen behind the router.
var everyGuardedRoute = []routeCase{
	{"GET", "/api/v1/dashboard", "dashboard:read", ""},
	{"GET", "/api/v1/items", "items:read", ""},
	{"GET", "/api/v1/items/" + zeroUUID, "items:read", ""},
	{"POST", "/api/v1/items", "items:manage", `{"sku":"X","item_type":"finished_good","base_uom":"bottle","translations":[]}`},
	{"PATCH", "/api/v1/items/" + zeroUUID, "items:manage", `{"version":1,"sku":"X","item_type":"finished_good","base_uom":"bottle","translations":[]}`},
	{"DELETE", "/api/v1/items/" + zeroUUID, "items:manage", ""},
	{"POST", "/api/v1/items/" + zeroUUID + "/prices", "items:manage", `{"price_type":"retail","amount":"10.00","valid_from":"2026-01-01"}`},
	{"POST", "/api/v1/batches", "items:manage", `{"batch_no":"B-1","item_id":"` + zeroUUID + `"}`},
	{"GET", "/api/v1/batches/" + zeroUUID, "items:read", ""},
	{"POST", "/api/v1/batches/" + zeroUUID + "/qr", "items:manage", ""},
	{"GET", "/api/v1/batches/" + zeroUUID + "/qr.svg", "items:read", ""},
	{"GET", "/api/v1/batches/qr-export", "items:read", ""},
	{"GET", "/api/v1/stock", "inventory:read", ""},
	{"GET", "/api/v1/stock/ledger", "inventory:read", ""},
	{"GET", "/api/v1/locations", "inventory:read", ""},
	{"POST", "/api/v1/stock/movements", "inventory:manage", `{"item_id":"` + zeroUUID + `","location_id":"` + zeroUUID + `","qty_delta":"1","reason":"goods_receipt"}`},
	{"POST", "/api/v1/stock/transfers", "inventory:manage", `{"item_id":"` + zeroUUID + `","from_location_id":"` + zeroUUID + `","to_location_id":"` + zeroUUID + `","qty":"1"}`},
	{"GET", "/api/v1/manufacturing-orders", "production:read", ""},
	{"POST", "/api/v1/manufacturing-orders", "production:manage", `{"mo_no":"MO-1","item_id":"` + zeroUUID + `","batch_no":"B-1","planned_qty":"1"}`},
	{"GET", "/api/v1/manufacturing-orders/" + zeroUUID, "production:read", ""},
	{"POST", "/api/v1/manufacturing-orders/" + zeroUUID + "/entries", "production:manage", `{"good_qty":"1"}`},
	{"POST", "/api/v1/manufacturing-orders/" + zeroUUID + "/complete", "production:manage", ""},
	{"GET", "/api/v1/quality/batches", "quality:read", ""},
	{"GET", "/api/v1/batches/" + zeroUUID + "/detail", "quality:read", ""},
	{"POST", "/api/v1/batches/" + zeroUUID + "/tests", "quality:manage", `{"test_type":"ph"}`},
	{"POST", "/api/v1/batches/" + zeroUUID + "/transition", "quality:manage", `{"to":"quarantine"}`},
	{"GET", "/api/v1/suppliers", "procurement:read", ""},
	{"POST", "/api/v1/suppliers", "procurement:manage", `{"name":"Тест"}`},
	{"GET", "/api/v1/purchase-orders", "procurement:read", ""},
	{"GET", "/api/v1/purchase-orders/" + zeroUUID, "procurement:read", ""},
	{"POST", "/api/v1/purchase-orders", "procurement:manage", `{"po_no":"PO-1","supplier_id":"` + zeroUUID + `","lines":[]}`},
	{"POST", "/api/v1/purchase-orders/" + zeroUUID + "/transition", "procurement:manage", `{"to":"approval"}`},
	{"POST", "/api/v1/purchase-orders/" + zeroUUID + "/receipts", "procurement:manage", `{"location_id":"` + zeroUUID + `","lines":[]}`},
	{"GET", "/api/v1/sales-orders", "crm:read", ""},
	{"GET", "/api/v1/sales-orders/" + zeroUUID, "crm:read", ""},
	{"POST", "/api/v1/sales-orders", "crm:manage", `{"so_no":"SO-1","customer_id":"` + zeroUUID + `","lines":[]}`},
	{"POST", "/api/v1/sales-orders/" + zeroUUID + "/confirm", "crm:manage", `{"location_id":"` + zeroUUID + `"}`},
	{"GET", "/api/v1/shipments", "logistics:read", ""},
	{"GET", "/api/v1/shipments/" + zeroUUID, "logistics:read", ""},
	{"POST", "/api/v1/shipments", "logistics:manage", `{"trip_no":"T-1"}`},
	{"POST", "/api/v1/shipments/" + zeroUUID + "/lines", "logistics:manage", `{"item_id":"` + zeroUUID + `","batch_id":"` + zeroUUID + `","qty":"1"}`},
	{"GET", "/api/v1/inquiries", "inquiries:read", ""},
	{"POST", "/api/v1/inquiries/" + zeroUUID + "/convert", "inquiries:manage", ""},
	{"GET", "/api/v1/employees", "hr:read", ""},
	{"POST", "/api/v1/employees", "hr:manage", `{"full_name":"Тест","version":0}`},
	{"PATCH", "/api/v1/employees/" + zeroUUID, "hr:manage", `{"full_name":"Тест","version":1}`},
	{"GET", "/api/v1/assets", "equipment:read", ""},
	{"POST", "/api/v1/assets", "equipment:manage", `{"asset_no":"EQ-1","name":"Линия","version":0}`},
	{"GET", "/api/v1/assets/" + zeroUUID + "/maintenance", "equipment:read", ""},
	{"POST", "/api/v1/assets/" + zeroUUID + "/maintenance", "equipment:manage", `{"event_type":"planned"}`},
	{"GET", "/api/v1/documents", "documents:read", ""},
	{"POST", "/api/v1/documents", "documents:manage", `{"doc_no":"D-1","title":"Тест","version":0}`},
	{"POST", "/api/v1/documents/" + zeroUUID + "/transition", "documents:manage", `{"to":"approval"}`},
	{"GET", "/api/v1/admin/roles", "admin:read", ""},
	{"POST", "/api/v1/admin/roles", "admin:manage", `{"key":"test","name":"Тест"}`},
	{"PUT", "/api/v1/admin/roles/" + zeroUUID + "/permissions", "admin:manage", `{"permissions":[]}`},
	{"DELETE", "/api/v1/admin/roles/" + zeroUUID, "admin:manage", ""},
	{"GET", "/api/v1/admin/users", "admin:read", ""},
	{"PUT", "/api/v1/admin/users/" + zeroUUID + "/roles", "admin:manage", `{"role_ids":[]}`},
	{"PUT", "/api/v1/admin/users/" + zeroUUID + "/active", "admin:manage", `{"active":false}`},
	{"GET", "/api/v1/admin/permissions", "admin:read", ""},
	{"GET", "/api/v1/audit", "audit:read", ""},
}

const zeroUUID = "00000000-0000-0000-0000-000000000000"

// Every guarded endpoint must refuse a caller who holds a permission on a
// different module. This is the whole RBAC claim, tested once per route.
func TestEveryGuardedRouteRefusesTheWrongPermission(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)

	// One user holding a grant that no route in the table demands. If any route
	// below answers them, that route is either unguarded or guarded on the wrong
	// resource.
	//
	// The grant is DERIVED rather than hard-coded: an earlier version used
	// documents:manage, which was irrelevant until Документы was built, at which
	// point three routes correctly admitted the "outsider" and the test failed for
	// the wrong reason.
	outsider := irrelevantPermission(t)
	seedUser(t, pool, "outsider@samari-kuhsor.tj", outsider)
	token := loginAs(t, handler, "outsider@samari-kuhsor.tj")

	for _, tc := range everyGuardedRoute {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			res := do(t, handler, tc.method, tc.path, token, bodyOf(tc.body))
			if res.Code != http.StatusForbidden {
				t.Errorf("status = %d holding only %s, want 403 (needs %s)",
					res.Code, outsider, tc.needs)
			}
			// And the refusal uses the documented envelope, so the BFF can branch
			// on it without parsing prose.
			var env struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &env); err != nil {
				t.Fatalf("refusal body was not the error envelope: %s", res.Body.String())
			}
			if env.Error.Code != "forbidden" {
				t.Errorf("error code = %q, want forbidden", env.Error.Code)
			}
		})
	}
}

// The mirror of the test above: holding the right permission must get past the
// guard. Without this, a route guarded on a permission nobody can hold would pass
// the refusal test and be permanently unusable.
//
// It asserts only "not 401/403" — the handler may still 404 or 422 on the fake
// IDs, which is the correct answer and not what this test is about.
func TestEveryGuardedRouteAdmitsTheRightPermission(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)

	// Collect every permission the table demands and grant them all to one user.
	perms := make([]string, 0, len(everyGuardedRoute))
	seen := map[string]bool{}
	for _, tc := range everyGuardedRoute {
		if tc.needs != "" && !seen[tc.needs] {
			seen[tc.needs] = true
			perms = append(perms, tc.needs)
		}
	}
	seedUser(t, pool, "insider@samari-kuhsor.tj", perms...)
	token := loginAs(t, handler, "insider@samari-kuhsor.tj")

	for _, tc := range everyGuardedRoute {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			res := do(t, handler, tc.method, tc.path, token, bodyOf(tc.body))
			if res.Code == http.StatusForbidden || res.Code == http.StatusUnauthorized {
				t.Errorf("status = %d holding %s — the guard is unreachable",
					res.Code, tc.needs)
			}
		})
	}
}

// A `manage` grant implies `read` (docs/04-RBAC.md §3), so a user who can manage
// a module can open its list. Asserted end-to-end because the implication lives
// in rbac.Set and the routes depend on it.
func TestManageImpliesReadOnEveryModule(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)

	reads := map[string]string{}
	for _, tc := range everyGuardedRoute {
		res, action, ok := strings.Cut(tc.needs, ":")
		if ok && action == "read" && tc.method == "GET" {
			reads[res] = tc.path
		}
	}

	i := 0
	for resource, path := range reads {
		email := fmt.Sprintf("manager%d@samari-kuhsor.tj", i)
		i++
		seedUser(t, pool, email, resource+":manage")
		token := loginAs(t, handler, email)
		res := do(t, handler, "GET", path, token, nil)
		if res.Code == http.StatusForbidden {
			t.Errorf("%s:manage was refused %s — manage must imply read", resource, path)
		}
	}
}

// An unauthenticated caller gets 401, not 403: the distinction tells the BFF
// whether to redirect to the login page or show "нет доступа"
// (docs/03-API-CONTRACT.md §4).
func TestGuardedRoutesRefuseAnonymousCallersWith401(t *testing.T) {
	t.Parallel()
	handler, _ := newServer(t)

	for _, tc := range everyGuardedRoute {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			res := do(t, handler, tc.method, tc.path, "", bodyOf(tc.body))
			if res.Code != http.StatusUnauthorized {
				t.Errorf("status = %d for an anonymous caller, want 401", res.Code)
			}
		})
	}
}

// The route table must not fall behind the router. A new guarded endpoint added
// to server.go without a line here would silently escape all of the above.
func TestRouteTableCoversEveryDeclaredRoute(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	srv := mustServer(t, pool)

	listed := map[string]bool{}
	for _, tc := range everyGuardedRoute {
		listed[tc.method+" "+normalisePattern(tc.path)] = true
	}

	var missing []string
	for _, d := range srv.Declarations() {
		if d.Permission == nil {
			continue // public routes are covered by their own tests
		}
		key := d.Method + " " + d.Pattern
		if !listed[key] {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Errorf("guarded routes with no entry in everyGuardedRoute:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// Each guarded route's table entry must name the permission the router actually
// declares. A drift here would make the refusal test assert against a permission
// nobody enforces.
func TestRouteTableMatchesTheDeclaredPermissions(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	srv := mustServer(t, pool)

	declared := map[string]string{}
	for _, d := range srv.Declarations() {
		if d.Permission != nil {
			declared[d.Method+" "+d.Pattern] = d.Permission.String()
		}
	}
	for _, tc := range everyGuardedRoute {
		key := tc.method + " " + normalisePattern(tc.path)
		got, ok := declared[key]
		if !ok {
			t.Errorf("%s is in the table but the router declares no such route", key)
			continue
		}
		if got != tc.needs {
			t.Errorf("%s: table says %s, router declares %s", key, tc.needs, got)
		}
	}
}

// irrelevantPermission returns a manage grant on a resource that no route in the
// table demands.
//
// It fails rather than guessing if every resource is now routed: at that point
// the refusal test needs a real answer — probably a resource invented for the
// purpose — and silently falling back to something admitted by three routes
// would make the whole suite pass while enforcing nothing.
func irrelevantPermission(t *testing.T) string {
	t.Helper()
	demanded := map[string]bool{}
	for _, tc := range everyGuardedRoute {
		res, _, _ := strings.Cut(tc.needs, ":")
		demanded[res] = true
	}
	for _, res := range rbac.Resources {
		if !demanded[res] {
			return res + ":manage"
		}
	}
	t.Fatal("every resource is now demanded by some route; the refusal test needs " +
		"a resource that no route uses")
	return ""
}

// normalisePattern turns a concrete test path back into the chi pattern, so the
// table can use real UUIDs while still matching the router's declarations.
func normalisePattern(path string) string {
	return strings.ReplaceAll(path, zeroUUID, "{id}")
}

// The public routes, listed explicitly. Each one carries a reason in server.go;
// this asserts the set has not grown, because "public" is the one property that
// must never be added by accident.
func TestPublicRoutesAreExactlyTheDocumentedSet(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	srv := mustServer(t, pool)

	want := map[string]bool{
		"POST /api/v1/auth/login":           true,
		"POST /api/v1/auth/logout":          true,
		"GET /api/v1/auth/me":               true,
		"GET /api/v1/health":                true,
		"POST /api/v1/public/inquiries":     true,
		"GET /api/v1/public/products":       true,
		"GET /api/v1/public/products/{sku}": true,
		"GET /api/v1/public/news":           true,
		// The alerts feed authenticates but is not guarded on a module: it filters
		// per-resource inside the service against the caller's own grants, so
		// guarding it on any single resource would be either too strict or a lie.
		"GET /api/v1/alerts":       true,
		"POST /api/v1/alerts/read": true,
	}
	for _, d := range srv.Declarations() {
		if d.Permission != nil {
			continue
		}
		key := d.Method + " " + d.Pattern
		if !want[key] {
			t.Errorf("%s is public and was not in the documented set — reason given: %q",
				key, d.PublicReason)
		}
		delete(want, key)
	}
	for key := range want {
		t.Errorf("%s was expected to be public but is not declared so", key)
	}
}

// The alerts feed authenticates even though it is not module-guarded. Public here
// means "no module permission", not "no session".
func TestAlertsFeedStillRequiresASession(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)

	if res := do(t, handler, "GET", "/api/v1/alerts", "", nil); res.Code != http.StatusUnauthorized {
		t.Errorf("anonymous alerts = %d, want 401", res.Code)
	}

	seedUser(t, pool, "reader@samari-kuhsor.tj", "inventory:read")
	token := loginAs(t, handler, "reader@samari-kuhsor.tj")
	res := do(t, handler, "GET", "/api/v1/alerts", token, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("authenticated alerts = %d: %s", res.Code, res.Body.String())
	}
	var env struct {
		Data struct {
			Counts map[string]int `json:"counts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	// Only the module they can read appears. A count for a module they cannot open
	// would leak how much work is outstanding in it.
	for resource := range env.Data.Counts {
		if resource != rbac.Inventory {
			t.Errorf("a viewer with only inventory:read saw a %s count", resource)
		}
	}
}

// The public inquiry endpoint is the one unauthenticated write. It must work with
// no session and still be behind the service key.
func TestPublicInquirySubmissionNeedsNoSessionButNeedsTheServiceKey(t *testing.T) {
	t.Parallel()
	handler, _ := newServer(t)
	body := map[string]string{"type": "contact", "name": "Гость", "contact": "+992 000 00 00"}

	res := do(t, handler, "POST", "/api/v1/public/inquiries", "", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("anonymous submission = %d: %s", res.Code, res.Body.String())
	}
	var env struct {
		Data struct {
			ReferenceNo string `json:"reference_no"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(env.Data.ReferenceNo, "CF-") {
		t.Errorf("reference = %q, want the CF- prefix", env.Data.ReferenceNo)
	}
	// And nothing else comes back: the visitor gets their receipt, not a copy of
	// what the system stored.
	var full map[string]any
	_ = json.Unmarshal(res.Body.Bytes(), &full)
	data, _ := full["data"].(map[string]any)
	if len(data) != 1 {
		t.Errorf("the receipt carried %d fields, want just reference_no: %v", len(data), data)
	}

	// Without the service key it is refused like everything else — "public" means
	// no user session, not an open door to the internet.
	req := httptest.NewRequest("POST", "/api/v1/public/inquiries",
		strings.NewReader(`{"type":"contact","name":"Гость","contact":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusCreated {
		t.Error("a submission with no service key was accepted")
	}
}

// bodyOf turns a table entry's raw JSON into something the shared `do` helper can
// encode. json.RawMessage passes through verbatim rather than being re-quoted as
// a string, and an empty body becomes nil so no Content-Type is sent.
func bodyOf(raw string) any {
	if raw == "" {
		return nil
	}
	return json.RawMessage(raw)
}

// mustServer builds the real router against a pool, for the declaration tests.
func mustServer(t *testing.T, pool *pgxpool.Pool) *samarihttp.Server {
	t.Helper()
	srv, err := samarihttp.NewServer(servicesFor(pool), samarihttp.Config{ServiceKey: serviceKey})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}
