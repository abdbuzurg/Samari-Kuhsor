package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/crm"
	"github.com/qoim/samari/backend/internal/http/common"
)

// CRM и продажи — customers, contacts, leads, deals and tasks.
//
// Sales orders live in commerce_handler.go. This file is the pipeline in front
// of an order.

// dealStage maps a pipeline stage to its display level.
//
// Replaces an earlier version in dashboard_handler.go whose cases — 'proposal',
// 'qualification' — are not in the `deals.stage` CHECK constraint at all. The
// four stages that ARE would have fallen through to the raw English key in a
// Russian interface. It never showed because nothing ever wrote a deal.
//
// Progress is deliberately not graded green: a deal at «КП отправлено» is not
// healthier than one at «Переговоры», it is further along. Green means healthy
// (CLAUDE.md §5), and colouring progress would make the funnel a scorecard.
func dealStage(stage string) api.Status {
	switch stage {
	case crm.StageNew:
		return api.Status{Key: stage, Label: "Новый лид", Level: string(common.LevelInfo)}
	case crm.StageNegotiation:
		return api.Status{Key: stage, Label: "Переговоры", Level: string(common.LevelInfo)}
	case crm.StageQuoted:
		// Amber: a quotation sent is a clock running against somebody.
		return api.Status{Key: stage, Label: "КП отправлено", Level: string(common.LevelWarn)}
	case crm.StageWon:
		return api.Status{Key: stage, Label: "Выиграно", Level: string(common.LevelOK)}
	case crm.StageLost:
		// Neutral, not danger: a lost deal is an ordinary outcome, and colouring
		// it red would make the pipeline look like a fault report.
		return api.Status{Key: stage, Label: "Проиграно", Level: string(common.LevelNeutral)}
	default:
		return api.Status{Key: stage, Label: stage, Level: string(common.LevelNeutral)}
	}
}

func leadStatus(status string) api.Status {
	switch status {
	case "won":
		return api.Status{Key: status, Label: "Выиграно", Level: string(common.LevelOK)}
	case "lost":
		return api.Status{Key: status, Label: "Проиграно", Level: string(common.LevelNeutral)}
	case "quoted":
		return api.Status{Key: status, Label: "КП отправлено", Level: string(common.LevelWarn)}
	case "negotiation":
		return api.Status{Key: status, Label: "Переговоры", Level: string(common.LevelInfo)}
	default:
		// A new lead is amber: it is work outstanding, not a healthy state.
		return api.Status{Key: status, Label: "Новый", Level: string(common.LevelWarn)}
	}
}

func taskStatus(status string) api.Status {
	switch status {
	case "done":
		return api.Status{Key: status, Label: "Выполнена", Level: string(common.LevelOK)}
	case "cancelled":
		return api.Status{Key: status, Label: "Отменена", Level: string(common.LevelNeutral)}
	default:
		return api.Status{Key: status, Label: "Открыта", Level: string(common.LevelInfo)}
	}
}

// ---------------------------------------------------------------------------
// Customers
// ---------------------------------------------------------------------------

