package api

// CRM и продажи — the customer side of the module (docs/05-MODULES.md:179).
//
// Sales orders and shipments live in commerce.go: this file is the pipeline in
// front of an order, not the order itself.

// CustomerRow is one row of the Клиенты register.
//
// OpenDeals and OpenAmount are computed at read time. There is no column behind
// either, for the same reason there is no stored stock balance: a cached count
// that disagrees with the deals table is worse than no count at all.
type CustomerRow struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	CustomerType *string `json:"customer_type" tstype:"string | null"`
	Region       *string `json:"region" tstype:"string | null"`
	Contact      *string `json:"contact" tstype:"string | null"`
	OpenDeals    int32   `json:"open_deals"`
	OpenAmount   string  `json:"open_amount"`
	LeadStatus   *Status `json:"lead_status" tstype:"Status | null"`
	Version      int32   `json:"version"`
}

// Customer is the detail payload: the header the related bands hang from.
type Customer struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	CustomerType *string `json:"customer_type" tstype:"string | null"`
	Region       *string `json:"region" tstype:"string | null"`
	Contact      *string `json:"contact" tstype:"string | null"`
	Version      int32   `json:"version"`
	CreatedAt    string  `json:"created_at"`
}

type CustomerWriteRequest struct {
	Name         string  `json:"name"`
	CustomerType *string `json:"customer_type"`
	Region       *string `json:"region"`
	Contact      *string `json:"contact"`
	Version      *int32  `json:"version"`
}

type Contact struct {
	ID       string  `json:"id"`
	FullName string  `json:"full_name"`
	Role     *string `json:"role" tstype:"string | null"`
	Email    *string `json:"email" tstype:"string | null"`
	Phone    *string `json:"phone" tstype:"string | null"`
}

