// Package quality implements Качество и безопасность.
//
// docs/05-MODULES.md:138 calls this the regulatory heart of the system and gives
// it the highest test coverage in the codebase. Everything here exists to make
// one guarantee true: a batch cannot be sold or shipped unless a person holding
// quality:approve decided it could, and that decision is recorded permanently
// with their name on it.
//
// The transition matrix (docs/02-SCHEMA.md §7):
//
//	in_production → quarantine   automatic on production completion
//	quarantine    → released     requires quality:approve
//	quarantine    → rejected     requires quality:approve
//	released      → rejected     recall — requires quality:approve, reason mandatory
//	rejected      → (terminal)
//
// Everything not in that list is illegal, including every self-transition. A
// batch that "moves" from released to released would write an event implying a
// second decision that nobody made.
package quality

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/alerts"
	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

const Resource = rbac.Quality

// Batch statuses.
const (
	StatusInProduction = "in_production"
	StatusQuarantine   = "quarantine"
	StatusReleased     = "released"
	StatusRejected     = "rejected"
)

var Statuses = []string{StatusInProduction, StatusQuarantine, StatusReleased, StatusRejected}

// TestTypes mirrors the CHECK constraint (docs/05-MODULES.md:144).
var TestTypes = []string{"ph", "microbiology", "brix", "viscosity", "metal_detection", "organoleptic"}

// Transition is one legal move.
type Transition struct {
	From, To string
	// RequiresApprove means quality:approve, not merely quality:manage. Recording
	// a test result is data entry; releasing a batch is an act of authority
	// (docs/04-RBAC.md §3).
	RequiresApprove bool
	// RequiresReason means the move is refused without an explanation. A recall
	// with no stated cause is not a record of anything.
	RequiresReason bool
}

// legalTransitions is the complete matrix. Anything absent is illegal.
var legalTransitions = []Transition{
	// Automatic on production completion — a machine event, not a decision, so it
	// needs no approval. Production staff must not be able to skip past it.
	{From: StatusInProduction, To: StatusQuarantine, RequiresApprove: false},

	{From: StatusQuarantine, To: StatusReleased, RequiresApprove: true},
	{From: StatusQuarantine, To: StatusRejected, RequiresApprove: true},

	// A recall. Modelled as released → rejected with a mandatory reason rather
	// than a separate status (docs/02-SCHEMA.md:314), so the batch's history reads
	// as one continuous chain instead of branching into a parallel vocabulary.
	{From: StatusReleased, To: StatusRejected, RequiresApprove: true, RequiresReason: true},
}

// Lookup returns the transition, if legal.
func Lookup(from, to string) (Transition, bool) {
	for _, t := range legalTransitions {
		if t.From == from && t.To == to {
			return t, true
		}
	}
	return Transition{}, false
}

// LegalTransitions exposes the matrix for tests and for the UI's action buttons.
func LegalTransitions() []Transition { return append([]Transition(nil), legalTransitions...) }

// IsTerminal reports whether a status admits no further moves.
func IsTerminal(status string) bool {
	for _, t := range legalTransitions {
		if t.From == status {
			return false
		}
	}
	return true
}

// CanSellOrShip reports whether a batch may leave the building.
//
// Enforced in the sales and logistics domains, not only in the UI
// (docs/02-SCHEMA.md:316). This function is the single definition both consult,
// so the rule cannot drift between them.
func CanSellOrShip(status string) bool { return status == StatusReleased }

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// TestSortSpec governs the tests attached to one batch.
var TestSortSpec = common.SortSpec{
	Allowed:     []string{"tested_at", "test_type"},
	Default:     "tested_at",
	DefaultDesc: true,
}

