// Package sales implements CRM и продажи and Логистика.
//
// Kept in one package because they enforce the same rule from two directions and
// splitting them would invite two copies of it:
//
//	Only `released` batches may be sold or shipped. Enforce this in the sales and
//	logistics domains, not only in the UI (docs/02-SCHEMA.md:316,
//	docs/05-MODULES.md:212).
//
// Both paths call quality.EnsureSellable, so there is exactly one definition of
// "sellable" and it lives with the module that decides it.
//
// docs/05-MODULES.md:183 notes this module will be EMPTY on opening day — the
// plant has produced nothing and has no customers. It is built to the same
// standard anyway, because the rule it enforces is the one that stops
// unreleased product leaving the building.
package sales

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

const (
	ResourceCRM       = rbac.CRM
	ResourceLogistics = rbac.Logistics
)

// Sales order statuses (docs/02-SCHEMA.md:344).
const (
	SOStatusDraft     = "draft"
	SOStatusConfirmed = "confirmed"
	SOStatusPicking   = "picking"
	SOStatusShipped   = "shipped"
	SOStatusClosed    = "closed"
	SOStatusCancelled = "cancelled"
)

// Shipment statuses (docs/05-MODULES.md:207).
const (
	ShipPlanned   = "planned"
	ShipLoading   = "loading"
	ShipInTransit = "in_transit"
	ShipDelivered = "delivered"
	ShipDelayed   = "delayed"
	ShipCancelled = "cancelled"
)

var SortSpec = common.SortSpec{
	Allowed:     []string{"so_no", "ordered_on", "status"},
	Default:     "ordered_on",
	DefaultDesc: true,
}

// ShipmentSortSpec covers Логистика. Trips sort by departure, not by number: the
// dispatcher's question is "what is going out today".
var ShipmentSortSpec = common.SortSpec{
	Allowed:     []string{"trip_no", "status", "created_at"},
	Default:     "created_at",
	DefaultDesc: true,
}

type Service struct {
	pool      *pgxpool.Pool
	inventory *inventory.Service
}

func NewService(pool *pgxpool.Pool, inv *inventory.Service) *Service {
	return &Service{pool: pool, inventory: inv}
}

// ---------------------------------------------------------------------------
// Sales orders
// ---------------------------------------------------------------------------

type OrderLineInput struct {
	ItemID    uuid.UUID
	BatchID   uuid.NullUUID
	Qty       decimal.Decimal
	UnitPrice decimal.Decimal
}

type OrderInput struct {
	SONo       string
	CustomerID uuid.UUID
	OrderedOn  pgtype.Date
	Lines      []OrderLineInput
}

// CreateOrder drafts a sales order.
//
// A draft may reference any batch: quoting for product still in quarantine is
// legitimate business. The gate is at CONFIRM, which is the moment the company
// commits to supplying it.
func (s *Service) CreateOrder(ctx context.Context, actor uuid.UUID, in OrderInput) (db.SalesOrder, error) {
	var details []common.FieldError
	if strings.TrimSpace(in.SONo) == "" {
		details = append(details, common.FieldError{
			Field: "so_no", Code: "required", Message: "Укажите номер заказа",
		})
	}
	for i, l := range in.Lines {
		if !l.Qty.IsPositive() {
			details = append(details, common.FieldError{
				Field: fmt.Sprintf("lines.%d.qty", i), Code: "invalid",
				Message: "Количество должно быть больше нуля",
			})
		}
	}
	if len(details) > 0 {
		return db.SalesOrder{}, common.Validation(details...)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.SalesOrder{}, fmt.Errorf("sales: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	so, err := q.CreateSalesOrder(ctx, db.CreateSalesOrderParams{
		SoNo: strings.TrimSpace(in.SONo), CustomerID: in.CustomerID, OrderedOn: in.OrderedOn,
		CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		if strings.Contains(err.Error(), "sales_orders_no_key") {
			return db.SalesOrder{}, common.Validation(common.FieldError{
				Field: "so_no", Code: "already_exists", Message: "Заказ с таким номером уже существует",
			})
		}
		if strings.Contains(err.Error(), "customer") {
			return db.SalesOrder{}, common.Validation(common.FieldError{
				Field: "customer_id", Code: "not_found", Message: "Клиент не найден",
			})
		}
		return db.SalesOrder{}, fmt.Errorf("sales: create order: %w", err)
	}

	for _, l := range in.Lines {
		if _, err := q.CreateSalesOrderLine(ctx, db.CreateSalesOrderLineParams{
			SalesOrderID: so.ID, ItemID: l.ItemID, BatchID: l.BatchID,
			Qty: l.Qty, UnitPrice: l.UnitPrice,
			CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
		}); err != nil {
			return db.SalesOrder{}, fmt.Errorf("sales: create line: %w", err)
		}
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate, Resource: ResourceCRM,
		ResourceID: audit.Target(so.ID),
		After:      map[string]any{"so_no": so.SoNo, "lines": len(in.Lines)},
	}); err != nil {
		return db.SalesOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.SalesOrder{}, fmt.Errorf("sales: commit: %w", err)
	}
	return so, nil
}

