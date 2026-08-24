package http

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/registries"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Персонал, Оборудование и ТО, Документы — docs/05-MODULES.md §12, §13, §14.

// employeeStatus — active is the only green. `on_leave` is neutral rather than
// amber: someone on leave is not a problem, they are simply not on shift.
func employeeStatus(status string) api.Status {
	switch status {
	case registries.EmployeeOnLeave:
		return api.Status{Key: status, Label: "В отпуске", Level: string(common.LevelNeutral)}
	case registries.EmployeeSuspended:
		return api.Status{Key: status, Label: "Отстранён", Level: string(common.LevelWarn)}
	case registries.EmployeeTerminated:
		return api.Status{Key: status, Label: "Уволен", Level: string(common.LevelNeutral)}
	default:
		return api.Status{Key: status, Label: "Работает", Level: string(common.LevelOK)}
	}
}

// assetStatus — `broken` stops the line, so it is danger; `maintenance_due` is a
// warning because it is still running but on borrowed time.
func assetStatus(status string) api.Status {
	switch status {
	case registries.AssetMaintenanceDue:
		return api.Status{Key: status, Label: "Требует ТО", Level: string(common.LevelWarn)}
	case registries.AssetBroken:
		return api.Status{Key: status, Label: "Неисправно", Level: string(common.LevelDanger)}
	case registries.AssetRetired:
		return api.Status{Key: status, Label: "Списано", Level: string(common.LevelNeutral)}
	default:
		return api.Status{Key: status, Label: "В работе", Level: string(common.LevelOK)}
	}
}

// documentStatus — `expired` is danger because an expired certificate is a
// regulatory exposure, not a filing inconvenience.
func documentStatus(status string) api.Status {
	switch status {
	case registries.DocApproval:
		return api.Status{Key: status, Label: "На согласовании", Level: string(common.LevelWarn)}
	case registries.DocActive:
		return api.Status{Key: status, Label: "Действует", Level: string(common.LevelOK)}
	case registries.DocExpiring:
		return api.Status{Key: status, Label: "Истекает", Level: string(common.LevelWarn)}
	case registries.DocExpired:
		return api.Status{Key: status, Label: "Истёк", Level: string(common.LevelDanger)}
	case registries.DocArchived:
		return api.Status{Key: status, Label: "В архиве", Level: string(common.LevelNeutral)}
	default:
		return api.Status{Key: status, Label: "Черновик", Level: string(common.LevelNeutral)}
	}
}

// ---------------------------------------------------------------------------
// Персонал
// ---------------------------------------------------------------------------

func employeeResponse(e db.Employee, position, department *string) api.Employee {
	out := api.Employee{
		ID: e.ID.String(), FullName: e.FullName,
		PositionTitle: position, Department: department, Shift: e.Shift,
		HiredOn: common.Date(e.HiredOn), ContractUntil: common.Date(e.ContractUntil),
		Status: employeeStatus(e.Status), Version: e.Version,
	}
	if e.PositionID.Valid {
		id := e.PositionID.UUID.String()
		out.PositionID = &id
	}
	return out
}

