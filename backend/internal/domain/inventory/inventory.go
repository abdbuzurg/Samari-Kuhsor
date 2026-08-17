// Package inventory implements Склад и запасы — the stock ledger.
//
// This is the hardest correctness problem in the system, and the rules below are
// not conventions to be followed but invariants to be enforced:
//
//  1. There is NO balance column. Anywhere. A balance is a SUM over
//     stock_movements, computed at read time (CLAUDE.md §4.2).
//
//  2. Movements are APPEND-ONLY. To fix a wrong receipt of 100, post -100 with
//     reason 'adjustment'. Never update, never tombstone the original: the
//     original is evidence of what someone recorded, and a ledger you can edit
//     is not a ledger (docs/02-SCHEMA.md:240).
//
//  3. A transfer is TWO rows sharing a ref_id — negative at source, positive at
//     destination — so it can never leak or create stock.
//
//  4. Any transaction posting a NEGATIVE delta takes an advisory lock on the
//     position and re-reads the balance inside it. Without that, two shipments
//     each reading "100 available" both post -80 and the batch goes to -60
//     (docs/07-IMPLEMENTATION-PLAN.md I6).
//
//  5. Going negative is refused — EXCEPT for reason 'adjustment', which must be
//     allowed to, because it is the correction mechanism. If the tool for fixing
//     mistakes could be blocked by the mistake, mistakes would be permanent.
//
// The UI must never offer "set stock to X" (docs/05-MODULES.md:112), and no
// request type in this package accepts an absolute quantity.
package inventory

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

const Resource = rbac.Inventory

// Reason codes, mirroring the CHECK constraint in migration 00002.
const (
	ReasonGoodsReceipt     = "goods_receipt"
	ReasonProductionOutput = "production_output"
	ReasonMaterialIssue    = "material_issue"
	ReasonSale             = "sale"
	ReasonTransfer         = "transfer"
	ReasonAdjustment       = "adjustment"
	ReasonScrap            = "scrap"
	ReasonReturn           = "return"
)

var Reasons = []string{
	ReasonGoodsReceipt, ReasonProductionOutput, ReasonMaterialIssue, ReasonSale,
	ReasonTransfer, ReasonAdjustment, ReasonScrap, ReasonReturn,
}

// AllowsNegative reports whether a reason may drive a position below zero.
//
// Only 'adjustment'. Everything else is refused, because negative stock in a food
// traceability system is bad evidence: it says the records disagree with the
// shelf, and the system should force that to be reconciled rather than record it.
func AllowsNegative(reason string) bool { return reason == ReasonAdjustment }

