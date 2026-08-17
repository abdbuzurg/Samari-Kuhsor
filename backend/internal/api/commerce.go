package api

// Wire types for Закупки, Продажи, Логистика, Интеграция с сайтом and
// Администрирование.

// ---------------------------------------------------------------------------
// Закупки
// ---------------------------------------------------------------------------

type Supplier struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	TaxID   *string `json:"tax_id" tstype:"string | null"`
	Contact *string `json:"contact" tstype:"string | null"`
	Region  *string `json:"region" tstype:"string | null"`
	Rating  *int32  `json:"rating" tstype:"number | null"`
	Version int32   `json:"version"`
}

type SupplierWriteRequest struct {
	Name    string  `json:"name"`
	TaxID   *string `json:"tax_id"`
	Contact *string `json:"contact"`
	Region  *string `json:"region"`
	Rating  *int32  `json:"rating"`
}

type PurchaseOrderRow struct {
	ID           string  `json:"id"`
	PONo         string  `json:"po_no"`
	SupplierID   string  `json:"supplier_id"`
	SupplierName string  `json:"supplier_name"`
	ExpectedAt   *string `json:"expected_at" tstype:"string | null"`
	Total        string  `json:"total"`
	Status       Status  `json:"status"`
	Version      int32   `json:"version"`
	CreatedAt    string  `json:"created_at"`
}

// PurchaseOrder is the detail payload. Fields repeated rather than embedded —
// see the note on ManufacturingOrder.
type PurchaseOrder struct {
	ID           string  `json:"id"`
	PONo         string  `json:"po_no"`
	SupplierID   string  `json:"supplier_id"`
	SupplierName string  `json:"supplier_name"`
	ExpectedAt   *string `json:"expected_at" tstype:"string | null"`
	Total        string  `json:"total"`
	Status       Status  `json:"status"`
	Version      int32   `json:"version"`
	CreatedAt    string  `json:"created_at"`

	Lines []PurchaseOrderLine `json:"lines"`
	// AllowedTransitions is what this actor may move the order to next, given the
	// current status and their procurement:approve grant.
	AllowedTransitions []string `json:"allowed_transitions"`
}

type PurchaseOrderLine struct {
	ID          string `json:"id"`
	ItemID      string `json:"item_id"`
	SKU         string `json:"sku"`
	ItemName    string `json:"item_name"`
	Qty         string `json:"qty"`
	ReceivedQty string `json:"received_qty"`
	UnitPrice   string `json:"unit_price"`
	LineTotal   string `json:"line_total"`
}

type PurchaseOrderLineWrite struct {
	ItemID    string `json:"item_id"`
	Qty       string `json:"qty"`
	UnitPrice string `json:"unit_price"`
}

type PurchaseOrderWriteRequest struct {
	PONo       string                   `json:"po_no"`
	SupplierID string                   `json:"supplier_id"`
	ExpectedAt *string                  `json:"expected_at"`
	Lines      []PurchaseOrderLineWrite `json:"lines"`
}

// TransitionRequest moves a document to a new status. Shared by purchase orders
// and sales orders, which have different matrices but the same request shape.
type TransitionRequest struct {
	To string `json:"to"`
}

type GoodsReceiptLineWrite struct {
	POLineID string  `json:"po_line_id"`
	Qty      string  `json:"qty"`
	BatchID  *string `json:"batch_id"`
}

type GoodsReceiptRequest struct {
	LocationID string                  `json:"location_id"`
	Note       *string                 `json:"note"`
	Lines      []GoodsReceiptLineWrite `json:"lines"`
}

type GoodsReceipt struct {
	ID         string `json:"id"`
	POID       string `json:"po_id"`
	ReceivedAt string `json:"received_at"`
}

// ---------------------------------------------------------------------------
// Продажи
// ---------------------------------------------------------------------------

type SalesOrderRow struct {
	ID           string  `json:"id"`
	SONo         string  `json:"so_no"`
	CustomerID   string  `json:"customer_id"`
	CustomerName string  `json:"customer_name"`
	OrderedOn    *string `json:"ordered_on" tstype:"string | null"`
	Total        string  `json:"total"`
	Status       Status  `json:"status"`
	Version      int32   `json:"version"`
	CreatedAt    string  `json:"created_at"`
}

// SalesOrder is the detail payload. Fields repeated rather than embedded — see
// the note on ManufacturingOrder.
type SalesOrder struct {
	ID           string           `json:"id"`
	SONo         string           `json:"so_no"`
	CustomerID   string           `json:"customer_id"`
	CustomerName string           `json:"customer_name"`
	OrderedOn    *string          `json:"ordered_on" tstype:"string | null"`
	Total        string           `json:"total"`
	Status       Status           `json:"status"`
	Version      int32            `json:"version"`
	CreatedAt    string           `json:"created_at"`
	Lines        []SalesOrderLine `json:"lines"`
}

