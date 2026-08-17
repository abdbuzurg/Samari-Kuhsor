package http_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/testsupport"
)

// Товары и цены — the reference slice. docs/06-BUILD-PLAN.md §1 requires every
// module to cover the happy path, validation failure, 403 without permission,
// 401 unauthenticated, and an asserted audit row. This file is the template the
// next eleven modules copy.

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func itemPayload(sku string, overrides map[string]any) map[string]any {
	body := map[string]any{
		"sku":       sku,
		"item_type": "finished_good",
		"category":  "juice",
		"base_uom":  "bottle",
		"status":    "active",
		"translations": map[string]any{
			"ru": map[string]any{"name": "Яблочный сок прямого отжима"},
		},
		"packaging_units": []any{
			map[string]any{"code": "BOTTLE", "qty_in_base": "1.000"},
		},
	}
	for k, v := range overrides {
		if v == nil {
			delete(body, k)
			continue
		}
		body[k] = v
	}
	return body
}

// createItem posts a product and returns its id and version.
func createItem(t *testing.T, h http.Handler, token, sku string, overrides map[string]any) (string, int32) {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/api/v1/items", token, itemPayload(sku, overrides))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s: %d %s", sku, rec.Code, rec.Body)
	}
	var resp struct {
		Data struct {
			ID      string `json:"id"`
			Version int32  `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.Data.ID, resp.Data.Version
}

// manager can write items; reader can only read.
func managerToken(t *testing.T, h http.Handler, pool *pgxpool.Pool, email string) string {
	t.Helper()
	seedUser(t, pool, email, "items:manage")
	return loginAs(t, h, email)
}

func readerToken(t *testing.T, h http.Handler, pool *pgxpool.Pool, email string) string {
	t.Helper()
	seedUser(t, pool, email, "items:read")
	return loginAs(t, h, email)
}

// ---------------------------------------------------------------------------
// The permission matrix — docs/04-RBAC.md §7
// ---------------------------------------------------------------------------

// Every endpoint gets all three: permitted succeeds, authenticated-without-the-
// grant is 403, unauthenticated is 401.
func TestItemsPermissionMatrix(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)

	manage := managerToken(t, h, pool, "m@samari-kuhsor.tj")
	read := readerToken(t, h, pool, "r@samari-kuhsor.tj")
	seedUser(t, pool, "none@samari-kuhsor.tj") // no roles at all
	none := loginAs(t, h, "none@samari-kuhsor.tj")

	id, version := createItem(t, h, manage, "APJ-1000", nil)

	cases := []struct {
		name   string
		method string
		path   string
		body   any
		// what each token should get
		wantManage, wantRead, wantNone int
	}{
		{"list", http.MethodGet, "/api/v1/items", nil, 200, 200, 403},
		{"get", http.MethodGet, "/api/v1/items/" + id, nil, 200, 200, 403},
		{"create", http.MethodPost, "/api/v1/items", itemPayload("WAT-500", nil), 201, 403, 403},
		{"update", http.MethodPatch, "/api/v1/items/" + id,
			itemPayload("", map[string]any{
				"sku": nil, "item_type": nil, "packaging_units": nil, "version": version,
			}), 200, 403, 403},
		{"add price", http.MethodPost, "/api/v1/items/" + id + "/prices",
			map[string]any{"amount": "18.50", "valid_from": "2026-09-09"}, 201, 403, 403},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 401: no session at all.
			if rec := do(t, h, c.method, c.path, "", c.body); rec.Code != http.StatusUnauthorized {
				t.Errorf("unauthenticated: got %d, want 401", rec.Code)
			}
			// 403: authenticated, no grant on this resource. Never 404, never a
			// silently empty list (docs/04-RBAC.md:117).
			if rec := do(t, h, c.method, c.path, none, c.body); rec.Code != c.wantNone {
				t.Errorf("no permission: got %d, want %d — body %s", rec.Code, c.wantNone, rec.Body)
			}
			// items:read: reads pass, writes are 403. read must NOT imply manage.
			if rec := do(t, h, c.method, c.path, read, c.body); rec.Code != c.wantRead {
				t.Errorf("items:read: got %d, want %d — body %s", rec.Code, c.wantRead, rec.Body)
			}
			// items:manage: everything passes. manage implies read.
			if rec := do(t, h, c.method, c.path, manage, c.body); rec.Code != c.wantManage {
				t.Errorf("items:manage: got %d, want %d — body %s", rec.Code, c.wantManage, rec.Body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestCreateItemHappyPath(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")

	rec := do(t, h, http.MethodPost, "/api/v1/items", token, itemPayload("APJ-1000", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}

	var resp struct {
		Data struct {
			ID             string `json:"id"`
			SKU            string `json:"sku"`
			Version        int32  `json:"version"`
			Status         struct{ Key, Label, Level string }
			Translations   map[string]struct{ Name string }
			PackagingUnits []struct {
				Code      string `json:"code"`
				QtyInBase string `json:"qty_in_base"`
			} `json:"packaging_units"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body)
	}

	if resp.Data.SKU != "APJ-1000" || resp.Data.Version != 1 {
		t.Errorf("item = %+v", resp.Data)
	}
	// docs/03-API-CONTRACT.md:177 — the backend decides the level.
	if resp.Data.Status.Key != "active" || resp.Data.Status.Level != "ok" {
		t.Errorf("status = %+v, want key=active level=ok", resp.Data.Status)
	}
	if resp.Data.Translations["ru"].Name == "" {
		t.Error("the Russian name did not round-trip")
	}
	// Quantities are strings with three decimals (docs/03-API-CONTRACT.md:147).
	if len(resp.Data.PackagingUnits) != 1 || resp.Data.PackagingUnits[0].QtyInBase != "1.000" {
		t.Errorf("packaging = %+v", resp.Data.PackagingUnits)
	}

	id := uuid.MustParse(resp.Data.ID)
	testsupport.AssertAudited(t, pool, "items", id, "create")
}

