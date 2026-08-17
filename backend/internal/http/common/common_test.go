package common_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/http/common"
)

// ---------------------------------------------------------------------------
// Error codes and status mapping — docs/03-API-CONTRACT.md §4
// ---------------------------------------------------------------------------

func TestEveryCodeMapsToItsDocumentedStatus(t *testing.T) {
	t.Parallel()
	want := map[common.Code]int{
		common.CodeValidationFailed: 400,
		common.CodeUnauthenticated:  401,
		common.CodeForbidden:        403,
		common.CodeNotFound:         404,
		common.CodeConflict:         409,
		common.CodeVersionConflict:  409,
		common.CodeBusinessRule:     422,
		common.CodeRateLimited:      429,
		common.CodeInternal:         500,
	}
	for code, status := range want {
		if got := code.Status(); got != status {
			t.Errorf("%s → %d, want %d", code, got, status)
		}
	}
	// An unrecognised code must fail closed, at 500 — never 200.
	if got := common.Code("something_new").Status(); got != 500 {
		t.Errorf("unknown code → %d, want 500", got)
	}
}

func TestEveryCodeHasARussianMessage(t *testing.T) {
	t.Parallel()
	codes := []common.Code{
		common.CodeValidationFailed, common.CodeUnauthenticated, common.CodeForbidden,
		common.CodeNotFound, common.CodeConflict, common.CodeVersionConflict,
		common.CodeBusinessRule, common.CodeRateLimited, common.CodeInternal,
	}
	for _, c := range codes {
		msg := common.New(c).Message
		if msg == "" {
			t.Errorf("%s has no message", c)
			continue
		}
		// docs/03-API-CONTRACT.md:117 — the message is Russian and user-facing.
		if !containsCyrillic(msg) {
			t.Errorf("%s message %q is not Russian", c, msg)
		}
	}
}

func containsCyrillic(s string) bool {
	for _, r := range s {
		if r >= 'А' && r <= 'я' {
			return true
		}
	}
	return false
}

