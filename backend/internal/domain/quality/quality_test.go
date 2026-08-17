package quality_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/quality"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// Качество и безопасность — the regulatory heart.
//
// docs/04-RBAC.md:166: "The quarantine transitions need exhaustive tests — every
// from/to pair, legal and illegal, with and without quality:approve."
//
// Exhaustive means exhaustive: 4 statuses × 4 statuses × 2 permission states =
// 32 cases, generated rather than hand-listed, so a status added later is covered
// automatically instead of quietly untested.

type fixture struct {
	pool  *pgxpool.Pool
	svc   *quality.Service
	actor uuid.UUID
	item  uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testsupport.NewDB(t)
	ctx := context.Background()

	var f fixture
	f.pool = pool
	f.svc = quality.NewService(pool)

	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ('qc@samari-kuhsor.tj', 'Н. Инспектор', 'x') RETURNING id`).Scan(&f.actor); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom, status)
		VALUES ('APJ-1000', 'finished_good', 'bottle', 'active') RETURNING id`).Scan(&f.item); err != nil {
		t.Fatal(err)
	}
	return f
}

// batchAt creates a batch already in the given status.
func (f fixture) batchAt(t *testing.T, status string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	no := "B-" + uuid.NewString()[:8]
	if err := f.pool.QueryRow(context.Background(), `
		INSERT INTO batches (batch_no, item_id, status) VALUES ($1, $2, $3) RETURNING id`,
		no, f.item, status).Scan(&id); err != nil {
		t.Fatalf("seed batch in %s: %v", status, err)
	}
	return id
}

func (f fixture) statusOf(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var s string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status FROM batches WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// ---------------------------------------------------------------------------
// THE MATRIX
// ---------------------------------------------------------------------------

// Every from/to pair, with and without quality:approve. Generated from the status
// list so nothing can be omitted by hand.
func TestFullTransitionMatrix(t *testing.T) {
	t.Parallel()

	reason := "Партия отозвана: обнаружено несоответствие"

	for _, from := range quality.Statuses {
		for _, to := range quality.Statuses {
			for _, hasApprove := range []bool{false, true} {
				name := fmt.Sprintf("%s→%s/approve=%v", from, to, hasApprove)
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					f := setup(t)
					batchID := f.batchAt(t, from)

					rule, legal := quality.Lookup(from, to)

					// Always supply a reason: the point of each case is the
					// transition and the permission, not the reason validation,
					// which has its own tests below.
					_, err := f.svc.Transition(context.Background(), f.actor, quality.TransitionInput{
						BatchID: batchID, To: to, Reason: &reason, HasApprove: hasApprove,
					})

					switch {
					case !legal:
						// Every pair not in the matrix must be refused, including
						// every self-transition: a released→released move would
						// write an event implying a decision nobody made.
						if err == nil {
							t.Fatalf("illegal transition %s→%s was accepted", from, to)
						}
						if code := common.AsError(err).Code; code != common.CodeBusinessRule {
							t.Errorf("code = %s, want business_rule", code)
						}
						if got := f.statusOf(t, batchID); got != from {
							t.Errorf("status changed to %s despite refusal", got)
						}

					case rule.RequiresApprove && !hasApprove:
						// The whole point of the `approve` action: editing a QC
						// record and signing a batch out of quarantine are
						// different authorities (docs/01-DECISIONS.md D9).
						if err == nil {
							t.Fatalf("%s→%s succeeded without quality:approve", from, to)
						}
						if code := common.AsError(err).Code; code != common.CodeForbidden {
							t.Errorf("code = %s, want forbidden (403)", code)
						}
						if got := f.statusOf(t, batchID); got != from {
							t.Errorf("status changed to %s despite refusal", got)
						}

					default:
						if err != nil {
							t.Fatalf("legal transition %s→%s was refused: %v", from, to, err)
						}
						if got := f.statusOf(t, batchID); got != to {
							t.Errorf("status = %s, want %s", got, to)
						}
					}
				})
			}
		}
	}
}