// CLAUDE.md §4.5 — a mutation that is refused must leave no audit trail for work
// that never happened.
func TestRejectedCreateWritesNoAudit(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")

	before := testsupport.CountAudit(t, pool)
	rec := do(t, h, http.MethodPost, "/api/v1/items", token,
		itemPayload("", map[string]any{"sku": ""}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if after := testsupport.CountAudit(t, pool); after != before {
		t.Errorf("a rejected create wrote %d audit rows", after-before)
	}
}

func TestCreateItemValidation(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")

	cases := map[string]map[string]any{
		"missing sku":      {"sku": ""},
		"unknown type":     {"item_type": "widget"},
		"missing base uom": {"base_uom": ""},
		"unknown status":   {"status": "published"},
		"no translations":  {"translations": map[string]any{}},
		// docs/07 C2 — the prototype's `tj` must be refused with a clear message,
		// not surface as a CHECK-constraint 500.
		"tj locale": {"translations": map[string]any{"tj": map[string]any{"name": "Оби себ"}}},
		"empty name": {"translations": map[string]any{
			"ru": map[string]any{"name": "   "},
		}},
		"zero packaging qty": {"packaging_units": []any{
			map[string]any{"code": "BOTTLE", "qty_in_base": "0"},
		}},
		"negative packaging qty": {"packaging_units": []any{
			map[string]any{"code": "BOTTLE", "qty_in_base": "-5"},
		}},
		"duplicate packaging code": {"packaging_units": []any{
			map[string]any{"code": "BOTTLE", "qty_in_base": "1"},
			map[string]any{"code": "BOTTLE", "qty_in_base": "12"},
		}},
	}

	for name, overrides := range cases {
		t.Run(name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/items", token,
				itemPayload("TST-"+strings.ReplaceAll(name, " ", "-"), overrides))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 — %s", rec.Code, rec.Body)
			}
			if got := codeOf(t, rec); got != "validation_failed" {
				t.Errorf("code = %q", got)
			}
		})
	}
}

// D8 — raw materials are RAW-, packaging is PKG-. Finished goods keep the five
// approved codes, which share no prefix, so they are exempt.
func TestSKUPrefixRules(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")

	accepted := []struct{ sku, itemType string }{
		{"RAW-SUG-50", "raw_material"},
		{"PKG-CAP-82", "packaging"},
		{"APJ-1000", "finished_good"}, // no shared prefix, so unconstrained
		{"WAT-1000", "finished_good"},
	}
	for _, c := range accepted {
		t.Run("accepts "+c.sku, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/items", token,
				itemPayload(c.sku, map[string]any{"item_type": c.itemType}))
			if rec.Code != http.StatusCreated {
				t.Errorf("got %d, want 201 — %s", rec.Code, rec.Body)
			}
		})
	}

	refused := []struct{ sku, itemType string }{
		{"SUG-50", "raw_material"},     // missing RAW-
		{"CAP-82", "packaging"},        // missing PKG-
		{"PKG-SUG-50", "raw_material"}, // right shape, wrong prefix for the type
	}
	for _, c := range refused {
		t.Run("refuses "+c.sku, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/items", token,
				itemPayload(c.sku, map[string]any{"item_type": c.itemType}))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("got %d, want 400 — %s", rec.Code, rec.Body)
			}
		})
	}
}

