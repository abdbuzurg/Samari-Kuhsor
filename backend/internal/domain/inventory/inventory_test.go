package inventory_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/domain/inventory"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// The ledger. docs/02-SCHEMA.md §5 calls stock_movements the most important table
// in the system, and CLAUDE.md's risk register rates "ledger arithmetic wrong" as
// severe. These tests exist to make that specific failure impossible.

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

type fixture struct {
	pool   *pgxpool.Pool
	svc    *inventory.Service
	actor  uuid.UUID
	item   uuid.UUID
	batch  uuid.NullUUID
	locA   uuid.UUID
	locB   uuid.UUID
	quaran uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testsupport.NewDB(t)
	ctx := context.Background()

	var f fixture
	f.pool = pool
	f.svc = inventory.NewService(pool)

	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ('warehouse@samari-kuhsor.tj', 'Склад', 'x') RETURNING id`).Scan(&f.actor); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom, status)
		VALUES ('APJ-1000', 'finished_good', 'bottle', 'active') RETURNING id`).Scan(&f.item); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	var batchID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO batches (batch_no, item_id) VALUES ('B-2617', $1) RETURNING id`,
		f.item).Scan(&batchID); err != nil {
		t.Fatalf("seed batch: %v", err)
	}
	f.batch = uuid.NullUUID{UUID: batchID, Valid: true}

	for _, l := range []struct {
		code, zone string
		dest       *uuid.UUID
	}{
		{"A-12", "finished_goods", &f.locA},
		{"C-07", "finished_goods", &f.locB},
		{"Q-01", "quarantine", &f.quaran},
	} {
		if err := pool.QueryRow(ctx, `
			INSERT INTO locations (code, name, zone) VALUES ($1, $1, $2) RETURNING id`,
			l.code, l.zone).Scan(l.dest); err != nil {
			t.Fatalf("seed location %s: %v", l.code, err)
		}
	}
	return f
}

func (f fixture) at(loc uuid.UUID) inventory.Position {
	return inventory.Position{ItemID: f.item, BatchID: f.batch, LocationID: loc}
}

func (f fixture) post(t *testing.T, loc uuid.UUID, qty string, reason string) error {
	t.Helper()
	_, err := f.svc.Post(context.Background(), f.actor, inventory.Movement{
		Position: f.at(loc), QtyDelta: dec(qty), Reason: reason,
	})
	return err
}

func (f fixture) mustPost(t *testing.T, loc uuid.UUID, qty string, reason string) {
	t.Helper()
	if err := f.post(t, loc, qty, reason); err != nil {
		t.Fatalf("post %s %s: %v", qty, reason, err)
	}
}

func (f fixture) balance(t *testing.T, loc uuid.UUID) decimal.Decimal {
	t.Helper()
	b, err := f.svc.BalanceOf(context.Background(), f.at(loc))
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return b
}

// ---------------------------------------------------------------------------
// The balance IS the sum. There is no other definition.
// ---------------------------------------------------------------------------

func TestBalanceIsTheSumOfDeltas(t *testing.T) {
	t.Parallel()
	f := setup(t)

	f.mustPost(t, f.locA, "1000", inventory.ReasonGoodsReceipt)
	f.mustPost(t, f.locA, "-250.500", inventory.ReasonSale)
	f.mustPost(t, f.locA, "-99.250", inventory.ReasonScrap)
	f.mustPost(t, f.locA, "40.750", inventory.ReasonReturn)

	// 1000 - 250.5 - 99.25 + 40.75 = 691.000, exactly.
	if got := f.balance(t, f.locA); !got.Equal(dec("691")) {
		t.Errorf("balance = %s, want 691", got.StringFixed(3))
	}
}

// Decimal arithmetic must stay exact over many movements. A float would drift,
// and a drifting stock ledger is a compliance problem, not a rounding one.
func TestBalanceStaysExactOverManyMovements(t *testing.T) {
	t.Parallel()
	f := setup(t)

	// 300 movements of 0.001 each. In float64 this accumulates visible error.
	for range 300 {
		f.mustPost(t, f.locA, "0.001", inventory.ReasonGoodsReceipt)
	}
	if got := f.balance(t, f.locA); !got.Equal(dec("0.300")) {
		t.Errorf("balance = %s, want exactly 0.300", got.StringFixed(3))
	}
}

// docs/02-SCHEMA.md:240 — corrections are compensating entries. The original row
// is evidence of what someone recorded and must survive untouched.
func TestCorrectionIsACompensatingEntryNotAnEdit(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	original, err := f.svc.Post(ctx, f.actor, inventory.Movement{
		Position: f.at(f.locA), QtyDelta: dec("100"), Reason: inventory.ReasonGoodsReceipt,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The receipt was wrong: it should have been zero. Correct it.
	f.mustPost(t, f.locA, "-100", inventory.ReasonAdjustment)

	if got := f.balance(t, f.locA); !got.IsZero() {
		t.Errorf("balance = %s, want 0", got.StringFixed(3))
	}

	// The original row is byte-identical and still live.
	var qty string
	var deletedAt *string
	var version int32
	if err := f.pool.QueryRow(ctx,
		`SELECT qty_delta::text, deleted_at::text, version FROM stock_movements WHERE id = $1`,
		original.ID).Scan(&qty, &deletedAt, &version); err != nil {
		t.Fatal(err)
	}
	if qty != "100.000" {
		t.Errorf("the original movement was edited: qty_delta = %s", qty)
	}
	if deletedAt != nil {
		t.Error("the original movement was tombstoned; corrections must not remove evidence")
	}
	if version != 1 {
		t.Errorf("the original movement was updated: version = %d", version)
	}

	// And two rows exist, not one.
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM stock_movements`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("%d movements, want 2 — the correction must be its own row", n)
	}
}