// ConfirmOrder commits the company to supplying the order.
//
// This is where the released-batch rule bites, and where stock is consumed. Both
// checks happen inside one transaction with the ledger's advisory lock, so a
// confirmation cannot succeed against stock another confirmation is taking.
func (s *Service) ConfirmOrder(ctx context.Context, actor uuid.UUID, soID uuid.UUID, locationID uuid.UUID) (db.SalesOrder, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.SalesOrder{}, fmt.Errorf("sales: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	so, err := q.GetSalesOrder(ctx, soID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.SalesOrder{}, common.NotFound()
		}
		return db.SalesOrder{}, fmt.Errorf("sales: load: %w", err)
	}
	if so.Status != SOStatusDraft {
		return db.SalesOrder{}, common.BusinessRule(fmt.Sprintf(
			"Заказ уже имеет статус «%s»: повторное подтверждение невозможно.", so.Status))
	}

	lines, err := q.ListSalesOrderLines(ctx, soID)
	if err != nil {
		return db.SalesOrder{}, fmt.Errorf("sales: lines: %w", err)
	}
	if len(lines) == 0 {
		return db.SalesOrder{}, common.BusinessRule("В заказе нет позиций.")
	}

	refType := "sales_order"
	for _, l := range lines {
		// THE RULE. Checked for every line, not once for the order: one
		// unreleased batch among ten is still unreleased product leaving.
		if !l.BatchID.Valid {
			return db.SalesOrder{}, common.BusinessRule(fmt.Sprintf(
				"Позиция %s не привязана к партии: отгружать можно только выпущенные партии.", l.Sku))
		}
		batch, err := q.GetBatchByID(ctx, l.BatchID.UUID)
		if err != nil {
			return db.SalesOrder{}, fmt.Errorf("sales: load batch: %w", err)
		}
		if err := quality.EnsureSellable(batch); err != nil {
			return db.SalesOrder{}, err
		}

		// Consuming stock. Negative delta, so the ledger's advisory lock applies
		// and an oversell is refused.
		if _, err := s.inventory.PostTx(ctx, tx, actor, inventory.Movement{
			Position: inventory.Position{
				ItemID: l.ItemID, BatchID: l.BatchID, LocationID: locationID,
			},
			QtyDelta: l.Qty.Neg(),
			Reason:   inventory.ReasonSale,
			RefType:  &refType,
			RefID:    uuid.NullUUID{UUID: soID, Valid: true},
		}); err != nil {
			return db.SalesOrder{}, err
		}
	}

	confirmed, err := q.SetSalesOrderStatus(ctx, db.SetSalesOrderStatusParams{
		ID: soID, FromStatus: SOStatusDraft, ToStatus: SOStatusConfirmed,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.SalesOrder{}, common.BusinessRule("Статус заказа изменился. Обновите страницу.")
		}
		return db.SalesOrder{}, fmt.Errorf("sales: confirm: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate, Resource: ResourceCRM,
		ResourceID: audit.Target(soID),
		Before:     map[string]any{"status": SOStatusDraft},
		After:      map[string]any{"status": SOStatusConfirmed, "so_no": so.SoNo},
	}); err != nil {
		return db.SalesOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.SalesOrder{}, fmt.Errorf("sales: commit: %w", err)
	}
	return confirmed, nil
}

// ---------------------------------------------------------------------------
// Shipments
// ---------------------------------------------------------------------------

type ShipmentInput struct {
	TripNo        string
	RouteFrom     *string
	RouteTo       *string
	DriverID      uuid.NullUUID
	VehicleID     uuid.NullUUID
	TransportCost decimal.NullDecimal
}