// A duplicate SKU is the user's mistake, not a server fault: it must be a 400
// with a usable message, never a 500 carrying a Postgres constraint name.
func TestDuplicateSKUIsAValidationErrorNotACrash(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")

	createItem(t, h, token, "APJ-1000", nil)
	rec := do(t, h, http.MethodPost, "/api/v1/items", token, itemPayload("APJ-1000", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 — %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "already_exists") {
		t.Errorf("expected an already_exists field code: %s", body)
	}
	for _, leak := range []string{"23505", "pq:", "constraint", "items_sku_key"} {
		if strings.Contains(body, leak) {
			t.Errorf("response leaked %q: %s", leak, body)
		}
	}
}

// docs/02-SCHEMA.md:34 — tombstoning frees the unique key, so a discontinued
// product's SKU can be reused.
func TestTombstonedSKUCanBeReused(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")

	id, version := createItem(t, h, token, "APJ-1000", nil)
	rec := do(t, h, http.MethodDelete, "/api/v1/items/"+id, token,
		map[string]any{"version": version})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	testsupport.AssertAudited(t, pool, "items", uuid.MustParse(id), "delete")

	// The tombstoned record is gone from reads...
	if rec := do(t, h, http.MethodGet, "/api/v1/items/"+id, token, nil); rec.Code != http.StatusNotFound {
		t.Errorf("a tombstoned item is still readable: %d", rec.Code)
	}
	// ...and its SKU is free.
	if rec := do(t, h, http.MethodPost, "/api/v1/items", token, itemPayload("APJ-1000", nil)); rec.Code != http.StatusCreated {
		t.Errorf("SKU was not freed by the tombstone: %d %s", rec.Code, rec.Body)
	}
}

// ---------------------------------------------------------------------------
// Update and optimistic concurrency — docs/03-API-CONTRACT.md §7
// ---------------------------------------------------------------------------

func TestUpdateRequiresVersionAndRejectsStale(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")
	id, version := createItem(t, h, token, "APJ-1000", nil)

	patch := func(v any) map[string]any {
		return itemPayload("", map[string]any{
			"sku": nil, "item_type": nil, "packaging_units": nil,
			"version": v, "status": "draft",
		})
	}

	// A missing version must be refused, never treated as "whatever is current" —
	// otherwise the guard is opt-in and one forgetful client clobbers an edit.
	body := patch(nil)
	delete(body, "version")
	if rec := do(t, h, http.MethodPatch, "/api/v1/items/"+id, token, body); rec.Code != http.StatusBadRequest {
		t.Errorf("missing version: got %d, want 400 — %s", rec.Code, rec.Body)
	}

	// The correct version succeeds and bumps the stored version.
	rec := do(t, h, http.MethodPatch, "/api/v1/items/"+id, token, patch(version))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	var updated struct {
		Data struct {
			Version int32 `json:"version"`
			Status  struct{ Key string }
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Data.Version != version+1 {
		t.Errorf("version = %d, want %d", updated.Data.Version, version+1)
	}
	if updated.Data.Status.Key != "draft" {
		t.Errorf("status did not change: %+v", updated.Data.Status)
	}

	// Replaying the now-stale version is a 409, not a silent overwrite.
	stale := do(t, h, http.MethodPatch, "/api/v1/items/"+id, token, patch(version))
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale version: got %d, want 409 — %s", stale.Code, stale.Body)
	}
	if got := codeOf(t, stale); got != "version_conflict" {
		t.Errorf("code = %q, want version_conflict", got)
	}
	// The current version comes back, so the client need not re-read to recover.
	if !strings.Contains(stale.Body.String(), "2") {
		t.Errorf("the conflict does not report the current version: %s", stale.Body)
	}

	testsupport.AssertAudited(t, pool, "items", uuid.MustParse(id), "update")
}

// The audit entry must record what actually changed, in both directions.
func TestUpdateAuditRecordsBeforeAndAfter(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")
	id, version := createItem(t, h, token, "APJ-1000", nil)

	rec := do(t, h, http.MethodPatch, "/api/v1/items/"+id, token,
		itemPayload("", map[string]any{
			"sku": nil, "item_type": nil, "packaging_units": nil,
			"version": version, "status": "archived",
		}))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}

	entry := testsupport.AssertAudited(t, pool, "items", uuid.MustParse(id), "update")

	// Parsed, not substring-matched: Postgres reformats jsonb on storage, so
	// `"status":"active"` comes back as `"status": "active"` and a substring
	// assertion would fail for a reason that has nothing to do with the audit.
	var before, after map[string]any
	if err := json.Unmarshal(entry.Before, &before); err != nil {
		t.Fatalf("before is not valid JSON: %v", err)
	}
	if err := json.Unmarshal(entry.After, &after); err != nil {
		t.Fatalf("after is not valid JSON: %v", err)
	}
	if before["status"] != "active" {
		t.Errorf("before.status = %v, want active", before["status"])
	}
	if after["status"] != "archived" {
		t.Errorf("after.status = %v, want archived", after["status"])
	}
	// The projection must carry enough to identify the record, not just what moved.
	if before["sku"] != "APJ-1000" {
		t.Errorf("audit entry does not identify the item: %v", before)
	}
}

// ---------------------------------------------------------------------------
// List, search, filter, sort
// ---------------------------------------------------------------------------

func TestListSearchFilterSort(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")

	createItem(t, h, token, "APJ-1000", map[string]any{
		"category":     "juice",
		"translations": map[string]any{"ru": map[string]any{"name": "Яблочный сок прямого отжима"}},
	})
	createItem(t, h, token, "WAT-500", map[string]any{
		"category":     "water",
		"status":       "draft",
		"translations": map[string]any{"ru": map[string]any{"name": "Негазированная питьевая вода 0,5 л"}},
	})
	createItem(t, h, token, "RAW-SUG-50", map[string]any{
		"item_type":    "raw_material",
		"category":     "sugar",
		"base_uom":     "kg",
		"translations": map[string]any{"ru": map[string]any{"name": "Сахар"}},
	})

	list := func(query string) (skus []string, total int64) {
		t.Helper()
		rec := do(t, h, http.MethodGet, "/api/v1/items?"+query, token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("list %q: %d %s", query, rec.Code, rec.Body)
		}
		var resp struct {
			Data []struct {
				SKU  string `json:"sku"`
				Name string `json:"name"`
			} `json:"data"`
			Meta struct {
				Total int64 `json:"total"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v\n%s", err, rec.Body)
		}
		for _, r := range resp.Data {
			skus = append(skus, r.SKU)
		}
		return skus, resp.Meta.Total
	}

	t.Run("all three are listed", func(t *testing.T) {
		skus, total := list("")
		if total != 3 || len(skus) != 3 {
			t.Errorf("got %d rows, total %d", len(skus), total)
		}
	})

	t.Run("filter by item_type", func(t *testing.T) {
		skus, total := list("item_type=raw_material")
		if total != 1 || len(skus) != 1 || skus[0] != "RAW-SUG-50" {
			t.Errorf("got %v total %d", skus, total)
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		_, total := list("status=draft")
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
	})

	t.Run("search matches the SKU", func(t *testing.T) {
		skus, _ := list(url.Values{"q": {"wat"}}.Encode()) // case-insensitive
		if len(skus) != 1 || skus[0] != "WAT-500" {
			t.Errorf("got %v", skus)
		}
	})

	t.Run("search matches the Russian name", func(t *testing.T) {
		skus, _ := list(url.Values{"q": {"яблочный"}}.Encode())
		if len(skus) != 1 || skus[0] != "APJ-1000" {
			t.Errorf("got %v", skus)
		}
	})

	t.Run("search that matches nothing returns an empty array", func(t *testing.T) {
		rec := do(t, h, http.MethodGet,
			"/api/v1/items?"+url.Values{"q": {"нетнетнет"}}.Encode(), token, nil)
		// A nil slice would marshal to null and break every client that maps over it.
		if !strings.Contains(rec.Body.String(), `"data":[]`) {
			t.Errorf("empty result was not an array: %s", rec.Body)
		}
	})

	t.Run("sort ascending and descending", func(t *testing.T) {
		asc, _ := list("sort=sku")
		desc, _ := list("sort=-sku")
		if len(asc) != 3 || asc[0] != "APJ-1000" {
			t.Errorf("ascending = %v", asc)
		}
		if len(desc) != 3 || desc[0] != "WAT-500" {
			t.Errorf("descending = %v", desc)
		}
	})

	t.Run("unknown sort field is refused", func(t *testing.T) {
		rec := do(t, h, http.MethodGet,
			"/api/v1/items?"+url.Values{"sort": {"password_hash"}}.Encode(), token, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("got %d, want 400", rec.Code)
		}
	})

	t.Run("pagination metadata describes the filtered set", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/v1/items?per_page=2&page=1", token, nil)
		var resp struct {
			Data []any `json:"data"`
			Meta struct {
				Page       int   `json:"page"`
				PerPage    int   `json:"per_page"`
				Total      int64 `json:"total"`
				TotalPages int   `json:"total_pages"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if len(resp.Data) != 2 || resp.Meta.Total != 3 || resp.Meta.TotalPages != 2 {
			t.Errorf("meta = %+v with %d rows", resp.Meta, len(resp.Data))
		}
	})
}

// A tombstoned item must disappear from the list and from its count, or the
// pagination describes a different collection from the one returned.
func TestListExcludesTombstoned(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")

	id, version := createItem(t, h, token, "APJ-1000", nil)
	createItem(t, h, token, "WAT-500", nil)

	do(t, h, http.MethodDelete, "/api/v1/items/"+id, token, map[string]any{"version": version})

	rec := do(t, h, http.MethodGet, "/api/v1/items", token, nil)
	var resp struct {
		Data []struct {
			SKU string `json:"sku"`
		} `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Meta.Total != 1 || len(resp.Data) != 1 {
		t.Errorf("tombstoned item still listed: %d rows, total %d", len(resp.Data), resp.Meta.Total)
	}
}

// Search must find a product by its Tajik name even while the list renders in
// Russian — a Tajik-speaking operator should not have to know the Russian one.
func TestSearchMatchesEveryLocale(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")

	createItem(t, h, token, "APJ-1000", map[string]any{
		"translations": map[string]any{
			"ru": map[string]any{"name": "Яблочный сок"},
			"tg": map[string]any{"name": "Оби себи табиӣ"},
		},
	})

	rec := do(t, h, http.MethodGet,
		"/api/v1/items?"+url.Values{"q": {"себи"}}.Encode(), token, nil)
	if !strings.Contains(rec.Body.String(), "APJ-1000") {
		t.Errorf("Tajik name did not match: %s", rec.Body)
	}
}

// The list label follows the requested locale, falling back to Russian.
func TestListLabelUsesRequestedLocale(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")

	createItem(t, h, token, "APJ-1000", map[string]any{
		"translations": map[string]any{
			"ru": map[string]any{"name": "Яблочный сок"},
			"tg": map[string]any{"name": "Оби себи табиӣ"},
		},
	})
	// Russian only — must fall back rather than render blank.
	createItem(t, h, token, "WAT-500", map[string]any{
		"translations": map[string]any{"ru": map[string]any{"name": "Вода 0,5 л"}},
	})

	rec := do(t, h, http.MethodGet, "/api/v1/items?locale=tg", token, nil)
	body := rec.Body.String()
	if !strings.Contains(body, "Оби себи табиӣ") {
		t.Errorf("Tajik label missing: %s", body)
	}
	if !strings.Contains(body, "Вода 0,5 л") {
		t.Errorf("Russian fallback missing for an untranslated item: %s", body)
	}
}

// ---------------------------------------------------------------------------
// Prices
// ---------------------------------------------------------------------------

// Prices are never overwritten: a new one supersedes the open one and the
// history is kept, because it records what a product cost when an order was placed.
func TestAddPriceSupersedesRatherThanReplaces(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")
	id, _ := createItem(t, h, token, "APJ-1000", nil)

	for _, p := range []struct{ amount, from string }{
		{"18.00", "2026-01-01"},
		{"19.50", "2026-06-01"},
	} {
		rec := do(t, h, http.MethodPost, "/api/v1/items/"+id+"/prices", token,
			map[string]any{"amount": p.amount, "valid_from": p.from})
		if rec.Code != http.StatusCreated {
			t.Fatalf("add price %s: %d %s", p.amount, rec.Code, rec.Body)
		}
	}

	rec := do(t, h, http.MethodGet, "/api/v1/items/"+id, token, nil)
	var resp struct {
		Data struct {
			CurrentPrice *struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			} `json:"current_price"`
			PriceHistory []struct {
				Amount  string  `json:"amount"`
				ValidTo *string `json:"valid_to"`
			} `json:"price_history"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body)
	}

	if len(resp.Data.PriceHistory) != 2 {
		t.Fatalf("price history has %d entries, want 2 — the old price must be kept",
			len(resp.Data.PriceHistory))
	}
	if resp.Data.CurrentPrice == nil {
		t.Fatal("no current price")
	}
	// Money is a string with two decimals, and the base currency is TJS.
	if resp.Data.CurrentPrice.Amount != "19.50" {
		t.Errorf("current price = %q, want \"19.50\"", resp.Data.CurrentPrice.Amount)
	}
	if resp.Data.CurrentPrice.Currency != "TJS" {
		t.Errorf("currency = %q, want TJS", resp.Data.CurrentPrice.Currency)
	}
	// The superseded price was closed rather than deleted.
	closed := 0
	for _, p := range resp.Data.PriceHistory {
		if p.ValidTo != nil {
			closed++
		}
	}
	if closed != 1 {
		t.Errorf("%d prices are closed, want exactly 1", closed)
	}
}

func TestPriceValidation(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")
	id, _ := createItem(t, h, token, "APJ-1000", nil)

	cases := map[string]map[string]any{
		"negative amount": {"amount": "-1.00", "valid_from": "2026-09-09"},
		"unparseable":     {"amount": "около двадцати", "valid_from": "2026-09-09"},
		"missing from":    {"amount": "18.00", "valid_from": ""},
		"bad date":        {"amount": "18.00", "valid_from": "09.09.2026"},
		"to before from":  {"amount": "18.00", "valid_from": "2026-09-09", "valid_to": "2026-01-01"},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/items/"+id+"/prices", token, body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("got %d, want 400 — %s", rec.Code, rec.Body)
			}
		})
	}
}

