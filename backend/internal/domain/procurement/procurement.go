// Package procurement implements Закупки и поставщики.
//
// The rule that connects this module to the rest of the system
// (docs/05-MODULES.md:196): goods receipt posts `goods_receipt` stock movements.
// This is how raw material enters inventory, and it is the only way.
//
// Receipt and stock posting happen in ONE transaction. A receipt recorded without
// the stock would show material the warehouse does not have; stock posted without
// the receipt would be material with no provenance. Either is worse than failing.
package procurement

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
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

const Resource = rbac.Procurement

// The status ladder (docs/05-MODULES.md:193):
// Черновик → Согласование → Подтверждён → В пути → Приёмка → Закрыт
const (
	StatusDraft     = "draft"
	StatusApproval  = "approval"
	StatusConfirmed = "confirmed"
	StatusInTransit = "in_transit"
	StatusReceiving = "receiving"
	StatusClosed    = "closed"
	StatusCancelled = "cancelled"
)

// Transition describes a legal move and whether it carries authority.
type Transition struct {
	From, To        string
	RequiresApprove bool
}

// legalTransitions. Only one requires procurement:approve — leaving Согласование
// (docs/05-MODULES.md:196). Approving a purchase commits company money, which is
// a different authority from drafting the order.
var legalTransitions = []Transition{
	{From: StatusDraft, To: StatusApproval},
	{From: StatusApproval, To: StatusConfirmed, RequiresApprove: true},
	{From: StatusApproval, To: StatusDraft}, // sent back for changes
	{From: StatusConfirmed, To: StatusInTransit},
	{From: StatusInTransit, To: StatusReceiving},
	{From: StatusReceiving, To: StatusClosed},
	// Cancellable until goods are moving.
	{From: StatusDraft, To: StatusCancelled},
	{From: StatusApproval, To: StatusCancelled},
	{From: StatusConfirmed, To: StatusCancelled},
}

func Lookup(from, to string) (Transition, bool) {
	for _, t := range legalTransitions {
		if t.From == from && t.To == to {
			return t, true
		}
	}
	return Transition{}, false
}

func LegalTransitions() []Transition { return append([]Transition(nil), legalTransitions...) }

var SortSpec = common.SortSpec{
	Allowed:     []string{"po_no", "expected_at", "status"},
	Default:     "expected_at",
	DefaultDesc: true,
}

// SupplierSortSpec is separate because suppliers and orders share no columns —
// one spec covering both would let `?sort=po_no` through on the supplier list and
// then fail in SQL.
var SupplierSortSpec = common.SortSpec{
	Allowed: []string{"name", "region", "rating"},
	Default: "name",
}

type Service struct {
	pool      *pgxpool.Pool
	inventory *inventory.Service
}

func NewService(pool *pgxpool.Pool, inv *inventory.Service) *Service {
	return &Service{pool: pool, inventory: inv}
}

// ---------------------------------------------------------------------------
// Suppliers
// ---------------------------------------------------------------------------

type SupplierInput struct {
	Name    string
	TaxID   *string
	Contact *string
	Region  *string
	Rating  *int32
}

