package analytics_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/domain/analytics"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// R22's targeted tests — the guarantees an end-to-end run cannot reach.
//
// The E2E gate proves a click becomes a number in the CRM. It cannot prove that
// a forged SKU is refused, that deletion actually happens ninety days later, or
// that ten views in one session count once. Those are the properties the owner's
// ranking rests on, so they are asserted here against a real Postgres.

func fixture(t *testing.T) (*analytics.Service, *pgxpool.Pool, string) {
	t.Helper()
	pool := testsupport.NewDB(t)
	svc := analytics.NewService(pool, analytics.Config{
		IPSalt: "test-salt", MaxPerWindow: 300, Window: time.Hour, MaxBatch: 50,
	})

	var sku string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO items (sku, item_type, base_uom, status)
		VALUES ('APJ-1000', 'finished_good', 'bottle', 'active')
		RETURNING sku`).Scan(&sku)
	if err != nil {
		t.Fatalf("seed item: %v", err)
	}
	return svc, pool, sku
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// ---------------------------------------------------------------------------
// Targeted test 2 — a forged SKU is dropped
// ---------------------------------------------------------------------------

// The endpoint is unauthenticated and accepts "product X was viewed". Without
// this, anyone could make any product look popular, and the owner's ranking
// would be decoration rather than evidence.
func TestAProductViewNamingAnUnknownSKUIsDropped(t *testing.T) {
	t.Parallel()
	svc, pool, realSKU := fixture(t)
	ctx := context.Background()

	accepted, err := svc.Ingest(ctx, analytics.Batch{
		SessionID: "session-forged-01",
		IP:        "203.0.113.9",
		Events: []analytics.Event{
			{Kind: analytics.KindProductView, Target: "FAKE-999",
				Source: analytics.SourceProductPage, Locale: "ru"},
			{Kind: analytics.KindProductView, Target: realSKU,
				Source: analytics.SourceProductPage, Locale: "ru"},
		},
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// The real one lands, the forged one does not — and no error is returned,
	// because the endpoint answers 204 either way rather than telling a prober
	// which of their guesses worked.
	if accepted != 1 {
		t.Errorf("accepted = %d, want 1 (the real SKU only)", accepted)
	}
	if n := count(t, pool, "analytics_events"); n != 1 {
		t.Errorf("stored %d events, want 1", n)
	}
}

func TestUnknownKindsSourcesCategoriesAndLocalesAreDropped(t *testing.T) {
	t.Parallel()
	svc, pool, sku := fixture(t)
	ctx := context.Background()

	_, err := svc.Ingest(ctx, analytics.Batch{
		SessionID: "session-junk-01",
		IP:        "203.0.113.9",
		Events: []analytics.Event{
			{Kind: "keystroke", Target: "/ru", Locale: "ru"},
			{Kind: analytics.KindProductView, Target: sku, Source: "telepathy", Locale: "ru"},
			{Kind: analytics.KindLinkClick, Target: "/ru/contact", Category: "banner", Locale: "ru"},
			{Kind: analytics.KindPageView, Target: "/ru", Locale: "fr"},
			// A URL rather than a path: no host, no query, no fragment.
			{Kind: analytics.KindPageView, Target: "https://elsewhere.example/ru", Locale: "ru"},
			{Kind: analytics.KindPageView, Target: "/ru?utm_source=spam", Locale: "ru"},
			// A product_view with no source cannot be attributed to a surface.
			{Kind: analytics.KindProductView, Target: sku, Locale: "ru"},
			// A link_click with no category cannot be filtered out of the panel.
			{Kind: analytics.KindLinkClick, Target: "/ru/contact", Locale: "ru"},
		},
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if n := count(t, pool, "analytics_events"); n != 0 {
		t.Errorf("stored %d events, want 0 — every one of these is malformed", n)
	}
}

// A click attributed to a product the catalogue does not have would corrupt the
// conversion figure, so it is dropped rather than stored unattributed.
func TestAProductAttributedClickWithAnUnknownSKUIsDropped(t *testing.T) {
	t.Parallel()
	svc, pool, _ := fixture(t)
	ctx := context.Background()

	if _, err := svc.Ingest(ctx, analytics.Batch{
		SessionID: "session-attr-01", IP: "203.0.113.9",
		Events: []analytics.Event{{
			Kind: analytics.KindLinkClick, Target: "/ru/contact",
			Category: analytics.CategoryCTA, SKU: "FAKE-999", Locale: "ru",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, "analytics_events"); n != 0 {
		t.Errorf("stored %d, want 0", n)
	}
}

// ---------------------------------------------------------------------------
// Targeted test 4 — visits, not events
// ---------------------------------------------------------------------------

// The whole reason a session id is carried at all. One person refreshing ten
// times must not outrank ten people looking once — that was the argument against
// anonymous counting in D12, and it only holds if the rollup actually counts
// distinct sessions.
func TestTenViewsInOneSessionCountAsOneVisit(t *testing.T) {
	t.Parallel()
	svc, pool, sku := fixture(t)
	ctx := context.Background()

	yesterday := time.Now().AddDate(0, 0, -1)

	// One session, ten views.
	for i := 0; i < 10; i++ {
		if _, err := svc.Ingest(ctx, analytics.Batch{
			SessionID: "session-keen-0001", IP: "203.0.113.1",
			Events: []analytics.Event{{
				Kind: analytics.KindProductView, Target: sku,
				Source: analytics.SourceBeltModal, Locale: "ru",
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Three other sessions, once each.
	for i := 0; i < 3; i++ {
		if _, err := svc.Ingest(ctx, analytics.Batch{
			SessionID: fmt.Sprintf("session-other-%04d", i), IP: "203.0.113.2",
			Events: []analytics.Event{{
				Kind: analytics.KindProductView, Target: sku,
				Source: analytics.SourceProductPage, Locale: "ru",
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	backdate(t, pool, yesterday)
	if _, err := svc.Maintain(ctx); err != nil {
		t.Fatalf("maintain: %v", err)
	}

	report, err := svc.Report(ctx, "month", "ru", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Products) != 1 {
		t.Fatalf("got %d product rows, want 1", len(report.Products))
	}
	got := report.Products[0]
	if got.Visits != 4 {
		t.Errorf("visits = %d, want 4 (one keen session + three others)", got.Visits)
	}
	if got.Views != 13 {
		t.Errorf("views = %d, want 13", got.Views)
	}
}

// ---------------------------------------------------------------------------
// Targeted test 3 — retention really deletes, and history survives it
// ---------------------------------------------------------------------------

// Against an injected clock rather than by waiting ninety days. Two properties,
// and the second is the one that is easy to lose: the row is gone AND the day's
// count is still readable.
func TestRetentionDeletesPastNinetyDaysAndKeepsTheCounts(t *testing.T) {
	t.Parallel()
	svc, pool, sku := fixture(t)
	ctx := context.Background()

	seed := func(session string, age time.Duration) {
		if _, err := svc.Ingest(ctx, analytics.Batch{
			SessionID: session, IP: "203.0.113.3",
			Events: []analytics.Event{{
				Kind: analytics.KindProductView, Target: sku,
				Source: analytics.SourceProductPage, Locale: "ru",
			}},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			UPDATE analytics_events SET occurred_at = now() - $1::interval
			WHERE session_id = $2`, fmt.Sprintf("%d hours", int(age.Hours())), session); err != nil {
			t.Fatal(err)
		}
	}

	seed("session-old-000001", 91*24*time.Hour)
	seed("session-new-000001", 89*24*time.Hour)

	res, err := svc.Maintain(ctx)
	if err != nil {
		t.Fatalf("maintain: %v", err)
	}
	if res.RowsDeleted != 1 {
		t.Errorf("deleted %d rows, want 1", res.RowsDeleted)
	}
	if n := count(t, pool, "analytics_events"); n != 1 {
		t.Errorf("%d raw events survive, want 1 (the 89-day-old one)", n)
	}

	// The deleted day's count must still be readable, or retention has quietly
	// destroyed the history it was meant to outlive.
	var days int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM analytics_daily WHERE kind='product_view'`).Scan(&days); err != nil {
		t.Fatal(err)
	}
	if days != 2 {
		t.Errorf("rolled-up days = %d, want 2 — the deleted day's count must survive", days)
	}

	// And it is provable: a run writes exactly one audit row.
	var audits int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE resource = 'analytics'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Errorf("audit rows = %d, want exactly 1 per run (D12)", audits)
	}
}

// Ingestion writes NO audit row. One per click would bury the trail that proves
// who released a batch under web traffic within a week (D12).
func TestIngestionWritesNoAuditRows(t *testing.T) {
	t.Parallel()
	svc, pool, sku := fixture(t)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		if _, err := svc.Ingest(ctx, analytics.Batch{
			SessionID: fmt.Sprintf("session-quiet-%04d", i), IP: "203.0.113.4",
			Events: []analytics.Event{{
				Kind: analytics.KindProductView, Target: sku,
				Source: analytics.SourceBeltModal, Locale: "ru",
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, pool, "audit_log"); n != 0 {
		t.Errorf("ingestion wrote %d audit rows, want 0", n)
	}
}

// ---------------------------------------------------------------------------
// Rate limiting and the staleness check
// ---------------------------------------------------------------------------

func TestIngestionIsRateLimitedPerHashedIP(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	svc := analytics.NewService(pool, analytics.Config{
		IPSalt: "test-salt", MaxPerWindow: 3, Window: time.Hour, MaxBatch: 50,
	})
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := svc.Ingest(ctx, analytics.Batch{
			SessionID: fmt.Sprintf("session-flood-%04d", i), IP: "203.0.113.5",
			Events: []analytics.Event{{Kind: analytics.KindPageView, Target: "/ru", Locale: "ru"}},
		}); err != nil {
			t.Fatalf("batch %d: %v", i, err)
		}
	}
	if _, err := svc.Ingest(ctx, analytics.Batch{
		SessionID: "session-flood-9999", IP: "203.0.113.5",
		Events: []analytics.Event{{Kind: analytics.KindPageView, Target: "/ru", Locale: "ru"}},
	}); err == nil {
		t.Error("the fourth batch was accepted; the rate limit did not fire")
	}

	// A different address is unaffected — the limit is per hasher, not global.
	if _, err := svc.Ingest(ctx, analytics.Batch{
		SessionID: "session-other-9999", IP: "203.0.113.6",
		Events: []analytics.Event{{Kind: analytics.KindPageView, Target: "/ru", Locale: "ru"}},
	}); err != nil {
		t.Errorf("a different IP was refused: %v", err)
	}
}

// The raw address is never stored. Only a salted hash, and only to count.
func TestTheRawIPIsNeverStored(t *testing.T) {
	t.Parallel()
	svc, pool, _ := fixture(t)
	ctx := context.Background()

	const ip = "198.51.100.77"
	if _, err := svc.Ingest(ctx, analytics.Batch{
		SessionID: "session-privacy-01", IP: ip,
		Events: []analytics.Event{{Kind: analytics.KindPageView, Target: "/ru", Locale: "ru"}},
	}); err != nil {
		t.Fatal(err)
	}

	var found int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM analytics_events WHERE ip_hash LIKE '%' || $1 || '%'`, ip,
	).Scan(&found); err != nil {
		t.Fatal(err)
	}
	if found != 0 {
		t.Error("the raw address appears in ip_hash")
	}
}

