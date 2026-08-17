// Package registries implements Персонал, Оборудование и ТО, and Документы.
//
// docs/05-MODULES.md §12, §13, §14. They share a package because they share a
// shape: each is a register of things that are created, kept current, and
// watched for a date running out — a contract, a service interval, a
// certificate. None of them moves stock or money, and none has a transition
// matrix worth the name.
//
// What they do have is the reason three of the seven standing conditions exist
// (docs/07-IMPLEMENTATION-PLAN.md I15). Expiry is not stored as a flag here for
// the same reason a stock balance is not stored: a flag needs something to clear
// it, and the query does not.
//
// Документы is the exception on one point: it carries an approval ladder, and
// `active` requires documents:approve. A certificate that says the water is
// potable is a claim the company makes to a regulator, so someone must sign it.
package registries

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Employee statuses — migrations/00003_commerce.sql:161.
const (
	EmployeeActive     = "active"
	EmployeeOnLeave    = "on_leave"
	EmployeeSuspended  = "suspended"
	EmployeeTerminated = "terminated"
)

// Asset statuses — migrations/00003_commerce.sql:427.
const (
	AssetRunning        = "running"
	AssetMaintenanceDue = "maintenance_due"
	AssetBroken         = "broken"
	AssetRetired        = "retired"
)

// Document statuses — migrations/00003_commerce.sql:472.
const (
	DocDraft    = "draft"
	DocApproval = "approval"
	DocActive   = "active"
	DocExpiring = "expiring"
	DocExpired  = "expired"
	DocArchived = "archived"
)

// Shifts a factory employee can be on.
var Shifts = []string{"day", "night", "rotating"}