func (s *Service) CreateSupplier(ctx context.Context, actor uuid.UUID, in SupplierInput) (db.Supplier, error) {
	if strings.TrimSpace(in.Name) == "" {
		return db.Supplier{}, common.Validation(common.FieldError{
			Field: "name", Code: "required", Message: "Укажите наименование поставщика",
		})
	}
	if in.Rating != nil && (*in.Rating < 1 || *in.Rating > 5) {
		return db.Supplier{}, common.Validation(common.FieldError{
			Field: "rating", Code: "invalid", Message: "Рейтинг должен быть от 1 до 5",
		})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Supplier{}, fmt.Errorf("procurement: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	sup, err := q.CreateSupplier(ctx, db.CreateSupplierParams{
		Name: strings.TrimSpace(in.Name), TaxID: in.TaxID, Contact: in.Contact,
		Region: in.Region, Rating: in.Rating,
		CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		return db.Supplier{}, fmt.Errorf("procurement: create supplier: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate, Resource: Resource,
		ResourceID: audit.Target(sup.ID), After: map[string]any{"name": sup.Name},
	}); err != nil {
		return db.Supplier{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Supplier{}, fmt.Errorf("procurement: commit: %w", err)
	}
	return sup, nil
}

// ---------------------------------------------------------------------------
// Purchase orders
// ---------------------------------------------------------------------------

type LineInput struct {
	ItemID    uuid.UUID
	Qty       decimal.Decimal
	UnitPrice decimal.Decimal
}

type OrderInput struct {
	PONo       string
	SupplierID uuid.UUID
	ExpectedAt pgtype.Date
	Lines      []LineInput
}

func (s *Service) CreateOrder(ctx context.Context, actor uuid.UUID, in OrderInput) (db.PurchaseOrder, error) {
	var details []common.FieldError
	if strings.TrimSpace(in.PONo) == "" {
		details = append(details, common.FieldError{
			Field: "po_no", Code: "required", Message: "Укажите номер заказа",
		})
	}
	if len(in.Lines) == 0 {
		// An order with no lines commits to nothing and cannot be received.
		details = append(details, common.FieldError{
			Field: "lines", Code: "required", Message: "Добавьте хотя бы одну позицию",
		})
	}
	for i, l := range in.Lines {
		if !l.Qty.IsPositive() {
			details = append(details, common.FieldError{
				Field: fmt.Sprintf("lines.%d.qty", i), Code: "invalid",
				Message: "Количество должно быть больше нуля",
			})
		}
		if l.UnitPrice.IsNegative() {
			details = append(details, common.FieldError{
				Field: fmt.Sprintf("lines.%d.unit_price", i), Code: "invalid",
				Message: "Цена не может быть отрицательной",
			})
		}
	}
	if len(details) > 0 {
		return db.PurchaseOrder{}, common.Validation(details...)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.PurchaseOrder{}, fmt.Errorf("procurement: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	if _, err := q.GetSupplier(ctx, in.SupplierID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.PurchaseOrder{}, common.Validation(common.FieldError{
				Field: "supplier_id", Code: "not_found", Message: "Поставщик не найден",
			})
		}
		return db.PurchaseOrder{}, fmt.Errorf("procurement: supplier: %w", err)
	}

	po, err := q.CreatePurchaseOrder(ctx, db.CreatePurchaseOrderParams{
		PoNo: strings.TrimSpace(in.PONo), SupplierID: in.SupplierID, ExpectedAt: in.ExpectedAt,
		CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		if strings.Contains(err.Error(), "purchase_orders_no_key") {
			return db.PurchaseOrder{}, common.Validation(common.FieldError{
				Field: "po_no", Code: "already_exists", Message: "Заказ с таким номером уже существует",
			})
		}
		return db.PurchaseOrder{}, fmt.Errorf("procurement: create order: %w", err)
	}

	for _, l := range in.Lines {
		if _, err := q.CreatePurchaseOrderLine(ctx, db.CreatePurchaseOrderLineParams{
			PoID: po.ID, ItemID: l.ItemID, Qty: l.Qty, UnitPrice: l.UnitPrice,
			CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
		}); err != nil {
			return db.PurchaseOrder{}, fmt.Errorf("procurement: create line: %w", err)
		}
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate, Resource: Resource,
		ResourceID: audit.Target(po.ID),
		After:      map[string]any{"po_no": po.PoNo, "lines": len(in.Lines), "status": po.Status},
	}); err != nil {
		return db.PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.PurchaseOrder{}, fmt.Errorf("procurement: commit: %w", err)
	}
	return po, nil
}

// TransitionInput describes a requested status move.
type TransitionInput struct {
	POID uuid.UUID
	To   string
	// HasApprove is the caller's procurement:approve grant, checked here as well
	// as at the route so the rule holds for any caller.
	HasApprove bool
}

func (s *Service) Transition(ctx context.Context, actor uuid.UUID, in TransitionInput) (db.PurchaseOrder, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.PurchaseOrder{}, fmt.Errorf("procurement: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	po, err := q.GetPurchaseOrder(ctx, in.POID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.PurchaseOrder{}, common.NotFound()
		}
		return db.PurchaseOrder{}, fmt.Errorf("procurement: load: %w", err)
	}

	rule, legal := Lookup(po.Status, in.To)
	if !legal {
		return db.PurchaseOrder{}, common.BusinessRule(fmt.Sprintf(
			"Недопустимый переход заказа: из «%s» в «%s».", po.Status, in.To))
	}
	if rule.RequiresApprove && !in.HasApprove {
		return db.PurchaseOrder{}, common.Forbidden()
	}

	updated, err := q.SetPurchaseOrderStatus(ctx, db.SetPurchaseOrderStatusParams{
		ID: in.POID, FromStatus: po.Status, ToStatus: in.To,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.PurchaseOrder{}, common.BusinessRule("Статус заказа изменился. Обновите страницу.")
		}
		return db.PurchaseOrder{}, fmt.Errorf("procurement: transition: %w", err)
	}

	action := audit.ActionUpdate
	if rule.RequiresApprove {
		action = audit.ActionApprove
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: action, Resource: Resource,
		ResourceID: audit.Target(in.POID),
		Before:     map[string]any{"status": po.Status},
		After:      map[string]any{"status": in.To, "po_no": po.PoNo},
	}); err != nil {
		return db.PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.PurchaseOrder{}, fmt.Errorf("procurement: commit: %w", err)
	}
	return updated, nil
}