type SalesOrderLine struct {
	ID        string  `json:"id"`
	ItemID    string  `json:"item_id"`
	SKU       string  `json:"sku"`
	ItemName  string  `json:"item_name"`
	BatchID   *string `json:"batch_id" tstype:"string | null"`
	BatchNo   *string `json:"batch_no" tstype:"string | null"`
	Qty       string  `json:"qty"`
	UnitPrice string  `json:"unit_price"`
	LineTotal string  `json:"line_total"`
}

type SalesOrderLineWrite struct {
	ItemID    string  `json:"item_id"`
	BatchID   *string `json:"batch_id"`
	Qty       string  `json:"qty"`
	UnitPrice string  `json:"unit_price"`
}

type SalesOrderWriteRequest struct {
	SONo       string                `json:"so_no"`
	CustomerID string                `json:"customer_id"`
	OrderedOn  *string               `json:"ordered_on"`
	Lines      []SalesOrderLineWrite `json:"lines"`
}

// ConfirmOrderRequest names the location the stock leaves from. Confirmation is
// the moment the company commits, so it is also the moment the released-batch
// rule and the stock check apply (docs/05-MODULES.md §9).
type ConfirmOrderRequest struct {
	LocationID string `json:"location_id"`
}

// ---------------------------------------------------------------------------
// Логистика
// ---------------------------------------------------------------------------

type Shipment struct {
	ID            string         `json:"id"`
	TripNo        string         `json:"trip_no"`
	RouteFrom     *string        `json:"route_from" tstype:"string | null"`
	RouteTo       *string        `json:"route_to" tstype:"string | null"`
	DriverID      *string        `json:"driver_id" tstype:"string | null"`
	DriverName    *string        `json:"driver_name" tstype:"string | null"`
	VehicleID     *string        `json:"vehicle_id" tstype:"string | null"`
	VehiclePlate  *string        `json:"vehicle_plate" tstype:"string | null"`
	TransportCost *string        `json:"transport_cost" tstype:"string | null"`
	Status        Status         `json:"status"`
	Version       int32          `json:"version"`
	CreatedAt     string         `json:"created_at"`
	Lines         []ShipmentLine `json:"lines"`
}

type ShipmentLine struct {
	ID       string `json:"id"`
	ItemID   string `json:"item_id"`
	SKU      string `json:"sku"`
	ItemName string `json:"item_name"`
	BatchID  string `json:"batch_id"`
	BatchNo  string `json:"batch_no"`
	Qty      string `json:"qty"`
}

type ShipmentWriteRequest struct {
	TripNo        string  `json:"trip_no"`
	RouteFrom     *string `json:"route_from"`
	RouteTo       *string `json:"route_to"`
	DriverID      *string `json:"driver_id"`
	VehicleID     *string `json:"vehicle_id"`
	TransportCost *string `json:"transport_cost"`
}

// ShipmentLoadRequest loads one batch onto a trip. The batch must be released —
// checked in Go, because a lorry leaving with quarantined product is the failure
// this whole chain exists to prevent.
type ShipmentLoadRequest struct {
	ItemID  string `json:"item_id"`
	BatchID string `json:"batch_id"`
	Qty     string `json:"qty"`
}

// ---------------------------------------------------------------------------
// Интеграция с сайтом
// ---------------------------------------------------------------------------

type Inquiry struct {
	ID          string  `json:"id"`
	ReferenceNo string  `json:"reference_no"`
	Type        Status  `json:"type"`
	Name        string  `json:"name"`
	Company     *string `json:"company" tstype:"string | null"`
	Contact     string  `json:"contact"`
	Message     *string `json:"message" tstype:"string | null"`
	BatchID     *string `json:"batch_id" tstype:"string | null"`
	BatchNo     *string `json:"batch_no" tstype:"string | null"`
	Status      Status  `json:"status"`
	SubmittedAt string  `json:"submitted_at"`
	Version     int32   `json:"version"`
}

// InquirySubmitRequest is the only request shape an unauthenticated caller can
// send. Everything here is untrusted: it is length-bounded, type-checked and
// rate-limited by IP before a row is written.
type InquirySubmitRequest struct {
	Type    string  `json:"type"`
	Name    string  `json:"name"`
	Company *string `json:"company"`
	Contact string  `json:"contact"`
	Message *string `json:"message"`
	BatchID *string `json:"batch_id"`
}