func (s *Server) handleListCustomers(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, crm.CustomerSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.CRM.Customers(r.Context(), params, optionalQuery(r, "region"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.CustomerRow, 0, len(rows))
	for _, row := range rows {
		c := api.CustomerRow{
			ID: row.Customer.ID.String(), Name: row.Customer.Name,
			CustomerType: row.Customer.CustomerType, Region: row.Customer.Region,
			Contact: row.Customer.Contact, OpenDeals: row.OpenDeals,
			OpenAmount: row.OpenAmount.String(), Version: row.Customer.Version,
		}
		// max(status) over the customer's leads; empty when they have none.
		if row.LeadStatus != "" {
			st := leadStatus(row.LeadStatus)
			c.LeadStatus = &st
		}
		out = append(out, c)
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

func customerResponse(c db.Customer) api.Customer {
	return api.Customer{
		ID: c.ID.String(), Name: c.Name, CustomerType: c.CustomerType,
		Region: c.Region, Contact: c.Contact, Version: c.Version,
		CreatedAt: common.Timestamp(c.CreatedAt),
	}
}

// handleGetCustomer is the detail view: header plus every band
// (docs/05-MODULES.md:179).
func (s *Server) handleGetCustomer(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	ctx := r.Context()

	customer, err := s.svc.CRM.Customer(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	contacts, err := s.svc.CRM.Contacts(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	leads, err := s.svc.CRM.LeadsFor(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	deals, err := s.svc.CRM.DealsFor(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	orders, err := s.svc.CRM.OrdersFor(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	enquiries, err := s.svc.CRM.InquiriesFor(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	detail := api.CustomerDetail{
		Customer:  customerResponse(customer),
		Contacts:  make([]api.Contact, 0, len(contacts)),
		Leads:     make([]api.CustomerLead, 0, len(leads)),
		Deals:     make([]api.DealRow, 0, len(deals)),
		Orders:    make([]api.CustomerOrder, 0, len(orders)),
		Inquiries: make([]api.CustomerInquiry, 0, len(enquiries)),
	}
	for _, c := range contacts {
		detail.Contacts = append(detail.Contacts, api.Contact{
			ID: c.ID.String(), FullName: c.FullName, Role: c.Role,
			Email: c.Email, Phone: c.Phone,
		})
	}
	for _, l := range leads {
		lead := api.CustomerLead{
			ID: l.ID.String(), Source: l.Source, Status: leadStatus(l.Status),
			CreatedAt: common.Timestamp(l.CreatedAt),
		}
		if l.InquiryID.Valid {
			iid := l.InquiryID.UUID.String()
			lead.InquiryID = &iid
		}
		detail.Leads = append(detail.Leads, lead)
	}
	for _, d := range deals {
		detail.Deals = append(detail.Deals, api.DealRow{
			ID: d.Deal.ID.String(), CustomerID: d.Deal.CustomerID.String(),
			CustomerName: customer.Name, Region: customer.Region,
			Amount: moneyOrNil(d.Deal.Amount), Stage: dealStage(d.Deal.Stage),
			OwnerName: d.OwnerName, ExpectedClose: common.Date(d.Deal.ExpectedClose),
			Version: d.Deal.Version,
		})
	}
	for _, o := range orders {
		detail.Orders = append(detail.Orders, api.CustomerOrder{
			ID: o.ID.String(), SONo: o.SoNo, OrderedOn: common.Date(o.OrderedOn),
			Total: o.Total.String(), Status: soStatus(o.Status),
		})
	}
	for _, i := range enquiries {
		detail.Inquiries = append(detail.Inquiries, api.CustomerInquiry{
			ID: i.ID.String(), ReferenceNo: i.ReferenceNo,
			Type: inquiryType(i.InquiryType), Status: inquiryStatus(i.Status),
			SubmittedAt: common.Timestamp(i.CreatedAt),
		})
	}

	common.JSON(w, http.StatusOK, detail)
}

func (s *Server) handleCreateCustomer(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.CustomerWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	customer, err := s.svc.CRM.CreateCustomer(r.Context(), ident.User.ID, crm.CustomerInput{
		Name: req.Name, Type: deref(req.CustomerType),
		Region: deref(req.Region), Contact: deref(req.Contact),
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusCreated, customerResponse(customer))
}

func (s *Server) handleUpdateCustomer(w http.ResponseWriter, r *http.Request) {
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
	var req api.CustomerWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	version, err := common.RequireVersion(common.Versioned{Version: req.Version})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	customer, err := s.svc.CRM.UpdateCustomer(r.Context(), ident.User.ID, id, crm.CustomerInput{
		Name: req.Name, Type: deref(req.CustomerType),
		Region: deref(req.Region), Contact: deref(req.Contact), Version: version,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, customerResponse(customer))
}

func (s *Server) handleCreateContact(w http.ResponseWriter, r *http.Request) {
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
	var req api.ContactWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	contact, err := s.svc.CRM.CreateContact(r.Context(), ident.User.ID, crm.ContactInput{
		CustomerID: id, FullName: req.FullName, Role: deref(req.Role),
		Email: deref(req.Email), Phone: deref(req.Phone),
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusCreated, api.Contact{
		ID: contact.ID.String(), FullName: contact.FullName, Role: contact.Role,
		Email: contact.Email, Phone: contact.Phone,
	})
}

// ---------------------------------------------------------------------------
// Deals
// ---------------------------------------------------------------------------

func (s *Server) handleListDeals(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, crm.DealSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var customer uuid.NullUUID
	if raw := r.URL.Query().Get("customer_id"); raw != "" {
		id, err := parseUUIDField(raw, "customer_id")
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		customer = uuid.NullUUID{UUID: id, Valid: true}
	}

	rows, total, err := s.svc.CRM.Deals(r.Context(), params, optionalQuery(r, "stage"), customer)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.DealRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.DealRow{
			ID: row.Deal.ID.String(), CustomerID: row.Deal.CustomerID.String(),
			CustomerName: row.CustomerName, Region: row.CustomerRegion,
			Amount: moneyOrNil(row.Deal.Amount), Stage: dealStage(row.Deal.Stage),
			OwnerName: row.OwnerName, ExpectedClose: common.Date(row.Deal.ExpectedClose),
			Version: row.Deal.Version,
		})
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

func (s *Server) handleGetDeal(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	ctx := r.Context()

	row, err := s.svc.CRM.Deal(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	events, err := s.svc.CRM.StageEvents(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	detail := api.Deal{
		ID: row.Deal.ID.String(), CustomerID: row.Deal.CustomerID.String(),
		CustomerName: row.CustomerName, Amount: moneyOrNil(row.Deal.Amount),
		Stage: dealStage(row.Deal.Stage), OwnerName: row.OwnerName,
		ExpectedClose: common.Date(row.Deal.ExpectedClose),
		Version:       row.Deal.Version, CreatedAt: common.Timestamp(row.Deal.CreatedAt),
		History: make([]api.DealStageEvent, 0, len(events)),
		// Computed from the same matrix the domain enforces, so the UI cannot
		// offer a move the server would refuse.
		AllowedTransitions: crm.AllowedFrom(row.Deal.Stage),
	}
	for _, e := range events {
		event := api.DealStageEvent{
			ID: e.ID.String(), ToStage: dealStage(e.ToStage),
			OccurredAt: common.Timestamp(e.OccurredAt),
			ChangedBy:  e.ChangedByName, Note: e.Note,
		}
		if e.FromStage != nil {
			from := dealStage(*e.FromStage)
			event.FromStage = &from
		}
		detail.History = append(detail.History, event)
	}

	common.JSON(w, http.StatusOK, detail)
}

func (s *Server) handleCreateDeal(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.DealWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	customerID, err := parseUUIDField(req.CustomerID, "customer_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	amountPtr, err := optionalDecimal(req.Amount, "amount")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var amount decimal.NullDecimal
	if amountPtr != nil {
		amount = decimal.NullDecimal{Decimal: *amountPtr, Valid: true}
	}
	expected, err := parseDate(deref(req.ExpectedClose), "expected_close", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	owner, err := parseNullUUIDField(req.OwnerID, "owner_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	deal, err := s.svc.CRM.CreateDeal(r.Context(), ident.User.ID, crm.DealInput{
		CustomerID: customerID, Amount: amount, Stage: deref(req.Stage),
		OwnerID: owner, ExpectedClose: expected,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusCreated, api.DealRow{
		ID: deal.ID.String(), CustomerID: deal.CustomerID.String(),
		Amount: moneyOrNil(deal.Amount), Stage: dealStage(deal.Stage),
		ExpectedClose: common.Date(deal.ExpectedClose), Version: deal.Version,
	})
}

// handleMoveDealStage is a POST to an action rather than a PATCH of `stage`:
// the move is guarded by a matrix and writes an immutable event, neither of
// which a generic field update could express.
func (s *Server) handleMoveDealStage(w http.ResponseWriter, r *http.Request) {
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
	var req api.DealStageRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	deal, err := s.svc.CRM.MoveStage(r.Context(), ident.User.ID, crm.StageInput{
		DealID: id, To: req.To, Note: req.Note,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, api.DealRow{
		ID: deal.ID.String(), CustomerID: deal.CustomerID.String(),
		Amount: moneyOrNil(deal.Amount), Stage: dealStage(deal.Stage),
		ExpectedClose: common.Date(deal.ExpectedClose), Version: deal.Version,
	})
}

// ---------------------------------------------------------------------------
// Tasks and KPIs
// ---------------------------------------------------------------------------

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, crm.TaskSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.CRM.Tasks(r.Context(), params, optionalQuery(r, "status"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.Task, 0, len(rows))
	for _, row := range rows {
		t := api.Task{
			ID: row.ID.String(), Title: row.Title,
			AssigneeName: row.AssigneeName, DueOn: common.Date(row.DueOn),
			Status: taskStatus(row.Status), RelatedType: row.RelatedType,
			Version: row.Version,
		}
		if row.RelatedID.Valid {
			rid := row.RelatedID.UUID.String()
			t.RelatedID = &rid
		}
		out = append(out, t)
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.TaskWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	due, err := parseDate(deref(req.DueOn), "due_on", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	assignee, err := parseNullUUIDField(req.AssigneeID, "assignee_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	related, err := parseNullUUIDField(req.RelatedID, "related_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	task, err := s.svc.CRM.CreateTask(r.Context(), ident.User.ID, crm.TaskInput{
		Title: req.Title, AssigneeID: assignee, DueOn: due,
		RelatedType: deref(req.RelatedType), RelatedID: related,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusCreated, api.Task{
		ID: task.ID.String(), Title: task.Title, DueOn: common.Date(task.DueOn),
		Status: taskStatus(task.Status), Version: task.Version,
	})
}

func (s *Server) handleSetTaskStatus(w http.ResponseWriter, r *http.Request) {
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
	var req api.TaskStatusRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	task, err := s.svc.CRM.SetTaskStatus(r.Context(), ident.User.ID, id, req.Status)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, api.Task{
		ID: task.ID.String(), Title: task.Title, DueOn: common.Date(task.DueOn),
		Status: taskStatus(task.Status), Version: task.Version,
	})
}

func (s *Server) handleCRMKPIs(w http.ResponseWriter, r *http.Request) {
	kpis, err := s.svc.CRM.KPIs(r.Context())
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, api.CRMKPIs{
		NewLeads: int32(kpis.NewLeads), OpenDeals: int32(kpis.OpenDeals),
		Conversion: kpis.Conversion, OverdueTasks: kpis.OverdueTasks,
	})
}

// moneyOrNil renders a nullable amount as a string, never a float.
func moneyOrNil(d decimal.NullDecimal) *string {
	if !d.Valid {
		return nil
	}
	s := d.Decimal.StringFixed(2)
	return &s
}