type ContactWriteRequest struct {
	FullName string  `json:"full_name"`
	Role     *string `json:"role"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
}

// CustomerLead is a lead as the customer's detail view shows it. Distinct from
// api.Lead in commerce.go, which is the flatter shape the enquiry conversion
// returns and which predates this module having a screen.
type CustomerLead struct {
	ID          string  `json:"id"`
	Source      *string `json:"source" tstype:"string | null"`
	InquiryID   *string `json:"inquiry_id" tstype:"string | null"`
	Status      Status  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	ReferenceNo *string `json:"reference_no" tstype:"string | null"`
}

// DealRow is one row of the Сделки pipeline.
type DealRow struct {
	ID            string  `json:"id"`
	CustomerID    string  `json:"customer_id"`
	CustomerName  string  `json:"customer_name"`
	Region        *string `json:"region" tstype:"string | null"`
	Amount        *string `json:"amount" tstype:"string | null"`
	Stage         Status  `json:"stage"`
	OwnerName     *string `json:"owner_name" tstype:"string | null"`
	ExpectedClose *string `json:"expected_close" tstype:"string | null"`
	Version       int32   `json:"version"`
}

// Deal is the detail payload, with the stage history that makes the pipeline
// auditable rather than merely current.
type Deal struct {
	ID            string           `json:"id"`
	CustomerID    string           `json:"customer_id"`
	CustomerName  string           `json:"customer_name"`
	Amount        *string          `json:"amount" tstype:"string | null"`
	Stage         Status           `json:"stage"`
	OwnerName     *string          `json:"owner_name" tstype:"string | null"`
	ExpectedClose *string          `json:"expected_close" tstype:"string | null"`
	Version       int32            `json:"version"`
	CreatedAt     string           `json:"created_at"`
	History       []DealStageEvent `json:"history"`
	// AllowedTransitions is what this actor may move the deal to next, computed
	// from the same matrix the domain enforces. Won and lost are terminal, so a
	// closed deal returns an empty list.
	AllowedTransitions []string `json:"allowed_transitions"`
}

// DealStageEvent is immutable evidence: who moved it, when, and why. No version
// and no deleted_at, exactly like BatchStatusEvent.
type DealStageEvent struct {
	ID         string  `json:"id"`
	FromStage  *Status `json:"from_stage" tstype:"Status | null"`
	ToStage    Status  `json:"to_stage"`
	OccurredAt string  `json:"occurred_at"`
	ChangedBy  *string `json:"changed_by" tstype:"string | null"`
	Note       *string `json:"note" tstype:"string | null"`
}

type DealWriteRequest struct {
	CustomerID    string  `json:"customer_id"`
	Amount        *string `json:"amount"`
	Stage         *string `json:"stage"`
	OwnerID       *string `json:"owner_id"`
	ExpectedClose *string `json:"expected_close"`
}

// DealStageRequest moves a deal. `to` is the destination; legality is decided in
// Go from the same matrix that computes AllowedTransitions.
type DealStageRequest struct {
	To   string  `json:"to"`
	Note *string `json:"note"`
}

type Task struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	AssigneeName *string `json:"assignee_name" tstype:"string | null"`
	DueOn        *string `json:"due_on" tstype:"string | null"`
	Status       Status  `json:"status"`
	RelatedType  *string `json:"related_type" tstype:"string | null"`
	RelatedID    *string `json:"related_id" tstype:"string | null"`
	Version      int32   `json:"version"`
}

type TaskWriteRequest struct {
	Title       string  `json:"title"`
	AssigneeID  *string `json:"assignee_id"`
	DueOn       *string `json:"due_on"`
	RelatedType *string `json:"related_type"`
	RelatedID   *string `json:"related_id"`
}

type TaskStatusRequest struct {
	Status string `json:"status"`
}

// CustomerDetail bundles the header with every band the detail view shows
// (docs/05-MODULES.md:179): "customer header · contacts · deals with stage
// history · linked inquiries · orders · activity".
type CustomerDetail struct {
	Customer  Customer          `json:"customer"`
	Contacts  []Contact         `json:"contacts"`
	Leads     []CustomerLead    `json:"leads"`
	Deals     []DealRow         `json:"deals"`
	Orders    []CustomerOrder   `json:"orders"`
	Inquiries []CustomerInquiry `json:"inquiries"`
}

type CustomerOrder struct {
	ID        string  `json:"id"`
	SONo      string  `json:"so_no"`
	OrderedOn *string `json:"ordered_on" tstype:"string | null"`
	Total     string  `json:"total"`
	Status    Status  `json:"status"`
}

type CustomerInquiry struct {
	ID          string `json:"id"`
	ReferenceNo string `json:"reference_no"`
	Type        Status `json:"type"`
	Status      Status `json:"status"`
	SubmittedAt string `json:"submitted_at"`
}

// CRMKPIs are the four figures specified at docs/05-MODULES.md:179.
type CRMKPIs struct {
	NewLeads  int32 `json:"new_leads"`
	OpenDeals int32 `json:"open_deals"`
	// Conversion is null before anything has closed, never "0".
	Conversion   *string `json:"conversion" tstype:"string | null"`
	OverdueTasks int32   `json:"overdue_tasks"`
}

// ---------------------------------------------------------------------------
// Аналитика сайта — docs/01-DECISIONS.md D12
// ---------------------------------------------------------------------------

// AnalyticsEventRequest is one thing that happened in a browser.
//
// Every field is untrusted. Kind, source, category and locale are checked
// against fixed sets and the SKU against the real catalogue; anything else is
// dropped silently, because validation feedback on an anonymous endpoint is a
// probing oracle.
type AnalyticsEventRequest struct {
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Source   string `json:"source,omitempty"`
	Category string `json:"category,omitempty"`
	Locale   string `json:"locale"`
	SKU      string `json:"sku,omitempty"`
}

// AnalyticsBatchRequest is one flush from one browser. The client buffers and
// sends via navigator.sendBeacon, so batches arrive rather than single events.
type AnalyticsBatchRequest struct {
	// SessionID is generated in the browser, held in sessionStorage and gone
	// when the tab closes. Never persisted across visits.
	SessionID string                  `json:"session_id"`
	Events    []AnalyticsEventRequest `json:"events"`
}

// AnalyticsProductRow is one line of «Что смотрят на сайте».
type AnalyticsProductRow struct {
	SKU  string `json:"sku"`
	Name string `json:"name"`
	// Visits is distinct sessions — ten views in one sitting count once.
	Visits int32 `json:"visits"`
	Views  int32 `json:"views"`
}

// AnalyticsLinkRow is one line of «Популярные ссылки». Only `cta` and
// `outbound` reach the panel.
type AnalyticsLinkRow struct {
	Target   string `json:"target"`
	Category string `json:"category"`
	Visits   int32  `json:"visits"`
	Clicks   int32  `json:"clicks"`
}

// AnalyticsReport is both panels for one period.
type AnalyticsReport struct {
	Period string `json:"period"`
	// Visits counts consented sessions, NOT raw traffic: events fire only after
	// the banner is accepted, so crawlers are excluded by construction and these
	// figures read lower than a server log would.
	Visits        int32                 `json:"visits"`
	ProductVisits int32                 `json:"product_visits"`
	Products      []AnalyticsProductRow `json:"products"`
	Links         []AnalyticsLinkRow    `json:"links"`
}