// SortSpec governs the Качество batch list.
//
// The list's natural order puts quarantined batches first, because those are the
// only ones waiting on a human — that ordering is in the query itself. This
// whitelist governs only what an explicit ?sort= may override it with.
var SortSpec = common.SortSpec{
	Allowed:     []string{"batch_no", "produced_on", "expires_on", "status"},
	Default:     "produced_on",
	DefaultDesc: true,
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

type TestInput struct {
	BatchID     uuid.UUID
	TestType    string
	ResultValue *string
	Passed      *bool
	Notes       *string
}

// RecordTest stores a QC result. Recording a result is data entry and needs
// quality:manage; it does not move the batch.
func (s *Service) RecordTest(ctx context.Context, actor uuid.UUID, in TestInput) (db.QualityTest, error) {
	if !contains(TestTypes, in.TestType) {
		return db.QualityTest{}, common.Validation(common.FieldError{
			Field: "test_type", Code: "invalid", Message: "Неизвестный тип испытания",
		})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.QualityTest{}, fmt.Errorf("quality: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	if _, err := q.GetBatchByID(ctx, in.BatchID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.QualityTest{}, common.Validation(common.FieldError{
				Field: "batch_id", Code: "not_found", Message: "Партия не найдена",
			})
		}
		return db.QualityTest{}, fmt.Errorf("quality: load batch: %w", err)
	}

	test, err := q.CreateQualityTest(ctx, db.CreateQualityTestParams{
		BatchID:     in.BatchID,
		TestType:    in.TestType,
		ResultValue: in.ResultValue,
		Passed:      in.Passed,
		InspectorID: uuid.NullUUID{UUID: actor, Valid: true},
		Notes:       in.Notes,
	})
	if err != nil {
		return db.QualityTest{}, fmt.Errorf("quality: create test: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionCreate,
		Resource:   Resource,
		ResourceID: audit.Target(test.ID),
		After: map[string]any{
			"batch_id": in.BatchID.String(), "test_type": in.TestType, "passed": in.Passed,
		},
	}); err != nil {
		return db.QualityTest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.QualityTest{}, fmt.Errorf("quality: commit: %w", err)
	}
	return test, nil
}

// ---------------------------------------------------------------------------
// Transitions
// ---------------------------------------------------------------------------

// TransitionInput describes a requested move.
type TransitionInput struct {
	BatchID uuid.UUID
	To      string
	Reason  *string
	// HasApprove is the caller's quality:approve grant. Passed in rather than
	// re-derived here so authorization stays in one place, but checked here too:
	// the HTTP route guard protects the endpoint, and this protects the rule from
	// any other caller — production completion calls straight into the domain.
	HasApprove bool
}

// Transition moves a batch's status, writing the immutable event that records who
// decided and why.
//
// Also usable inside a caller's transaction via TransitionTx, because production
// completion must move the batch and post the stock movement atomically.
func (s *Service) Transition(ctx context.Context, actor uuid.UUID, in TransitionInput) (db.Batch, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Batch{}, fmt.Errorf("quality: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	batch, err := s.TransitionTx(ctx, tx, actor, in)
	if err != nil {
		return db.Batch{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Batch{}, fmt.Errorf("quality: commit: %w", err)
	}
	return batch, nil
}

// TransitionTx performs the move inside a caller's transaction.
func (s *Service) TransitionTx(ctx context.Context, tx pgx.Tx, actor uuid.UUID, in TransitionInput) (db.Batch, error) {
	q := db.New(tx)

	batch, err := q.GetBatchByID(ctx, in.BatchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Batch{}, common.NotFound()
		}
		return db.Batch{}, fmt.Errorf("quality: load batch: %w", err)
	}

	rule, legal := Lookup(batch.Status, in.To)
	if !legal {
		// Named explicitly: "нельзя перевести из X в Y" tells the operator what
		// the system thinks the current state is, which is usually the surprise.
		return db.Batch{}, common.BusinessRule(fmt.Sprintf(
			"Недопустимый переход статуса партии: из «%s» в «%s».", batch.Status, in.To))
	}
	if rule.RequiresApprove && !in.HasApprove {
		return db.Batch{}, common.Forbidden()
	}
	if rule.RequiresReason && (in.Reason == nil || strings.TrimSpace(*in.Reason) == "") {
		return db.Batch{}, common.Validation(common.FieldError{
			Field: "reason", Code: "required",
			Message: "Укажите причину отзыва партии",
		})
	}

	// Guarded on the current status, so a concurrent transition cannot slip
	// between the read above and this write.
	updated, err := q.TransitionBatchStatus(ctx, db.TransitionBatchStatusParams{
		ID: in.BatchID, FromStatus: batch.Status, ToStatus: in.To,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Batch{}, common.BusinessRule(
				"Статус партии изменился другим пользователем. Обновите страницу.")
		}
		return db.Batch{}, fmt.Errorf("quality: transition: %w", err)
	}

	// The evidence row. Immutable by construction: batch_status_events has no
	// deleted_at and no version.
	if _, err := q.InsertBatchStatusEvent(ctx, db.InsertBatchStatusEventParams{
		BatchID:    in.BatchID,
		FromStatus: &batch.Status,
		ToStatus:   in.To,
		DecidedBy:  actor,
		Reason:     in.Reason,
	}); err != nil {
		return db.Batch{}, fmt.Errorf("quality: status event: %w", err)
	}

	// Audited as `approve` when it carried authority, `update` when it was the
	// automatic move out of production. The verb is the whole reason the audit
	// write lives in the domain rather than in a trigger.
	action := audit.ActionUpdate
	if rule.RequiresApprove {
		action = audit.ActionApprove
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     action,
		Resource:   Resource,
		ResourceID: audit.Target(in.BatchID),
		Before:     map[string]any{"status": batch.Status},
		After:      map[string]any{"status": in.To, "reason": in.Reason, "batch_no": batch.BatchNo},
	}); err != nil {
		return db.Batch{}, err
	}

	// Two of the three persisted notifications originate here
	// (docs/05-MODULES.md §17). They are written on the caller's transaction, so a
	// rolled-back transition leaves no notification behind.
	if kind, level, ok := notifyFor(in.To); ok {
		title := fmt.Sprintf("Партия %s — %s", batch.BatchNo, statusNoun(in.To))
		body := ""
		if in.Reason != nil {
			body = *in.Reason
		}
		if err := alerts.Emit(ctx, tx, audit.Actor(actor), kind, Resource, in.BatchID, level, title, body); err != nil {
			return db.Batch{}, fmt.Errorf("quality: notify: %w", err)
		}
	}
	return updated, nil
}