// ---------------------------------------------------------------------------
// Transfers
// ---------------------------------------------------------------------------

func TestTransferIsTwoRowsSharingARefAndNetsToZero(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	f.mustPost(t, f.locA, "500", inventory.ReasonGoodsReceipt)

	moves, err := f.svc.Transfer(ctx, f.actor, f.at(f.locA), f.locB, dec("120"), nil)
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if len(moves) != 2 {
		t.Fatalf("transfer produced %d movements, want 2", len(moves))
	}

	// Both legs share a ref_id, so the pair can always be found together.
	if moves[0].RefID != moves[1].RefID || !moves[0].RefID.Valid {
		t.Errorf("legs do not share a ref_id: %v / %v", moves[0].RefID, moves[1].RefID)
	}
	// And they net to zero: a transfer can never create or destroy stock.
	if sum := moves[0].QtyDelta.Add(moves[1].QtyDelta); !sum.IsZero() {
		t.Errorf("transfer nets to %s, want 0", sum)
	}

	if got := f.balance(t, f.locA); !got.Equal(dec("380")) {
		t.Errorf("source = %s, want 380", got.StringFixed(3))
	}
	if got := f.balance(t, f.locB); !got.Equal(dec("120")) {
		t.Errorf("destination = %s, want 120", got.StringFixed(3))
	}
}

func TestTransferValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	f.mustPost(t, f.locA, "100", inventory.ReasonGoodsReceipt)

	// Same location is a no-op that would write two cancelling rows.
	if _, err := f.svc.Transfer(ctx, f.actor, f.at(f.locA), f.locA, dec("10"), nil); err == nil {
		t.Error("a transfer to the same location was accepted")
	}
	for _, qty := range []string{"0", "-5"} {
		if _, err := f.svc.Transfer(ctx, f.actor, f.at(f.locA), f.locB, dec(qty), nil); err == nil {
			t.Errorf("a transfer of %s was accepted", qty)
		}
	}
}

// A transfer that would overdraw the source must fail ENTIRELY: neither leg may
// commit, or stock appears at the destination that never left the source.
func TestFailedTransferLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	f.mustPost(t, f.locA, "50", inventory.ReasonGoodsReceipt)

	if _, err := f.svc.Transfer(ctx, f.actor, f.at(f.locA), f.locB, dec("500"), nil); err == nil {
		t.Fatal("an overdrawing transfer was accepted")
	}

	if got := f.balance(t, f.locA); !got.Equal(dec("50")) {
		t.Errorf("source = %s, want 50 unchanged", got.StringFixed(3))
	}
	if got := f.balance(t, f.locB); !got.IsZero() {
		t.Errorf("destination = %s, want 0 — a failed transfer must not deliver", got.StringFixed(3))
	}
}

// ---------------------------------------------------------------------------
// The oversell guard — docs/07-IMPLEMENTATION-PLAN.md I6
// ---------------------------------------------------------------------------

func TestNegativeBalanceIsRefused(t *testing.T) {
	t.Parallel()
	f := setup(t)

	f.mustPost(t, f.locA, "100", inventory.ReasonGoodsReceipt)

	err := f.post(t, f.locA, "-101", inventory.ReasonSale)
	if err == nil {
		t.Fatal("an overdraw was accepted")
	}
	if code := common.AsError(err).Code; code != common.CodeBusinessRule {
		t.Errorf("code = %s, want business_rule (422)", code)
	}
	if got := f.balance(t, f.locA); !got.Equal(dec("100")) {
		t.Errorf("balance changed to %s after a refused posting", got.StringFixed(3))
	}
}