// InquiryReceipt is what the website shows the visitor. Deliberately minimal: the
// reference number and nothing that could confirm what else is in the database.
type InquiryReceipt struct {
	ReferenceNo string `json:"reference_no"`
}

type Lead struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Source *string `json:"source" tstype:"string | null"`
	Status Status  `json:"status"`
}

// ---------------------------------------------------------------------------
// Администрирование
// ---------------------------------------------------------------------------

type RoleDetail struct {
	ID          string   `json:"id"`
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	UserCount   int64    `json:"user_count"`
	Version     int32    `json:"version"`
}

type RoleWriteRequest struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type RolePermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

type UserRolesRequest struct {
	RoleIDs []string `json:"role_ids"`
}

type UserActiveRequest struct {
	Active bool `json:"active"`
}

type AdminUserRow struct {
	ID       string   `json:"id"`
	Email    string   `json:"email"`
	FullName string   `json:"full_name"`
	IsActive bool     `json:"is_active"`
	Status   Status   `json:"status"`
	Roles    []string `json:"roles"`
	Version  int32    `json:"version"`
}

// PermissionCatalogue is every resource:action the system recognises, so the role
// editor is generated from the enforced list rather than a hand-kept copy that
// could drift from it.
type PermissionCatalogue struct {
	Resources []PermissionResource `json:"resources"`
}

type PermissionResource struct {
	Key     string   `json:"key"`
	Actions []string `json:"actions"`
}

// ---------------------------------------------------------------------------
// Уведомления
// ---------------------------------------------------------------------------

type Notification struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	Resource   string  `json:"resource"`
	ResourceID *string `json:"resource_id" tstype:"string | null"`
	Level      string  `json:"level"`
	Title      string  `json:"title"`
	Body       *string `json:"body" tstype:"string | null"`
	OccurredAt string  `json:"occurred_at"`
	IsRead     bool    `json:"is_read"`
}

// AlertFeed is the bell: persisted events plus the live standing conditions, and
// the counts that drive the sidebar pills. Both come from one endpoint so the
// bell and the sidebar cannot disagree.
type AlertFeed struct {
	Notifications []Notification   `json:"notifications"`
	Conditions    []AlertCondition `json:"conditions"`
	Unread        int              `json:"unread"`
	// Counts is resource → number of open items, for the nav pills.
	Counts map[string]int `json:"counts"`
}

type AlertCondition struct {
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
	Level    string `json:"level"`
	Count    int    `json:"count"`
}

// ---------------------------------------------------------------------------
// Персонал, Оборудование и ТО, Документы
// ---------------------------------------------------------------------------

type Employee struct {
	ID            string  `json:"id"`
	FullName      string  `json:"full_name"`
	PositionID    *string `json:"position_id" tstype:"string | null"`
	PositionTitle *string `json:"position_title" tstype:"string | null"`
	Department    *string `json:"department" tstype:"string | null"`
	Shift         *string `json:"shift" tstype:"string | null"`
	HiredOn       *string `json:"hired_on" tstype:"string | null"`
	ContractUntil *string `json:"contract_until" tstype:"string | null"`
	Status        Status  `json:"status"`
	Version       int32   `json:"version"`
}

// Version is a POINTER on every write request: an absent version must be a
// validation error, not a silent zero that overwrites whatever the record now
// holds. See common.RequireVersion.
type EmployeeWriteRequest struct {
	FullName      string  `json:"full_name"`
	PositionID    *string `json:"position_id"`
	Shift         *string `json:"shift"`
	HiredOn       *string `json:"hired_on"`
	ContractUntil *string `json:"contract_until"`
	Status        string  `json:"status"`
	Version       *int32  `json:"version"`
}

type Asset struct {
	ID             string  `json:"id"`
	AssetNo        string  `json:"asset_no"`
	Name           string  `json:"name"`
	AssetType      *string `json:"asset_type" tstype:"string | null"`
	Line           *string `json:"line" tstype:"string | null"`
	CommissionedOn *string `json:"commissioned_on" tstype:"string | null"`
	WarrantyUntil  *string `json:"warranty_until" tstype:"string | null"`
	NextDueOn      *string `json:"next_due_on" tstype:"string | null"`
	LastServiceAt  *string `json:"last_service_at" tstype:"string | null"`
	Status         Status  `json:"status"`
	Version        int32   `json:"version"`
}

