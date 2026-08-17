package http

import (
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/production"
	"github.com/qoim/samari/backend/internal/http/common"
)

// Производство — docs/05-MODULES.md §6.
//
// An order is planned, shift entries accumulate against it, and completing it
// posts the output to quarantine. Nothing here writes stock directly; the
// inventory service is the only ledger writer.

// moProgress is good ÷ planned as a whole percentage.
//
// Capped at 100 for display only: overproduction is real and the underlying
// numbers keep it, but a progress bar rendering 140% looks like a bug rather than
// a good shift. The uncapped figure is recoverable from good_qty and planned_qty.
func moProgress(good, planned decimal.Decimal) int {
	if planned.IsZero() {
		return 0
	}
	pct := good.Div(planned).Mul(decimal.NewFromInt(100)).IntPart()
	if pct > 100 {
		return 100
	}
	if pct < 0 {
		return 0
	}
	return int(pct)
}

// moStatus — docs/05-MODULES.md:131. `planned` is neutral because a plan is not
// an achievement, and `done` is the only green: the client's rule is that
// green means healthy, not merely "a thing happened".
func moStatus(status string) api.Status {
	switch status {
	case production.StatusInProgress:
		return api.Status{Key: status, Label: "В работе", Level: string(common.LevelInfo)}
	case production.StatusDone:
		return api.Status{Key: status, Label: "Завершён", Level: string(common.LevelOK)}
	case production.StatusCancelled:
		return api.Status{Key: status, Label: "Отменён", Level: string(common.LevelNeutral)}
	default:
		return api.Status{Key: status, Label: "Запланирован", Level: string(common.LevelNeutral)}
	}
}

func moRowResponse(r db.ListManufacturingOrdersRow) api.ManufacturingOrderRow {
	out := api.ManufacturingOrderRow{
		ID:           r.ManufacturingOrder.ID.String(),
		MONo:         r.ManufacturingOrder.MoNo,
		ItemID:       r.ManufacturingOrder.ItemID.String(),
		SKU:          r.Sku,
		ItemName:     r.ItemName,
		Line:         r.ManufacturingOrder.Line,
		ScheduledFor: common.Date(r.ManufacturingOrder.ScheduledFor),
		PlannedQty:   r.ManufacturingOrder.PlannedQty.String(),
		GoodQty:      r.GoodQty.String(),
		ScrapQty:     r.ScrapQty.String(),
		Status:       moStatus(r.ManufacturingOrder.Status),
		Version:      r.ManufacturingOrder.Version,
		CreatedAt:    common.Timestamp(r.ManufacturingOrder.CreatedAt),
	}
	if r.ManufacturingOrder.BatchID.Valid {
		id := r.ManufacturingOrder.BatchID.UUID.String()
		out.BatchID = &id
	}
	out.BatchNo = r.BatchNo
	return out
}

func (s *Server) handleListManufacturingOrders(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, production.SortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.Production.List(r.Context(), params, optionalQuery(r, "status"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.ManufacturingOrderRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, moRowResponse(row))
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

func (s *Server) handleGetManufacturingOrder(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	ctx := r.Context()

	mo, err := s.svc.Production.Get(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	totals, err := s.svc.Production.TotalsFor(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	entries, err := s.svc.Production.Entries(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	detail := api.ManufacturingOrder{
		ID:           mo.ID.String(),
		MONo:         mo.MoNo,
		ItemID:       mo.ItemID.String(),
		Line:         mo.Line,
		ScheduledFor: common.Date(mo.ScheduledFor),
		PlannedQty:   mo.PlannedQty.String(),
		GoodQty:      totals.GoodQty.String(),
		ScrapQty:     totals.ScrapQty.String(),
		Status:       moStatus(mo.Status),
		Version:      mo.Version,
		CreatedAt:    common.Timestamp(mo.CreatedAt),
		Progress:     moProgress(totals.GoodQty, mo.PlannedQty),
		DowntimeMin:  totals.DowntimeMin,
		Entries:      make([]api.ProductionEntry, 0, len(entries)),
	}
	if mo.BatchID.Valid {
		id := mo.BatchID.UUID.String()
		detail.BatchID = &id
	}
	if y := totals.YieldPercent(); y != nil {
		v := y.String()
		detail.YieldPercent = &v
	}
	for _, e := range entries {
		entry := api.ProductionEntry{
			ID:          e.ID.String(),
			RecordedAt:  common.Timestamp(e.RecordedAt),
			GoodQty:     e.GoodQty.String(),
			ScrapQty:    e.ScrapQty.String(),
			DowntimeMin: e.DowntimeMin,
			Note:        e.Note,
		}
		if e.RecordedBy.Valid {
			by := e.RecordedBy.UUID.String()
			entry.RecordedBy = &by
		}
		detail.Entries = append(detail.Entries, entry)
	}
	common.JSON(w, http.StatusOK, detail)
}

func (s *Server) handleCreateManufacturingOrder(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.ManufacturingOrderWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	itemID, err := parseUUIDField(req.ItemID, "item_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	plannedQty, err := parseDecimalField(req.PlannedQty, "planned_qty")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	scheduledFor, err := parseDate(deref(req.ScheduledFor), "scheduled_for", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	mo, err := s.svc.Production.Create(r.Context(), ident.User.ID, production.CreateInput{
		MONo:         req.MONo,
		ItemID:       itemID,
		BatchNo:      req.BatchNo,
		Line:         req.Line,
		PlannedQty:   plannedQty,
		ScheduledFor: scheduledFor,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.ManufacturingOrderRow{
		ID:         mo.ID.String(),
		MONo:       mo.MoNo,
		ItemID:     mo.ItemID.String(),
		PlannedQty: mo.PlannedQty.String(),
		GoodQty:    "0",
		ScrapQty:   "0",
		Status:     moStatus(mo.Status),
		Version:    mo.Version,
		CreatedAt:  common.Timestamp(mo.CreatedAt),
	})
}

func (s *Server) handleRecordProductionEntry(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var req api.ProductionEntryWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	good, err := parseDecimalField(req.GoodQty, "good_qty")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	scrap, err := parseNullDecimalField(req.ScrapQty, "scrap_qty")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var downtime int32
	if req.DowntimeMin != nil {
		downtime = *req.DowntimeMin
	}

	entry, err := s.svc.Production.RecordEntry(r.Context(), ident.User.ID, production.EntryInput{
		MOID: id, GoodQty: good, ScrapQty: scrap, DowntimeMin: downtime, Note: req.Note,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.ProductionEntry{
		ID:          entry.ID.String(),
		RecordedAt:  common.Timestamp(entry.RecordedAt),
		GoodQty:     entry.GoodQty.String(),
		ScrapQty:    entry.ScrapQty.String(),
		DowntimeMin: entry.DowntimeMin,
		Note:        entry.Note,
	})
}

// handleCompleteManufacturingOrder posts the output to quarantine and moves the
// batch, in one transaction. It is a POST to an action rather than a PATCH of a
// status field, because completion is not an edit — it moves stock.
func (s *Server) handleCompleteManufacturingOrder(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	mo, err := s.svc.Production.Complete(r.Context(), ident.User.ID, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, api.ManufacturingOrderRow{
		ID:         mo.ID.String(),
		MONo:       mo.MoNo,
		ItemID:     mo.ItemID.String(),
		PlannedQty: mo.PlannedQty.String(),
		Status:     moStatus(mo.Status),
		Version:    mo.Version,
		CreatedAt:  common.Timestamp(mo.CreatedAt),
	})
}