// The exemption. Adjustment is the correction mechanism; if the mistake could
// block the tool for fixing it, mistakes would be permanent.
func TestAdjustmentMayGoNegative(t *testing.T) {
	t.Parallel()
	f := setup(t)

	// No stock at all, and an adjustment still posts.
	if err := f.post(t, f.locA, "-25", inventory.ReasonAdjustment); err != nil {
		t.Fatalf("adjustment was refused: %v", err)
	}
	if got := f.balance(t, f.locA); !got.Equal(dec("-25")) {
		t.Errorf("balance = %s, want -25", got.StringFixed(3))
	}
}

// Every other reason is refused. Tested per reason rather than once, because the
// exemption is a single `==` and a future edit could widen it silently.
func TestOnlyAdjustmentMayGoNegative(t *testing.T) {
	t.Parallel()

	for _, reason := range []string{
		inventory.ReasonSale, inventory.ReasonMaterialIssue, inventory.ReasonScrap,
		inventory.ReasonTransfer, inventory.ReasonGoodsReceipt,
		inventory.ReasonProductionOutput, inventory.ReasonReturn,
	} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			f := setup(t)
			if err := f.post(t, f.locA, "-10", reason); err == nil {
				t.Errorf("%s drove the balance negative; only adjustment may", reason)
			}
		})
	}

	t.Run("adjustment", func(t *testing.T) {
		t.Parallel()
		f := setup(t)
		if err := f.post(t, f.locA, "-10", inventory.ReasonAdjustment); err != nil {
			t.Errorf("adjustment was refused: %v", err)
		}
	})
	if !inventory.AllowsNegative(inventory.ReasonAdjustment) {
		t.Error("AllowsNegative disagrees with the posting rule")
	}
}

// THE concurrency test. Two shipments each read "100 available" and each try to
// take 80. Without the advisory lock both succeed and the batch goes to -60.
func TestConcurrentIssuesCannotOversell(t *testing.T) {
	t.Parallel()
	f := setup(t)

	f.mustPost(t, f.locA, "100", inventory.ReasonGoodsReceipt)

	const workers = 8
	var wg sync.WaitGroup
	results := make(chan error, workers)

	start := make(chan struct{})
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release them together, to actually contend
			_, err := f.svc.Post(context.Background(), f.actor, inventory.Movement{
				Position: f.at(f.locA), QtyDelta: dec("-80"), Reason: inventory.ReasonSale,
			})
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		}
	}

	// Only one 80 fits into 100.
	if succeeded != 1 {
		t.Errorf("%d of %d concurrent issues succeeded, want exactly 1", succeeded, workers)
	}
	if got := f.balance(t, f.locA); !got.Equal(dec("20")) {
		t.Errorf("balance = %s, want 20 — stock was oversold", got.StringFixed(3))
	}
	if f.balance(t, f.locA).IsNegative() {
		t.Fatal("the ledger went negative under concurrency")
	}
}

// Receipts are not serialised: locking them would stall the warehouse for a race
// that cannot happen, since adding stock can never overdraw.
func TestConcurrentReceiptsAllSucceed(t *testing.T) {
	t.Parallel()
	f := setup(t)

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.Post(context.Background(), f.actor, inventory.Movement{
				Position: f.at(f.locA), QtyDelta: dec("10"), Reason: inventory.ReasonGoodsReceipt,
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("a concurrent receipt failed: %v", err)
		}
	}
	if got := f.balance(t, f.locA); !got.Equal(dec("80")) {
		t.Errorf("balance = %s, want 80", got.StringFixed(3))
	}
}

// ---------------------------------------------------------------------------
// Position isolation
// ---------------------------------------------------------------------------

// A balance is per (item, batch, location). Aggregating wrongly would let stock
// in one location satisfy a shipment from another.
func TestBalancesAreIsolatedPerPosition(t *testing.T) {
	t.Parallel()
	f := setup(t)

	f.mustPost(t, f.locA, "100", inventory.ReasonGoodsReceipt)
	f.mustPost(t, f.locB, "7", inventory.ReasonGoodsReceipt)

	if got := f.balance(t, f.locA); !got.Equal(dec("100")) {
		t.Errorf("A = %s", got.StringFixed(3))
	}
	if got := f.balance(t, f.locB); !got.Equal(dec("7")) {
		t.Errorf("B = %s", got.StringFixed(3))
	}

	// Stock at A cannot satisfy an issue from B.
	if err := f.post(t, f.locB, "-50", inventory.ReasonSale); err == nil {
		t.Error("an issue from B was satisfied by stock at A")
	}
}