// ---------------------------------------------------------------------------
// Goods receipt — the join to inventory
// ---------------------------------------------------------------------------

type ReceiptLineInput struct {
	POLineID uuid.UUID
	Qty      decimal.Decimal
	BatchID  uuid.NullUUID
}

type ReceiptInput struct {
	POID       uuid.UUID
	LocationID uuid.UUID
	Lines      []ReceiptLineInput
	Note       *string
}

// Receive records a goods receipt and posts the matching stock movements.
//
// One transaction, for the reason in the package doc. Also: the received quantity
// per line is checked against what the order actually ordered, so a typo cannot
// receive ten times what was purchased and silently inflate stock.
func (s *Service) Receive(ctx context.Context, actor uuid.UUID, in ReceiptInput) (db.GoodsReceipt, error) {
	if len(in.Lines) == 0 {
		return db.GoodsReceipt{}, common.Validation(common.FieldError{
			Field: "lines", Code: "required", Message: "Укажите принимаемые позиции",
		})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.GoodsReceipt{}, fmt.Errorf("procurement: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	po, err := q.GetPurchaseOrder(ctx, in.POID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.GoodsReceipt{}, common.NotFound()
		}
		return db.GoodsReceipt{}, fmt.Errorf("procurement: load order: %w", err)
	}
	// A closed order's stock has already been accounted for; a cancelled one was
	// never expected to arrive.
	if po.Status == StatusClosed || po.Status == StatusCancelled {
		return db.GoodsReceipt{}, common.BusinessRule(fmt.Sprintf(
			"Заказ %s закрыт: приёмка невозможна.", po.PoNo))
	}
	if po.Status == StatusDraft || po.Status == StatusApproval {
		return db.GoodsReceipt{}, common.BusinessRule(
			"Заказ ещё не согласован: приёмка невозможна.")
	}

	if err := s.inventory.EnsureLocation(ctx, q, in.LocationID, "location_id"); err != nil {
		return db.GoodsReceipt{}, err
	}

	receipt, err := q.CreateGoodsReceipt(ctx, db.CreateGoodsReceiptParams{
		PoID: in.POID, LocationID: in.LocationID,
		ReceivedBy: uuid.NullUUID{UUID: actor, Valid: true}, Note: in.Note,
	})
	if err != nil {
		return db.GoodsReceipt{}, fmt.Errorf("procurement: create receipt: %w", err)
	}

	refType := "purchase_order"
	for i, l := range in.Lines {
		if !l.Qty.IsPositive() {
			return db.GoodsReceipt{}, common.Validation(common.FieldError{
				Field: fmt.Sprintf("lines.%d.qty", i), Code: "invalid",
				Message: "Принимаемое количество должно быть больше нуля",
			})
		}

		line, err := q.GetPurchaseOrderLine(ctx, l.POLineID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return db.GoodsReceipt{}, common.Validation(common.FieldError{
					Field: fmt.Sprintf("lines.%d.po_line_id", i), Code: "not_found",
					Message: "Позиция заказа не найдена",
				})
			}
			return db.GoodsReceipt{}, fmt.Errorf("procurement: load line: %w", err)
		}
		if line.PoID != in.POID {
			return db.GoodsReceipt{}, common.Validation(common.FieldError{
				Field: fmt.Sprintf("lines.%d.po_line_id", i), Code: "invalid",
				Message: "Позиция принадлежит другому заказу",
			})
		}

		// Over-receipt is refused rather than flagged. Accepting more than was
		// ordered puts stock into the ledger that no purchase justifies, and the
		// discrepancy should be resolved with the supplier, not absorbed.
		already, err := q.SumReceivedForLine(ctx, l.POLineID)
		if err != nil {
			return db.GoodsReceipt{}, fmt.Errorf("procurement: sum received: %w", err)
		}
		if already.Add(l.Qty).GreaterThan(line.Qty) {
			return db.GoodsReceipt{}, common.BusinessRule(fmt.Sprintf(
				"Превышение заказанного количества: заказано %s, уже принято %s, принимается %s.",
				line.Qty.StringFixed(3), already.StringFixed(3), l.Qty.StringFixed(3)))
		}

		if _, err := q.CreateGoodsReceiptLine(ctx, db.CreateGoodsReceiptLineParams{
			ReceiptID: receipt.ID, PoLineID: l.POLineID, Qty: l.Qty,
			CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
		}); err != nil {
			return db.GoodsReceipt{}, fmt.Errorf("procurement: receipt line: %w", err)
		}

		// The stock movement. Same transaction, same actor, same ref.
		if _, err := s.inventory.PostTx(ctx, tx, actor, inventory.Movement{
			Position: inventory.Position{
				ItemID: line.ItemID, BatchID: l.BatchID, LocationID: in.LocationID,
			},
			QtyDelta: l.Qty,
			Reason:   inventory.ReasonGoodsReceipt,
			RefType:  &refType,
			RefID:    uuid.NullUUID{UUID: in.POID, Valid: true},
		}); err != nil {
			return db.GoodsReceipt{}, err
		}
	}

	// Receiving moves the order into `receiving` if it was in transit, so the
	// board reflects that goods have started arriving.
	if po.Status == StatusConfirmed || po.Status == StatusInTransit {
		if _, err := q.SetPurchaseOrderStatus(ctx, db.SetPurchaseOrderStatusParams{
			ID: in.POID, FromStatus: po.Status, ToStatus: StatusReceiving,
		}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return db.GoodsReceipt{}, fmt.Errorf("procurement: set receiving: %w", err)
		}
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate, Resource: Resource,
		ResourceID: audit.Target(receipt.ID),
		After:      map[string]any{"po_no": po.PoNo, "lines": len(in.Lines)},
	}); err != nil {
		return db.GoodsReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.GoodsReceipt{}, fmt.Errorf("procurement: commit: %w", err)
	}
	return receipt, nil
}

// Get loads one order.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.PurchaseOrder, error) {
	po, err := db.New(s.pool).GetPurchaseOrder(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.PurchaseOrder{}, common.NotFound()
		}
		return db.PurchaseOrder{}, fmt.Errorf("procurement: get: %w", err)
	}
	return po, nil
}

// Lines returns the order's lines with how much of each has been received.
func (s *Service) Lines(ctx context.Context, poID uuid.UUID) ([]db.ListPurchaseOrderLinesRow, error) {
	rows, err := db.New(s.pool).ListPurchaseOrderLines(ctx, poID)
	if err != nil {
		return nil, fmt.Errorf("procurement: lines: %w", err)
	}
	return rows, nil
}

// List returns a page of orders.
func (s *Service) List(ctx context.Context, p common.Params, status *string) ([]db.ListPurchaseOrdersRow, int64, error) {
	q := db.New(s.pool)
	var search *string
	if p.Query != "" {
		search = &p.Query
	}
	rows, err := q.ListPurchaseOrders(ctx, db.ListPurchaseOrdersParams{
		Status: status, Q: search, Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("procurement: list: %w", err)
	}
	total, err := q.CountPurchaseOrders(ctx, db.CountPurchaseOrdersParams{Status: status, Q: search})
	if err != nil {
		return nil, 0, fmt.Errorf("procurement: count: %w", err)
	}
	return rows, total, nil
}

// Suppliers lists suppliers for the module list and the order form's picker.
func (s *Service) Suppliers(ctx context.Context, p common.Params) ([]db.Supplier, int64, error) {
	q := db.New(s.pool)
	rows, err := q.ListSuppliers(ctx, db.ListSuppliersParams{
		Q: nilIfEmpty(p.Query), Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("procurement: suppliers: %w", err)
	}
	total, err := q.CountSuppliers(ctx, nilIfEmpty(p.Query))
	if err != nil {
		return nil, 0, fmt.Errorf("procurement: count suppliers: %w", err)
	}
	return rows, total, nil
}

// AllowedFrom projects the purchase-order matrix onto one status, the same way
// quality.AllowedFrom does for batches — so the buttons and the rules share a
// single definition.
func AllowedFrom(status string, hasApprove bool) []string {
	out := make([]string, 0, len(legalTransitions))
	for _, t := range legalTransitions {
		if t.From != status || (t.RequiresApprove && !hasApprove) {
			continue
		}
		out = append(out, t.To)
	}
	return out
}

// nilIfEmpty turns an absent search string into a NULL, so the query's
// `narg IS NULL OR ...` branch skips the filter rather than matching "".
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