// rejected admits no moves at all. Stated separately from the matrix because it
// is the property that makes a rejection final rather than a suggestion.
func TestRejectedIsTerminal(t *testing.T) {
	t.Parallel()

	if !quality.IsTerminal(quality.StatusRejected) {
		t.Fatal("rejected is not terminal")
	}
	for _, s := range []string{quality.StatusInProduction, quality.StatusQuarantine, quality.StatusReleased} {
		if quality.IsTerminal(s) {
			t.Errorf("%s should not be terminal", s)
		}
	}

	f := setup(t)
	batchID := f.batchAt(t, quality.StatusRejected)
	for _, to := range quality.Statuses {
		reason := "попытка"
		_, err := f.svc.Transition(context.Background(), f.actor, quality.TransitionInput{
			BatchID: batchID, To: to, Reason: &reason, HasApprove: true,
		})
		if err == nil {
			t.Errorf("a rejected batch was moved to %s", to)
		}
	}
}

// ---------------------------------------------------------------------------
// The recall
// ---------------------------------------------------------------------------

// docs/02-SCHEMA.md:314 — a recall is released → rejected with a MANDATORY
// reason. A recall with no stated cause is not a record of anything.
func TestRecallRequiresAReason(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	blank := "   "
	for name, reason := range map[string]*string{
		"nil":   nil,
		"empty": ptr(""),
		"blank": &blank,
	} {
		t.Run(name, func(t *testing.T) {
			batchID := f.batchAt(t, quality.StatusReleased)
			_, err := f.svc.Transition(ctx, f.actor, quality.TransitionInput{
				BatchID: batchID, To: quality.StatusRejected, Reason: reason, HasApprove: true,
			})
			if err == nil {
				t.Fatal("a recall with no reason was accepted")
			}
			if code := common.AsError(err).Code; code != common.CodeValidationFailed {
				t.Errorf("code = %s, want validation_failed", code)
			}
			if got := f.statusOf(t, batchID); got != quality.StatusReleased {
				t.Errorf("status changed to %s", got)
			}
		})
	}

	// With a reason it succeeds, and the reason is preserved.
	batchID := f.batchAt(t, quality.StatusReleased)
	reason := "Обнаружено превышение по микробиологии в контрольной пробе"
	if _, err := f.svc.Transition(ctx, f.actor, quality.TransitionInput{
		BatchID: batchID, To: quality.StatusRejected, Reason: &reason, HasApprove: true,
	}); err != nil {
		t.Fatalf("recall with a reason was refused: %v", err)
	}

	events, err := f.svc.StatusEvents(ctx, batchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("%d status events, want 1", len(events))
	}
	if events[0].Reason == nil || *events[0].Reason != reason {
		t.Errorf("the reason was not recorded: %v", events[0].Reason)
	}
}

// Releasing does not need a reason — the QC tests are the justification.
func TestReleaseDoesNotRequireAReason(t *testing.T) {
	t.Parallel()
	f := setup(t)
	batchID := f.batchAt(t, quality.StatusQuarantine)

	if _, err := f.svc.Transition(context.Background(), f.actor, quality.TransitionInput{
		BatchID: batchID, To: quality.StatusReleased, HasApprove: true,
	}); err != nil {
		t.Fatalf("release without a reason was refused: %v", err)
	}
}

// ---------------------------------------------------------------------------
// The evidence trail
// ---------------------------------------------------------------------------

