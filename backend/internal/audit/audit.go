// Package audit writes the mandatory audit trail.
//
// CLAUDE.md §4.5: every mutation writes an audit row — actor, action, resource,
// resource id, before/after, timestamp. This is a regulatory requirement, and for
// batch release it is the evidence trail behind the website's laboratory-control
// claim (docs/02-SCHEMA.md:318).
//
// The write happens HERE, in the domain layer, inside the mutating transaction
// (docs/07-IMPLEMENTATION-PLAN.md I4). Two alternatives were rejected:
//
//   - HTTP middleware sees the request body but not the prior row state, so it
//     cannot produce `before` at all.
//   - A database trigger produces perfect before/after and cannot be bypassed, but
//     a batch release is mechanically an UPDATE on batches; a trigger cannot know
//     it was an *approval*. `login` mutates no business row whatsoever.
//
// Only the domain layer knows what a change meant. Being in the same transaction
// means an audit row can never be lost to a partial failure — and it means a
// failed audit write rolls back the mutation, which is the correct direction: an
// unauditable change must not happen.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/google/uuid"

	"github.com/qoim/samari/backend/internal/db"
)

// Action is the verb recorded against a change. The set is open — modules add
// their own transition verbs — but these are the ones shared across the system.
type Action string

const (
	ActionCreate  Action = "create"
	ActionUpdate  Action = "update"
	ActionDelete  Action = "delete"  // tombstone; there are no hard deletes
	ActionApprove Action = "approve" // a decision carrying authority, not a data edit
	ActionLogin   Action = "login"
	ActionLogout  Action = "logout"
)

// Entry describes one auditable change.
type Entry struct {
	// ActorID is the user who made the change. Nil only for system actions such as
	// seeding; a nil actor on a user-initiated change is a bug.
	ActorID uuid.NullUUID
	Action  Action
	// Resource is the module key from docs/04-RBAC.md §2, so audit rows join to the
	// permission model and the activity panel can filter by resource.
	Resource   string
	ResourceID uuid.NullUUID
	// Before and After are marshalled to JSON. Nil is legitimate: a create has no
	// before, a delete has no after.
	Before any
	After  any
	IP     *netip.Addr
}

// Record writes the audit row. Pass the transaction that carries the mutation —
// never the pool — so the two commit or fail together.
func Record(ctx context.Context, tx db.DBTX, e Entry) error {
	if e.Action == "" {
		return fmt.Errorf("audit: action is required")
	}
	if e.Resource == "" {
		return fmt.Errorf("audit: resource is required")
	}

	before, err := marshal(e.Before)
	if err != nil {
		return fmt.Errorf("audit: marshal before: %w", err)
	}
	after, err := marshal(e.After)
	if err != nil {
		return fmt.Errorf("audit: marshal after: %w", err)
	}

	q := db.New(tx)
	if _, err := q.InsertAuditEntry(ctx, db.InsertAuditEntryParams{
		ActorID:    e.ActorID,
		Action:     string(e.Action),
		Resource:   e.Resource,
		ResourceID: e.ResourceID,
		Before:     before,
		After:      after,
		Ip:         e.IP,
	}); err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}

func marshal(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// Actor wraps a user id for the ActorID field.
func Actor(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: true} }

// Target wraps a resource id for the ResourceID field.
func Target(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: true} }
