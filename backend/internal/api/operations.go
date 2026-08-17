package api

// Wire types for the operating chain: Склад, Производство, Качество.
//
// Every quantity is a string, not a number: JSON numbers are IEEE-754 doubles and
// numeric(14,3) does not survive the round trip (docs/03-API-CONTRACT.md §6).
// Money is the same, and for the same reason.
//
// Response DTOs annotate nullable fields `tstype:"X | null"`. Without it tygo
// emits `field?: T` for a Go pointer, and the API sends `"field": null` — the
// distinction TypeScript actually cares about.

// ---------------------------------------------------------------------------
// Склад и запасы
// ---------------------------------------------------------------------------

type Location struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
	Zone string `json:"zone"`
}

// StockBalanceRow is one position in the warehouse list. `OnHand` is a SUM
// computed at read time; there is no column behind it (CLAUDE.md §4.2).
type StockBalanceRow struct {
	ItemID         string  `json:"item_id"`
	SKU            string  `json:"sku"`
	ItemName       string  `json:"item_name"`
	BaseUOM        string  `json:"base_uom"`
	BatchID        *string `json:"batch_id" tstype:"string | null"`
	BatchNo        *string `json:"batch_no" tstype:"string | null"`
	BatchStatus    *Status `json:"batch_status" tstype:"Status | null"`
	ExpiresOn      *string `json:"expires_on" tstype:"string | null"`
	LocationID     string  `json:"location_id"`
	LocationCode   string  `json:"location_code"`
	LocationZone   string  `json:"location_zone"`
	OnHand         string  `json:"on_hand"`
	MinQty         *string `json:"min_qty" tstype:"string | null"`
	Status         Status  `json:"status"`
	LastMovementAt *string `json:"last_movement_at" tstype:"string | null"`
}

// StockMovementRow is one line of the ledger, with the balance after it.
type StockMovementRow struct {
	ID             string  `json:"id"`
	OccurredAt     string  `json:"occurred_at"`
	QtyDelta       string  `json:"qty_delta"`
	RunningBalance string  `json:"running_balance"`
	Reason         Status  `json:"reason"`
	RefType        *string `json:"ref_type" tstype:"string | null"`
	RefID          *string `json:"ref_id" tstype:"string | null"`
	Note           *string `json:"note" tstype:"string | null"`
	CreatedBy      *string `json:"created_by" tstype:"string | null"`
}

type MovementWriteRequest struct {
	ItemID     string  `json:"item_id"`
	BatchID    *string `json:"batch_id"`
	LocationID string  `json:"location_id"`
	QtyDelta   string  `json:"qty_delta"`
	Reason     string  `json:"reason"`
	Note       *string `json:"note"`
}

type TransferRequest struct {
	ItemID       string  `json:"item_id"`
	BatchID      *string `json:"batch_id"`
	FromLocation string  `json:"from_location_id"`
	ToLocation   string  `json:"to_location_id"`
	Qty          string  `json:"qty"`
	Note         *string `json:"note"`
}

// ---------------------------------------------------------------------------
// Производство
// ---------------------------------------------------------------------------

type ManufacturingOrderRow struct {
	ID           string  `json:"id"`
	MONo         string  `json:"mo_no"`
	ItemID       string  `json:"item_id"`
	SKU          string  `json:"sku"`
	ItemName     string  `json:"item_name"`
	BatchID      *string `json:"batch_id" tstype:"string | null"`
	BatchNo      *string `json:"batch_no" tstype:"string | null"`
	Line         *string `json:"line" tstype:"string | null"`
	ScheduledFor *string `json:"scheduled_for" tstype:"string | null"`
	PlannedQty   string  `json:"planned_qty"`
	GoodQty      string  `json:"good_qty"`
	ScrapQty     string  `json:"scrap_qty"`
	Status       Status  `json:"status"`
	Version      int32   `json:"version"`
	CreatedAt    string  `json:"created_at"`
}

// ManufacturingOrder is the detail payload.
//
// The list row's fields are repeated rather than embedded. Go's encoding/json
// FLATTENS an embedded struct, but tygo emits it as a nested field — so an
// embedded row would generate TypeScript describing a shape the API never sends,
// which is exactly the silent Go/TS drift the generated types exist to prevent.
type ManufacturingOrder struct {
	ID           string  `json:"id"`
	MONo         string  `json:"mo_no"`
	ItemID       string  `json:"item_id"`
	SKU          string  `json:"sku"`
	ItemName     string  `json:"item_name"`
	BatchID      *string `json:"batch_id" tstype:"string | null"`
	BatchNo      *string `json:"batch_no" tstype:"string | null"`
	Line         *string `json:"line" tstype:"string | null"`
	ScheduledFor *string `json:"scheduled_for" tstype:"string | null"`
	PlannedQty   string  `json:"planned_qty"`
	GoodQty      string  `json:"good_qty"`
	ScrapQty     string  `json:"scrap_qty"`
	Status       Status  `json:"status"`
	Version      int32   `json:"version"`
	CreatedAt    string  `json:"created_at"`

	// Progress is good ÷ planned as a whole percentage, capped at 100 for display.
	// Computed here so the two frontends cannot disagree about rounding.
	Progress int `json:"progress"`
	// Yield is good ÷ (good + scrap). Null rather than 0 when nothing has run:
	// "0% yield" and "not started" read very differently on a shift report.
	YieldPercent *string           `json:"yield_percent" tstype:"string | null"`
	DowntimeMin  int64             `json:"downtime_min"`
	Entries      []ProductionEntry `json:"entries"`
}