// docs/02-SCHEMA.md:318 — releasing writes an audit entry naming the deciding
// user. This is the evidence behind the website's laboratory-control claim.
func TestEveryTransitionRecordsWhoDecided(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	batchID := f.batchAt(t, quality.StatusQuarantine)
	if _, err := f.svc.Transition(ctx, f.actor, quality.TransitionInput{
		BatchID: batchID, To: quality.StatusReleased, HasApprove: true,
	}); err != nil {
		t.Fatal(err)
	}

	// The immutable event.
	events, err := f.svc.StatusEvents(ctx, batchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("%d events, want 1", len(events))
	}
	if events[0].DecidedBy != f.actor {
		t.Errorf("decided_by = %s, want %s", events[0].DecidedBy, f.actor)
	}
	if events[0].DecidedByName == "" {
		t.Error("the event does not resolve to a person's name")
	}
	if events[0].FromStatus == nil || *events[0].FromStatus != quality.StatusQuarantine {
		t.Errorf("from_status = %v", events[0].FromStatus)
	}

	// And the audit row, as `approve` rather than `update`.
	entry := testsupport.AssertAudited(t, f.pool, "quality", batchID, "approve")
	if entry.ActorID.UUID != f.actor {
		t.Errorf("audit actor = %s", entry.ActorID.UUID)
	}
}

// The automatic move out of production is not an act of authority, so it is
// audited as `update`. The verb distinction is why the audit write lives in the
// domain rather than in a database trigger.
func TestAutomaticQuarantineIsAuditedAsUpdateNotApprove(t *testing.T) {
	t.Parallel()
	f := setup(t)
	batchID := f.batchAt(t, quality.StatusInProduction)

	if _, err := f.svc.Transition(context.Background(), f.actor, quality.TransitionInput{
		BatchID: batchID, To: quality.StatusQuarantine, HasApprove: false,
	}); err != nil {
		t.Fatalf("automatic quarantine was refused: %v", err)
	}
	testsupport.AssertAudited(t, f.pool, "quality", batchID, "update")
}