// An item created before it is priced is the normal order of work, so a missing
// price must be null rather than a scan failure or a zero.
func TestItemWithNoPriceIsFine(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")
	id, _ := createItem(t, h, token, "APJ-1000", nil)

	for _, path := range []string{"/api/v1/items/" + id, "/api/v1/items"} {
		rec := do(t, h, http.MethodGet, path, token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), `"current_price":null`) {
			t.Errorf("%s: expected a null current_price, got %s", path, rec.Body)
		}
	}
}

// ---------------------------------------------------------------------------
// Unverified claims — docs/02-SCHEMA.md:176
// ---------------------------------------------------------------------------

// Composition, nutrition and shelf life stay null until lab-verified. The API
// must return null rather than "" so the UI can render «уточняется» and never
// publish an unverified claim.
func TestUnverifiedFieldsStayNull(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")
	id, _ := createItem(t, h, token, "APJ-1000", nil)

	rec := do(t, h, http.MethodGet, "/api/v1/items/"+id, token, nil)
	var resp struct {
		Data struct {
			ShelfLifeDays *int32 `json:"shelf_life_days"`
			Translations  map[string]struct {
				Ingredients *string `json:"ingredients"`
				Nutrition   *string `json:"nutrition"`
			} `json:"translations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ShelfLifeDays != nil {
		t.Errorf("shelf_life_days = %v, want null until lab-verified", *resp.Data.ShelfLifeDays)
	}
	ru := resp.Data.Translations["ru"]
	if ru.Ingredients != nil || ru.Nutrition != nil {
		t.Errorf("composition/nutrition are set: %+v", ru)
	}
}

// ---------------------------------------------------------------------------
// Not found
// ---------------------------------------------------------------------------

func TestGetUnknownAndMalformedIDs(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")

	for name, id := range map[string]string{
		"unknown uuid": uuid.NewString(),
		"not a uuid":   "not-a-uuid",
		"sql fragment": "1%20OR%201=1",
	} {
		t.Run(name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, "/api/v1/items/"+id, token, nil)
			if rec.Code != http.StatusNotFound {
				t.Errorf("got %d, want 404 — %s", rec.Code, rec.Body)
			}
			if got := codeOf(t, rec); got != "not_found" {
				t.Errorf("code = %q", got)
			}
		})
	}
}

// The stock ledger is the only source of quantities. If a balance column ever
// appears on items, this endpoint would start returning it — so assert it does not.
func TestItemsNeverExposeAStockQuantity(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")
	id, _ := createItem(t, h, token, "APJ-1000", nil)

	for _, path := range []string{"/api/v1/items", "/api/v1/items/" + id} {
		body := do(t, h, http.MethodGet, path, token, nil).Body.String()
		for _, forbidden := range []string{"quantity_on_hand", "on_hand", "stock_qty", "balance"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s exposes %q; stock is derived from the ledger only (CLAUDE.md §4.2)",
					path, forbidden)
			}
		}
	}
}
