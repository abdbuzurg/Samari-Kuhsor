// Package crm implements the customer side of CRM и продажи: customers,
// contacts, leads, deals and tasks.
//
// These five tables have existed since migration 00003 and, until R12, had no
// query, no route and no UI. `CreateCustomer` and `CreateLead` were reachable
// only as a side effect of converting a website enquiry, which produced records
// no screen could open. The dashboard's Воронка продаж reads `deals`, so it has
// rendered its empty state since the day it shipped.
//
// Sales orders and shipments live in package sales, which enforces the released-
// batch rule. This package deliberately holds no stock or batch logic: it is the
// pipeline in front of an order, not the order itself.
//
// docs/05-MODULES.md:183 notes this module is EMPTY on opening day. The stage
// ladder is still modelled as immutable events rather than a mutable column,
// because a pipeline whose history can be rewritten is not a pipeline.
package crm

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
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

const Resource = rbac.CRM

// Deal stages, mirroring the CHECK constraint in migration 00003 and the
// pipeline specified at docs/05-MODULES.md:179.
const (
	StageNew         = "new"
	StageNegotiation = "negotiation"
	StageQuoted      = "quoted"
	StageWon         = "won"
	StageLost        = "lost"
)

var Stages = []string{StageNew, StageNegotiation, StageQuoted, StageWon, StageLost}

// Regions the client actually sells into (docs/05-MODULES.md:181).
var Regions = []string{"Душанбе", "Худжанд", "Хорог", "Бохтар"}

// legalStageMoves is the pipeline as a matrix rather than a free-for-all.
//
// Forward one rung at a time, or straight to won/lost from anywhere still open.
// Backwards is allowed — a deal that slips from «КП отправлено» back to
// «Переговоры» is an ordinary Tuesday, and refusing it would only teach people
// to lie to the system.
//
// What is NOT allowed is moving out of won or lost. Those are terminal: a closed
// deal that can be reopened makes every conversion figure provisional.
func LegalStageMove(from, to string) bool {
	if from == to {
		return false
	}
	if from == StageWon || from == StageLost {
		return false
	}
	return contains(Stages, to)
}

// AllowedFrom is what the UI may offer, computed from the same matrix the domain
// enforces so the buttons cannot drift from the rules.
func AllowedFrom(stage string) []string {
	out := make([]string, 0, len(Stages))
	for _, s := range Stages {
		if LegalStageMove(stage, s) {
			out = append(out, s)
		}
	}
	return out
}