// A ticker that dies silently must be distinguishable from one that is working.
func TestMaintenanceStalenessIsVisible(t *testing.T) {
	t.Parallel()
	svc, _, _ := fixture(t)
	ctx := context.Background()

	stale, last, err := svc.MaintenanceStale(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !stale || last != nil {
		t.Error("a system that has never run maintenance must report stale")
	}

	if _, err := svc.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	stale, last, err = svc.MaintenanceStale(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stale || last == nil {
		t.Error("a run that just happened must not report stale")
	}

	// Three days later, with nothing having run, it says so.
	future := svc.WithClock(func() time.Time { return time.Now().Add(72 * time.Hour) })
	stale, _, err = future.MaintenanceStale(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Error("72 hours without a run must report stale")
	}
}

// The rollup is idempotent: the CLI exists so a missed day can be caught up, and
// catching up twice must not double the counts.
func TestRollingUpTheSameDayTwiceDoesNotDoubleIt(t *testing.T) {
	t.Parallel()
	svc, pool, sku := fixture(t)
	ctx := context.Background()

	if _, err := svc.Ingest(ctx, analytics.Batch{
		SessionID: "session-idem-0001", IP: "203.0.113.7",
		Events: []analytics.Event{{
			Kind: analytics.KindProductView, Target: sku,
			Source: analytics.SourceProductPage, Locale: "ru",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	backdate(t, pool, time.Now().AddDate(0, 0, -1))

	for i := 0; i < 3; i++ {
		if _, err := svc.Maintain(ctx); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	var events int
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(sum(event_count),0)::int FROM analytics_daily WHERE kind='product_view'`,
	).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("event_count = %d after three runs, want 1", events)
	}
}

// backdate moves every raw event into a completed day, because the rollup only
// touches days that are over.
func backdate(t *testing.T, pool *pgxpool.Pool, to time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE analytics_events SET occurred_at = $1`, to); err != nil {
		t.Fatal(err)
	}
}

var _ = uuid.Nil
