// Package domain_test holds the five end-to-end workflows from the client's ToR.
//
// docs/06-BUILD-PLAN.md:91 lists these as the launch acceptance criteria and
// schedules them as a MANUAL smoke test on the day before launch. They are
// written here as API-level integration tests instead
// (docs/07-IMPLEMENTATION-PLAN.md I10/I26): they test the business rules rather
// than the DOM, they run in seconds, and they can be written as each module
// lands rather than discovered broken the day before.
//
// Each test walks one workflow across several modules and asserts the invariants
// that hold BETWEEN them — which is exactly what per-module tests cannot see.
package domain_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/domain/inquiries"
	"github.com/qoim/samari/backend/internal/domain/inventory"
	"github.com/qoim/samari/backend/internal/domain/procurement"
	"github.com/qoim/samari/backend/internal/domain/production"
	"github.com/qoim/samari/backend/internal/domain/quality"
	"github.com/qoim/samari/backend/internal/domain/sales"
	"github.com/qoim/samari/backend/internal/testsupport"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

type plant struct {
	pool *pgxpool.Pool
	inv  *inventory.Service
	qc   *quality.Service
	prod *production.Service
	proc *procurement.Service
	sale *sales.Service
	inq  *inquiries.Service

	actor    uuid.UUID
	juice    uuid.UUID // APJ-1000, finished good
	sugar    uuid.UUID // RAW-SUG-50, raw material
	raw      uuid.UUID // raw store
	quaran   uuid.UUID // quarantine
	finished uuid.UUID // finished goods
	customer uuid.UUID
	supplier uuid.UUID
}

func newPlant(t *testing.T) plant {
	t.Helper()
	pool := testsupport.NewDB(t)
	ctx := context.Background()

	p := plant{pool: pool}
	p.inv = inventory.NewService(pool)
	p.qc = quality.NewService(pool)
	p.prod = production.NewService(pool, p.inv, p.qc)
	p.proc = procurement.NewService(pool, p.inv)
	p.sale = sales.NewService(pool, p.inv)
	p.inq = inquiries.NewService(pool, inquiries.DefaultRateLimit())

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	must(pool.QueryRow(ctx, `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ('ops@samari-kuhsor.tj', 'А. Раҳимов', 'x') RETURNING id`).Scan(&p.actor))
	must(pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, category, base_uom, status)
		VALUES ('APJ-1000','finished_good','juice','bottle','active') RETURNING id`).Scan(&p.juice))
	must(pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom, status)
		VALUES ('RAW-SUG-50','raw_material','kg','active') RETURNING id`).Scan(&p.sugar))

	for _, l := range []struct {
		code, zone string
		dest       *uuid.UUID
	}{
		{"D-01", "raw", &p.raw},
		{"Q-01", "quarantine", &p.quaran},
		{"A-12", "finished_goods", &p.finished},
	} {
		must(pool.QueryRow(ctx,
			`INSERT INTO locations (code, name, zone) VALUES ($1,$1,$2) RETURNING id`,
			l.code, l.zone).Scan(l.dest))
	}

	must(pool.QueryRow(ctx,
		`INSERT INTO customers (name, region) VALUES ('Ориён Маркет','Душанбе') RETURNING id`).Scan(&p.customer))
	must(pool.QueryRow(ctx,
		`INSERT INTO suppliers (name, region) VALUES ('Сахарный завод','Худжанд') RETURNING id`).Scan(&p.supplier))

	return p
}