type ProductionEntry struct {
	ID          string  `json:"id"`
	RecordedAt  string  `json:"recorded_at"`
	GoodQty     string  `json:"good_qty"`
	ScrapQty    string  `json:"scrap_qty"`
	DowntimeMin int32   `json:"downtime_min"`
	Note        *string `json:"note" tstype:"string | null"`
	RecordedBy  *string `json:"recorded_by" tstype:"string | null"`
}

type ManufacturingOrderWriteRequest struct {
	MONo         string  `json:"mo_no"`
	ItemID       string  `json:"item_id"`
	BatchNo      string  `json:"batch_no"`
	PlannedQty   string  `json:"planned_qty"`
	ScheduledFor *string `json:"scheduled_for"`
	Line         *string `json:"line"`
}

type ProductionEntryWriteRequest struct {
	GoodQty     string  `json:"good_qty"`
	ScrapQty    *string `json:"scrap_qty"`
	DowntimeMin *int32  `json:"downtime_min"`
	Note        *string `json:"note"`
}

// ---------------------------------------------------------------------------
// Качество
// ---------------------------------------------------------------------------

type QualityTest struct {
	ID          string  `json:"id"`
	BatchID     string  `json:"batch_id"`
	TestType    string  `json:"test_type"`
	Result      Status  `json:"result"`
	ResultValue *string `json:"result_value" tstype:"string | null"`
	TestedAt    string  `json:"tested_at"`
	InspectorID *string `json:"inspector_id" tstype:"string | null"`
	Inspector   *string `json:"inspector" tstype:"string | null"`
	Notes       *string `json:"notes" tstype:"string | null"`
}

type QualityTestWriteRequest struct {
	TestType    string  `json:"test_type"`
	ResultValue *string `json:"result_value"`
	Passed      *bool   `json:"passed"`
	Notes       *string `json:"notes"`
}

// BatchListRow is one row of the Качество list.
//
// TestCount and FailedCount are carried so the list can show at a glance which
// batches have been examined and which failed — the two facts a QC lead scans
// for before opening anything.
type BatchListRow struct {
	ID          string  `json:"id"`
	BatchNo     string  `json:"batch_no"`
	ItemID      string  `json:"item_id"`
	SKU         string  `json:"sku"`
	ItemName    string  `json:"item_name"`
	ProducedOn  *string `json:"produced_on" tstype:"string | null"`
	ExpiresOn   *string `json:"expires_on" tstype:"string | null"`
	TestCount   int32   `json:"test_count"`
	FailedCount int32   `json:"failed_count"`
	Status      Status  `json:"status"`
	Version     int32   `json:"version"`
}

// BatchStatusEvent is immutable evidence: who decided, when, and why. It has no
// version and no deleted_at, so there is nothing here to edit.
type BatchStatusEvent struct {
	ID          string  `json:"id"`
	FromStatus  *Status `json:"from_status" tstype:"Status | null"`
	ToStatus    Status  `json:"to_status"`
	OccurredAt  string  `json:"occurred_at"`
	DecidedBy   string  `json:"decided_by"`
	DeciderName *string `json:"decider_name" tstype:"string | null"`
	Reason      *string `json:"reason" tstype:"string | null"`
}

// BatchTransitionRequest moves a batch. `to` is the destination status; the
// legality of the move and whether it needs quality:approve are decided in Go.
type BatchTransitionRequest struct {
	To     string  `json:"to"`
	Reason *string `json:"reason"`
}

// BatchDetail is the traceability view: the batch, its tests, its status history
// and where its stock currently sits.
type BatchDetail struct {
	Batch    Batch              `json:"batch"`
	SKU      string             `json:"sku"`
	ItemName string             `json:"item_name"`
	Tests    []QualityTest      `json:"tests"`
	History  []BatchStatusEvent `json:"history"`
	Stock    []StockBalanceRow  `json:"stock"`
	// AllowedTransitions is what this user may do next, given the current status
	// and their permissions. The frontend renders buttons from it rather than
	// re-implementing the transition matrix (docs/04-RBAC.md — hiding is not
	// enforcement, but showing an impossible button is a bug too).
	AllowedTransitions []string `json:"allowed_transitions"`
}