var (
	EmployeeSortSpec = common.SortSpec{
		Allowed: []string{"full_name", "hired_on", "contract_until", "status"},
		Default: "contract_until",
	}
	AssetSortSpec = common.SortSpec{
		Allowed: []string{"asset_no", "name", "commissioned_on", "status"},
		Default: "asset_no",
	}
	DocumentSortSpec = common.SortSpec{
		Allowed: []string{"doc_no", "title", "valid_until", "status"},
		Default: "valid_until",
	}
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ---------------------------------------------------------------------------
// Персонал
// ---------------------------------------------------------------------------

type EmployeeInput struct {
	FullName      string
	PositionID    uuid.NullUUID
	Shift         *string
	HiredOn       pgtype.Date
	ContractUntil pgtype.Date
	Status        string
	Version       int32
}

func (in EmployeeInput) validate() error {
	var details []common.FieldError
	if strings.TrimSpace(in.FullName) == "" {
		details = append(details, common.FieldError{
			Field: "full_name", Code: "required", Message: "Укажите ФИО сотрудника",
		})
	}
	if in.Shift != nil && *in.Shift != "" && !contains(Shifts, *in.Shift) {
		details = append(details, common.FieldError{
			Field: "shift", Code: "invalid", Message: "Недопустимая смена",
		})
	}
	if in.Status != "" && !contains(
		[]string{EmployeeActive, EmployeeOnLeave, EmployeeSuspended, EmployeeTerminated},
		in.Status) {
		details = append(details, common.FieldError{
			Field: "status", Code: "invalid", Message: "Недопустимый статус сотрудника",
		})
	}
	// A contract that ended before it began is a typo, and it would show as
	// permanently expiring in the alerts feed.
	if in.HiredOn.Valid && in.ContractUntil.Valid &&
		in.ContractUntil.Time.Before(in.HiredOn.Time) {
		details = append(details, common.FieldError{
			Field: "contract_until", Code: "invalid",
			Message: "Договор не может закончиться раньше даты приёма",
		})
	}
	if len(details) > 0 {
		return common.Validation(details...)
	}
	return nil
}

func (s *Service) CreateEmployee(ctx context.Context, actor uuid.UUID, in EmployeeInput) (db.Employee, error) {
	if err := in.validate(); err != nil {
		return db.Employee{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Employee{}, fmt.Errorf("registries: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	status := nilIfEmpty(in.Status)
	employee, err := q.CreateEmployee(ctx, db.CreateEmployeeParams{
		FullName: in.FullName, PositionID: in.PositionID, Shift: in.Shift,
		HiredOn: in.HiredOn, ContractUntil: in.ContractUntil,
		Status: status, CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.Employee{}, fmt.Errorf("registries: create employee: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate,
		Resource: rbac.HR, ResourceID: audit.Target(employee.ID),
		After: map[string]any{"full_name": employee.FullName, "status": employee.Status},
	}); err != nil {
		return db.Employee{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Employee{}, fmt.Errorf("registries: commit: %w", err)
	}
	return employee, nil
}

func (s *Service) UpdateEmployee(ctx context.Context, actor, id uuid.UUID, in EmployeeInput) (db.Employee, error) {
	if err := in.validate(); err != nil {
		return db.Employee{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Employee{}, fmt.Errorf("registries: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	before, err := q.GetEmployee(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Employee{}, common.NotFound()
		}
		return db.Employee{}, fmt.Errorf("registries: load employee: %w", err)
	}
	if err := common.CheckVersion(in.Version, before.Employee.Version); err != nil {
		return db.Employee{}, err
	}

	status := in.Status
	if status == "" {
		status = before.Employee.Status
	}
	updated, err := q.UpdateEmployee(ctx, db.UpdateEmployeeParams{
		ID: id, FullName: in.FullName, PositionID: in.PositionID, Shift: in.Shift,
		HiredOn: in.HiredOn, ContractUntil: in.ContractUntil,
		Status: status, Version: in.Version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Employee{}, common.VersionConflict(before.Employee.Version)
		}
		return db.Employee{}, fmt.Errorf("registries: update employee: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate,
		Resource: rbac.HR, ResourceID: audit.Target(id),
		Before: map[string]any{"full_name": before.Employee.FullName, "status": before.Employee.Status},
		After:  map[string]any{"full_name": updated.FullName, "status": updated.Status},
	}); err != nil {
		return db.Employee{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Employee{}, fmt.Errorf("registries: commit: %w", err)
	}
	return updated, nil
}

func (s *Service) Employees(ctx context.Context, p common.Params, status *string) ([]db.ListEmployeesRow, int64, error) {
	q := db.New(s.pool)
	rows, err := q.ListEmployees(ctx, db.ListEmployeesParams{
		Status: status, Q: nilIfEmpty(p.Query), Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("registries: list employees: %w", err)
	}
	total, err := q.CountEmployees(ctx, db.CountEmployeesParams{Status: status, Q: nilIfEmpty(p.Query)})
	if err != nil {
		return nil, 0, fmt.Errorf("registries: count employees: %w", err)
	}
	return rows, total, nil
}

func (s *Service) Employee(ctx context.Context, id uuid.UUID) (db.GetEmployeeRow, error) {
	row, err := db.New(s.pool).GetEmployee(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.GetEmployeeRow{}, common.NotFound()
		}
		return db.GetEmployeeRow{}, fmt.Errorf("registries: employee: %w", err)
	}
	return row, nil
}

// ---------------------------------------------------------------------------
// Оборудование и ТО
// ---------------------------------------------------------------------------

type AssetInput struct {
	AssetNo        string
	Name           string
	AssetType      *string
	Line           *string
	CommissionedOn pgtype.Date
	WarrantyUntil  pgtype.Date
	Status         string
	Version        int32
}

func (in AssetInput) validate() error {
	var details []common.FieldError
	if strings.TrimSpace(in.AssetNo) == "" {
		details = append(details, common.FieldError{
			Field: "asset_no", Code: "required", Message: "Укажите инвентарный номер",
		})
	}
	if strings.TrimSpace(in.Name) == "" {
		details = append(details, common.FieldError{
			Field: "name", Code: "required", Message: "Укажите наименование оборудования",
		})
	}
	if in.Status != "" && !contains(
		[]string{AssetRunning, AssetMaintenanceDue, AssetBroken, AssetRetired}, in.Status) {
		details = append(details, common.FieldError{
			Field: "status", Code: "invalid", Message: "Недопустимый статус оборудования",
		})
	}
	if len(details) > 0 {
		return common.Validation(details...)
	}
	return nil
}

func (s *Service) CreateAsset(ctx context.Context, actor uuid.UUID, in AssetInput) (db.Asset, error) {
	if err := in.validate(); err != nil {
		return db.Asset{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Asset{}, fmt.Errorf("registries: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	asset, err := q.CreateAsset(ctx, db.CreateAssetParams{
		AssetNo: in.AssetNo, Name: in.Name, AssetType: in.AssetType, Line: in.Line,
		CommissionedOn: in.CommissionedOn, WarrantyUntil: in.WarrantyUntil,
		Status: nilIfEmpty(in.Status), CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.Asset{}, fmt.Errorf("registries: create asset: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate,
		Resource: rbac.Equipment, ResourceID: audit.Target(asset.ID),
		After: map[string]any{"asset_no": asset.AssetNo, "name": asset.Name},
	}); err != nil {
		return db.Asset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Asset{}, fmt.Errorf("registries: commit: %w", err)
	}
	return asset, nil
}

type MaintenanceInput struct {
	AssetID     uuid.UUID
	EventType   *string
	PerformedAt pgtype.Timestamptz
	NextDueOn   pgtype.Date
	Notes       *string
}

// RecordMaintenance appends a service record and, when the asset was flagged as
// due, returns it to `running`.
//
// The status change is here rather than left to the operator because forgetting
// it is the failure mode: the asset stays amber on the dashboard after it has
// been serviced, and the factory learns to ignore the colour.
func (s *Service) RecordMaintenance(ctx context.Context, actor uuid.UUID, in MaintenanceInput) (db.MaintenanceEvent, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.MaintenanceEvent{}, fmt.Errorf("registries: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	asset, err := q.GetAsset(ctx, in.AssetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.MaintenanceEvent{}, common.Validation(common.FieldError{
				Field: "asset_id", Code: "not_found", Message: "Оборудование не найдено",
			})
		}
		return db.MaintenanceEvent{}, fmt.Errorf("registries: load asset: %w", err)
	}
	if asset.Status == AssetRetired {
		return db.MaintenanceEvent{}, common.BusinessRule(
			"Нельзя записать обслуживание для списанного оборудования.")
	}

	event, err := q.CreateMaintenanceEvent(ctx, db.CreateMaintenanceEventParams{
		AssetID: in.AssetID, EventType: in.EventType, PerformedAt: in.PerformedAt,
		NextDueOn: in.NextDueOn, Notes: in.Notes, CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.MaintenanceEvent{}, fmt.Errorf("registries: create maintenance: %w", err)
	}

	if asset.Status == AssetMaintenanceDue {
		if _, err := q.UpdateAsset(ctx, db.UpdateAssetParams{
			ID: asset.ID, AssetNo: asset.AssetNo, Name: asset.Name,
			AssetType: asset.AssetType, Line: asset.Line,
			CommissionedOn: asset.CommissionedOn, WarrantyUntil: asset.WarrantyUntil,
			Status: AssetRunning, Version: asset.Version,
		}); err != nil {
			return db.MaintenanceEvent{}, fmt.Errorf("registries: clear due flag: %w", err)
		}
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate,
		Resource: rbac.Equipment, ResourceID: audit.Target(in.AssetID),
		After: map[string]any{
			"event": "maintenance", "asset_no": asset.AssetNo,
			"next_due_on": in.NextDueOn.Time.Format("2006-01-02"),
		},
	}); err != nil {
		return db.MaintenanceEvent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.MaintenanceEvent{}, fmt.Errorf("registries: commit: %w", err)
	}
	return event, nil
}

func (s *Service) Assets(ctx context.Context, p common.Params, status *string) ([]db.ListAssetsRow, int64, error) {
	q := db.New(s.pool)
	rows, err := q.ListAssets(ctx, db.ListAssetsParams{
		Status: status, Q: nilIfEmpty(p.Query), Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("registries: list assets: %w", err)
	}
	total, err := q.CountAssets(ctx, db.CountAssetsParams{Status: status, Q: nilIfEmpty(p.Query)})
	if err != nil {
		return nil, 0, fmt.Errorf("registries: count assets: %w", err)
	}
	return rows, total, nil
}

func (s *Service) MaintenanceFor(ctx context.Context, assetID uuid.UUID) ([]db.MaintenanceEvent, error) {
	rows, err := db.New(s.pool).ListMaintenanceEvents(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("registries: maintenance: %w", err)
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// Документы
// ---------------------------------------------------------------------------

// DocTransition is one legal move in the document ladder.
type DocTransition struct {
	From, To        string
	RequiresApprove bool
}

// docTransitions is the complete matrix. Anything absent is illegal.
//
// `active` requires documents:approve on both routes into it. A certificate
// saying the water is potable is a claim the company makes to a regulator, so
// someone with authority signs it — the same reasoning as releasing a batch.
//
// `expiring` and `expired` are absent as destinations: those are conditions of a
// date passing, not decisions anyone makes, and the alerts service derives them.
var docTransitions = []DocTransition{
	{DocDraft, DocApproval, false},
	{DocApproval, DocDraft, false}, // sent back for correction
	{DocApproval, DocActive, true},
	{DocActive, DocArchived, true},
	{DocArchived, DocActive, true},
}

func LookupDoc(from, to string) (DocTransition, bool) {
	for _, t := range docTransitions {
		if t.From == from && t.To == to {
			return t, true
		}
	}
	return DocTransition{}, false
}

// DocAllowedFrom projects the matrix onto one status and one permission, so the
// buttons and the rules share a single definition.
func DocAllowedFrom(status string, hasApprove bool) []string {
	out := make([]string, 0, len(docTransitions))
	for _, t := range docTransitions {
		if t.From != status || (t.RequiresApprove && !hasApprove) {
			continue
		}
		out = append(out, t.To)
	}
	return out
}

// DocTransitions returns the whole matrix, for tests and for documentation.
func DocTransitions() []DocTransition {
	return append([]DocTransition(nil), docTransitions...)
}

type DocumentInput struct {
	DocNo      string
	Title      string
	DocType    *string
	OwnerID    uuid.NullUUID
	ValidUntil pgtype.Date
	Version    int32
}

func (in DocumentInput) validate() error {
	var details []common.FieldError
	if strings.TrimSpace(in.DocNo) == "" {
		details = append(details, common.FieldError{
			Field: "doc_no", Code: "required", Message: "Укажите номер документа",
		})
	}
	if strings.TrimSpace(in.Title) == "" {
		details = append(details, common.FieldError{
			Field: "title", Code: "required", Message: "Укажите название документа",
		})
	}
	if len(details) > 0 {
		return common.Validation(details...)
	}
	return nil
}

func (s *Service) CreateDocument(ctx context.Context, actor uuid.UUID, in DocumentInput) (db.Document, error) {
	if err := in.validate(); err != nil {
		return db.Document{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Document{}, fmt.Errorf("registries: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	// Always created as a draft. There is no way to post a document straight to
	// `active`, because that would be an approval without an approver.
	doc, err := q.CreateDocument(ctx, db.CreateDocumentParams{
		DocNo: in.DocNo, Title: in.Title, DocType: in.DocType,
		OwnerID: in.OwnerID, ValidUntil: in.ValidUntil,
		Status: nil, CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.Document{}, fmt.Errorf("registries: create document: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate,
		Resource: rbac.Documents, ResourceID: audit.Target(doc.ID),
		After: map[string]any{"doc_no": doc.DocNo, "title": doc.Title, "status": doc.Status},
	}); err != nil {
		return db.Document{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Document{}, fmt.Errorf("registries: commit: %w", err)
	}
	return doc, nil
}

type DocTransitionInput struct {
	DocID      uuid.UUID
	To         string
	HasApprove bool
}

func (s *Service) TransitionDocument(ctx context.Context, actor uuid.UUID, in DocTransitionInput) (db.Document, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Document{}, fmt.Errorf("registries: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	before, err := q.GetDocument(ctx, in.DocID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Document{}, common.NotFound()
		}
		return db.Document{}, fmt.Errorf("registries: load document: %w", err)
	}

	rule, legal := LookupDoc(before.Status, in.To)
	if !legal {
		return db.Document{}, common.BusinessRule(fmt.Sprintf(
			"Недопустимый переход документа: из «%s» в «%s».", before.Status, in.To))
	}
	if rule.RequiresApprove && !in.HasApprove {
		return db.Document{}, common.Forbidden()
	}

	updated, err := q.TransitionDocument(ctx, db.TransitionDocumentParams{
		ID: in.DocID, FromStatus: before.Status, ToStatus: in.To,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Document{}, common.BusinessRule(
				"Статус документа изменился другим пользователем. Обновите страницу.")
		}
		return db.Document{}, fmt.Errorf("registries: transition document: %w", err)
	}

	action := audit.ActionUpdate
	if rule.RequiresApprove {
		action = audit.ActionApprove
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: action,
		Resource: rbac.Documents, ResourceID: audit.Target(in.DocID),
		Before: map[string]any{"status": before.Status},
		After:  map[string]any{"status": in.To, "doc_no": before.DocNo},
	}); err != nil {
		return db.Document{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Document{}, fmt.Errorf("registries: commit: %w", err)
	}
	return updated, nil
}

func (s *Service) Documents(ctx context.Context, p common.Params, status *string) ([]db.ListDocumentsRow, int64, error) {
	q := db.New(s.pool)
	rows, err := q.ListDocuments(ctx, db.ListDocumentsParams{
		Status: status, Q: nilIfEmpty(p.Query), Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("registries: list documents: %w", err)
	}
	total, err := q.CountDocuments(ctx, db.CountDocumentsParams{Status: status, Q: nilIfEmpty(p.Query)})
	if err != nil {
		return nil, 0, fmt.Errorf("registries: count documents: %w", err)
	}
	return rows, total, nil
}

func (s *Service) Document(ctx context.Context, id uuid.UUID) (db.GetDocumentRow, error) {
	row, err := db.New(s.pool).GetDocument(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.GetDocumentRow{}, common.NotFound()
		}
		return db.GetDocumentRow{}, fmt.Errorf("registries: document: %w", err)
	}
	return row, nil
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
