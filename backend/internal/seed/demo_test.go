package seed_test

import (
	"context"
	"testing"

	"github.com/qoim/samari/backend/internal/seed"
)

// R14 — the demonstration seed.
//
// Its whole purpose is that no module screen is empty when the client first logs
// in, so the test that matters is a census: every table behind a screen has rows.
// A seed that silently skipped a module would still pass a "no error" check and
// fail the only thing it was built for.
func TestDemoPopulatesEveryModuleWithAScreen(t *testing.T) {
	t.Parallel()
	pool := load(t)
	ctx := context.Background()

	if _, err := seed.Demo(ctx, pool); err != nil {
		t.Fatalf("demo seed: %v", err)
	}

	// One row per module the CRM has a screen for. Финансы и бюджет is absent
	// because it is out of scope, not because it was forgotten.
	for _, table := range []string{
		"customers", "contacts", "leads", "deals", "deal_stage_events", "tasks",
		"suppliers", "purchase_orders", "purchase_order_lines",
		"employees", "assets", "maintenance_events", "documents",
		"batches", "batch_status_events", "quality_tests",
		"manufacturing_orders", "production_entries",
		"stock_movements", "sales_orders", "sales_order_lines",
		"shipments", "shipment_lines", "vehicles", "drivers", "inquiries",
	} {
		var n int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM "+table+" WHERE deleted_at IS NULL").Scan(&n); err != nil {
			// deal_stage_events and batch_status_events are immutable evidence and
			// carry no deleted_at, so they are counted without the filter.
			if err2 := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err2 != nil {
				t.Fatalf("count %s: %v", table, err2)
			}
		}
		if n == 0 {
			t.Errorf("%s is empty — its screen will show nothing to the client", table)
		}
	}
}

// The dashboard's Воронка продаж reads `deals`, which nothing wrote before R12.
// The panel has therefore always rendered its empty state. A demo that does not
// fix that demonstrates a broken dashboard.
func TestDemoFillsTheSalesFunnelAcrossEveryStage(t *testing.T) {
	t.Parallel()
	pool := load(t)
	ctx := context.Background()

	if _, err := seed.Demo(ctx, pool); err != nil {
		t.Fatalf("demo seed: %v", err)
	}

	for _, stage := range []string{"new", "negotiation", "quoted", "won", "lost"} {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM deals WHERE stage=$1 AND deleted_at IS NULL`, stage).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("no deal at stage %q — the funnel will have a gap", stage)
		}
	}
}

// Stock is posted as movements and summed at read time. If the demo ever wrote a
// balance directly there would be a column to write it to, and there is not —
// this asserts the demo went through the ledger like everything else.
func TestDemoPostsStockAsMovementsThatSumToAPositiveBalance(t *testing.T) {
	t.Parallel()
	pool := load(t)
	ctx := context.Background()

	if _, err := seed.Demo(ctx, pool); err != nil {
		t.Fatalf("demo seed: %v", err)
	}

	var positions int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT item_id, batch_id, location_id, SUM(qty_delta) AS bal
			FROM stock_movements WHERE deleted_at IS NULL
			GROUP BY item_id, batch_id, location_id
			HAVING SUM(qty_delta) > 0
		) t`).Scan(&positions); err != nil {
		t.Fatal(err)
	}
	if positions == 0 {
		t.Error("no position has a positive balance — the Склад screen will be empty")
	}
}

// All five enquiry types, each with its own reference prefix. The prefixes are a
// client-facing feature and a demo showing one type does not demonstrate them.
func TestDemoCoversEveryInquiryType(t *testing.T) {
	t.Parallel()
	pool := load(t)
	ctx := context.Background()

	if _, err := seed.Demo(ctx, pool); err != nil {
		t.Fatalf("demo seed: %v", err)
	}

	for kind, prefix := range map[string]string{
		"wholesale": "WR-", "contact": "CF-", "distributor": "DA-",
		"complaint": "CP-", "job": "JB-",
	} {
		var refNo string
		if err := pool.QueryRow(ctx,
			`SELECT reference_no FROM inquiries WHERE inquiry_type=$1 AND deleted_at IS NULL LIMIT 1`,
			kind).Scan(&refNo); err != nil {
			t.Errorf("no %s enquiry: %v", kind, err)
			continue
		}
		if len(refNo) < 3 || refNo[:3] != prefix {
			t.Errorf("%s enquiry has reference %q, want prefix %s", kind, refNo, prefix)
		}
	}

	// A complaint must name a batch, or the traceability workflow has no entry
	// point and the complaint is just a message.
	var batchID *string
	if err := pool.QueryRow(ctx,
		`SELECT batch_id::text FROM inquiries WHERE inquiry_type='complaint' AND deleted_at IS NULL LIMIT 1`,
	).Scan(&batchID); err != nil {
		t.Fatal(err)
	}
	if batchID == nil {
		t.Error("the demo complaint names no batch — traceability has no entry point")
	}
}

// Running it twice would double every figure on the dashboard. It refuses.
func TestDemoRefusesToRunTwice(t *testing.T) {
	t.Parallel()
	pool := load(t)
	ctx := context.Background()

	if _, err := seed.Demo(ctx, pool); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if _, err := seed.Demo(ctx, pool); err == nil {
		t.Error("second run succeeded; it must refuse rather than double the data")
	}
}

// The demo attaches to the five real products (D8) rather than inventing a sixth.
// The CRM prototype's pomegranate juice and strawberry jam were design filler and
// must not appear anywhere in the system.
func TestDemoAddsNoProductsToTheCatalogue(t *testing.T) {
	t.Parallel()
	pool := load(t)
	ctx := context.Background()

	var before int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM items WHERE deleted_at IS NULL`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Demo(ctx, pool); err != nil {
		t.Fatalf("demo seed: %v", err)
	}
	var after int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM items WHERE deleted_at IS NULL`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("catalogue went from %d to %d products; it is exactly five (D8)", before, after)
	}
}

// A fresh test database can never reproduce this, which is why an earlier draft
// shipped broken: the demo seed hard-coded 'WR-0001' and friends, and collided
// the first time it met a database that had already received a real public
// submission. References now come from the same per-type sequences the
// submission path uses.
func TestDemoDoesNotCollideWithEnquiriesThatAlreadyExist(t *testing.T) {
	t.Parallel()
	pool := load(t)
	ctx := context.Background()

	// A visitor submitted before anyone thought to seed a demo. This takes
	// WR-0001 and CP-0001 from the sequences.
	for _, prefix := range []string{"WR-", "CP-"} {
		var ref string
		if err := pool.QueryRow(ctx, `SELECT next_inquiry_reference($1)`, prefix).Scan(&ref); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO inquiries (inquiry_type, reference_no, name, contact, status)
			VALUES ($1, $2, 'Реальный посетитель', '+992 900 000 000', 'new')`,
			map[string]string{"WR-": "wholesale", "CP-": "complaint"}[prefix], ref); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := seed.Demo(ctx, pool); err != nil {
		t.Fatalf("demo seed collided with existing enquiries: %v", err)
	}

	// Every reference in the table is still unique — the constraint is what
	// broke before, so assert the property rather than only the absence of an
	// error.
	var dupes int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT reference_no FROM inquiries GROUP BY reference_no HAVING count(*) > 1
		) t`).Scan(&dupes); err != nil {
		t.Fatal(err)
	}
	if dupes != 0 {
		t.Errorf("%d duplicated reference numbers", dupes)
	}
}