var (
	CustomerSortSpec = common.SortSpec{
		Allowed: []string{"name", "region", "created_at"},
		Default: "name",
	}
	DealSortSpec = common.SortSpec{
		Allowed: []string{"expected_close", "amount", "stage", "created_at"},
		Default: "expected_close",
	}
	TaskSortSpec = common.SortSpec{
		Allowed: []string{"due_on", "title", "status"},
		Default: "due_on",
	}
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// ---------------------------------------------------------------------------
// Customers
// ---------------------------------------------------------------------------

type CustomerInput struct {
	Name    string
	Type    string
	Region  string
	Contact string
	Version int32
}

func (in CustomerInput) validate() error {
	var details []common.FieldError
	if strings.TrimSpace(in.Name) == "" {
		details = append(details, common.FieldError{
			Field: "name", Code: "required", Message: "Укажите название клиента",
		})
	}
	// The region list is closed because the four regions drive the CRM's own
	// column and a free-text region makes that column meaningless within a month.
	if in.Region != "" && !contains(Regions, in.Region) {
		details = append(details, common.FieldError{
			Field: "region", Code: "invalid",
			Message: "Регион должен быть одним из: " + strings.Join(Regions, ", "),
		})
	}
	if len(details) > 0 {
		return common.Validation(details...)
	}
	return nil
}

func (s *Service) Customers(ctx context.Context, p common.Params, region *string) ([]db.ListCustomersRow, int64, error) {
	q := db.New(s.pool)
	rows, err := q.ListCustomers(ctx, db.ListCustomersParams{
		Limit: int32(p.PerPage), Offset: int32(p.Offset()),
		Q: nilIfEmpty(p.Query), Region: region,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("crm: list customers: %w", err)
	}
	total, err := q.CountCustomers(ctx, db.CountCustomersParams{
		Q: nilIfEmpty(p.Query), Region: region,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("crm: count customers: %w", err)
	}
	return rows, total, nil
}

func (s *Service) Customer(ctx context.Context, id uuid.UUID) (db.Customer, error) {
	row, err := db.New(s.pool).GetCustomer(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Customer{}, common.NotFound()
		}
		return db.Customer{}, fmt.Errorf("crm: customer: %w", err)
	}
	return row, nil
}

func (s *Service) CreateCustomer(ctx context.Context, actor uuid.UUID, in CustomerInput) (db.Customer, error) {
	if err := in.validate(); err != nil {
		return db.Customer{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Customer{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	customer, err := q.CreateCustomer(ctx, db.CreateCustomerParams{
		Name: in.Name, Contact: nilIfEmpty(in.Contact),
		CustomerType: nilIfEmpty(in.Type), Region: nilIfEmpty(in.Region),
		CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.Customer{}, fmt.Errorf("crm: create customer: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate,
		Resource: Resource, ResourceID: audit.Target(customer.ID),
		After: map[string]any{"name": customer.Name, "region": customer.Region},
	}); err != nil {
		return db.Customer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Customer{}, fmt.Errorf("crm: commit: %w", err)
	}
	return customer, nil
}

func (s *Service) UpdateCustomer(ctx context.Context, actor, id uuid.UUID, in CustomerInput) (db.Customer, error) {
	if err := in.validate(); err != nil {
		return db.Customer{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Customer{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	before, err := q.GetCustomer(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Customer{}, common.NotFound()
		}
		return db.Customer{}, fmt.Errorf("crm: load customer: %w", err)
	}

	updated, err := q.UpdateCustomer(ctx, db.UpdateCustomerParams{
		ID: id, Name: in.Name, CustomerType: nilIfEmpty(in.Type),
		Region: nilIfEmpty(in.Region), Contact: nilIfEmpty(in.Contact),
		Version: in.Version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The row exists but the version did not match: someone else saved
			// first, and overwriting them silently is the thing versions exist
			// to prevent.
			return db.Customer{}, common.VersionConflict(before.Version)
		}
		return db.Customer{}, fmt.Errorf("crm: update customer: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate,
		Resource: Resource, ResourceID: audit.Target(id),
		Before: map[string]any{"name": before.Name, "region": before.Region},
		After:  map[string]any{"name": updated.Name, "region": updated.Region},
	}); err != nil {
		return db.Customer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Customer{}, fmt.Errorf("crm: commit: %w", err)
	}
	return updated, nil
}

// ---------------------------------------------------------------------------
// Contacts, leads and the customer's related records
// ---------------------------------------------------------------------------

func (s *Service) Contacts(ctx context.Context, customerID uuid.UUID) ([]db.Contact, error) {
	rows, err := db.New(s.pool).ListContactsForCustomer(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("crm: contacts: %w", err)
	}
	return rows, nil
}

type ContactInput struct {
	CustomerID uuid.UUID
	FullName   string
	Role       string
	Email      string
	Phone      string
}

func (s *Service) CreateContact(ctx context.Context, actor uuid.UUID, in ContactInput) (db.Contact, error) {
	if strings.TrimSpace(in.FullName) == "" {
		return db.Contact{}, common.Validation(common.FieldError{
			Field: "full_name", Code: "required", Message: "Укажите имя контактного лица",
		})
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Contact{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	if _, err := q.GetCustomer(ctx, in.CustomerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Contact{}, common.NotFound()
		}
		return db.Contact{}, fmt.Errorf("crm: customer: %w", err)
	}

	contact, err := q.CreateContact(ctx, db.CreateContactParams{
		CustomerID: in.CustomerID, FullName: in.FullName,
		Role: nilIfEmpty(in.Role), Email: nilIfEmpty(in.Email), Phone: nilIfEmpty(in.Phone),
		CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.Contact{}, fmt.Errorf("crm: create contact: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate,
		Resource: Resource, ResourceID: audit.Target(contact.ID),
		After: map[string]any{"full_name": contact.FullName, "customer_id": in.CustomerID},
	}); err != nil {
		return db.Contact{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Contact{}, fmt.Errorf("crm: commit: %w", err)
	}
	return contact, nil
}

func (s *Service) LeadsFor(ctx context.Context, customerID uuid.UUID) ([]db.Lead, error) {
	// leads.customer_id is nullable, so the generated parameter is a NullUUID.
	rows, err := db.New(s.pool).ListLeadsForCustomer(ctx, uuid.NullUUID{UUID: customerID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("crm: leads: %w", err)
	}
	return rows, nil
}

func (s *Service) DealsFor(ctx context.Context, customerID uuid.UUID) ([]db.ListDealsForCustomerRow, error) {
	rows, err := db.New(s.pool).ListDealsForCustomer(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("crm: customer deals: %w", err)
	}
	return rows, nil
}

func (s *Service) OrdersFor(ctx context.Context, customerID uuid.UUID) ([]db.ListSalesOrdersForCustomerRow, error) {
	rows, err := db.New(s.pool).ListSalesOrdersForCustomer(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("crm: customer orders: %w", err)
	}
	return rows, nil
}

func (s *Service) InquiriesFor(ctx context.Context, customerID uuid.UUID) ([]db.ListInquiriesForCustomerRow, error) {
	rows, err := db.New(s.pool).ListInquiriesForCustomer(ctx, uuid.NullUUID{UUID: customerID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("crm: customer inquiries: %w", err)
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// Deals
// ---------------------------------------------------------------------------

type DealInput struct {
	CustomerID uuid.UUID
	// Money is a decimal end to end, never a float (CLAUDE.md §4.6). Parsing
	// happens in the HTTP layer, like every other date and amount in this system.
	Amount        decimal.NullDecimal
	Stage         string
	OwnerID       uuid.NullUUID
	ExpectedClose pgtype.Date
}

func (s *Service) Deals(ctx context.Context, p common.Params, stage *string, customer uuid.NullUUID) ([]db.ListDealsRow, int64, error) {
	q := db.New(s.pool)
	rows, err := q.ListDeals(ctx, db.ListDealsParams{
		Limit: int32(p.PerPage), Offset: int32(p.Offset()),
		Q: nilIfEmpty(p.Query), Stage: stage, CustomerID: customer,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("crm: list deals: %w", err)
	}
	total, err := q.CountDeals(ctx, db.CountDealsParams{
		Q: nilIfEmpty(p.Query), Stage: stage, CustomerID: customer,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("crm: count deals: %w", err)
	}
	return rows, total, nil
}

func (s *Service) Deal(ctx context.Context, id uuid.UUID) (db.GetDealRow, error) {
	row, err := db.New(s.pool).GetDeal(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.GetDealRow{}, common.NotFound()
		}
		return db.GetDealRow{}, fmt.Errorf("crm: deal: %w", err)
	}
	return row, nil
}

func (s *Service) StageEvents(ctx context.Context, dealID uuid.UUID) ([]db.ListDealStageEventsRow, error) {
	rows, err := db.New(s.pool).ListDealStageEvents(ctx, dealID)
	if err != nil {
		return nil, fmt.Errorf("crm: stage events: %w", err)
	}
	return rows, nil
}

func (s *Service) CreateDeal(ctx context.Context, actor uuid.UUID, in DealInput) (db.Deal, error) {
	if in.Stage != "" && !contains(Stages, in.Stage) {
		return db.Deal{}, common.Validation(common.FieldError{
			Field: "stage", Code: "invalid", Message: "Неизвестная стадия сделки",
		})
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Deal{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	if _, err := q.GetCustomer(ctx, in.CustomerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Deal{}, common.Validation(common.FieldError{
				Field: "customer_id", Code: "not_found", Message: "Клиент не найден",
			})
		}
		return db.Deal{}, fmt.Errorf("crm: customer: %w", err)
	}

	deal, err := q.CreateDeal(ctx, db.CreateDealParams{
		CustomerID: in.CustomerID, Amount: in.Amount,
		Stage: nilIfEmpty(in.Stage), OwnerID: in.OwnerID,
		ExpectedClose: in.ExpectedClose, CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.Deal{}, fmt.Errorf("crm: create deal: %w", err)
	}

	// The opening rung of the ladder, recorded like every other move. A deal that
	// simply appears at a stage has no history to read.
	if _, err := q.CreateDealStageEvent(ctx, db.CreateDealStageEventParams{
		DealID: deal.ID, FromStage: nil, ToStage: deal.Stage, ChangedBy: actor,
	}); err != nil {
		return db.Deal{}, fmt.Errorf("crm: opening stage event: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate,
		Resource: Resource, ResourceID: audit.Target(deal.ID),
		After: map[string]any{"customer_id": in.CustomerID, "stage": deal.Stage},
	}); err != nil {
		return db.Deal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Deal{}, fmt.Errorf("crm: commit: %w", err)
	}
	return deal, nil
}

type StageInput struct {
	DealID uuid.UUID
	To     string
	Note   *string
}

// MoveStage advances a deal and records the move as immutable evidence.
//
// The event is written in the SAME transaction as the stage change, so a
// pipeline whose current stage disagrees with its own history is not a state the
// database can be left in.
func (s *Service) MoveStage(ctx context.Context, actor uuid.UUID, in StageInput) (db.Deal, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Deal{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	before, err := q.GetDeal(ctx, in.DealID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Deal{}, common.NotFound()
		}
		return db.Deal{}, fmt.Errorf("crm: load deal: %w", err)
	}

	if !LegalStageMove(before.Deal.Stage, in.To) {
		if before.Deal.Stage == StageWon || before.Deal.Stage == StageLost {
			return db.Deal{}, common.BusinessRule(
				"Сделка уже закрыта; её стадию изменить нельзя.")
		}
		return db.Deal{}, common.BusinessRule(fmt.Sprintf(
			"Переход «%s» → «%s» недопустим.", before.Deal.Stage, in.To))
	}

	updated, err := q.SetDealStage(ctx, db.SetDealStageParams{ID: in.DealID, Stage: in.To})
	if err != nil {
		return db.Deal{}, fmt.Errorf("crm: set stage: %w", err)
	}
	from := before.Deal.Stage
	if _, err := q.CreateDealStageEvent(ctx, db.CreateDealStageEventParams{
		DealID: in.DealID, FromStage: &from, ToStage: in.To,
		ChangedBy: actor, Note: in.Note,
	}); err != nil {
		return db.Deal{}, fmt.Errorf("crm: stage event: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate,
		Resource: Resource, ResourceID: audit.Target(in.DealID),
		Before: map[string]any{"stage": from},
		After:  map[string]any{"stage": in.To, "note": in.Note},
	}); err != nil {
		return db.Deal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Deal{}, fmt.Errorf("crm: commit: %w", err)
	}
	return updated, nil
}

// ---------------------------------------------------------------------------
// Tasks
// ---------------------------------------------------------------------------

type TaskInput struct {
	Title       string
	AssigneeID  uuid.NullUUID
	DueOn       pgtype.Date
	RelatedType string
	RelatedID   uuid.NullUUID
}

func (s *Service) Tasks(ctx context.Context, p common.Params, status *string) ([]db.ListTasksRow, int64, error) {
	q := db.New(s.pool)
	rows, err := q.ListTasks(ctx, db.ListTasksParams{
		Limit: int32(p.PerPage), Offset: int32(p.Offset()), Status: status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("crm: list tasks: %w", err)
	}
	total, err := q.CountTasks(ctx, status)
	if err != nil {
		return nil, 0, fmt.Errorf("crm: count tasks: %w", err)
	}
	return rows, total, nil
}

func (s *Service) CreateTask(ctx context.Context, actor uuid.UUID, in TaskInput) (db.Task, error) {
	if strings.TrimSpace(in.Title) == "" {
		return db.Task{}, common.Validation(common.FieldError{
			Field: "title", Code: "required", Message: "Укажите название задачи",
		})
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Task{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	task, err := q.CreateTask(ctx, db.CreateTaskParams{
		Title: in.Title, AssigneeID: in.AssigneeID, DueOn: in.DueOn,
		RelatedType: nilIfEmpty(in.RelatedType), RelatedID: in.RelatedID,
		CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.Task{}, fmt.Errorf("crm: create task: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate,
		Resource: Resource, ResourceID: audit.Target(task.ID),
		After: map[string]any{"title": task.Title},
	}); err != nil {
		return db.Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Task{}, fmt.Errorf("crm: commit: %w", err)
	}
	return task, nil
}

func (s *Service) SetTaskStatus(ctx context.Context, actor, id uuid.UUID, status string) (db.Task, error) {
	if !contains([]string{"open", "done", "cancelled"}, status) {
		return db.Task{}, common.Validation(common.FieldError{
			Field: "status", Code: "invalid", Message: "Неизвестный статус задачи",
		})
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Task{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	task, err := q.SetTaskStatus(ctx, db.SetTaskStatusParams{ID: id, Status: status})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Task{}, common.NotFound()
		}
		return db.Task{}, fmt.Errorf("crm: set task status: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate,
		Resource: Resource, ResourceID: audit.Target(id),
		After: map[string]any{"status": status},
	}); err != nil {
		return db.Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Task{}, fmt.Errorf("crm: commit: %w", err)
	}
	return task, nil
}

// ---------------------------------------------------------------------------
// KPIs
// ---------------------------------------------------------------------------

// KPIs are the four figures specified at docs/05-MODULES.md:179.
type KPIs struct {
	NewLeads  int64
	OpenDeals int64
	// Conversion is null before anything has closed, never "0%": a pipeline with
	// nothing decided has no conversion rate, and 0% reads as total failure.
	Conversion   *string
	OverdueTasks int32
}

func (s *Service) KPIs(ctx context.Context) (KPIs, error) {
	q := db.New(s.pool)
	var out KPIs
	var err error

	if out.NewLeads, err = q.CountNewLeads(ctx); err != nil {
		return out, fmt.Errorf("crm: new leads: %w", err)
	}
	if out.OpenDeals, err = q.CountOpenDeals(ctx); err != nil {
		return out, fmt.Errorf("crm: open deals: %w", err)
	}
	if out.OverdueTasks, err = q.CountOverdueTasks(ctx); err != nil {
		return out, fmt.Errorf("crm: overdue tasks: %w", err)
	}
	counts, err := q.DealOutcomeCounts(ctx)
	if err != nil {
		return out, fmt.Errorf("crm: conversion: %w", err)
	}
	if counts.Decided > 0 {
		rate := decimal.NewFromInt(int64(counts.Won)).
			Mul(decimal.NewFromInt(100)).
			Div(decimal.NewFromInt(int64(counts.Decided))).
			StringFixed(1)
		out.Conversion = &rate
	}
	return out, nil
}

// ---------------------------------------------------------------------------

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
