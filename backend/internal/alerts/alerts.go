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
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/db"
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
	// Count is how many items currently satisfy a standing condition. Zero for a
	// discrete event, which is one thing that happened rather than a tally.
	Count int `json:"count,omitempty"`
}

// condition describes one derived standing condition.
type condition struct {
	kind     Kind
	resource string
	level    common.Level
	// module names the task that implemented this condition's query. Kept as
	// provenance now that all seven are attached; Pending() asserts none is nil.
	module string
	count  func(context.Context, *pgxpool.Pool) (int, error)
}

// ExpiryWindow is the horizon for the "expiring soon" conditions
// (docs/05-MODULES.md §17 says 30 days for batches, documents and contracts).
const ExpiryWindow = 30

// conditions is the full set of derived standing conditions, with the severity
// each carries per docs/05-MODULES.md §17.
//
// Every count() is a live query. None of these is stored, so a resolved condition
// disappears the moment the underlying data changes — no retraction logic, and no
// possibility of alarming the factory about a problem solved days ago.
var conditions = []condition{
	{KindStockBelowMinimum, rbac.Inventory, common.LevelDanger, "T16",
		func(ctx context.Context, p *pgxpool.Pool) (int, error) {
			n, err := db.New(p).CountItemsBelowMinimum(ctx)
			return int(n), err
		}},
	{KindBatchExpiring, rbac.Inventory, common.LevelWarn, "T16",
		func(ctx context.Context, p *pgxpool.Pool) (int, error) {
			n, err := db.New(p).CountBatchesExpiringWithin(ctx, ExpiryWindow)
			return int(n), err
		}},
	{KindPOAwaitingApprove, rbac.Procurement, common.LevelWarn, "T19",
		func(ctx context.Context, p *pgxpool.Pool) (int, error) {
			n, err := db.New(p).CountPurchaseOrdersAwaitingApproval(ctx)
			return int(n), err
		}},
	{KindDeliveryOverdue, rbac.Logistics, common.LevelDanger, "T21",
		func(ctx context.Context, p *pgxpool.Pool) (int, error) {
			n, err := db.New(p).CountOverdueDeliveries(ctx)
			return int(n), err
		}},
	{KindDocumentExpiring, rbac.Documents, common.LevelWarn, "T22",
		func(ctx context.Context, p *pgxpool.Pool) (int, error) {
			n, err := db.New(p).CountDocumentsExpiringWithin(ctx, ExpiryWindow)
			return int(n), err
		}},
	{KindContractExpiring, rbac.HR, common.LevelWarn, "T23",
		func(ctx context.Context, p *pgxpool.Pool) (int, error) {
			n, err := db.New(p).CountContractsExpiringWithin(ctx, ExpiryWindow)
			return int(n), err
		}},
	{KindMaintenanceDue, rbac.Equipment, common.LevelWarn, "T24",
		func(ctx context.Context, p *pgxpool.Pool) (int, error) {
			n, err := db.New(p).CountMaintenanceDue(ctx, ExpiryWindow)
			return int(n), err
		}},
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

// Open returns every standing condition that is currently true and that this
// viewer may see, as one Alert each.
//
// Level and resource travel with it: the sidebar pill and the bell entry are the
// same fact rendered twice, so they share a source.
func (s *Service) Open(ctx context.Context, perms rbac.Set) ([]Alert, error) {
	out := make([]Alert, 0, len(conditions))
	for _, c := range conditions {
		if !perms.CanRead(c.resource) || c.count == nil {
			continue
		}
		n, err := c.count(ctx, s.pool)
		if err != nil {
			return nil, fmt.Errorf("alerts: %s: %w", c.kind, err)
		}
		if n == 0 {
			continue
		}
		out = append(out, Alert{
			Kind: c.kind, Resource: c.resource, Level: c.level, Count: n,
		})
	}
	return out, nil
}

// ConditionKinds lists the derived standing conditions, in feed order.
func ConditionKinds() []Kind {
	out := make([]Kind, 0, len(conditions))
	for _, c := range conditions {
		out = append(out, c.kind)
	}
	return out
}

// Pending lists standing conditions whose query is not attached yet, mapped to the
// task that will attach it.
func Pending() map[Kind]string {
	out := make(map[Kind]string)
	for _, c := range conditions {
		if c.count == nil {
			out[c.kind] = c.module
		}
	}
	return out
}

// Emit writes one of the three discrete events.
//
// It takes the caller's transaction, exactly as audit.Record does, so the
// notification commits with the mutation that caused it. A notification about a
// batch that was never quarantined — because the transaction rolled back after
// the notification was written — would be worse than no notification at all.
//
// Called by the domain that caused it: inquiries on submission, quality on
// quarantine and rejection.
func Emit(ctx context.Context, tx db.DBTX, actor uuid.NullUUID, kind Kind, resource string, resourceID uuid.UUID, level common.Level, title, body string) error {
	var bodyPtr *string
	if body != "" {
		bodyPtr = &body
	}
	_, err := db.New(tx).InsertNotification(ctx, db.InsertNotificationParams{
		Kind:       string(kind),
		Resource:   resource,
		ResourceID: uuid.NullUUID{UUID: resourceID, Valid: true},
		Level:      string(level),
		Title:      title,
		Body:       bodyPtr,
		CreatedBy:  actor,
	})
	return err
}

// Feed returns the persisted events the viewer may see, newest first.
//
// Filtered by the viewer's READABLE resources, so a user who cannot open a module
// never learns from a notification that it has a problem (docs/05-MODULES.md:294).
func (s *Service) Feed(ctx context.Context, viewer uuid.UUID, perms rbac.Set, limit int32) ([]db.ListNotificationsRow, error) {
	readable := perms.ReadableResources()
	if len(readable) == 0 {
		return nil, nil
	}
	return db.New(s.pool).ListNotifications(ctx, db.ListNotificationsParams{
		UserID: viewer, Resources: readable, Limit: limit,
	})
}

// Unread is the bell badge.
func (s *Service) Unread(ctx context.Context, viewer uuid.UUID, perms rbac.Set) (int, error) {
	readable := perms.ReadableResources()
	if len(readable) == 0 {
		return 0, nil
	}
	n, err := db.New(s.pool).CountUnreadNotifications(ctx, db.CountUnreadNotificationsParams{
		UserID: viewer, Resources: readable,
	})
	return int(n), err
}

// MarkAllRead marks every notification the viewer CAN SEE as read.
//
// Scoped to readable resources for the same reason the feed is: marking "all"
// must not silently acknowledge notifications the user was never shown.
func (s *Service) MarkAllRead(ctx context.Context, viewer uuid.UUID, perms rbac.Set) error {
	readable := perms.ReadableResources()
	if len(readable) == 0 {
		return nil
	}
	return db.New(s.pool).MarkNotificationsRead(ctx, db.MarkNotificationsReadParams{
		UserID: viewer, Resources: readable,
	})
}

// IsPersisted reports whether a kind is a discrete event (written to
// notifications) rather than a standing condition (derived on read).
func IsPersisted(k Kind) bool {
	switch k {
	case KindInquiryReceived, KindBatchQuarantined, KindBatchRejected:
		return true
	}
	return false
}

// Kinds returns every trigger this package knows about, derived and persisted.
func Kinds() []Kind {
	return []Kind{
		KindStockBelowMinimum, KindBatchExpiring, KindPOAwaitingApprove,
		KindDeliveryOverdue, KindDocumentExpiring, KindContractExpiring,
		KindMaintenanceDue, KindInquiryReceived, KindBatchQuarantined, KindBatchRejected,
	}
}