// notifyFor maps a destination status to its notification, if it has one.
//
// Only quarantine and rejection notify. A release is good news that the person
// who approved it already knows, and notifying on it would train the factory to
// dismiss the bell without reading it.
func notifyFor(to string) (alerts.Kind, common.Level, bool) {
	switch to {
	case StatusQuarantine:
		return alerts.KindBatchQuarantined, common.LevelWarn, true
	case StatusRejected:
		return alerts.KindBatchRejected, common.LevelDanger, true
	}
	return "", "", false
}

func statusNoun(status string) string {
	switch status {
	case StatusQuarantine:
		return "на карантине"
	case StatusRejected:
		return "забракована"
	}
	return status
}

// EnsureSellable refuses a batch that may not leave the building.
//
// Called by sales and logistics. Both must consult this rather than compare the
// string themselves, so there is exactly one definition of "sellable"
// (docs/02-SCHEMA.md:316, docs/05-MODULES.md:212).
func EnsureSellable(batch db.Batch) error {
	if !CanSellOrShip(batch.Status) {
		return common.BusinessRule(fmt.Sprintf(
			"Партия %s имеет статус «%s» и не может быть продана или отгружена. Отгружать можно только выпущенные партии.",
			batch.BatchNo, batch.Status))
	}
	return nil
}

// StatusEvents returns the batch's decision history, newest first.
func (s *Service) StatusEvents(ctx context.Context, batchID uuid.UUID) ([]db.ListBatchStatusEventsRow, error) {
	rows, err := db.New(s.pool).ListBatchStatusEvents(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("quality: status events: %w", err)
	}
	return rows, nil
}

// TestsFor returns every recorded test for a batch.
func (s *Service) TestsFor(ctx context.Context, batchID uuid.UUID) ([]db.ListQualityTestsRow, error) {
	rows, err := db.New(s.pool).ListQualityTests(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("quality: tests: %w", err)
	}
	return rows, nil
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// AllowedFrom lists the destinations legally reachable from `status` by an actor
// with (or without) quality:approve.
//
// Derived from the same table the domain enforces, so a button can never be
// offered for a move that will then be refused — and adding a transition to the
// matrix adds its button with no second edit.
func AllowedFrom(status string, hasApprove bool) []string {
	out := make([]string, 0, len(legalTransitions))
	for _, t := range legalTransitions {
		if t.From != status {
			continue
		}
		if t.RequiresApprove && !hasApprove {
			continue
		}
		out = append(out, t.To)
	}
	return out
}

// BatchWithItem loads a batch together with enough of its product to name it.
func (s *Service) BatchWithItem(ctx context.Context, id uuid.UUID) (db.GetBatchWithItemRow, error) {
	row, err := db.New(s.pool).GetBatchWithItem(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.GetBatchWithItemRow{}, common.NotFound()
		}
		return db.GetBatchWithItemRow{}, fmt.Errorf("quality: batch: %w", err)
	}
	return row, nil
}

// List returns batches for the Качество module.
func (s *Service) List(ctx context.Context, p common.Params, status *string) ([]db.ListBatchesForQualityRow, int64, error) {
	q := db.New(s.pool)
	rows, err := q.ListBatchesForQuality(ctx, db.ListBatchesForQualityParams{
		Status: status, Q: nilIfEmpty(p.Query), Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("quality: list: %w", err)
	}
	total, err := q.CountBatchesForQuality(ctx, db.CountBatchesForQualityParams{
		Status: status, Q: nilIfEmpty(p.Query),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("quality: count: %w", err)
	}
	return rows, total, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