func (p plant) onHand(t *testing.T, item uuid.UUID, batch uuid.NullUUID, loc uuid.UUID) decimal.Decimal {
	t.Helper()
	b, err := p.inv.BalanceOf(context.Background(),
		inventory.Position{ItemID: item, BatchID: batch, LocationID: loc})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (p plant) batchOf(t *testing.T, moID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := p.pool.QueryRow(context.Background(),
		`SELECT batch_id FROM manufacturing_orders WHERE id=$1`, moID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// ---------------------------------------------------------------------------
// Workflow 1 — Procurement
// request → approval → PO → delivery → inspection → receipt into stock
// ---------------------------------------------------------------------------

func TestWorkflowProcurement(t *testing.T) {
	t.Parallel()
	p := newPlant(t)
	ctx := context.Background()

	po, err := p.proc.CreateOrder(ctx, p.actor, procurement.OrderInput{
		PONo: "PO-0231", SupplierID: p.supplier,
		Lines: []procurement.LineInput{{ItemID: p.sugar, Qty: dec("500"), UnitPrice: dec("12.50")}},
	})
	if err != nil {
		t.Fatalf("create PO: %v", err)
	}
	if po.Status != procurement.StatusDraft {
		t.Fatalf("new PO status = %s, want draft", po.Status)
	}

	// Submitted for approval — no special permission needed to ask.
	if _, err := p.proc.Transition(ctx, p.actor, procurement.TransitionInput{
		POID: po.ID, To: procurement.StatusApproval,
	}); err != nil {
		t.Fatalf("submit for approval: %v", err)
	}

	// Approving commits company money and needs procurement:approve.
	if _, err := p.proc.Transition(ctx, p.actor, procurement.TransitionInput{
		POID: po.ID, To: procurement.StatusConfirmed, HasApprove: false,
	}); err == nil {
		t.Fatal("a purchase order was approved without procurement:approve")
	}
	if _, err := p.proc.Transition(ctx, p.actor, procurement.TransitionInput{
		POID: po.ID, To: procurement.StatusConfirmed, HasApprove: true,
	}); err != nil {
		t.Fatalf("approve: %v", err)
	}

	if _, err := p.proc.Transition(ctx, p.actor, procurement.TransitionInput{
		POID: po.ID, To: procurement.StatusInTransit,
	}); err != nil {
		t.Fatalf("in transit: %v", err)
	}

	// Receipt. This is the step that puts material into the ledger.
	lines, err := p.proc.Lines(ctx, po.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.proc.Receive(ctx, p.actor, procurement.ReceiptInput{
		POID: po.ID, LocationID: p.raw,
		Lines: []procurement.ReceiptLineInput{{POLineID: lines[0].ID, Qty: dec("500")}},
	}); err != nil {
		t.Fatalf("receive: %v", err)
	}

	if got := p.onHand(t, p.sugar, uuid.NullUUID{}, p.raw); !got.Equal(dec("500")) {
		t.Errorf("raw store holds %s, want 500 — receipt must post to the ledger", got.StringFixed(3))
	}

	// Over-receipt is refused: stock with no purchase behind it.
	if _, err := p.proc.Receive(ctx, p.actor, procurement.ReceiptInput{
		POID: po.ID, LocationID: p.raw,
		Lines: []procurement.ReceiptLineInput{{POLineID: lines[0].ID, Qty: dec("1")}},
	}); err == nil {
		t.Error("over-receipt was accepted")
	}
}

// ---------------------------------------------------------------------------
// Workflow 2 — Production
// plan → material issue → batch → output into quarantine
// ---------------------------------------------------------------------------

func TestWorkflowProduction(t *testing.T) {
	t.Parallel()
	p := newPlant(t)
	ctx := context.Background()

	// Material on hand to consume.
	if _, err := p.inv.Post(ctx, p.actor, inventory.Movement{
		Position: inventory.Position{ItemID: p.sugar, LocationID: p.raw},
		QtyDelta: dec("500"), Reason: inventory.ReasonGoodsReceipt,
	}); err != nil {
		t.Fatal(err)
	}

	mo, err := p.prod.Create(ctx, p.actor, production.CreateInput{
		MONo: "MO-0612", ItemID: p.juice, BatchNo: "B-2617", PlannedQty: dec("12000"),
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	// Material issue — a negative movement, so the ledger's guard applies.
	refType := "manufacturing_order"
	if _, err := p.inv.Post(ctx, p.actor, inventory.Movement{
		Position: inventory.Position{ItemID: p.sugar, LocationID: p.raw},
		QtyDelta: dec("-120"), Reason: inventory.ReasonMaterialIssue,
		RefType: &refType, RefID: uuid.NullUUID{UUID: mo.ID, Valid: true},
	}); err != nil {
		t.Fatalf("material issue: %v", err)
	}
	if got := p.onHand(t, p.sugar, uuid.NullUUID{}, p.raw); !got.Equal(dec("380")) {
		t.Errorf("raw store = %s, want 380", got.StringFixed(3))
	}

	// Output recorded across two shifts.
	for _, e := range []struct{ good, scrap string }{{"4300", "150"}, {"4340", "150"}} {
		if _, err := p.prod.RecordEntry(ctx, p.actor, production.EntryInput{
			MOID: mo.ID, GoodQty: dec(e.good), ScrapQty: dec(e.scrap),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := p.prod.Complete(ctx, p.actor, mo.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}

	batchID := p.batchOf(t, mo.ID)
	batch := uuid.NullUUID{UUID: batchID, Valid: true}

	// THE assertion: output lands in QUARANTINE, and nothing is sellable yet.
	if got := p.onHand(t, p.juice, batch, p.quaran); !got.Equal(dec("8640")) {
		t.Errorf("quarantine = %s, want 8640", got.StringFixed(3))
	}
	if got := p.onHand(t, p.juice, batch, p.finished); !got.IsZero() {
		t.Errorf("finished goods = %s, want 0 before QC", got.StringFixed(3))
	}

	var status string
	if err := p.pool.QueryRow(ctx, `SELECT status FROM batches WHERE id=$1`, batchID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != quality.StatusQuarantine {
		t.Errorf("batch status = %s, want quarantine", status)
	}
	if quality.CanSellOrShip(status) {
		t.Fatal("production alone made the batch sellable")
	}
}

// ---------------------------------------------------------------------------
// Workflow 3 — Quality
// tests recorded → release or reject → stock becomes sellable
// ---------------------------------------------------------------------------

func TestWorkflowQuality(t *testing.T) {
	t.Parallel()
	p := newPlant(t)
	ctx := context.Background()

	mo, batchID := p.produceBatch(t, "MO-0612", "B-2617", "8640")
	_ = mo
	batch := uuid.NullUUID{UUID: batchID, Valid: true}

	// QC records its results. Recording alone changes nothing.
	for _, tc := range []struct {
		kind, value string
		passed      bool
	}{
		{"ph", "3.6", true},
		{"microbiology", "соответствует", true},
		{"brix", "11.2", true},
	} {
		v, ok := tc.value, tc.passed
		if _, err := p.qc.RecordTest(ctx, p.actor, quality.TestInput{
			BatchID: batchID, TestType: tc.kind, ResultValue: &v, Passed: &ok,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Before release the batch cannot be sold or shipped.
	if err := p.sellabilityOf(t, batchID); err == nil {
		t.Fatal("a quarantined batch was sellable")
	}

	// Release requires quality:approve.
	if _, err := p.qc.Transition(ctx, p.actor, quality.TransitionInput{
		BatchID: batchID, To: quality.StatusReleased, HasApprove: false,
	}); err == nil {
		t.Fatal("a batch was released without quality:approve")
	}
	if _, err := p.qc.Transition(ctx, p.actor, quality.TransitionInput{
		BatchID: batchID, To: quality.StatusReleased, HasApprove: true,
	}); err != nil {
		t.Fatalf("release: %v", err)
	}

	if err := p.sellabilityOf(t, batchID); err != nil {
		t.Fatalf("a released batch is not sellable: %v", err)
	}

	// The stock moves from quarantine to finished goods as a transfer: two rows,
	// netting to zero, so nothing is created by the change of status.
	if _, err := p.inv.Transfer(ctx, p.actor,
		inventory.Position{ItemID: p.juice, BatchID: batch, LocationID: p.quaran},
		p.finished, dec("8640"), nil); err != nil {
		t.Fatalf("transfer to finished goods: %v", err)
	}
	if got := p.onHand(t, p.juice, batch, p.quaran); !got.IsZero() {
		t.Errorf("quarantine = %s, want 0", got.StringFixed(3))
	}
	if got := p.onHand(t, p.juice, batch, p.finished); !got.Equal(dec("8640")) {
		t.Errorf("finished goods = %s, want 8640", got.StringFixed(3))
	}

	// And the decision is on the record with a name against it.
	events, err := p.qc.StatusEvents(ctx, batchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 1 || events[0].DecidedByName == "" {
		t.Error("the release does not name who decided it")
	}
}

// ---------------------------------------------------------------------------
// Workflow 4 — Sales
// inquiry → lead → order → stock check → shipment of a released batch
// ---------------------------------------------------------------------------

func TestWorkflowSales(t *testing.T) {
	t.Parallel()
	p := newPlant(t)
	ctx := context.Background()

	// An enquiry arrives from the public website.
	inq, err := p.inq.Submit(ctx, inquiries.SubmitInput{
		Type: inquiries.TypeWholesale, Name: "Ориён Маркет",
		Company: strptr("Ориён Маркет"), Contact: "+992 000 00 00",
	})
	if err != nil {
		t.Fatalf("submit enquiry: %v", err)
	}
	if !strings.HasPrefix(inq.ReferenceNo, "WR-") {
		t.Errorf("reference = %s, want a WR- prefix", inq.ReferenceNo)
	}

	lead, err := p.inq.ConvertToLead(ctx, p.actor, inq.ID)
	if err != nil {
		t.Fatalf("convert to lead: %v", err)
	}
	// The reference travels with the lead, so the trail from website to order is
	// unbroken (docs/05-MODULES.md:164).
	if lead.Source == nil || *lead.Source != inq.ReferenceNo {
		t.Errorf("the lead did not carry the reference: %v", lead.Source)
	}

	// Product exists but is still quarantined.
	_, batchID := p.produceBatch(t, "MO-0612", "B-2617", "8640")
	batch := uuid.NullUUID{UUID: batchID, Valid: true}

	so, err := p.sale.CreateOrder(ctx, p.actor, sales.OrderInput{
		SONo: "SO-0912", CustomerID: p.customer,
		Lines: []sales.OrderLineInput{{
			ItemID: p.juice, BatchID: batch, Qty: dec("240"), UnitPrice: dec("18.50"),
		}},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	// Confirming against a quarantined batch is refused. This is the rule that
	// stops unreleased product being sold.
	if _, err := p.sale.ConfirmOrder(ctx, p.actor, so.ID, p.quaran); err == nil {
		t.Fatal("an order was confirmed against a quarantined batch")
	}

	// Release, move to finished goods, then confirm.
	if _, err := p.qc.Transition(ctx, p.actor, quality.TransitionInput{
		BatchID: batchID, To: quality.StatusReleased, HasApprove: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.inv.Transfer(ctx, p.actor,
		inventory.Position{ItemID: p.juice, BatchID: batch, LocationID: p.quaran},
		p.finished, dec("8640"), nil); err != nil {
		t.Fatal(err)
	}

	if _, err := p.sale.ConfirmOrder(ctx, p.actor, so.ID, p.finished); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Confirming consumed stock.
	if got := p.onHand(t, p.juice, batch, p.finished); !got.Equal(dec("8400")) {
		t.Errorf("finished goods = %s, want 8400 after selling 240", got.StringFixed(3))
	}

	// Shipment of the released batch.
	sh, err := p.sale.CreateShipment(ctx, p.actor, sales.ShipmentInput{
		TripNo: "TR-0044", RouteFrom: strptr("Хорог"), RouteTo: strptr("Душанбе"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.sale.LoadLine(ctx, p.actor, sh.ID, p.juice, batchID, dec("240")); err != nil {
		t.Fatalf("load: %v", err)
	}
}

// The logistics side of the same rule, from the other direction.
func TestWorkflowShipmentRefusesUnreleasedBatch(t *testing.T) {
	t.Parallel()
	p := newPlant(t)
	ctx := context.Background()

	_, batchID := p.produceBatch(t, "MO-0612", "B-2617", "1000") // quarantined

	sh, err := p.sale.CreateShipment(ctx, p.actor, sales.ShipmentInput{TripNo: "TR-0044"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.sale.LoadLine(ctx, p.actor, sh.ID, p.juice, batchID, dec("100")); err == nil {
		t.Fatal("a quarantined batch was loaded onto a shipment")
	}

	// Rejected is refused too, not merely quarantined.
	reason := "не прошла микробиологию"
	if _, err := p.qc.Transition(ctx, p.actor, quality.TransitionInput{
		BatchID: batchID, To: quality.StatusRejected, Reason: &reason, HasApprove: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.sale.LoadLine(ctx, p.actor, sh.ID, p.juice, batchID, dec("100")); err == nil {
		t.Fatal("a rejected batch was loaded onto a shipment")
	}
}

// ---------------------------------------------------------------------------
// Workflow 5 — Complaint
// complaint with reference → batch traceability → investigation → closure
// ---------------------------------------------------------------------------

func TestWorkflowComplaint(t *testing.T) {
	t.Parallel()
	p := newPlant(t)
	ctx := context.Background()

	_, batchID := p.produceBatch(t, "MO-0612", "B-2617", "8640")
	if _, err := p.qc.Transition(ctx, p.actor, quality.TransitionInput{
		BatchID: batchID, To: quality.StatusReleased, HasApprove: true,
	}); err != nil {
		t.Fatal(err)
	}

	// A complaint must name a batch, or traceability has no entry point.
	if _, err := p.inq.Submit(ctx, inquiries.SubmitInput{
		Type: inquiries.TypeComplaint, Name: "Покупатель", Contact: "+992 000 00 00",
	}); err == nil {
		t.Fatal("a complaint with no batch was accepted")
	}

	complaint, err := p.inq.Submit(ctx, inquiries.SubmitInput{
		Type: inquiries.TypeComplaint, Name: "Покупатель", Contact: "+992 000 00 00",
		Message: strptr("Посторонний привкус"),
		BatchID: uuid.NullUUID{UUID: batchID, Valid: true},
	})
	if err != nil {
		t.Fatalf("submit complaint: %v", err)
	}
	if !strings.HasPrefix(complaint.ReferenceNo, "CP-") {
		t.Errorf("reference = %s, want a CP- prefix", complaint.ReferenceNo)
	}

	// Traceability: from the complaint's batch, the investigator reaches the QC
	// tests, the production order and the movements — the whole chain.
	tests, err := p.qc.TestsFor(ctx, batchID)
	if err != nil {
		t.Fatal(err)
	}
	_ = tests

	var moNo string
	if err := p.pool.QueryRow(ctx,
		`SELECT mo_no FROM manufacturing_orders WHERE batch_id=$1`, batchID).Scan(&moNo); err != nil {
		t.Fatalf("the complaint's batch does not trace to a production order: %v", err)
	}
	if moNo != "MO-0612" {
		t.Errorf("traced to %s, want MO-0612", moNo)
	}

	var movements int
	if err := p.pool.QueryRow(ctx,
		`SELECT count(*) FROM stock_movements WHERE batch_id=$1`, batchID).Scan(&movements); err != nil {
		t.Fatal(err)
	}
	if movements == 0 {
		t.Error("the batch has no movement history to investigate")
	}

	// Investigation concludes in a recall: released → rejected with a reason.
	reason := "Жалоба " + complaint.ReferenceNo + ": посторонний привкус, партия отозвана"
	if _, err := p.qc.Transition(ctx, p.actor, quality.TransitionInput{
		BatchID: batchID, To: quality.StatusRejected, Reason: &reason, HasApprove: true,
	}); err != nil {
		t.Fatalf("recall: %v", err)
	}

	// The recall is on the record, naming the complaint that caused it.
	events, err := p.qc.StatusEvents(ctx, batchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[0].Reason == nil ||
		!strings.Contains(*events[0].Reason, complaint.ReferenceNo) {
		t.Error("the recall does not reference the complaint that caused it")
	}

	// And the recalled batch can no longer be shipped.
	sh, err := p.sale.CreateShipment(ctx, p.actor, sales.ShipmentInput{TripNo: "TR-0099"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.sale.LoadLine(ctx, p.actor, sh.ID, p.juice, batchID, dec("10")); err == nil {
		t.Fatal("a recalled batch could still be shipped")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// produceBatch runs an order through to completion, leaving the batch quarantined
// with `good` units in the quarantine location.
func (p plant) produceBatch(t *testing.T, moNo, batchNo, good string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	mo, err := p.prod.Create(ctx, p.actor, production.CreateInput{
		MONo: moNo, ItemID: p.juice, BatchNo: batchNo, PlannedQty: dec("12000"),
		ScheduledFor: pgtype.Date{Valid: false},
	})
	if err != nil {
		t.Fatalf("plan %s: %v", moNo, err)
	}
	if _, err := p.prod.RecordEntry(ctx, p.actor, production.EntryInput{
		MOID: mo.ID, GoodQty: dec(good),
	}); err != nil {
		t.Fatalf("record output: %v", err)
	}
	if _, err := p.prod.Complete(ctx, p.actor, mo.ID); err != nil {
		t.Fatalf("complete %s: %v", moNo, err)
	}
	return mo.ID, p.batchOf(t, mo.ID)
}

// sellabilityOf reports whether the batch may currently leave the building.
func (p plant) sellabilityOf(t *testing.T, batchID uuid.UUID) error {
	t.Helper()
	var status, no string
	if err := p.pool.QueryRow(context.Background(),
		`SELECT status, batch_no FROM batches WHERE id=$1`, batchID).Scan(&status, &no); err != nil {
		t.Fatal(err)
	}
	if !quality.CanSellOrShip(status) {
		return errNotSellable{status: status}
	}
	return nil
}

type errNotSellable struct{ status string }

func (e errNotSellable) Error() string { return "batch is " + e.status }

func strptr(s string) *string { return &s }
