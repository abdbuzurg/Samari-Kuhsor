package testsupport

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TB is the slice of *testing.T these helpers need. Taking an interface rather
// than the concrete type is what lets the helpers' own failure paths be tested —
// an assertion that silently passes is worse than no assertion at all.
type TB interface {
	Helper()
	Fatalf(format string, args ...any)
}

// AuditEntry is a read-back audit row, shaped for assertions rather than for
// application use.
type AuditEntry struct {
	ActorID    uuid.NullUUID
	Action     string
	Resource   string
	ResourceID uuid.NullUUID
	Before     []byte
	After      []byte
}

// AssertAudited fails the test unless exactly one audit_log row exists for the
// given resource, id and action.
//
// CLAUDE.md §4.5 makes an audit row mandatory on every mutation and calls it a
// regulatory requirement. docs/07-IMPLEMENTATION-PLAN.md I4 puts the write in the
// domain layer, which is precise but forgettable — so forgetting has to be caught
// mechanically. Every integration test covering a mutating endpoint calls this.
//
// It asserts exactly one row, not at least one: a duplicate audit entry means the
// mutation ran twice, or was audited at two layers, and both are defects.
func AssertAudited(t TB, pool *pgxpool.Pool, resource string, id uuid.UUID, action string) AuditEntry {
	t.Helper()

	rows, err := pool.Query(context.Background(), `
		SELECT actor_id, action, resource, resource_id, before, after
		FROM audit_log
		WHERE resource = $1 AND resource_id = $2 AND action = $3
		ORDER BY occurred_at`, resource, id, action)
	if err != nil {
		t.Fatalf("query audit_log: %v", err)
	}
	defer rows.Close()

	var found []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ActorID, &e.Action, &e.Resource, &e.ResourceID, &e.Before, &e.After); err != nil {
			t.Fatalf("scan audit_log: %v", err)
		}
		found = append(found, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate audit_log: %v", err)
	}

	switch len(found) {
	case 1:
		return found[0]
	case 0:
		t.Fatalf("no audit_log row for %s %s on %s — every mutation must write one (CLAUDE.md §4.5)",
			action, resource, id)
	default:
		t.Fatalf("%d audit_log rows for %s %s on %s — expected exactly one; the mutation ran twice or was audited at two layers",
			len(found), action, resource, id)
	}
	return AuditEntry{} // unreachable
}

// AssertNotAudited fails if any audit row exists for the resource and id. Used to
// prove that a rejected mutation left no trace — a 403 or a validation failure
// must not write an audit entry for work that never happened.
func AssertNotAudited(t TB, pool *pgxpool.Pool, resource string, id uuid.UUID) {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE resource = $1 AND resource_id = $2`,
		resource, id).Scan(&n)
	if err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected no audit_log rows for %s %s, found %d", resource, id, n)
	}
}

// CountAudit returns the total number of audit rows, for tests that care about
// the absence of extra writes rather than a specific entry.
func CountAudit(t TB, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log`).Scan(&n); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}