func (s *Service) CreateShipment(ctx context.Context, actor uuid.UUID, in ShipmentInput) (db.Shipment, error) {
	if strings.TrimSpace(in.TripNo) == "" {
		return db.Shipment{}, common.Validation(common.FieldError{
			Field: "trip_no", Code: "required", Message: "Укажите номер рейса",
		})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Shipment{}, fmt.Errorf("sales: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	sh, err := q.CreateShipment(ctx, db.CreateShipmentParams{
		TripNo: strings.TrimSpace(in.TripNo), RouteFrom: in.RouteFrom, RouteTo: in.RouteTo,
		DriverID: in.DriverID, VehicleID: in.VehicleID, TransportCost: in.TransportCost,
		CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		if strings.Contains(err.Error(), "shipments_trip_key") {
			return db.Shipment{}, common.Validation(common.FieldError{
				Field: "trip_no", Code: "already_exists", Message: "Рейс с таким номером уже существует",
			})
		}
		return db.Shipment{}, fmt.Errorf("sales: create shipment: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate, Resource: ResourceLogistics,
		ResourceID: audit.Target(sh.ID), After: map[string]any{"trip_no": sh.TripNo},
	}); err != nil {
		return db.Shipment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Shipment{}, fmt.Errorf("sales: commit: %w", err)
	}
	return sh, nil
}

// LoadLine adds product to a shipment.
//
// docs/05-MODULES.md:212 — "Loading a shipment line must reject any batch not in
// `released` status. Enforce server-side." This is that enforcement; the UI
// hiding the option is not.
func (s *Service) LoadLine(
	ctx context.Context, actor uuid.UUID,
	shipmentID, itemID, batchID uuid.UUID, qty decimal.Decimal,
) (db.ShipmentLine, error) {
	if !qty.IsPositive() {
		return db.ShipmentLine{}, common.Validation(common.FieldError{
			Field: "qty", Code: "invalid", Message: "Количество должно быть больше нуля",
		})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.ShipmentLine{}, fmt.Errorf("sales: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	sh, err := q.GetShipment(ctx, shipmentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ShipmentLine{}, common.NotFound()
		}
		return db.ShipmentLine{}, fmt.Errorf("sales: load shipment: %w", err)
	}
	// Once a truck has left, its manifest is a record of what went, not a form.
	if sh.Status != ShipPlanned && sh.Status != ShipLoading {
		return db.ShipmentLine{}, common.BusinessRule(fmt.Sprintf(
			"Рейс имеет статус «%s»: загрузка невозможна.", sh.Status))
	}

	batch, err := q.GetBatchByID(ctx, batchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ShipmentLine{}, common.Validation(common.FieldError{
				Field: "batch_id", Code: "not_found", Message: "Партия не найдена",
			})
		}
		return db.ShipmentLine{}, fmt.Errorf("sales: load batch: %w", err)
	}
	// THE RULE, from the logistics side. Same function the sales side calls.
	if err := quality.EnsureSellable(batch); err != nil {
		return db.ShipmentLine{}, err
	}

	line, err := q.CreateShipmentLine(ctx, db.CreateShipmentLineParams{
		ShipmentID: shipmentID, ItemID: itemID, BatchID: batchID, Qty: qty,
		CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		return db.ShipmentLine{}, fmt.Errorf("sales: create line: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate, Resource: ResourceLogistics,
		ResourceID: audit.Target(line.ID),
		After: map[string]any{
			"shipment_id": shipmentID.String(), "batch_no": batch.BatchNo,
			"qty": qty.StringFixed(3),
		},
	}); err != nil {
		return db.ShipmentLine{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.ShipmentLine{}, fmt.Errorf("sales: commit: %w", err)
	}
	return line, nil
}

// Reads.

func (s *Service) GetOrder(ctx context.Context, id uuid.UUID) (db.SalesOrder, error) {
	so, err := db.New(s.pool).GetSalesOrder(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.SalesOrder{}, common.NotFound()
		}
		return db.SalesOrder{}, fmt.Errorf("sales: get: %w", err)
	}
	return so, nil
}

func (s *Service) OrderLines(ctx context.Context, soID uuid.UUID) ([]db.ListSalesOrderLinesRow, error) {
	return db.New(s.pool).ListSalesOrderLines(ctx, soID)
}

func (s *Service) GetShipment(ctx context.Context, id uuid.UUID) (db.Shipment, error) {
	sh, err := db.New(s.pool).GetShipment(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Shipment{}, common.NotFound()
		}
		return db.Shipment{}, fmt.Errorf("sales: get shipment: %w", err)
	}
	return sh, nil
}

func (s *Service) ShipmentLines(ctx context.Context, id uuid.UUID) ([]db.ListShipmentLinesRow, error) {
	return db.New(s.pool).ListShipmentLines(ctx, id)
}

func (s *Service) ListOrders(ctx context.Context, p common.Params, status *string) ([]db.ListSalesOrdersRow, int64, error) {
	q := db.New(s.pool)
	var search *string
	if p.Query != "" {
		search = &p.Query
	}
	rows, err := q.ListSalesOrders(ctx, db.ListSalesOrdersParams{
		Status: status, Q: search, Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("sales: list: %w", err)
	}
	total, err := q.CountSalesOrders(ctx, db.CountSalesOrdersParams{Status: status, Q: search})
	if err != nil {
		return nil, 0, fmt.Errorf("sales: count: %w", err)
	}
	return rows, total, nil
}

// Shipments lists trips with the driver and vehicle resolved. A trip identified
// only by two UUIDs is unreadable in a list, and the dispatcher's whole job is
// knowing which lorry and which driver.
func (s *Service) Shipments(ctx context.Context, p common.Params, status *string) ([]db.ListShipmentsRow, int64, error) {
	q := db.New(s.pool)
	rows, err := q.ListShipments(ctx, db.ListShipmentsParams{
		Status: status, Q: nilIfEmpty(p.Query), Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("sales: shipments: %w", err)
	}
	total, err := q.CountShipments(ctx, db.CountShipmentsParams{Status: status, Q: nilIfEmpty(p.Query)})
	if err != nil {
		return nil, 0, fmt.Errorf("sales: count shipments: %w", err)
	}
	return rows, total, nil
}

// nilIfEmpty turns an absent search string into a NULL, so the query's
// `narg IS NULL OR ...` branch skips the filter rather than matching "".
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
