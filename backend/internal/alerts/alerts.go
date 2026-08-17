// Package alerts produces the notification feed and the sidebar count pills.
//
// docs/05-MODULES.md §17 lists ten triggers. Sorting them reveals two different
// species, and docs/07-IMPLEMENTATION-PLAN.md I15 treats them differently:
//
//	Discrete events (3)   — happen once, at a point in time. Persisted.
//	                        new inquiry · batch quarantined · batch rejected
//
//	Standing conditions (7) — a query result that stays true until someone fixes
//	                        it. Derived on read, never stored.
//	                        low stock · expiring batch · PO awaiting approval ·
//	                        overdue delivery · expiring document · expiring
//	                        contract · maintenance due
//
// Persisting a standing condition is a trap: stock dips below min_qty and you
// write a row — then do you write another tomorrow? What retracts it when a goods
// receipt fixes it? Any bug in that retraction logic shows the factory alarms
// about problems that were solved days ago. Deriving them makes them self-healing
// and removes the reconciliation entirely.
//
// The same queries drive the nav count pills (NAVCOUNT in the prototype), so the
// bell and the sidebar can never disagree.
package alerts

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Kind identifies a trigger from docs/05-MODULES.md §17.
type Kind string

const (
	// Standing conditions — derived.
	KindStockBelowMinimum Kind = "stock_below_minimum"
	KindBatchExpiring     Kind = "batch_expiring"
	KindPOAwaitingApprove Kind = "po_awaiting_approval"
	KindDeliveryOverdue   Kind = "delivery_overdue"
	KindDocumentExpiring  Kind = "document_expiring"
	KindContractExpiring  Kind = "contract_expiring"
	KindMaintenanceDue    Kind = "maintenance_due"

	// Discrete events — persisted.
	KindInquiryReceived  Kind = "inquiry_received"
	KindBatchQuarantined Kind = "batch_quarantined"
	KindBatchRejected    Kind = "batch_rejected"
)

// Alert is one entry in the feed.
type Alert struct {
	Kind Kind `json:"kind"`
	// Resource is the module key, used to filter by the viewer's permissions —
	// "users see only notifications for resources they can read"
	// (docs/05-MODULES.md:294).
	Resource   string        `json:"resource"`
	ResourceID uuid.NullUUID `json:"resource_id"`
	Level      common.Level  `json:"level"`
	// Label is the Russian fallback; the frontend renders from Kind via its i18n
	// dictionary, per docs/07-IMPLEMENTATION-PLAN.md C3.
	Label      string `json:"label"`
	OccurredAt string `json:"occurred_at,omitempty"`
}

// condition describes one derived standing condition.
type condition struct {
	kind     Kind
	resource string
	level    common.Level
	// module names the task that implements this condition's query. Until then
	// count() returns zero rather than failing — a module that does not exist yet
	// genuinely has nothing to alert about.
	module string
	count  func(context.Context, *pgxpool.Pool) (int, error)
}

// conditions is the full set of derived standing conditions, with the severity
// each carries per docs/05-MODULES.md §17. Queries are attached as their modules
// land; the shape is fixed here so the feed and the count pills are built once.
var conditions = []condition{
	{KindStockBelowMinimum, rbac.Inventory, common.LevelDanger, "T16", nil},
	{KindBatchExpiring, rbac.Inventory, common.LevelWarn, "T16", nil},
	{KindPOAwaitingApprove, rbac.Procurement, common.LevelWarn, "T19", nil},
	{KindDeliveryOverdue, rbac.Logistics, common.LevelDanger, "T21", nil},
	{KindDocumentExpiring, rbac.Documents, common.LevelWarn, "T22", nil},
	{KindContractExpiring, rbac.HR, common.LevelWarn, "T23", nil},
	{KindMaintenanceDue, rbac.Equipment, common.LevelWarn, "T24", nil},
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Counts returns the per-resource pending-work counts that drive the sidebar
// pills, filtered to what the viewer may read.
//
// A user who cannot read a module must not learn from a badge that it has three
// problems — the count is data about that module, and hiding the nav entry while
// leaking its count would defeat the point.
func (s *Service) Counts(ctx context.Context, perms rbac.Set) (map[string]int, error) {
	out := make(map[string]int)
	for _, c := range conditions {
		if !perms.CanRead(c.resource) {
			continue
		}
		if c.count == nil {
			continue // module not built yet; genuinely nothing to report
		}
		n, err := c.count(ctx, s.pool)
		if err != nil {
			return nil, err
		}
		out[c.resource] += n
	}
	return out, nil
}

// Pending reports which conditions still have no query attached, so the gap is
// visible in a test rather than discovered as a silently empty feed.
func Pending() map[Kind]string {
	out := make(map[Kind]string)
	for _, c := range conditions {
		if c.count == nil {
			out[c.kind] = c.module
		}
	}
	return out
}

// Kinds returns every trigger this package knows about, derived and persisted.
func Kinds() []Kind {
	return []Kind{
		KindStockBelowMinimum, KindBatchExpiring, KindPOAwaitingApprove,
		KindDeliveryOverdue, KindDocumentExpiring, KindContractExpiring,
		KindMaintenanceDue, KindInquiryReceived, KindBatchQuarantined, KindBatchRejected,
	}
}
