// Package production implements Производство.
//
// The rule that matters most (docs/05-MODULES.md:128):
//
//	Completing an order posts a production_output movement into a QUARANTINE
//	location and moves the batch to `quarantine`. It does NOT make the batch
//	sellable — only Качество can.
//
// That is the seam between "we made it" and "it may be sold", and it is the
// reason production output cannot land in a finished-goods location: if it did,
// the stock would be shippable the moment it was recorded, and the quality gate
// would be advisory.
//
// Actual output, yield and downtime are SUMS over production_entries, never
// columns on the order (docs/02-SCHEMA.md:274).
package production

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/inventory"
	"github.com/qoim/samari/backend/internal/domain/quality"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

const Resource = rbac.Production

// Order statuses (docs/05-MODULES.md:123).
const (
	StatusPlanned    = "planned"
	StatusInProgress = "in_progress"
	StatusQCHold     = "qc_hold"
	StatusDone       = "done"
	StatusCancelled  = "cancelled"
)

var Statuses = []string{StatusPlanned, StatusInProgress, StatusQCHold, StatusDone, StatusCancelled}

var SortSpec = common.SortSpec{
	Allowed:     []string{"mo_no", "scheduled_for", "status"},
	Default:     "scheduled_for",
	DefaultDesc: true,
}

type Service struct {
	pool      *pgxpool.Pool
	inventory *inventory.Service
	quality   *quality.Service
}

func NewService(pool *pgxpool.Pool, inv *inventory.Service, qc *quality.Service) *Service {
	return &Service{pool: pool, inventory: inv, quality: qc}
}

// Totals are computed, never stored.
type Totals struct {
	GoodQty     decimal.Decimal
	ScrapQty    decimal.Decimal
	DowntimeMin int64
	Entries     int64
}

// YieldPercent is good / (good + scrap), to one decimal place.
//
// Returns nil rather than 0 when nothing has been produced: a yield of 0% and
// "no output yet" mean very different things on a shift report, and rendering the
// second as the first would look like a catastrophic run.
func (t Totals) YieldPercent() *decimal.Decimal {
	produced := t.GoodQty.Add(t.ScrapQty)
	if produced.IsZero() {
		return nil
	}
	y := t.GoodQty.Div(produced).Mul(decimal.NewFromInt(100)).Round(1)
	return &y
}

type CreateInput struct {
	MONo         string
	ItemID       uuid.UUID
	BatchNo      string
	Line         *string
	PlannedQty   decimal.Decimal
	ScheduledFor pgtype.Date
}