type AssetWriteRequest struct {
	AssetNo        string  `json:"asset_no"`
	Name           string  `json:"name"`
	AssetType      *string `json:"asset_type"`
	Line           *string `json:"line"`
	CommissionedOn *string `json:"commissioned_on"`
	WarrantyUntil  *string `json:"warranty_until"`
	Status         string  `json:"status"`
	Version        *int32  `json:"version"`
}

type MaintenanceEvent struct {
	ID          string  `json:"id"`
	AssetID     string  `json:"asset_id"`
	EventType   *string `json:"event_type" tstype:"string | null"`
	PerformedAt *string `json:"performed_at" tstype:"string | null"`
	NextDueOn   *string `json:"next_due_on" tstype:"string | null"`
	Notes       *string `json:"notes" tstype:"string | null"`
}

type MaintenanceWriteRequest struct {
	EventType   *string `json:"event_type"`
	PerformedAt *string `json:"performed_at"`
	NextDueOn   *string `json:"next_due_on"`
	Notes       *string `json:"notes"`
}

type Document struct {
	ID         string  `json:"id"`
	DocNo      string  `json:"doc_no"`
	Title      string  `json:"title"`
	DocType    *string `json:"doc_type" tstype:"string | null"`
	OwnerID    *string `json:"owner_id" tstype:"string | null"`
	OwnerName  *string `json:"owner_name" tstype:"string | null"`
	ValidUntil *string `json:"valid_until" tstype:"string | null"`
	Status     Status  `json:"status"`
	Version    int32   `json:"version"`
	// AllowedTransitions is what this actor may do next. Activation needs
	// documents:approve, so the button set differs per user.
	AllowedTransitions []string `json:"allowed_transitions"`
}

type DocumentWriteRequest struct {
	DocNo      string  `json:"doc_no"`
	Title      string  `json:"title"`
	DocType    *string `json:"doc_type"`
	OwnerID    *string `json:"owner_id"`
	ValidUntil *string `json:"valid_until"`
	Version    *int32  `json:"version"`
}

// ---------------------------------------------------------------------------
// Панель управления
// ---------------------------------------------------------------------------

// Dashboard is the landing page.
//
// Every panel is a POINTER, and null means "you may not read this module" — not
// "this figure is zero". The two look identical rendered and mean completely
// different things, so the distinction is carried on the wire rather than
// flattened to 0 (docs/05-MODULES.md §2).
type Dashboard struct {
	Period     string               `json:"period"`
	Sales      *DashboardSales      `json:"sales" tstype:"DashboardSales | null"`
	Stock      *DashboardStock      `json:"stock" tstype:"DashboardStock | null"`
	Quality    *DashboardQuality    `json:"quality" tstype:"DashboardQuality | null"`
	Production *DashboardProduction `json:"production" tstype:"DashboardProduction | null"`
	Pipeline   []DashboardStage     `json:"pipeline"`
	Recent     []DashboardOrder     `json:"recent_orders"`
	Feed       []DashboardEvent     `json:"feed"`
	Revenue    []DashboardRevenue   `json:"revenue"`
}

type DashboardSales struct {
	Revenue    string `json:"revenue"`
	OrderCount int    `json:"order_count"`
	OpenOrders int    `json:"open_orders"`
	OverduePOs int    `json:"overdue_purchase_orders"`
}

type DashboardStock struct {
	Value        string `json:"value"`
	BelowMinimum int    `json:"below_minimum"`
}

type DashboardQuality struct {
	Quarantined int `json:"quarantined"`
}

type DashboardProduction struct {
	GoodQty    string `json:"good_qty"`
	ScrapQty   string `json:"scrap_qty"`
	PlannedQty string `json:"planned_qty"`
	// Progress is good ÷ planned as a whole percentage, capped at 100 for display.
	Progress int `json:"progress"`
}

type DashboardStage struct {
	Stage  Status `json:"stage"`
	Count  int    `json:"count"`
	Amount string `json:"amount"`
}

type DashboardOrder struct {
	ID           string `json:"id"`
	SONo         string `json:"so_no"`
	CustomerName string `json:"customer_name"`
	Total        string `json:"total"`
	Status       Status `json:"status"`
	CreatedAt    string `json:"created_at"`
}

// DashboardEvent is one line of Лента событий, read from the audit log.
//
// `action` and `resource` are keys, not sentences: the frontend renders them
// through its own dictionary in the reader's locale (docs/07 C3).
type DashboardEvent struct {
	ID         string `json:"id"`
	Action     string `json:"action"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
	ActorName  string `json:"actor_name"`
	OccurredAt string `json:"occurred_at"`
}

type DashboardRevenue struct {
	Day        string `json:"day"`
	Revenue    string `json:"revenue"`
	OrderCount int    `json:"order_count"`
}