// A batchless position must not share a balance with a batched one at the same
// location — raw materials often have no batch, finished goods always do.
func TestBatchlessPositionIsSeparate(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	batchless := inventory.Position{ItemID: f.item, LocationID: f.locA}

	if _, err := f.svc.Post(ctx, f.actor, inventory.Movement{
		Position: batchless, QtyDelta: dec("40"), Reason: inventory.ReasonGoodsReceipt,
	}); err != nil {
		t.Fatal(err)
	}
	f.mustPost(t, f.locA, "100", inventory.ReasonGoodsReceipt) // batched

	got, err := f.svc.BalanceOf(ctx, batchless)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(dec("40")) {
		t.Errorf("batchless balance = %s, want 40", got.StringFixed(3))
	}
	if b := f.balance(t, f.locA); !b.Equal(dec("100")) {
		t.Errorf("batched balance = %s, want 100", b.StringFixed(3))
	}
}

// ---------------------------------------------------------------------------
// Validation and audit
// ---------------------------------------------------------------------------

func TestMovementValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	cases := map[string]inventory.Movement{
		"zero quantity":  {Position: f.at(f.locA), QtyDelta: decimal.Zero, Reason: inventory.ReasonGoodsReceipt},
		"unknown reason": {Position: f.at(f.locA), QtyDelta: dec("10"), Reason: "borrowed"},
		"no item":        {Position: inventory.Position{LocationID: f.locA}, QtyDelta: dec("10"), Reason: inventory.ReasonGoodsReceipt},
		"no location":    {Position: inventory.Position{ItemID: f.item}, QtyDelta: dec("10"), Reason: inventory.ReasonGoodsReceipt},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := f.svc.Post(ctx, f.actor, m)
			if err == nil {
				t.Fatal("expected a validation error")
			}
			if code := common.AsError(err).Code; code != common.CodeValidationFailed {
				t.Errorf("code = %s, want validation_failed", code)
			}
		})
	}
}

func TestEveryMovementIsAudited(t *testing.T) {
	t.Parallel()
	f := setup(t)

	move, err := f.svc.Post(context.Background(), f.actor, inventory.Movement{
		Position: f.at(f.locA), QtyDelta: dec("100"), Reason: inventory.ReasonGoodsReceipt,
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := testsupport.AssertAudited(t, f.pool, "inventory", move.ID, "create")
	if entry.ActorID.UUID != f.actor {
		t.Errorf("audited against %s, want %s", entry.ActorID.UUID, f.actor)
	}
}

// A refused posting must leave no audit row: the trail must not record work that
// never happened.
func TestRefusedPostingIsNotAudited(t *testing.T) {
	t.Parallel()
	f := setup(t)

	before := testsupport.CountAudit(t, f.pool)
	_ = f.post(t, f.locA, "-10", inventory.ReasonSale) // refused
	if after := testsupport.CountAudit(t, f.pool); after != before {
		t.Errorf("a refused posting wrote %d audit rows", after-before)
	}
}

// ---------------------------------------------------------------------------
// The invariant that outlives this file
// ---------------------------------------------------------------------------

// If a balance column is ever added to any table, this fails. The rule is
// CLAUDE.md §4.2 and it is the one most likely to be "helpfully" broken later by
// someone adding a cached total for performance.
func TestNoBalanceColumnExistsAnywhere(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)

	rows, err := pool.Query(context.Background(), `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND column_name ~ '(on_hand|quantity_on_hand|stock_qty|current_qty|balance|qty_total)'
		  AND table_name <> 'stock_balances'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		t.Errorf("%s.%s looks like a stored balance; stock is derived from the ledger only (CLAUDE.md §4.2)",
			table, column)
	}
}

// stock_balances must remain a VIEW. If someone converts it to a table for
// performance, the derived-not-stored rule is gone and nothing else would notice.
func TestStockBalancesIsAView(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)

	var kind string
	err := pool.QueryRow(context.Background(), `
		SELECT table_type FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'stock_balances'`).Scan(&kind)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "VIEW" {
		t.Errorf("stock_balances is a %s; it must be a view so balances stay derived (I5)", kind)
	}
}
