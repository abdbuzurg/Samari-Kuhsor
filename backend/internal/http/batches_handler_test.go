package http_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/testsupport"
)

// QR issuance — docs/01-DECISIONS.md D11.
//
// The rule that shapes every test here: wrappers are ordered externally against
// a planned batch volume, IN ADVANCE, and a printed wrapper cannot be corrected.

func createBatch(t *testing.T, h http.Handler, token, itemID, batchNo string) string {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/api/v1/batches", token, map[string]any{
		"batch_no": batchNo,
		"item_id":  itemID,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create batch %s: %d %s", batchNo, rec.Code, rec.Body)
	}
	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Data.ID
}

func issueQR(t *testing.T, h http.Handler, token, batchID string) string {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/api/v1/batches/"+batchID+"/qr", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("issue QR: %d %s", rec.Code, rec.Body)
	}
	var resp struct {
		Data struct {
			QRPayload  *string `json:"qr_payload"`
			QRIssuedAt *string `json:"qr_issued_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.QRPayload == nil {
		t.Fatal("no payload returned")
	}
	if resp.Data.QRIssuedAt == nil {
		t.Error("qr_issued_at was not stamped — the handoff date is part of the record")
	}
	return *resp.Data.QRPayload
}

func setupQR(t *testing.T) (http.Handler, *pgxpool.Pool, string, string) {
	t.Helper()
	h, pool := newServer(t)
	token := managerToken(t, h, pool, "m@samari-kuhsor.tj")
	itemID, _ := createItem(t, h, token, "APJ-1000", nil)
	return h, pool, token, itemID
}

// D11 — the code is generated before the plant produces anything. A batch with
// no production date and no output must still be able to receive one.
func TestQRIsIssuedForABatchThatHasNotBeenProduced(t *testing.T) {
	t.Parallel()
	h, pool, token, itemID := setupQR(t)

	batchID := createBatch(t, h, token, itemID, "B-2617")
	payload := issueQR(t, h, token, batchID)

	if !strings.HasSuffix(payload, "/b/B-2617") {
		t.Errorf("payload = %q, expected a URL ending /b/B-2617", payload)
	}

	// Issuing commits the company to ordering wrappers, so it is audited as a
	// decision (`approve`), not as an edit.
	testsupport.AssertAudited(t, pool, "items", uuid.MustParse(batchID), "approve")
}

// The central safety property. Re-issuing would produce a different payload and
// silently invalidate wrappers that may already be printed and in transit.
func TestQRCannotBeReissued(t *testing.T) {
	t.Parallel()
	h, _, token, itemID := setupQR(t)

	batchID := createBatch(t, h, token, itemID, "B-2617")
	first := issueQR(t, h, token, batchID)

	rec := do(t, h, http.MethodPost, "/api/v1/batches/"+batchID+"/qr", token, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("re-issue got %d, want 422 — %s", rec.Code, rec.Body)
	}
	if got := codeOf(t, rec); got != "business_rule" {
		t.Errorf("code = %q, want business_rule", got)
	}
	// The message must explain the consequence, not just refuse.
	if !strings.Contains(rec.Body.String(), "этикетки") {
		t.Errorf("the refusal does not explain why: %s", rec.Body)
	}

	// And the original payload is untouched.
	svg := do(t, h, http.MethodGet, "/api/v1/batches/"+batchID+"/qr.svg", token, nil)
	if svg.Code != http.StatusOK {
		t.Fatalf("qr.svg: %d", svg.Code)
	}
	second := issueQRPayloadFromDB(t, h, token, batchID)
	if second != first {
		t.Errorf("payload changed from %q to %q after a refused re-issue", first, second)
	}
}

func issueQRPayloadFromDB(t *testing.T, h http.Handler, token, batchID string) string {
	t.Helper()
	// Read it back through the export, which is the printer's own view.
	rec := do(t, h, http.MethodGet, "/api/v1/batches/qr-export", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("export: %d %s", rec.Code, rec.Body)
	}
	manifest := manifestOf(t, rec.Body.Bytes())
	for _, line := range strings.Split(manifest, "\n") {
		if strings.Contains(line, "B-2617") {
			fields := strings.Split(strings.TrimSpace(line), ";")
			if len(fields) >= 4 {
				return fields[3]
			}
		}
	}
	t.Fatal("batch not found in the export manifest")
	return ""
}

func TestQRSVGRendersAndIsScannableSize(t *testing.T) {
	t.Parallel()
	h, _, token, itemID := setupQR(t)

	batchID := createBatch(t, h, token, itemID, "B-2617")
	issueQR(t, h, token, batchID)

	rec := do(t, h, http.MethodGet, "/api/v1/batches/"+batchID+"/qr.svg", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "<svg") || !strings.HasSuffix(body, "</svg>") {
		t.Errorf("not a complete SVG: %.60s…", body)
	}
}

// A batch with no code yet is a 404 rather than an empty image: an empty SVG sent
// to a printer looks like a blank wrapper.
func TestQRSVGIsNotFoundBeforeIssuance(t *testing.T) {
	t.Parallel()
	h, _, token, itemID := setupQR(t)
	batchID := createBatch(t, h, token, itemID, "B-2617")

	rec := do(t, h, http.MethodGet, "/api/v1/batches/"+batchID+"/qr.svg", token, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestQRExportProducesTheHandoffArchive(t *testing.T) {
	t.Parallel()
	h, _, token, itemID := setupQR(t)

	for _, no := range []string{"B-2617", "B-2618", "B-2619"} {
		issueQR(t, h, token, createBatch(t, h, token, itemID, no))
	}
	// One batch deliberately left unissued: it must NOT appear in the export.
	createBatch(t, h, token, itemID, "B-2620")

	rec := do(t, h, http.MethodGet, "/api/v1/batches/qr-export", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, ".zip") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if n := rec.Header().Get("X-Batch-Count"); n != "3" {
		t.Errorf("X-Batch-Count = %q, want 3", n)
	}

	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("the archive is unreadable: %v", err)
	}

	svgs := 0
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".svg") {
			svgs++
		}
	}
	if svgs != 3 {
		t.Errorf("%d SVGs, want 3 — the unissued batch must not be exported", svgs)
	}

	manifest := manifestOf(t, rec.Body.Bytes())
	if strings.Contains(manifest, "B-2620") {
		t.Error("an unissued batch reached the printer manifest")
	}
	for _, want := range []string{"B-2617", "B-2618", "B-2619", "APJ-1000"} {
		if !strings.Contains(manifest, want) {
			t.Errorf("manifest is missing %s", want)
		}
	}
	// The product name is Russian; the printer opens this in Excel.
	if !strings.Contains(manifest, "Яблочный сок") {
		t.Errorf("manifest lost the Russian product name:\n%s", manifest)
	}
}

// Exporting nothing must be a clear refusal, not an empty archive the printer
// would treat as "no codes needed".
func TestQRExportRefusesWhenNothingIsIssued(t *testing.T) {
	t.Parallel()
	h, _, token, itemID := setupQR(t)
	createBatch(t, h, token, itemID, "B-2617") // created, not issued

	rec := do(t, h, http.MethodGet, "/api/v1/batches/qr-export", token, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 — %s", rec.Code, rec.Body)
	}
}

func TestBatchCreationValidation(t *testing.T) {
	t.Parallel()
	h, _, token, itemID := setupQR(t)

	cases := map[string]map[string]any{
		"missing batch_no": {"batch_no": "", "item_id": itemID},
		"unknown item":     {"batch_no": "B-2617", "item_id": uuid.NewString()},
		"malformed item":   {"batch_no": "B-2617", "item_id": "not-a-uuid"},
		"expiry before production": {
			"batch_no": "B-2617", "item_id": itemID,
			"produced_on": "2026-09-09", "expires_on": "2026-01-01",
		},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/batches", token, body)
			if rec.Code < 400 || rec.Code >= 500 {
				t.Errorf("got %d, want a 4xx — %s", rec.Code, rec.Body)
			}
		})
	}

	// A duplicate batch number is a validation error, not a crash.
	createBatch(t, h, token, itemID, "B-2617")
	rec := do(t, h, http.MethodPost, "/api/v1/batches", token,
		map[string]any{"batch_no": "B-2617", "item_id": itemID})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("duplicate batch: got %d, want 400 — %s", rec.Code, rec.Body)
	}
}

// A new batch is `in_production` and nothing here can change that: only quality
// events move a batch's status (docs/02-SCHEMA.md:197). Production staff must not
// be able to mark their own output released.
func TestBatchCreationCannotSetStatus(t *testing.T) {
	t.Parallel()
	h, _, token, itemID := setupQR(t)

	// The request type has no status field, so an attempt is rejected outright by
	// DecodeJSON's unknown-field check rather than silently ignored.
	rec := do(t, h, http.MethodPost, "/api/v1/batches", token, map[string]any{
		"batch_no": "B-2617", "item_id": itemID, "status": "released",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a status field was accepted: %d %s", rec.Code, rec.Body)
	}

	// And a batch created normally starts in_production.
	rec = do(t, h, http.MethodPost, "/api/v1/batches", token,
		map[string]any{"batch_no": "B-2619", "item_id": itemID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var resp struct {
		Data struct {
			Status struct {
				Key string `json:"key"`
			} `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Status.Key != "in_production" {
		t.Errorf("new batch status = %q, want in_production", resp.Data.Status.Key)
	}
}

// docs/05-MODULES.md:141 — Карантин is `danger`, not "in progress". Green means
// healthy (CLAUDE.md §5), so `released` is the only green batch status: a batch
// in quarantine is a problem until someone qualified says otherwise.
func TestBatchStatusLevels(t *testing.T) {
	t.Parallel()
	h, pool, token, itemID := setupQR(t)

	for status, want := range map[string]string{
		"in_production": "info",
		"quarantine":    "danger",
		"released":      "ok",
		"rejected":      "danger",
	} {
		t.Run(status, func(t *testing.T) {
			batchID := createBatch(t, h, token, itemID, "B-"+status)
			// Set the status directly: this test is about presentation. The
			// transitions themselves get the exhaustive matrix in T18.
			if _, err := pool.Exec(t.Context(),
				`UPDATE batches SET status = $2 WHERE id = $1`, batchID, status); err != nil {
				t.Fatal(err)
			}

			rec := do(t, h, http.MethodGet, "/api/v1/batches/"+batchID, token, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("get batch: %d %s", rec.Code, rec.Body)
			}
			var resp struct {
				Data struct {
					Status struct {
						Key   string `json:"key"`
						Level string `json:"level"`
						Label string `json:"label"`
					} `json:"status"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.Data.Status.Key != status {
				t.Errorf("key = %q, want %q", resp.Data.Status.Key, status)
			}
			if resp.Data.Status.Level != want {
				t.Errorf("%s → level %q, want %q", status, resp.Data.Status.Level, want)
			}
			if resp.Data.Status.Label == "" {
				t.Errorf("%s has no Russian label", status)
			}
		})
	}
}

// The permission matrix for the QR endpoints.
func TestQRPermissionMatrix(t *testing.T) {
	t.Parallel()
	h, pool := newServer(t)

	manage := managerToken(t, h, pool, "m@samari-kuhsor.tj")
	read := readerToken(t, h, pool, "r@samari-kuhsor.tj")
	seedUser(t, pool, "none@samari-kuhsor.tj")
	none := loginAs(t, h, "none@samari-kuhsor.tj")

	itemID, _ := createItem(t, h, manage, "APJ-1000", nil)
	batchID := createBatch(t, h, manage, itemID, "B-2617")

	cases := []struct {
		name                           string
		method, path                   string
		body                           any
		wantManage, wantRead, wantNone int
	}{
		{"create batch", http.MethodPost, "/api/v1/batches",
			map[string]any{"batch_no": "B-9999", "item_id": itemID}, 201, 403, 403},
		// Issuing commits the company to a wrapper order, so it needs manage.
		{"issue qr", http.MethodPost, "/api/v1/batches/" + batchID + "/qr", nil, 200, 403, 403},
		// Reading the export is items:read: the person sending it to the printer
		// is not necessarily the person who may edit the catalogue.
		{"export", http.MethodGet, "/api/v1/batches/qr-export", nil, 200, 200, 403},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rec := do(t, h, c.method, c.path, "", c.body); rec.Code != http.StatusUnauthorized {
				t.Errorf("unauthenticated: got %d, want 401", rec.Code)
			}
			if rec := do(t, h, c.method, c.path, none, c.body); rec.Code != c.wantNone {
				t.Errorf("no permission: got %d, want %d", rec.Code, c.wantNone)
			}
			if rec := do(t, h, c.method, c.path, read, c.body); rec.Code != c.wantRead {
				t.Errorf("items:read: got %d, want %d — %s", rec.Code, c.wantRead, rec.Body)
			}
			if rec := do(t, h, c.method, c.path, manage, c.body); rec.Code != c.wantManage {
				t.Errorf("items:manage: got %d, want %d — %s", rec.Code, c.wantManage, rec.Body)
			}
		})
	}
}

// manifestOf pulls manifest.csv out of an export archive.
func manifestOf(t *testing.T, archive []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("unreadable archive: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "manifest.csv" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			b, _ := io.ReadAll(rc)
			return string(bytes.TrimPrefix(b, []byte("\xEF\xBB\xBF")))
		}
	}
	t.Fatal("no manifest.csv in the archive")
	return ""
}
