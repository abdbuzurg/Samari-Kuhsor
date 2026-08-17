package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/inventory"
	"github.com/qoim/samari/backend/internal/domain/quality"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Качество — docs/05-MODULES.md §7.
//
// The batch detail view is the ToR's traceability answer: given a batch number
// from a complaint, show the tests, the decisions, and where the stock is now.

// testResult renders a pass/fail. `passed` is nullable because some tests record
// a measurement without a verdict — a pending result is not a failure, and
// colouring it red would have the lab chasing batches that are simply not back yet.
func testResult(passed *bool) api.Status {
	switch {
	case passed == nil:
		return api.Status{Key: "pending", Label: "Ожидает", Level: string(common.LevelNeutral)}
	case *passed:
		return api.Status{Key: "passed", Label: "Соответствует", Level: string(common.LevelOK)}
	default:
		return api.Status{Key: "failed", Label: "Не соответствует", Level: string(common.LevelDanger)}
	}
}

func qualityTestResponse(t db.ListQualityTestsRow) api.QualityTest {
	out := api.QualityTest{
		ID:          t.ID.String(),
		BatchID:     t.BatchID.String(),
		TestType:    t.TestType,
		Result:      testResult(t.Passed),
		ResultValue: t.ResultValue,
		TestedAt:    common.Timestamp(t.TestedAt),
		Inspector:   t.InspectorName,
		Notes:       t.Notes,
	}
	if t.InspectorID.Valid {
		id := t.InspectorID.UUID.String()
		out.InspectorID = &id
	}
	return out
}

func statusEventResponse(e db.ListBatchStatusEventsRow) api.BatchStatusEvent {
	out := api.BatchStatusEvent{
		ID:          e.ID.String(),
		ToStatus:    batchStatus(e.ToStatus),
		OccurredAt:  common.Timestamp(e.OccurredAt),
		DecidedBy:   e.DecidedBy.String(),
		DeciderName: &e.DecidedByName,
		Reason:      e.Reason,
	}
	if e.FromStatus != nil {
		from := batchStatus(*e.FromStatus)
		out.FromStatus = &from
	}
	return out
}

func (s *Server) handleListBatches(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, quality.SortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.Quality.List(r.Context(), params, optionalQuery(r, "status"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.BatchListRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.BatchListRow{
			ID:          row.Batch.ID.String(),
			BatchNo:     row.Batch.BatchNo,
			ItemID:      row.Batch.ItemID.String(),
			SKU:         row.Sku,
			ItemName:    row.ItemName,
			ProducedOn:  common.Date(row.Batch.ProducedOn),
			ExpiresOn:   common.Date(row.Batch.ExpiresOn),
			TestCount:   row.TestCount,
			FailedCount: row.FailedCount,
			Status:      batchStatus(row.Batch.Status),
			Version:     row.Batch.Version,
		})
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

// handleBatchDetail is the traceability view.
func (s *Server) handleBatchDetail(w http.ResponseWriter, r *http.Request) {
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
	ctx := r.Context()

	batch, err := s.svc.Quality.BatchWithItem(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	tests, err := s.svc.Quality.TestsFor(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	history, err := s.svc.Quality.StatusEvents(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	detail := api.BatchDetail{
		Batch:   batchResponse(batch.Batch),
		SKU:     batch.Sku,
		Tests:   make([]api.QualityTest, 0, len(tests)),
		History: make([]api.BatchStatusEvent, 0, len(history)),
		Stock:   []api.StockBalanceRow{},
	}
	detail.ItemName = batch.ItemName
	for _, t := range tests {
		detail.Tests = append(detail.Tests, qualityTestResponse(t))
	}
	for _, e := range history {
		detail.History = append(detail.History, statusEventResponse(e))
	}

	// Where the stock actually is. Only for a viewer who may read the warehouse:
	// a lab technician with quality:read but no inventory:read gets the decision
	// history without the locations, rather than a 403 for the whole page.
	perms := rbac.NewSet(ident.Permissions)
	if perms.CanRead(rbac.Inventory) {
		rows, _, err := s.svc.Inventory.List(ctx,
			common.Params{Page: 1, PerPage: 100},
			inventory.ListFilter{BatchID: uuid.NullUUID{UUID: id, Valid: true}})
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		for _, row := range rows {
			detail.Stock = append(detail.Stock, stockBalanceResponse(row))
		}
	}

	// What this user may do next. Derived from the same matrix the domain
	// enforces, so the buttons cannot drift from the rules (docs/04-RBAC.md —
	// hiding is not enforcement, but offering an impossible action is still a bug).
	detail.AllowedTransitions = quality.AllowedFrom(
		batch.Batch.Status, perms.Can(rbac.Quality, rbac.Approve))

	common.JSON(w, http.StatusOK, detail)
}

func (s *Server) handleRecordQualityTest(w http.ResponseWriter, r *http.Request) {
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
	var req api.QualityTestWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	test, err := s.svc.Quality.RecordTest(r.Context(), ident.User.ID, quality.TestInput{
		BatchID:     id,
		TestType:    req.TestType,
		ResultValue: req.ResultValue,
		Passed:      req.Passed,
		Notes:       req.Notes,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := api.QualityTest{
		ID:          test.ID.String(),
		BatchID:     test.BatchID.String(),
		TestType:    test.TestType,
		Result:      testResult(test.Passed),
		ResultValue: test.ResultValue,
		TestedAt:    common.Timestamp(test.TestedAt),
		Notes:       test.Notes,
	}
	if test.InspectorID.Valid {
		inspector := test.InspectorID.UUID.String()
		out.InspectorID = &inspector
	}
	common.Created(w, out)
}

// handleTransitionBatch moves a batch's status.
//
// A POST to an action rather than a PATCH of `status`: the move is guarded by a
// transition matrix and some destinations need quality:approve, none of which a
// generic field update could express.
func (s *Server) handleTransitionBatch(w http.ResponseWriter, r *http.Request) {
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
	var req api.BatchTransitionRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}

	// The approve permission is resolved here and passed in, so the domain decides
	// on facts rather than reaching back into the request.
	hasApprove := rbac.NewSet(ident.Permissions).Can(rbac.Quality, rbac.Approve)

	batch, err := s.svc.Quality.Transition(r.Context(), ident.User.ID, quality.TransitionInput{
		BatchID: id, To: req.To, Reason: req.Reason, HasApprove: hasApprove,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, batchResponse(batch))
}