var SortSpec = common.SortSpec{
	Allowed:     []string{"sku", "location", "on_hand"},
	Default:     "sku",
	DefaultDesc: false,
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Position identifies one place stock can sit.
type Position struct {
	ItemID     uuid.UUID
	BatchID    uuid.NullUUID
	LocationID uuid.UUID
}

// Movement is one signed entry to post.
type Movement struct {
	Position
	// QtyDelta is SIGNED: positive receives, negative issues. It is never an
	// absolute total, and there is deliberately no field here that is.
	QtyDelta decimal.Decimal
	Reason   string
	RefType  *string
	RefID    uuid.NullUUID
	Note     *string
}

// ---------------------------------------------------------------------------
// Posting
// ---------------------------------------------------------------------------

// Post writes one movement in its own transaction.
func (s *Service) Post(ctx context.Context, actor uuid.UUID, m Movement) (db.StockMovement, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.StockMovement{}, fmt.Errorf("inventory: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	out, err := s.PostTx(ctx, tx, actor, m)
	if err != nil {
		return db.StockMovement{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.StockMovement{}, fmt.Errorf("inventory: commit: %w", err)
	}
	return out, nil
}

// PostTx writes one movement inside a caller's transaction.
//
// Exported because production, procurement, quality and sales all post movements
// as part of their own atomic operations. A goods receipt that recorded the
// receipt but not the stock — or the reverse — would be worse than either.
func (s *Service) PostTx(ctx context.Context, tx pgx.Tx, actor uuid.UUID, m Movement) (db.StockMovement, error) {
	if err := validateMovement(m); err != nil {
		return db.StockMovement{}, err
	}

	q := db.New(tx)

	// The guard, and the reason it is here rather than in a CHECK constraint: a
	// constraint can only see one row, and the balance is a sum over many.
	if m.QtyDelta.IsNegative() && !AllowsNegative(m.Reason) {
		if err := s.guardSufficientStock(ctx, q, m); err != nil {
			return db.StockMovement{}, err
		}
	}

	out, err := q.InsertStockMovement(ctx, db.InsertStockMovementParams{
		ItemID:     m.ItemID,
		BatchID:    m.BatchID,
		LocationID: m.LocationID,
		QtyDelta:   m.QtyDelta,
		Reason:     m.Reason,
		RefType:    m.RefType,
		RefID:      m.RefID,
		Note:       m.Note,
		CreatedBy:  uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		return db.StockMovement{}, fmt.Errorf("inventory: insert movement: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionCreate,
		Resource:   Resource,
		ResourceID: audit.Target(out.ID),
		After: map[string]any{
			"item_id":   m.ItemID.String(),
			"qty_delta": m.QtyDelta.StringFixed(3),
			"reason":    m.Reason,
		},
	}); err != nil {
		return db.StockMovement{}, err
	}
	return out, nil
}

// guardSufficientStock takes the advisory lock and refuses an overdraw.
//
// The lock is held for the rest of the transaction, so the balance read inside it
// cannot change before the insert. Reading before the lock would be reading a
// number another transaction is free to invalidate.
func (s *Service) guardSufficientStock(ctx context.Context, q *db.Queries, m Movement) error {
	// batch_id is nullable, so it is coalesced to a fixed sentinel: hashing a
	// NULL would make every batchless position share one lock key with every
	// other, serialising unrelated writers.
	batchKey := "-"
	if m.BatchID.Valid {
		batchKey = m.BatchID.UUID.String()
	}
	if err := q.LockStockPosition(ctx, db.LockStockPositionParams{
		Column1: m.ItemID.String(),
		Column2: batchKey,
		Column3: m.LocationID.String(),
	}); err != nil {
		return fmt.Errorf("inventory: lock position: %w", err)
	}

	onHand, err := q.GetPositionBalance(ctx, db.GetPositionBalanceParams{
		ItemID:     m.ItemID,
		BatchID:    m.BatchID,
		LocationID: m.LocationID,
	})
	if err != nil {
		return fmt.Errorf("inventory: read balance: %w", err)
	}

	if onHand.Add(m.QtyDelta).IsNegative() {
		return common.BusinessRule(fmt.Sprintf(
			"Недостаточно остатка: доступно %s, требуется %s. Для исправления ошибочной записи используйте корректировку.",
			onHand.StringFixed(3), m.QtyDelta.Abs().StringFixed(3)))
	}
	return nil
}

// Transfer moves stock between locations as TWO movements sharing a ref_id.
//
// Modelled as a pair rather than a single row with two location columns so that
// the ledger has exactly one shape, and so a transfer nets to zero across the
// warehouse by construction — there is no arithmetic that could make it leak.
func (s *Service) Transfer(
	ctx context.Context, actor uuid.UUID,
	from Position, toLocation uuid.UUID, qty decimal.Decimal, note *string,
) ([]db.StockMovement, error) {
	if !qty.IsPositive() {
		return nil, common.Validation(common.FieldError{
			Field: "qty", Code: "invalid", Message: "Количество для перемещения должно быть больше нуля",
		})
	}
	if from.LocationID == toLocation {
		return nil, common.Validation(common.FieldError{
			Field: "to_location_id", Code: "invalid", Message: "Локации источника и назначения совпадают",
		})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("inventory: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Both legs share this, so the pair can always be found together.
	ref := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	refType := "transfer"

	out, err := s.PostTx(ctx, tx, actor, Movement{
		Position: from, QtyDelta: qty.Neg(), Reason: ReasonTransfer,
		RefType: &refType, RefID: ref, Note: note,
	})
	if err != nil {
		return nil, err
	}
	in, err := s.PostTx(ctx, tx, actor, Movement{
		Position: Position{ItemID: from.ItemID, BatchID: from.BatchID, LocationID: toLocation},
		QtyDelta: qty, Reason: ReasonTransfer,
		RefType: &refType, RefID: ref, Note: note,
	})
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("inventory: commit: %w", err)
	}
	return []db.StockMovement{out, in}, nil
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// BalanceOf returns the exact on-hand quantity for a position.
func (s *Service) BalanceOf(ctx context.Context, p Position) (decimal.Decimal, error) {
	onHand, err := db.New(s.pool).GetPositionBalance(ctx, db.GetPositionBalanceParams{
		ItemID: p.ItemID, BatchID: p.BatchID, LocationID: p.LocationID,
	})
	if err != nil {
		return decimal.Zero, fmt.Errorf("inventory: balance: %w", err)
	}
	return onHand, nil
}

// ListFilter narrows the stock list.
type ListFilter struct {
	ItemID uuid.NullUUID
	Zone   *string
}

func (s *Service) List(ctx context.Context, p common.Params, f ListFilter) ([]db.ListStockBalancesRow, int64, error) {
	q := db.New(s.pool)

	rows, err := q.ListStockBalances(ctx, db.ListStockBalancesParams{
		ItemID: f.ItemID,
		Zone:   f.Zone,
		Q:      nilIfEmpty(p.Query),
		Limit:  p.Limit(),
		Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("inventory: list: %w", err)
	}
	total, err := q.CountStockBalances(ctx, db.CountStockBalancesParams{
		ItemID: f.ItemID, Zone: f.Zone, Q: nilIfEmpty(p.Query),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("inventory: count: %w", err)
	}
	return rows, total, nil
}

// Ledger returns the movement history for one position with a running balance.
func (s *Service) Ledger(ctx context.Context, p Position, limit, offset int32) ([]db.ListMovementsForPositionRow, error) {
	rows, err := db.New(s.pool).ListMovementsForPosition(ctx, db.ListMovementsForPositionParams{
		ItemID: p.ItemID, BatchID: p.BatchID, LocationID: p.LocationID,
		Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("inventory: ledger: %w", err)
	}
	return rows, nil
}

// EnsureLocation looks up a location, returning a validation error rather than a
// 500 when it does not exist.
func (s *Service) EnsureLocation(ctx context.Context, q *db.Queries, id uuid.UUID, field string) error {
	if _, err := q.GetLocationByID(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return common.Validation(common.FieldError{
				Field: field, Code: "not_found", Message: "Локация не найдена",
			})
		}
		return fmt.Errorf("inventory: location: %w", err)
	}
	return nil
}

// QuarantineLocation returns the location production output goes to.
func (s *Service) QuarantineLocation(ctx context.Context, q *db.Queries) (db.Location, error) {
	loc, err := q.GetQuarantineLocation(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// A configuration error, not a user error: production cannot complete
			// anywhere else, so this must be loud.
			return db.Location{}, common.BusinessRule(
				"Не настроена карантинная зона склада. Обратитесь к администратору.")
		}
		return db.Location{}, fmt.Errorf("inventory: quarantine location: %w", err)
	}
	return loc, nil
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func validateMovement(m Movement) error {
	var details []common.FieldError

	if m.QtyDelta.IsZero() {
		// Also a CHECK constraint, but caught here so it names the field.
		details = append(details, common.FieldError{
			Field: "qty_delta", Code: "invalid", Message: "Количество не может быть нулевым",
		})
	}
	if !contains(Reasons, m.Reason) {
		details = append(details, common.FieldError{
			Field: "reason", Code: "invalid", Message: "Неизвестная причина движения",
		})
	}
	if m.ItemID == uuid.Nil {
		details = append(details, common.FieldError{
			Field: "item_id", Code: "required", Message: "Укажите товар",
		})
	}
	if m.LocationID == uuid.Nil {
		details = append(details, common.FieldError{
			Field: "location_id", Code: "required", Message: "Укажите локацию",
		})
	}
	if len(details) > 0 {
		return common.Validation(details...)
	}
	return nil
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