// batch_status_events is append-only by construction: it has no deleted_at and no
// version, so the schema itself prevents amendment.
func TestStatusEventsAreImmutableByConstruction(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)

	rows, err := pool.Query(context.Background(), `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'batch_status_events'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	cols := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		cols[c] = true
	}
	for _, forbidden := range []string{"deleted_at", "version", "updated_at"} {
		if cols[forbidden] {
			t.Errorf("batch_status_events has %s; the evidence trail must not be amendable", forbidden)
		}
	}
}

// A refused transition must write neither an event nor an audit row.
func TestRefusedTransitionLeavesNoTrace(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	batchID := f.batchAt(t, quality.StatusQuarantine)
	before := testsupport.CountAudit(t, f.pool)

	// Without approve.
	if _, err := f.svc.Transition(ctx, f.actor, quality.TransitionInput{
		BatchID: batchID, To: quality.StatusReleased, HasApprove: false,
	}); err == nil {
		t.Fatal("release without approve was accepted")
	}

	if after := testsupport.CountAudit(t, f.pool); after != before {
		t.Errorf("a refused transition wrote %d audit rows", after-before)
	}
	events, err := f.svc.StatusEvents(ctx, batchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("%d status events after a refused transition", len(events))
	}
}

// ---------------------------------------------------------------------------
// The rule the rest of the system depends on
// ---------------------------------------------------------------------------

// Only released batches may be sold or shipped (docs/02-SCHEMA.md:316). Sales and
// logistics both call EnsureSellable, so there is one definition rather than two
// that can drift.
func TestOnlyReleasedBatchesMaySellOrShip(t *testing.T) {
	t.Parallel()

	for _, status := range quality.Statuses {
		t.Run(status, func(t *testing.T) {
			batch := db.Batch{BatchNo: "B-2617", Status: status}
			err := quality.EnsureSellable(batch)

			if status == quality.StatusReleased {
				if err != nil {
					t.Errorf("a released batch was refused: %v", err)
				}
				if !quality.CanSellOrShip(status) {
					t.Error("CanSellOrShip disagrees with EnsureSellable")
				}
				return
			}

			if err == nil {
				t.Fatalf("a %s batch was accepted for sale or shipment", status)
			}
			if code := common.AsError(err).Code; code != common.CodeBusinessRule {
				t.Errorf("code = %s, want business_rule", code)
			}
			// The message must name the batch and its status: an operator needs to
			// know which batch and why, not just that something was refused.
			if msg := common.AsError(err).Message; msg == "" ||
				!contains(msg, "B-2617") || !contains(msg, status) {
				t.Errorf("unhelpful refusal message: %q", msg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests (the QC records themselves)
// ---------------------------------------------------------------------------

func TestRecordTest(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	batchID := f.batchAt(t, quality.StatusQuarantine)

	value, passed := "4.2", false
	test, err := f.svc.RecordTest(ctx, f.actor, quality.TestInput{
		BatchID: batchID, TestType: "ph", ResultValue: &value, Passed: &passed,
	})
	if err != nil {
		t.Fatalf("record test: %v", err)
	}
	if test.InspectorID.UUID != f.actor {
		t.Errorf("inspector = %s, want %s", test.InspectorID.UUID, f.actor)
	}
	testsupport.AssertAudited(t, f.pool, "quality", test.ID, "create")

	// Recording a failure does NOT move the batch. A failed test is evidence for a
	// decision; it is not the decision, and auto-rejecting would take the judgement
	// away from the person who holds quality:approve.
	if got := f.statusOf(t, batchID); got != quality.StatusQuarantine {
		t.Errorf("a failed test moved the batch to %s on its own", got)
	}
}

func TestRecordTestValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	if _, err := f.svc.RecordTest(ctx, f.actor, quality.TestInput{
		BatchID: f.batchAt(t, quality.StatusQuarantine), TestType: "taste_test",
	}); err == nil {
		t.Error("an unknown test type was accepted")
	}
	if _, err := f.svc.RecordTest(ctx, f.actor, quality.TestInput{
		BatchID: uuid.New(), TestType: "ph",
	}); err == nil {
		t.Error("a test against an unknown batch was accepted")
	}
}

// All six test types from docs/05-MODULES.md:144 must be accepted.
func TestAllDocumentedTestTypesAreAccepted(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	batchID := f.batchAt(t, quality.StatusQuarantine)

	for _, tt := range []string{"ph", "microbiology", "brix", "viscosity", "metal_detection", "organoleptic"} {
		if _, err := f.svc.RecordTest(ctx, f.actor, quality.TestInput{
			BatchID: batchID, TestType: tt,
		}); err != nil {
			t.Errorf("%s was refused: %v", tt, err)
		}
	}
	if len(quality.TestTypes) != 6 {
		t.Errorf("%d test types defined, docs/05-MODULES.md:144 lists 6", len(quality.TestTypes))
	}
}

// ---------------------------------------------------------------------------
// The matrix as data
// ---------------------------------------------------------------------------

// The matrix must match docs/02-SCHEMA.md §7 exactly — restated here
// independently, so a change to the rules shows up as a disagreement.
func TestMatrixMatchesTheSpecification(t *testing.T) {
	t.Parallel()

	want := map[string]struct{ approve, reason bool }{
		"in_production→quarantine": {approve: false, reason: false},
		"quarantine→released":      {approve: true, reason: false},
		"quarantine→rejected":      {approve: true, reason: false},
		"released→rejected":        {approve: true, reason: true},
	}

	got := quality.LegalTransitions()
	if len(got) != len(want) {
		t.Fatalf("%d legal transitions, docs/02-SCHEMA.md §7 lists %d", len(got), len(want))
	}
	for _, tr := range got {
		key := tr.From + "→" + tr.To
		w, ok := want[key]
		if !ok {
			t.Errorf("%s is not in the specification", key)
			continue
		}
		if tr.RequiresApprove != w.approve {
			t.Errorf("%s: RequiresApprove = %v, spec says %v", key, tr.RequiresApprove, w.approve)
		}
		if tr.RequiresReason != w.reason {
			t.Errorf("%s: RequiresReason = %v, spec says %v", key, tr.RequiresReason, w.reason)
		}
	}
}

func ptr(s string) *string { return &s }

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }
