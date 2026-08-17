package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/inventory"
	"github.com/qoim/samari/backend/internal/http/common"
)

// Склад и запасы — docs/05-MODULES.md §5.
//
// The read side serves balances derived by SUM; the write side appends movements.
// There is no endpoint that sets a quantity, because there is no column to set.

// movementReason maps a ledger reason to its display level.
//
// All neutral but two: `scrap` is a loss and `adjustment` is a correction someone
// made by hand — the two rows a supervisor scanning the ledger actually needs to
// find. Colouring the routine reasons would make the exceptional ones invisible.
func movementReason(reason string) api.Status {
	switch reason {
	case inventory.ReasonScrap:
		return api.Status{Key: reason, Label: "Списание", Level: string(common.LevelDanger)}
	case inventory.ReasonAdjustment:
		return api.Status{Key: reason, Label: "Корректировка", Level: string(common.LevelWarn)}
	case inventory.ReasonGoodsReceipt:
		return api.Status{Key: reason, Label: "Приёмка", Level: string(common.LevelNeutral)}
	case inventory.ReasonProductionOutput:
		return api.Status{Key: reason, Label: "Выпуск", Level: string(common.LevelNeutral)}
	case inventory.ReasonMaterialIssue:
		return api.Status{Key: reason, Label: "Отпуск в производство", Level: string(common.LevelNeutral)}
	case inventory.ReasonSale:
		return api.Status{Key: reason, Label: "Отгрузка", Level: string(common.LevelNeutral)}
	case inventory.ReasonTransfer:
		return api.Status{Key: reason, Label: "Перемещение", Level: string(common.LevelNeutral)}
	case inventory.ReasonReturn:
		return api.Status{Key: reason, Label: "Возврат", Level: string(common.LevelNeutral)}
	default:
		return api.Status{Key: reason, Label: reason, Level: string(common.LevelNeutral)}
	}
}

// stockLevel grades a position against the item's minimum.
//
// The client's rule (CLAUDE.md §5): green means healthy, not merely branded. A
// position at or above its minimum is ok; below it is danger, because for a food
// factory a stockout of packaging stops the line.
func stockLevel(onHand decimal.Decimal, minQty decimal.NullDecimal) api.Status {
	switch {
	case !minQty.Valid:
		return api.Status{Key: "tracked", Label: "Без минимума", Level: string(common.LevelNeutral)}
	case onHand.LessThan(minQty.Decimal):
		return api.Status{Key: "below_minimum", Label: "Ниже минимума", Level: string(common.LevelDanger)}
	// Within 20% of the minimum is the warning band: enough time to reorder before
	// it becomes a stoppage, which is the only point at which a warning is useful.
	case onHand.LessThan(minQty.Decimal.Mul(decimal.NewFromFloat(1.2))):
		return api.Status{Key: "low", Label: "Заканчивается", Level: string(common.LevelWarn)}
	default:
		return api.Status{Key: "ok", Label: "В норме", Level: string(common.LevelOK)}
	}
}

func stockBalanceResponse(r db.ListStockBalancesRow) api.StockBalanceRow {
	out := api.StockBalanceRow{
		ItemID:         r.StockBalance.ItemID.String(),
		SKU:            r.Sku,
		ItemName:       r.ItemName,
		BaseUOM:        r.BaseUom,
		LocationID:     r.StockBalance.LocationID.String(),
		LocationCode:   r.LocationCode,
		LocationZone:   r.LocationZone,
		OnHand:         r.StockBalance.OnHand.String(),
		Status:         stockLevel(r.StockBalance.OnHand, r.MinQty),
		LastMovementAt: common.NullTimestamp(r.StockBalance.LastMovementAt),
		ExpiresOn:      common.Date(r.ExpiresOn),
	}
	if r.MinQty.Valid {
		s := r.MinQty.Decimal.String()
		out.MinQty = &s
	}
	if r.StockBalance.BatchID.Valid {
		id := r.StockBalance.BatchID.UUID.String()
		out.BatchID = &id
	}
	out.BatchNo = r.BatchNo
	if r.BatchStatus != nil {
		st := batchStatus(*r.BatchStatus)
		out.BatchStatus = &st
	}
	return out
}

func (s *Server) handleListStock(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, inventory.SortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	// The search string travels on Params (the toolbar sends `q`); the zone and
	// item filters are this module's own.
	filter := inventory.ListFilter{Zone: optionalQuery(r, "zone")}
	if raw := r.URL.Query().Get("item_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			common.Fail(w, r, common.Validation(common.FieldError{
				Field: "item_id", Code: "invalid", Message: "Некорректный идентификатор товара",
			}))
			return
		}
		filter.ItemID = uuid.NullUUID{UUID: id, Valid: true}
	}

	rows, total, err := s.svc.Inventory.List(r.Context(), params, filter)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.StockBalanceRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, stockBalanceResponse(row))
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

// handleStockLedger is the detail view: every movement for one position, with the
// balance after each. This is the answer to "why does it say 480?".
func (s *Server) handleStockLedger(w http.ResponseWriter, r *http.Request) {
	pos, err := positionFromQuery(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	params, err := common.ParseParams(r, inventory.SortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, err := s.svc.Inventory.Ledger(r.Context(), pos, params.Limit(), params.Offset())
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.StockMovementRow, 0, len(rows))
	for _, m := range rows {
		row := api.StockMovementRow{
			ID:             m.ID.String(),
			OccurredAt:     common.Timestamp(m.OccurredAt),
			QtyDelta:       m.QtyDelta.String(),
			RunningBalance: m.RunningBalance.String(),
			Reason:         movementReason(m.Reason),
			RefType:        m.RefType,
			Note:           m.Note,
		}
		if m.RefID.Valid {
			id := m.RefID.UUID.String()
			row.RefID = &id
		}
		if m.CreatedBy.Valid {
			id := m.CreatedBy.UUID.String()
			row.CreatedBy = &id
		}
		out = append(out, row)
	}
	// No total: the ledger is unbounded and counting it on every page load would
	// scan the whole table for a number nobody reads.
	common.List(w, out, common.PageMeta{Page: params.Page, PerPage: params.PerPage})
}

func (s *Server) handlePostMovement(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.MovementWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}

	itemID, err := parseUUIDField(req.ItemID, "item_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	locationID, err := parseUUIDField(req.LocationID, "location_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	batchID, err := parseNullUUIDField(req.BatchID, "batch_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	qty, err := parseDecimalField(req.QtyDelta, "qty_delta")
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	movement, err := s.svc.Inventory.Post(r.Context(), ident.User.ID, inventory.Movement{
		Position: inventory.Position{ItemID: itemID, BatchID: batchID, LocationID: locationID},
		QtyDelta: qty,
		Reason:   req.Reason,
		Note:     req.Note,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.StockMovementRow{
		ID:         movement.ID.String(),
		OccurredAt: common.Timestamp(movement.OccurredAt),
		QtyDelta:   movement.QtyDelta.String(),
		Reason:     movementReason(movement.Reason),
		Note:       movement.Note,
	})
}

// handleTransfer is two movements in one transaction, so stock is never in flight
// and never counted twice.
func (s *Server) handleTransfer(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.TransferRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	itemID, err := parseUUIDField(req.ItemID, "item_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	from, err := parseUUIDField(req.FromLocation, "from_location_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	to, err := parseUUIDField(req.ToLocation, "to_location_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	batchID, err := parseNullUUIDField(req.BatchID, "batch_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	qty, err := parseDecimalField(req.Qty, "qty")
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	moves, err := s.svc.Inventory.Transfer(r.Context(), ident.User.ID,
		inventory.Position{ItemID: itemID, BatchID: batchID, LocationID: from},
		to, qty, req.Note)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	// Both legs are returned. A transfer that reported only the credit would let a
	// client render a receipt with no matching issue, and the pair is the point.
	out := make([]api.StockMovementRow, 0, len(moves))
	for _, m := range moves {
		out = append(out, api.StockMovementRow{
			ID:         m.ID.String(),
			OccurredAt: common.Timestamp(m.OccurredAt),
			QtyDelta:   m.QtyDelta.String(),
			Reason:     movementReason(m.Reason),
			Note:       m.Note,
		})
	}
	common.Created(w, out)
}

func (s *Server) handleListLocations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.Inventory.Locations(r.Context())
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.Location, 0, len(rows))
	for _, l := range rows {
		out = append(out, api.Location{
			ID: l.ID.String(), Code: l.Code, Name: l.Name, Zone: l.Zone,
		})
	}
	common.JSON(w, http.StatusOK, map[string]any{"data": out})
}

// positionFromQuery reads item_id / batch_id / location_id, which together
// identify one stock position.
func positionFromQuery(r *http.Request) (inventory.Position, error) {
	q := r.URL.Query()
	itemID, err := parseUUIDField(q.Get("item_id"), "item_id")
	if err != nil {
		return inventory.Position{}, err
	}
	locationID, err := parseUUIDField(q.Get("location_id"), "location_id")
	if err != nil {
		return inventory.Position{}, err
	}
	var batchPtr *string
	if raw := q.Get("batch_id"); raw != "" {
		batchPtr = &raw
	}
	batchID, err := parseNullUUIDField(batchPtr, "batch_id")
	if err != nil {
		return inventory.Position{}, err
	}
	return inventory.Position{ItemID: itemID, BatchID: batchID, LocationID: locationID}, nil
}