// Create plans an order and its batch together.
//
// The batch is created here rather than referenced because the MO↔batch relation
// is 1:1 (docs/05-MODULES.md:127) and creating them separately would allow an
// order with no batch, or two orders claiming one.
func (s *Service) Create(ctx context.Context, actor uuid.UUID, in CreateInput) (db.ManufacturingOrder, error) {
	var details []common.FieldError
	if strings.TrimSpace(in.MONo) == "" {
		details = append(details, common.FieldError{
			Field: "mo_no", Code: "required", Message: "Укажите номер заказа",
		})
	}
	if strings.TrimSpace(in.BatchNo) == "" {
		details = append(details, common.FieldError{
			Field: "batch_no", Code: "required", Message: "Укажите номер партии",
		})
	}
	if !in.PlannedQty.IsPositive() {
		details = append(details, common.FieldError{
			Field: "planned_qty", Code: "invalid", Message: "План должен быть больше нуля",
		})
	}
	if len(details) > 0 {
		return db.ManufacturingOrder{}, common.Validation(details...)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.ManufacturingOrder{}, fmt.Errorf("production: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	if _, err := q.GetItemByID(ctx, in.ItemID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ManufacturingOrder{}, common.Validation(common.FieldError{
				Field: "item_id", Code: "not_found", Message: "Товар не найден",
			})
		}
		return db.ManufacturingOrder{}, fmt.Errorf("production: load item: %w", err)
	}

	batch, err := q.CreateBatch(ctx, db.CreateBatchParams{
		BatchNo:   strings.TrimSpace(in.BatchNo),
		ItemID:    in.ItemID,
		CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		if strings.Contains(err.Error(), "batches_batch_no_key") {
			return db.ManufacturingOrder{}, common.Validation(common.FieldError{
				Field: "batch_no", Code: "already_exists", Message: "Партия с таким номером уже существует",
			})
		}
		return db.ManufacturingOrder{}, fmt.Errorf("production: create batch: %w", err)
	}

	mo, err := q.CreateManufacturingOrder(ctx, db.CreateManufacturingOrderParams{
		MoNo:         strings.TrimSpace(in.MONo),
		ItemID:       in.ItemID,
		BatchID:      uuid.NullUUID{UUID: batch.ID, Valid: true},
		Line:         in.Line,
		PlannedQty:   in.PlannedQty,
		ScheduledFor: in.ScheduledFor,
		CreatedBy:    uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		if strings.Contains(err.Error(), "manufacturing_orders_no_key") {
			return db.ManufacturingOrder{}, common.Validation(common.FieldError{
				Field: "mo_no", Code: "already_exists", Message: "Заказ с таким номером уже существует",
			})
		}
		return db.ManufacturingOrder{}, fmt.Errorf("production: create order: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionCreate,
		Resource:   Resource,
		ResourceID: audit.Target(mo.ID),
		After:      map[string]any{"mo_no": mo.MoNo, "batch_no": batch.BatchNo, "planned_qty": in.PlannedQty.StringFixed(3)},
	}); err != nil {
		return db.ManufacturingOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.ManufacturingOrder{}, fmt.Errorf("production: commit: %w", err)
	}
	return mo, nil
}

type EntryInput struct {
	MOID        uuid.UUID
	GoodQty     decimal.Decimal
	ScrapQty    decimal.Decimal
	DowntimeMin int32
	Note        *string
}

// RecordEntry appends a shift entry. Append-only: an entry is what someone
// observed on the line, and observations are not edited afterwards.
func (s *Service) RecordEntry(ctx context.Context, actor uuid.UUID, in EntryInput) (db.ProductionEntry, error) {
	if in.GoodQty.IsNegative() || in.ScrapQty.IsNegative() || in.DowntimeMin < 0 {
		return db.ProductionEntry{}, common.Validation(common.FieldError{
			Field: "good_qty", Code: "invalid", Message: "Значения не могут быть отрицательными",
		})
	}
	if in.GoodQty.IsZero() && in.ScrapQty.IsZero() && in.DowntimeMin == 0 {
		return db.ProductionEntry{}, common.Validation(common.FieldError{
			Field: "good_qty", Code: "invalid", Message: "Запись не содержит данных",
		})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.ProductionEntry{}, fmt.Errorf("production: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	mo, err := q.GetManufacturingOrder(ctx, in.MOID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ProductionEntry{}, common.NotFound()
		}
		return db.ProductionEntry{}, fmt.Errorf("production: load order: %w", err)
	}
	// A finished order's totals are already reflected in stock; a later entry
	// would change the yield without changing the stock it justified.
	if mo.Status == StatusDone || mo.Status == StatusCancelled {
		return db.ProductionEntry{}, common.BusinessRule(
			"Заказ закрыт: добавить выработку невозможно.")
	}

	entry, err := q.InsertProductionEntry(ctx, db.InsertProductionEntryParams{
		MoID:        in.MOID,
		GoodQty:     in.GoodQty,
		ScrapQty:    in.ScrapQty,
		DowntimeMin: in.DowntimeMin,
		Note:        in.Note,
		RecordedBy:  uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		return db.ProductionEntry{}, fmt.Errorf("production: insert entry: %w", err)
	}

	// First output moves the order into progress, so the board reflects the line.
	if mo.Status == StatusPlanned {
		if _, err := q.SetManufacturingOrderStatus(ctx, db.SetManufacturingOrderStatusParams{
			ID: mo.ID, Status: StatusInProgress,
		}); err != nil {
			return db.ProductionEntry{}, fmt.Errorf("production: set status: %w", err)
		}
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionCreate,
		Resource:   Resource,
		ResourceID: audit.Target(entry.ID),
		After: map[string]any{
			"mo_id": in.MOID.String(), "good_qty": in.GoodQty.StringFixed(3),
			"scrap_qty": in.ScrapQty.StringFixed(3), "downtime_min": in.DowntimeMin,
		},
	}); err != nil {
		return db.ProductionEntry{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.ProductionEntry{}, fmt.Errorf("production: commit: %w", err)
	}
	return entry, nil
}

// Complete closes an order: posts the good output into QUARANTINE and moves the
// batch to `quarantine`.
//
// All of it in one transaction. Stock that exists without the batch being
// quarantined would be sellable-looking stock that never passed QC; a quarantined
// batch with no stock would be a QC decision about nothing.
func (s *Service) Complete(ctx context.Context, actor uuid.UUID, moID uuid.UUID) (db.ManufacturingOrder, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.ManufacturingOrder{}, fmt.Errorf("production: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	mo, err := q.GetManufacturingOrder(ctx, moID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ManufacturingOrder{}, common.NotFound()
		}
		return db.ManufacturingOrder{}, fmt.Errorf("production: load order: %w", err)
	}
	if mo.Status == StatusDone {
		// Completing twice would post the output into stock a second time.
		return db.ManufacturingOrder{}, common.BusinessRule("Заказ уже завершён.")
	}
	if mo.Status == StatusCancelled {
		return db.ManufacturingOrder{}, common.BusinessRule("Заказ отменён.")
	}
	if !mo.BatchID.Valid {
		return db.ManufacturingOrder{}, common.BusinessRule("У заказа нет партии.")
	}

	totals, err := q.GetProductionTotals(ctx, moID)
	if err != nil {
		return db.ManufacturingOrder{}, fmt.Errorf("production: totals: %w", err)
	}
	if !totals.GoodQty.IsPositive() {
		return db.ManufacturingOrder{}, common.BusinessRule(
			"Нет годной продукции: завершать нечего.")
	}

	quarantine, err := s.inventory.QuarantineLocation(ctx, q)
	if err != nil {
		return db.ManufacturingOrder{}, err
	}

	refType := "manufacturing_order"
	if _, err := s.inventory.PostTx(ctx, tx, actor, inventory.Movement{
		Position: inventory.Position{
			ItemID:  mo.ItemID,
			BatchID: mo.BatchID,
			// Quarantine, never finished goods. This is the seam.
			LocationID: quarantine.ID,
		},
		QtyDelta: totals.GoodQty,
		Reason:   inventory.ReasonProductionOutput,
		RefType:  &refType,
		RefID:    uuid.NullUUID{UUID: moID, Valid: true},
	}); err != nil {
		return db.ManufacturingOrder{}, err
	}

	// in_production → quarantine. Automatic, so it needs no quality:approve —
	// but it goes through the quality domain so the batch's history is one chain
	// with one writer.
	if _, err := s.quality.TransitionTx(ctx, tx, actor, quality.TransitionInput{
		BatchID:    mo.BatchID.UUID,
		To:         quality.StatusQuarantine,
		HasApprove: false,
	}); err != nil {
		return db.ManufacturingOrder{}, err
	}

	done, err := q.SetManufacturingOrderStatus(ctx, db.SetManufacturingOrderStatusParams{
		ID: moID, Status: StatusDone,
	})
	if err != nil {
		return db.ManufacturingOrder{}, fmt.Errorf("production: set done: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionUpdate,
		Resource:   Resource,
		ResourceID: audit.Target(moID),
		Before:     map[string]any{"status": mo.Status},
		After: map[string]any{
			"status": StatusDone, "good_qty": totals.GoodQty.StringFixed(3),
			"posted_to": quarantine.Code,
		},
	}); err != nil {
		return db.ManufacturingOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.ManufacturingOrder{}, fmt.Errorf("production: commit: %w", err)
	}
	return done, nil
}

// TotalsFor returns the computed totals for an order.
func (s *Service) TotalsFor(ctx context.Context, moID uuid.UUID) (Totals, error) {
	row, err := db.New(s.pool).GetProductionTotals(ctx, moID)
	if err != nil {
		return Totals{}, fmt.Errorf("production: totals: %w", err)
	}
	return Totals{
		GoodQty: row.GoodQty, ScrapQty: row.ScrapQty,
		DowntimeMin: row.DowntimeMin, Entries: row.EntryCount,
	}, nil
}

// Get loads one order.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.ManufacturingOrder, error) {
	mo, err := db.New(s.pool).GetManufacturingOrder(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ManufacturingOrder{}, common.NotFound()
		}
		return db.ManufacturingOrder{}, fmt.Errorf("production: get: %w", err)
	}
	return mo, nil
}

// Entries returns the append-only shift log.
func (s *Service) Entries(ctx context.Context, moID uuid.UUID) ([]db.ProductionEntry, error) {
	rows, err := db.New(s.pool).ListProductionEntries(ctx, moID)
	if err != nil {
		return nil, fmt.Errorf("production: entries: %w", err)
	}
	return rows, nil
}

// List returns a page of orders.
func (s *Service) List(ctx context.Context, p common.Params, status *string) ([]db.ListManufacturingOrdersRow, int64, error) {
	q := db.New(s.pool)
	var search *string
	if p.Query != "" {
		search = &p.Query
	}
	rows, err := q.ListManufacturingOrders(ctx, db.ListManufacturingOrdersParams{
		Status: status, Q: search, Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("production: list: %w", err)
	}
	total, err := q.CountManufacturingOrders(ctx, db.CountManufacturingOrdersParams{Status: status, Q: search})
	if err != nil {
		return nil, 0, fmt.Errorf("production: count: %w", err)
	}
	return rows, total, nil
}