// docs/03-API-CONTRACT.md:118 — never leak SQL, stack traces or internal
// identifiers. An unexpected error is exactly the one most likely to contain them.
func TestInternalErrorsNeverLeakTheCause(t *testing.T) {
	t.Parallel()
	leak := errors.New(`pq: relation "users" does not exist at /home/dev/backend/db.go:42`)

	rec := httptest.NewRecorder()
	common.Fail(rec, httptest.NewRequest(http.MethodGet, "/items", nil), fmt.Errorf("loading items: %w", leak))

	if rec.Code != 500 {
		t.Fatalf("status %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	for _, secret := range []string{"pq:", "relation", "/home/dev", "db.go"} {
		if strings.Contains(body, secret) {
			t.Errorf("response leaked %q:\n%s", secret, body)
		}
	}
	if !strings.Contains(body, string(common.CodeInternal)) {
		t.Errorf("response should still carry the stable code:\n%s", body)
	}
}

func TestErrorEnvelopeShape(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	common.Fail(rec, httptest.NewRequest(http.MethodPost, "/items", nil),
		common.Validation(common.FieldError{
			Field: "sku", Code: "already_exists", Message: "SKU уже используется",
		}))

	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Field   string `json:"field"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, rec.Body.String())
	}
	if got.Error.Code != "validation_failed" {
		t.Errorf("code = %q", got.Error.Code)
	}
	if len(got.Error.Details) != 1 || got.Error.Details[0].Field != "sku" {
		t.Errorf("details = %+v", got.Error.Details)
	}
	if rec.Code != 400 {
		t.Errorf("status %d, want 400", rec.Code)
	}
}

// Cyrillic must survive the wire as Cyrillic, not as \u0421\u041a\u0423 escapes.
func TestRussianTextIsNotEscaped(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	common.JSON(rec, 200, map[string]string{"name": "Яблочный сок прямого отжима"})
	if !strings.Contains(rec.Body.String(), "Яблочный сок") {
		t.Errorf("Cyrillic was escaped:\n%s", rec.Body.String())
	}
}

func TestAsErrorPassesThroughAndWraps(t *testing.T) {
	t.Parallel()
	original := common.NotFound()
	if got := common.AsError(fmt.Errorf("wrapped: %w", original)); got != original {
		t.Error("an *Error deeper in the chain was not recovered")
	}
	if got := common.AsError(errors.New("plain")); got.Code != common.CodeInternal {
		t.Errorf("a plain error became %s, want internal_error", got.Code)
	}
	if common.AsError(nil) != nil {
		t.Error("AsError(nil) should be nil")
	}
}

// ---------------------------------------------------------------------------
// Collection parameters — docs/03-API-CONTRACT.md §5
// ---------------------------------------------------------------------------

var itemsSort = common.SortSpec{
	Allowed:     []string{"sku", "created_at", "status"},
	Default:     "created_at",
	DefaultDesc: true,
}

func parse(t *testing.T, query string) (common.Params, error) {
	t.Helper()
	return common.ParseParams(httptest.NewRequest(http.MethodGet, "/items?"+query, nil), itemsSort)
}

func TestParamDefaults(t *testing.T) {
	t.Parallel()
	p, err := parse(t, "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Page != 1 || p.PerPage != 50 {
		t.Errorf("defaults are page=%d per_page=%d, want 1 and 50", p.Page, p.PerPage)
	}
	if p.SortField != "created_at" || !p.SortDesc {
		t.Errorf("default sort is %s desc=%v", p.SortField, p.SortDesc)
	}
}

func TestPerPageIsClampedNotRejected(t *testing.T) {
	t.Parallel()
	p, err := parse(t, "per_page=100000")
	if err != nil {
		t.Fatalf("an oversized per_page should be clamped, not rejected: %v", err)
	}
	if p.PerPage != common.MaxPerPage {
		t.Errorf("per_page = %d, want the %d maximum", p.PerPage, common.MaxPerPage)
	}
}

// docs/03-API-CONTRACT.md:138 — "Never accept arbitrary SQL fragments." The sort
// value reaches an ORDER BY clause, so an unknown field must be refused outright.
func TestUnknownSortFieldIsRejected(t *testing.T) {
	t.Parallel()
	// URL-encoded, as a real request would arrive — a raw space never reaches the
	// parser because the HTTP layer rejects it first.
	for _, bad := range []string{
		"password_hash",        // a real column, but not sortable: must not leak an ordering oracle
		"id; DROP TABLE items", // injection
		"(SELECT 1)",           // subquery
		"created_at, sku",      // smuggling a second column
		"nonexistent",
	} {
		t.Run(bad, func(t *testing.T) {
			_, err := parse(t, url.Values{"sort": {bad}}.Encode())
			if err == nil {
				t.Fatalf("sort=%q was accepted; it would reach an ORDER BY clause", bad)
			}
			if common.AsError(err).Code != common.CodeValidationFailed {
				t.Errorf("got %s, want validation_failed", common.AsError(err).Code)
			}
		})
	}
}

func TestSortDirectionPrefix(t *testing.T) {
	t.Parallel()
	asc, err := parse(t, "sort=sku")
	if err != nil {
		t.Fatal(err)
	}
	if asc.SortField != "sku" || asc.SortDesc {
		t.Errorf("sort=sku gave %s desc=%v", asc.SortField, asc.SortDesc)
	}
	desc, err := parse(t, "sort=-sku")
	if err != nil {
		t.Fatal(err)
	}
	if desc.SortField != "sku" || !desc.SortDesc {
		t.Errorf("sort=-sku gave %s desc=%v", desc.SortField, desc.SortDesc)
	}
}

// docs/03-API-CONTRACT.md:139 — "Every collection response is deterministic:
// always include a tiebreaker sort on id." Without it, rows with equal sort keys
// can swap between pages and the client silently sees one twice and misses another.
func TestOrderByAlwaysCarriesTheIdTiebreaker(t *testing.T) {
	t.Parallel()
	p, err := parse(t, "sort=status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.OrderBy(), "id") {
		t.Errorf("OrderBy() = %q, missing the id tiebreaker", p.OrderBy())
	}
}

func TestInvalidPageAndPerPageAreRejected(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"page=0", "page=-1", "page=abc", "per_page=0", "per_page=-5", "per_page=x"} {
		if _, err := parse(t, bad); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}
}

func TestOffsetAndLimit(t *testing.T) {
	t.Parallel()
	p, err := parse(t, "page=3&per_page=25")
	if err != nil {
		t.Fatal(err)
	}
	if p.Offset() != 50 || p.Limit() != 25 {
		t.Errorf("offset=%d limit=%d, want 50 and 25", p.Offset(), p.Limit())
	}
}

func TestSortSpecValidatesItself(t *testing.T) {
	t.Parallel()
	if err := itemsSort.Validate(); err != nil {
		t.Errorf("a well-formed spec was rejected: %v", err)
	}
	// A default outside its own whitelist would produce an ORDER BY on an
	// unvetted column name.
	bad := common.SortSpec{Allowed: []string{"sku"}, Default: "created_at"}
	if err := bad.Validate(); err == nil {
		t.Error("a default outside the whitelist should be refused at startup")
	}
	if err := (common.SortSpec{Allowed: []string{"sku"}}).Validate(); err == nil {
		t.Error("a spec with no default should be refused")
	}
}

func TestPageMeta(t *testing.T) {
	t.Parallel()
	p, _ := parse(t, "per_page=50")
	if got := common.NewPageMeta(p, 212); got.TotalPages != 5 {
		t.Errorf("212 items at 50/page = %d pages, want 5", got.TotalPages)
	}
	if got := common.NewPageMeta(p, 0); got.TotalPages != 1 {
		t.Errorf("an empty collection is %d pages, want 1", got.TotalPages)
	}
	if got := common.NewPageMeta(p, 50); got.TotalPages != 1 {
		t.Errorf("exactly one full page reported %d pages", got.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// Wire format — docs/03-API-CONTRACT.md §6
// ---------------------------------------------------------------------------

// The single most important serialisation rule in the system: money and
// quantities are STRINGS. A JSON number becomes a float64 in every JavaScript
// client, and floats corrupt currency (:147).
func TestMoneyAndQuantitySerialiseAsStrings(t *testing.T) {
	t.Parallel()

	body, err := json.Marshal(struct {
		Amount common.Money    `json:"amount"`
		Qty    common.Quantity `json:"qty"`
	}{
		Amount: common.NewMoney(decimal.RequireFromString("2480000.00")),
		Qty:    common.NewQuantity(decimal.RequireFromString("8640")),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got := string(body)
	if !strings.Contains(got, `"amount":"2480000.00"`) {
		t.Errorf("money is not a quoted string with 2 decimals: %s", got)
	}
	if !strings.Contains(got, `"qty":"8640.000"`) {
		t.Errorf("quantity is not a quoted string with 3 decimals: %s", got)
	}
	// And explicitly: no bare numbers.
	if strings.Contains(got, `:2480000`) || strings.Contains(got, `:8640`) {
		t.Errorf("a bare JSON number reached the wire: %s", got)
	}
}

// The value that motivates the rule: 0.1 + 0.2 must be exactly 0.3.
func TestDecimalArithmeticIsExact(t *testing.T) {
	t.Parallel()
	a := decimal.RequireFromString("0.1")
	b := decimal.RequireFromString("0.2")
	sum := common.NewMoney(a.Add(b))

	body, _ := json.Marshal(sum)
	if string(body) != `"0.30"` {
		t.Errorf("0.1 + 0.2 serialised as %s, want \"0.30\"", body)
	}
}

func TestMoneyRoundTrips(t *testing.T) {
	t.Parallel()
	for _, in := range []string{`"18.50"`, `"0.00"`, `"14000000.99"`, `18.5`} {
		var m common.Money
		if err := json.Unmarshal([]byte(in), &m); err != nil {
			t.Errorf("unmarshal %s: %v", in, err)
			continue
		}
		out, _ := json.Marshal(m)
		var back common.Money
		if err := json.Unmarshal(out, &back); err != nil {
			t.Errorf("re-unmarshal %s: %v", out, err)
			continue
		}
		if !back.Equal(m.Decimal) {
			t.Errorf("%s round-tripped to %s", in, out)
		}
	}
}

func TestNullMoneyAndQuantity(t *testing.T) {
	t.Parallel()
	if common.NullMoney(decimal.NullDecimal{}) != nil {
		t.Error("an absent money value should serialise as null")
	}
	if common.NullQuantity(decimal.NullDecimal{}) != nil {
		t.Error("an absent quantity should serialise as null")
	}
	m := common.NullMoney(decimal.NullDecimal{Decimal: decimal.RequireFromString("5"), Valid: true})
	if m == nil {
		t.Fatal("a present money value became null")
	}
	if b, _ := json.Marshal(m); string(b) != `"5.00"` {
		t.Errorf("got %s", b)
	}
}

// docs/03-API-CONTRACT.md:145 — RFC 3339 UTC. Formatting for Dushanbe (UTC+5) is
// the frontend's job; the API has exactly one timezone.
func TestTimestampsAreRFC3339UTC(t *testing.T) {
	t.Parallel()
	dushanbe := time.FixedZone("UTC+5", 5*60*60)
	ts := pgtype.Timestamptz{Time: time.Date(2026, 9, 9, 11, 30, 0, 0, dushanbe), Valid: true}

	if got := common.Timestamp(ts); got != "2026-09-09T06:30:00Z" {
		t.Errorf("Timestamp = %q, want 2026-09-09T06:30:00Z", got)
	}
	if common.NullTimestamp(pgtype.Timestamptz{}) != nil {
		t.Error("an absent timestamp should serialise as null")
	}
	d := pgtype.Date{Time: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC), Valid: true}
	if got := common.Date(d); got == nil || *got != "2026-09-09" {
		t.Errorf("Date = %v, want 2026-09-09", got)
	}
}

// docs/03-API-CONTRACT.md:177 — the backend decides the semantic level, never a
// React component. CLAUDE.md §5: green means healthy, not merely branded.
func TestStatusPayloadCarriesKeyAndLevel(t *testing.T) {
	t.Parallel()
	body, err := json.Marshal(common.NewStatus("quarantine", "Карантин", common.LevelDanger))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{`"key":"quarantine"`, `"level":"danger"`, `"label":"Карантин"`} {
		if !strings.Contains(got, want) {
			t.Errorf("status payload missing %s: %s", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Optimistic concurrency — docs/03-API-CONTRACT.md §7
// ---------------------------------------------------------------------------

func TestVersionGuard(t *testing.T) {
	t.Parallel()

	if err := common.CheckVersion(3, 3); err != nil {
		t.Errorf("matching versions were rejected: %v", err)
	}

	err := common.CheckVersion(3, 5)
	if err == nil {
		t.Fatal("a stale version was accepted — the guard does not work")
	}
	apiErr := common.AsError(err)
	if apiErr.Code != common.CodeVersionConflict {
		t.Errorf("code = %s, want version_conflict", apiErr.Code)
	}
	if apiErr.Code.Status() != 409 {
		t.Errorf("status = %d, want 409", apiErr.Code.Status())
	}
	// The current version comes back so the client need not re-read to find out.
	if len(apiErr.Details) != 1 || !strings.Contains(apiErr.Details[0].Message, "5") {
		t.Errorf("details do not report the current version: %+v", apiErr.Details)
	}
}

// A missing version must be a validation error, never a silent overwrite —
// otherwise the guard is opt-in and the one client that forgets it clobbers a
// colleague's edit.
func TestMissingVersionIsRejected(t *testing.T) {
	t.Parallel()
	if _, err := common.RequireVersion(common.Versioned{}); err == nil {
		t.Fatal("a PATCH with no version was accepted")
	}
	v := int32(0)
	if got, err := common.RequireVersion(common.Versioned{Version: &v}); err != nil || got != 0 {
		t.Errorf("an explicit version of 0 should be accepted: got %d err %v", got, err)
	}
}

// A typo in a field name must not be silently ignored, or the update appears to
// succeed while changing nothing.
func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	var dst struct {
		Category string `json:"category"`
	}
	r := httptest.NewRequest(http.MethodPatch, "/items/1",
		strings.NewReader(`{"catgeory":"juice"}`))
	if err := common.DecodeJSON(r, &dst); err == nil {
		t.Fatal("a misspelled field was silently ignored")
	}
}

func TestDecodeJSONRejectsTrailingContent(t *testing.T) {
	t.Parallel()
	var dst struct {
		Category string `json:"category"`
	}
	r := httptest.NewRequest(http.MethodPatch, "/items/1",
		strings.NewReader(`{"category":"juice"}{"category":"water"}`))
	if err := common.DecodeJSON(r, &dst); err == nil {
		t.Fatal("a second JSON object smuggled past the decoder")
	}
}

// ---------------------------------------------------------------------------
// Envelope — docs/03-API-CONTRACT.md §4
// ---------------------------------------------------------------------------

func TestSuccessEnvelope(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	common.JSON(rec, 200, map[string]string{"sku": "APJ-1000"})

	var got map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["data"]; !ok {
		t.Errorf("response is not wrapped in `data`: %s", rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	// Per-user, permission-filtered data must never be cached by an intermediary.
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestListEnvelopeCarriesMeta(t *testing.T) {
	t.Parallel()
	p, _ := parse(t, "page=2&per_page=50")
	rec := httptest.NewRecorder()
	common.List(rec, []string{"a", "b"}, common.NewPageMeta(p, 212))

	var got struct {
		Data []string        `json:"data"`
		Meta common.PageMeta `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Meta.Page != 2 || got.Meta.PerPage != 50 || got.Meta.Total != 212 || got.Meta.TotalPages != 5 {
		t.Errorf("meta = %+v", got.Meta)
	}
}

// A nil slice marshals to `null`, which breaks every client that maps over the
// result. An empty collection must be `[]`.
func TestEmptyCollectionIsAnArrayNotNull(t *testing.T) {
	t.Parallel()
	p, _ := parse(t, "")
	rec := httptest.NewRecorder()
	common.List(rec, []string{}, common.NewPageMeta(p, 0))

	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("empty collection serialised as %s, want \"data\":[]", rec.Body.String())
	}
}
