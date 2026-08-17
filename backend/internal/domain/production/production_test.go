package production_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/domain/inventory"
	"github.com/qoim/samari/backend/internal/domain/production"
	"github.com/qoim/samari/backend/internal/domain/quality"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// Производство. The property under test throughout: completing an order records
// output but does NOT make it sellable. That seam is what the quality gate is.

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

type fixture struct {
	pool     *pgxpool.Pool
	svc      *production.Service
	inv      *inventory.Service
	qc       *quality.Service
	actor    uuid.UUID
	item     uuid.UUID
	quaran   uuid.UUID
	finished uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testsupport.NewDB(t)
	ctx := context.Background()

	f := fixture{pool: pool}
	f.inv = inventory.NewService(pool)
	f.qc = quality.NewService(pool)
	f.svc = production.NewService(pool, f.inv, f.qc)

	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ('line@samari-kuhsor.tj', 'Оператор линии', 'x') RETURNING id`).Scan(&f.actor); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom, status)
		VALUES ('APJ-1000', 'finished_good', 'bottle', 'active') RETURNING id`).Scan(&f.item); err != nil {
		t.Fatal(err)
	}
	for _, l := range []struct {
		code, zone string
		dest       *uuid.UUID
	}{
		{"Q-01", "quarantine", &f.quaran},
		{"A-12", "finished_goods", &f.finished},
	} {
		if err := pool.QueryRow(ctx, `
			INSERT INTO locations (code, name, zone) VALUES ($1, $1, $2) RETURNING id`,
			l.code, l.zone).Scan(l.dest); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f fixture) newOrder(t *testing.T, moNo, batchNo, planned string) uuid.UUID {
	t.Helper()
	mo, err := f.svc.Create(context.Background(), f.actor, production.CreateInput{
		MONo: moNo, ItemID: f.item, BatchNo: batchNo, PlannedQty: dec(planned),
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	return mo.ID
}

func (f fixture) batchOf(t *testing.T, moID uuid.UUID) (uuid.UUID, string) {
	t.Helper()
	var id uuid.UUID
	var status string
	if err := f.pool.QueryRow(context.Background(), `
		SELECT b.id, b.status FROM manufacturing_orders mo
		JOIN batches b ON b.id = mo.batch_id WHERE mo.id = $1`, moID).Scan(&id, &status); err != nil {
		t.Fatal(err)
	}
	return id, status
}

// ---------------------------------------------------------------------------
// Yield is computed, never stored
// ---------------------------------------------------------------------------

func TestYieldIsComputedFromEntries(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	moID := f.newOrder(t, "MO-0612", "B-2617", "12000")

	for _, e := range []struct {
		good, scrap string
		downtime    int32
	}{
		{"4000", "100", 12},
		{"4300", "150", 22},
		{"340", "50", 0},
	} {
		if _, err := f.svc.RecordEntry(ctx, f.actor, production.EntryInput{
			MOID: moID, GoodQty: dec(e.good), ScrapQty: dec(e.scrap), DowntimeMin: e.downtime,
		}); err != nil {
			t.Fatal(err)
		}
	}

	totals, err := f.svc.TotalsFor(ctx, moID)
	if err != nil {
		t.Fatal(err)
	}
	if !totals.GoodQty.Equal(dec("8640")) {
		t.Errorf("good = %s, want 8640", totals.GoodQty.StringFixed(3))
	}
	if !totals.ScrapQty.Equal(dec("300")) {
		t.Errorf("scrap = %s, want 300", totals.ScrapQty.StringFixed(3))
	}
	if totals.DowntimeMin != 34 {
		t.Errorf("downtime = %d, want 34", totals.DowntimeMin)
	}
	// 8640 / 8940 = 96.6%
	y := totals.YieldPercent()
	if y == nil || !y.Equal(dec("96.6")) {
		t.Errorf("yield = %v, want 96.6", y)
	}

	// And no column stores any of it.
	var n int
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'manufacturing_orders'
		  AND column_name IN ('good_qty','scrap_qty','actual_qty','yield','downtime_min')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("manufacturing_orders has %d stored total columns; totals are sums (docs/02-SCHEMA.md:274)", n)
	}
}

// "No output yet" and "0% yield" are different facts. Rendering the first as the
// second would look like a catastrophic run on a shift report.
func TestYieldIsNilBeforeAnyOutput(t *testing.T) {
	t.Parallel()
	f := setup(t)
	moID := f.newOrder(t, "MO-0612", "B-2617", "12000")

	totals, err := f.svc.TotalsFor(context.Background(), moID)
	if err != nil {
		t.Fatal(err)
	}
	if totals.YieldPercent() != nil {
		t.Errorf("yield = %v before any output, want nil", totals.YieldPercent())
	}
}

// ---------------------------------------------------------------------------
// The seam
// ---------------------------------------------------------------------------

// docs/05-MODULES.md:128 — completion posts to QUARANTINE and moves the batch to
// `quarantine`. It does not make the batch sellable.
func TestCompletionPostsToQuarantineNotFinishedGoods(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	moID := f.newOrder(t, "MO-0612", "B-2617", "12000")

	if _, err := f.svc.RecordEntry(ctx, f.actor, production.EntryInput{
		MOID: moID, GoodQty: dec("8640"), ScrapQty: dec("300"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Complete(ctx, f.actor, moID); err != nil {
		t.Fatalf("complete: %v", err)
	}

	batchID, status := f.batchOf(t, moID)

	// The batch is quarantined, NOT released.
	if status != quality.StatusQuarantine {
		t.Errorf("batch status = %s, want quarantine", status)
	}
	if quality.CanSellOrShip(status) {
		t.Fatal("production made a batch sellable; only Качество may")
	}

	// The stock is in quarantine, and nothing is in finished goods.
	pos := inventory.Position{
		ItemID: f.item, BatchID: uuid.NullUUID{UUID: batchID, Valid: true}, LocationID: f.quaran,
	}
	onHand, err := f.inv.BalanceOf(ctx, pos)
	if err != nil {
		t.Fatal(err)
	}
	if !onHand.Equal(dec("8640")) {
		t.Errorf("quarantine holds %s, want 8640 (good output only, not scrap)", onHand.StringFixed(3))
	}

	pos.LocationID = f.finished
	finished, err := f.inv.BalanceOf(ctx, pos)
	if err != nil {
		t.Fatal(err)
	}
	if !finished.IsZero() {
		t.Errorf("finished goods holds %s; output must land in quarantine only", finished.StringFixed(3))
	}
}

// Scrap is not stock. Posting it would put unsellable product into the ledger.
func TestScrapIsNotPostedToStock(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	moID := f.newOrder(t, "MO-0612", "B-2617", "1000")

	if _, err := f.svc.RecordEntry(ctx, f.actor, production.EntryInput{
		MOID: moID, GoodQty: dec("900"), ScrapQty: dec("100"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Complete(ctx, f.actor, moID); err != nil {
		t.Fatal(err)
	}

	batchID, _ := f.batchOf(t, moID)
	onHand, err := f.inv.BalanceOf(ctx, inventory.Position{
		ItemID: f.item, BatchID: uuid.NullUUID{UUID: batchID, Valid: true}, LocationID: f.quaran,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !onHand.Equal(dec("900")) {
		t.Errorf("stock = %s, want 900 — scrap must not enter the ledger", onHand.StringFixed(3))
	}
}

// Completing twice would post the output into stock a second time.
func TestCompletingTwiceIsRefused(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	moID := f.newOrder(t, "MO-0612", "B-2617", "1000")

	if _, err := f.svc.RecordEntry(ctx, f.actor, production.EntryInput{
		MOID: moID, GoodQty: dec("900"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Complete(ctx, f.actor, moID); err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.Complete(ctx, f.actor, moID); err == nil {
		t.Fatal("the order was completed twice")
	}

	batchID, _ := f.batchOf(t, moID)
	onHand, err := f.inv.BalanceOf(ctx, inventory.Position{
		ItemID: f.item, BatchID: uuid.NullUUID{UUID: batchID, Valid: true}, LocationID: f.quaran,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !onHand.Equal(dec("900")) {
		t.Errorf("stock = %s after a refused second completion, want 900", onHand.StringFixed(3))
	}
}

// Completion is atomic: stock without a quarantined batch would look sellable,
// and a quarantined batch with no stock would be a decision about nothing.
func TestCompletionWithNoQuarantineLocationChangesNothing(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	// Remove the quarantine zone: a misconfigured system.
	if _, err := f.pool.Exec(ctx, `UPDATE locations SET deleted_at = now() WHERE zone = 'quarantine'`); err != nil {
		t.Fatal(err)
	}

	moID := f.newOrder(t, "MO-0612", "B-2617", "1000")
	if _, err := f.svc.RecordEntry(ctx, f.actor, production.EntryInput{
		MOID: moID, GoodQty: dec("900"),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.Complete(ctx, f.actor, moID); err == nil {
		t.Fatal("completion succeeded with no quarantine location configured")
	}

	// Nothing moved.
	_, status := f.batchOf(t, moID)
	if status != quality.StatusInProduction {
		t.Errorf("batch status = %s after a failed completion, want in_production", status)
	}
	var movements int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM stock_movements`).Scan(&movements); err != nil {
		t.Fatal(err)
	}
	if movements != 0 {
		t.Errorf("%d stock movements after a failed completion", movements)
	}
}

func TestCompletingWithNoOutputIsRefused(t *testing.T) {
	t.Parallel()
	f := setup(t)
	moID := f.newOrder(t, "MO-0612", "B-2617", "1000")

	if _, err := f.svc.Complete(context.Background(), f.actor, moID); err == nil {
		t.Fatal("an order with no output was completed")
	}
}

// ---------------------------------------------------------------------------
// Orders and entries
// ---------------------------------------------------------------------------

// MO ↔ batch is 1:1 (docs/05-MODULES.md:127), enforced by a unique index rather
// than by convention.
func TestOrderAndBatchAreOneToOne(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	f.newOrder(t, "MO-0612", "B-2617", "1000")

	// A second order cannot reuse the batch number.
	if _, err := f.svc.Create(ctx, f.actor, production.CreateInput{
		MONo: "MO-0613", ItemID: f.item, BatchNo: "B-2617", PlannedQty: dec("500"),
	}); err == nil {
		t.Fatal("two orders were created against one batch number")
	}

	// And the database refuses two orders pointing at one batch id.
	var batchID uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT id FROM batches WHERE batch_no = 'B-2617'`).Scan(&batchID); err != nil {
		t.Fatal(err)
	}
	_, err := f.pool.Exec(ctx, `
		INSERT INTO manufacturing_orders (mo_no, item_id, batch_id, planned_qty)
		VALUES ('MO-0614', $1, $2, 100)`, f.item, batchID)
	if err == nil {
		t.Error("the schema allowed two orders against one batch")
	}
}

func TestFirstEntryMovesTheOrderIntoProgress(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	moID := f.newOrder(t, "MO-0612", "B-2617", "1000")

	mo, err := f.svc.Get(ctx, moID)
	if err != nil {
		t.Fatal(err)
	}
	if mo.Status != production.StatusPlanned {
		t.Fatalf("new order status = %s, want planned", mo.Status)
	}

	if _, err := f.svc.RecordEntry(ctx, f.actor, production.EntryInput{
		MOID: moID, GoodQty: dec("100"),
	}); err != nil {
		t.Fatal(err)
	}
	mo, err = f.svc.Get(ctx, moID)
	if err != nil {
		t.Fatal(err)
	}
	if mo.Status != production.StatusInProgress {
		t.Errorf("status = %s after the first entry, want in_progress", mo.Status)
	}
}

// A closed order's totals already justified the stock that was posted; a later
// entry would change the yield without changing the stock.
func TestEntriesAreRefusedOnAClosedOrder(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	moID := f.newOrder(t, "MO-0612", "B-2617", "1000")

	if _, err := f.svc.RecordEntry(ctx, f.actor, production.EntryInput{
		MOID: moID, GoodQty: dec("900"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Complete(ctx, f.actor, moID); err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.RecordEntry(ctx, f.actor, production.EntryInput{
		MOID: moID, GoodQty: dec("50"),
	}); err == nil {
		t.Error("an entry was accepted against a completed order")
	}
}

func TestEntryValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	moID := f.newOrder(t, "MO-0612", "B-2617", "1000")

	cases := map[string]production.EntryInput{
		"negative good":  {MOID: moID, GoodQty: dec("-1")},
		"negative scrap": {MOID: moID, ScrapQty: dec("-1")},
		"empty entry":    {MOID: moID},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.svc.RecordEntry(ctx, f.actor, in); err == nil {
				t.Error("accepted")
			} else if code := common.AsError(err).Code; code != common.CodeValidationFailed {
				t.Errorf("code = %s, want validation_failed", code)
			}
		})
	}
}

func TestOrderValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	cases := map[string]production.CreateInput{
		"no mo_no":     {ItemID: f.item, BatchNo: "B-1", PlannedQty: dec("100")},
		"no batch_no":  {MONo: "MO-1", ItemID: f.item, PlannedQty: dec("100")},
		"zero planned": {MONo: "MO-1", ItemID: f.item, BatchNo: "B-1", PlannedQty: decimal.Zero},
		"unknown item": {MONo: "MO-1", ItemID: uuid.New(), BatchNo: "B-1", PlannedQty: dec("100")},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.svc.Create(ctx, f.actor, in); err == nil {
				t.Error("accepted")
			}
		})
	}
}

func TestProductionIsAudited(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	mo, err := f.svc.Create(ctx, f.actor, production.CreateInput{
		MONo: "MO-0612", ItemID: f.item, BatchNo: "B-2617", PlannedQty: dec("1000"),
	})
	if err != nil {
		t.Fatal(err)
	}
	testsupport.AssertAudited(t, f.pool, "production", mo.ID, "create")

	entry, err := f.svc.RecordEntry(ctx, f.actor, production.EntryInput{MOID: mo.ID, GoodQty: dec("900")})
	if err != nil {
		t.Fatal(err)
	}
	testsupport.AssertAudited(t, f.pool, "production", entry.ID, "create")

	if _, err := f.svc.Complete(ctx, f.actor, mo.ID); err != nil {
		t.Fatal(err)
	}
	testsupport.AssertAudited(t, f.pool, "production", mo.ID, "update")
}
