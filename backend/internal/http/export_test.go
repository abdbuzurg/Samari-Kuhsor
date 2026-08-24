package http_test

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// R15 — CSV export. ToR §4 and §8 acceptance condition 7.
//
// The two things that matter and are easy to get wrong: an export that ignores
// the caller's permissions is a data leak wearing a download button, and an
// export that ignores the active filter is a different report from the screen
// it was launched from.

func TestExportRefusesACallerWithoutTheModulesRead(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)

	// Holds inventory, but not HR. Personnel data is the most sensitive payload
	// in the system and the export must not become a side door to it.
	seedUser(t, pool, "warehouse@samari-kuhsor.tj", "inventory:read")
	token := loginAs(t, handler, "warehouse@samari-kuhsor.tj")

	res := do(t, handler, http.MethodGet, "/api/v1/export/hr", token, nil)
	if res.Code != http.StatusForbidden {
		t.Fatalf("export/hr → %d holding only inventory:read, want 403", res.Code)
	}

	// And the one it may read still works.
	ok := do(t, handler, http.MethodGet, "/api/v1/export/stock", token, nil)
	if ok.Code != http.StatusOK {
		t.Fatalf("export/stock → %d holding inventory:read, want 200", ok.Code)
	}
}

func TestExportIsAnonymousProof(t *testing.T) {
	t.Parallel()
	handler, _ := newServer(t)

	res := do(t, handler, http.MethodGet, "/api/v1/export/items", "", nil)
	if res.Code != http.StatusUnauthorized {
		t.Errorf("anonymous export → %d, want 401", res.Code)
	}
}

// Excel on Windows reads a BOM-less UTF-8 CSV as the system codepage and renders
// every Cyrillic and Tajik character as mojibake. Three bytes are the difference
// between a usable report and a support call.
func TestExportIsUTF8WithABOMAndSemicolonDelimited(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)

	seedUser(t, pool, "exporter@samari-kuhsor.tj", "items:read")
	token := loginAs(t, handler, "exporter@samari-kuhsor.tj")

	res := do(t, handler, http.MethodGet, "/api/v1/export/items", token, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("export → %d %s", res.Code, truncate(res.Body.String()))
	}

	body := res.Body.Bytes()
	if len(body) < 3 || body[0] != 0xEF || body[1] != 0xBB || body[2] != 0xBF {
		t.Error("no UTF-8 BOM; Excel will render Cyrillic as mojibake")
	}
	if ct := res.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}
	if cd := res.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment", cd)
	}

	// A comma-delimited file opened in a Russian Windows locale lands entirely
	// in column A.
	r := csv.NewReader(strings.NewReader(string(body[3:])))
	r.Comma = ';'
	header, err := r.Read()
	if err != nil {
		t.Fatalf("not parseable as semicolon-delimited CSV: %v", err)
	}
	if len(header) < 2 {
		t.Errorf("header has %d columns, want the full set: %v", len(header), header)
	}
	if header[0] != "Артикул" {
		t.Errorf("first column = %q, want the Russian header", header[0])
	}
}

// The export re-enters the module's own list handler, so it carries whatever
// filter the screen had. A report that quietly ignores the filter is a different
// report from the one the user was looking at.
func TestExportCarriesTheActiveFilter(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)

	seedUser(t, pool, "filter@samari-kuhsor.tj", "items:manage")
	token := loginAs(t, handler, "filter@samari-kuhsor.tj")

	for _, sku := range []string{"APJ-1000", "WAT-500"} {
		createItem(t, handler, token, sku, nil)
	}

	all := rowsOf(t, do(t, handler, http.MethodGet, "/api/v1/export/items", token, nil))
	if len(all) != 2 {
		t.Fatalf("unfiltered export has %d rows, want 2", len(all))
	}

	filtered := rowsOf(t, do(t, handler, http.MethodGet, "/api/v1/export/items?q=WAT", token, nil))
	if len(filtered) != 1 {
		t.Errorf("filtered export has %d rows, want 1 — the filter was ignored", len(filtered))
	}
}

// A report that silently stops at one screen of results is worse than no report:
// it looks complete.
func TestExportIsNotLimitedToOnePageOfResults(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)

	seedUser(t, pool, "many@samari-kuhsor.tj", "items:manage")
	token := loginAs(t, handler, "many@samari-kuhsor.tj")

	const n = 30 // more than the default per_page
	for i := 0; i < n; i++ {
		createItem(t, handler, token, fmt.Sprintf("SKU-%03d", i), nil)
	}

	rows := rowsOf(t, do(t, handler, http.MethodGet, "/api/v1/export/items", token, nil))
	if len(rows) != n {
		t.Errorf("export has %d rows, want all %d", len(rows), n)
	}
}

func TestUnknownExportCollectionIs404(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)
	seedUser(t, pool, "unknown@samari-kuhsor.tj", "items:read")
	token := loginAs(t, handler, "unknown@samari-kuhsor.tj")

	res := do(t, handler, http.MethodGet, "/api/v1/export/nonsense", token, nil)
	if res.Code != http.StatusNotFound {
		t.Errorf("unknown collection → %d, want 404", res.Code)
	}
}

// rowsOf parses a CSV export body and returns its data rows, header excluded.
func rowsOf(t *testing.T, res interface{ Result() *http.Response }) [][]string {
	t.Helper()
	resp := res.Result()
	defer resp.Body.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	body := strings.TrimPrefix(string(buf), "\ufeff")
	r := csv.NewReader(strings.NewReader(body))
	r.Comma = ';'
	all, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(all) == 0 {
		return nil
	}
	return all[1:]
}