func (s *Server) handleListEmployees(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, registries.EmployeeSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.Registries.Employees(r.Context(), params, optionalQuery(r, "status"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.Employee, 0, len(rows))
	for _, row := range rows {
		out = append(out, employeeResponse(row.Employee, row.PositionTitle, row.DepartmentName))
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

// handleGetEmployee serves the Персонал detail view.
//
// Guarded on hr:read like the register. The T23 gate — personal data unreachable
// through every public endpoint — survives a new route only because this one is
// declared Guarded; there is no public counterpart and there must never be.
func (s *Server) handleGetEmployee(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	row, err := s.svc.Registries.Employee(r.Context(), id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, employeeResponse(row.Employee, row.PositionTitle, row.DepartmentName))
}

func (s *Server) handleCreateEmployee(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.EmployeeWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	in, err := employeeInput(req)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	employee, err := s.svc.Registries.CreateEmployee(r.Context(), ident.User.ID, in)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, employeeResponse(employee, nil, nil))
}

func (s *Server) handleUpdateEmployee(w http.ResponseWriter, r *http.Request) {
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
	var req api.EmployeeWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	version, err := common.RequireVersion(common.Versioned{Version: req.Version})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	in, err := employeeInput(req)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	in.Version = version
	employee, err := s.svc.Registries.UpdateEmployee(r.Context(), ident.User.ID, id, in)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, employeeResponse(employee, nil, nil))
}

func employeeInput(req api.EmployeeWriteRequest) (registries.EmployeeInput, error) {
	positionID, err := parseNullUUIDField(req.PositionID, "position_id")
	if err != nil {
		return registries.EmployeeInput{}, err
	}
	hiredOn, err := parseDate(deref(req.HiredOn), "hired_on", false)
	if err != nil {
		return registries.EmployeeInput{}, err
	}
	contractUntil, err := parseDate(deref(req.ContractUntil), "contract_until", false)
	if err != nil {
		return registries.EmployeeInput{}, err
	}
	// Version is set by the caller: create does not carry one, and update takes
	// it from RequireVersion so an absent value is refused rather than defaulted.
	return registries.EmployeeInput{
		FullName: req.FullName, PositionID: positionID, Shift: req.Shift,
		HiredOn: hiredOn, ContractUntil: contractUntil,
		Status: req.Status,
	}, nil
}

// ---------------------------------------------------------------------------
// Оборудование и ТО
// ---------------------------------------------------------------------------

func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, registries.AssetSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.Registries.Assets(r.Context(), params, optionalQuery(r, "status"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.Asset, 0, len(rows))
	for _, row := range rows {
		a := row.Asset
		out = append(out, api.Asset{
			ID: a.ID.String(), AssetNo: a.AssetNo, Name: a.Name,
			AssetType: a.AssetType, Line: a.Line,
			CommissionedOn: common.Date(a.CommissionedOn),
			WarrantyUntil:  common.Date(a.WarrantyUntil),
			NextDueOn:      common.Date(row.NextDueOn),
			LastServiceAt:  common.NullTimestamp(row.LastServiceAt),
			Status:         assetStatus(a.Status), Version: a.Version,
		})
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

// handleGetAsset serves the Оборудование detail view.
func (s *Server) handleGetAsset(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	row, err := s.svc.Registries.Asset(r.Context(), id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	a := row.Asset
	common.JSON(w, http.StatusOK, api.Asset{
		ID: a.ID.String(), AssetNo: a.AssetNo, Name: a.Name,
		AssetType: a.AssetType, Line: a.Line,
		CommissionedOn: common.Date(a.CommissionedOn),
		WarrantyUntil:  common.Date(a.WarrantyUntil),
		NextDueOn:      common.Date(row.NextDueOn),
		LastServiceAt:  common.NullTimestamp(row.LastServiceAt),
		Status:         assetStatus(a.Status), Version: a.Version,
	})
}

func (s *Server) handleCreateAsset(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.AssetWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	commissionedOn, err := parseDate(deref(req.CommissionedOn), "commissioned_on", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	warrantyUntil, err := parseDate(deref(req.WarrantyUntil), "warranty_until", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	asset, err := s.svc.Registries.CreateAsset(r.Context(), ident.User.ID, registries.AssetInput{
		AssetNo: req.AssetNo, Name: req.Name, AssetType: req.AssetType, Line: req.Line,
		CommissionedOn: commissionedOn, WarrantyUntil: warrantyUntil, Status: req.Status,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.Asset{
		ID: asset.ID.String(), AssetNo: asset.AssetNo, Name: asset.Name,
		AssetType: asset.AssetType, Line: asset.Line,
		CommissionedOn: common.Date(asset.CommissionedOn),
		WarrantyUntil:  common.Date(asset.WarrantyUntil),
		Status:         assetStatus(asset.Status), Version: asset.Version,
	})
}

// handleRecordMaintenance appends a service record. If the asset was flagged as
// due, this also returns it to `running` — see the domain comment on why that is
// not left to the operator.
func (s *Server) handleRecordMaintenance(w http.ResponseWriter, r *http.Request) {
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
	var req api.MaintenanceWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	nextDue, err := parseDate(deref(req.NextDueOn), "next_due_on", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var performedAt pgtype.Timestamptz
	if req.PerformedAt != nil && *req.PerformedAt != "" {
		t, parseErr := time.Parse(time.RFC3339, *req.PerformedAt)
		if parseErr != nil {
			common.Fail(w, r, common.Validation(common.FieldError{
				Field: "performed_at", Code: "invalid", Message: "Некорректная дата и время",
			}))
			return
		}
		performedAt = pgtype.Timestamptz{Time: t, Valid: true}
	}

	event, err := s.svc.Registries.RecordMaintenance(r.Context(), ident.User.ID,
		registries.MaintenanceInput{
			AssetID: id, EventType: req.EventType, PerformedAt: performedAt,
			NextDueOn: nextDue, Notes: req.Notes,
		})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.MaintenanceEvent{
		ID: event.ID.String(), AssetID: event.AssetID.String(),
		EventType:   event.EventType,
		PerformedAt: common.NullTimestamp(event.PerformedAt),
		NextDueOn:   common.Date(event.NextDueOn),
		Notes:       event.Notes,
	})
}

func (s *Server) handleAssetMaintenance(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	events, err := s.svc.Registries.MaintenanceFor(r.Context(), id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.MaintenanceEvent, 0, len(events))
	for _, e := range events {
		out = append(out, api.MaintenanceEvent{
			ID: e.ID.String(), AssetID: e.AssetID.String(),
			EventType:   e.EventType,
			PerformedAt: common.NullTimestamp(e.PerformedAt),
			NextDueOn:   common.Date(e.NextDueOn),
			Notes:       e.Notes,
		})
	}
	common.JSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// Документы
// ---------------------------------------------------------------------------

func documentResponse(d db.Document, owner *string, allowed []string) api.Document {
	out := api.Document{
		ID: d.ID.String(), DocNo: d.DocNo, Title: d.Title, DocType: d.DocType,
		OwnerName: owner, ValidUntil: common.Date(d.ValidUntil),
		Status: documentStatus(d.Status), Version: d.Version,
		AllowedTransitions: allowed,
	}
	if out.AllowedTransitions == nil {
		out.AllowedTransitions = []string{}
	}
	if d.OwnerID.Valid {
		id := d.OwnerID.UUID.String()
		out.OwnerID = &id
	}
	return out
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	params, err := common.ParseParams(r, registries.DocumentSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.Registries.Documents(r.Context(), params, optionalQuery(r, "status"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	hasApprove := rbac.NewSet(ident.Permissions).Can(rbac.Documents, rbac.Approve)

	out := make([]api.Document, 0, len(rows))
	for _, row := range rows {
		out = append(out, documentResponse(db.Document{
			ID: row.ID, DocNo: row.DocNo, Title: row.Title, DocType: row.DocType,
			OwnerID: row.OwnerID, ValidUntil: row.ValidUntil,
			Status: row.Status, Version: row.Version,
		}, row.OwnerName, registries.DocAllowedFrom(row.Status, hasApprove)))
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

// handleGetDocument serves the Документы detail view.
//
// `allowed_transitions` is computed here rather than in the frontend, from the
// same matrix the domain enforces — so the detail view cannot offer a rung the
// server would refuse (docs/05-MODULES.md §2).
func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	row, err := s.svc.Registries.Document(r.Context(), id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	hasApprove := rbac.NewSet(ident.Permissions).Can(rbac.Documents, rbac.Approve)
	common.JSON(w, http.StatusOK, documentResponse(db.Document{
		ID: row.ID, DocNo: row.DocNo, Title: row.Title, DocType: row.DocType,
		OwnerID: row.OwnerID, ValidUntil: row.ValidUntil,
		Status: row.Status, Version: row.Version,
	}, row.OwnerName, registries.DocAllowedFrom(row.Status, hasApprove)))
}

func (s *Server) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.DocumentWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	ownerID, err := parseNullUUIDField(req.OwnerID, "owner_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	validUntil, err := parseDate(deref(req.ValidUntil), "valid_until", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	doc, err := s.svc.Registries.CreateDocument(r.Context(), ident.User.ID, registries.DocumentInput{
		DocNo: req.DocNo, Title: req.Title, DocType: req.DocType,
		OwnerID: ownerID, ValidUntil: validUntil,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	hasApprove := rbac.NewSet(ident.Permissions).Can(rbac.Documents, rbac.Approve)
	common.Created(w, documentResponse(doc, nil,
		registries.DocAllowedFrom(doc.Status, hasApprove)))
}

// handleTransitionDocument moves a document along its approval ladder.
//
// Guarded as documents:manage at the route; activation additionally needs
// documents:approve, checked in the domain against the transition matrix. The
// route cannot be guarded on approve because sending a draft for review needs
// no authority at all.
func (s *Server) handleTransitionDocument(w http.ResponseWriter, r *http.Request) {
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
	var req api.TransitionRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	hasApprove := rbac.NewSet(ident.Permissions).Can(rbac.Documents, rbac.Approve)
	doc, err := s.svc.Registries.TransitionDocument(r.Context(), ident.User.ID,
		registries.DocTransitionInput{DocID: id, To: req.To, HasApprove: hasApprove})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, map[string]any{
		"data": documentResponse(doc, nil, registries.DocAllowedFrom(doc.Status, hasApprove)),
	})
}
