-- Audit log. Append-only: there is no update and no delete, by design
-- (docs/02-SCHEMA.md:123). Every mutation in the system writes one row, inside
-- the mutating transaction (docs/07-IMPLEMENTATION-PLAN.md I4).

-- name: InsertAuditEntry :one
INSERT INTO audit_log (actor_id, action, resource, resource_id, before, after, ip)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListAuditForResource :many
-- Powers the activity panel on every detail view (docs/05-MODULES.md §2).
SELECT * FROM audit_log
WHERE resource = $1 AND resource_id = $2
ORDER BY occurred_at DESC, id DESC
LIMIT $3 OFFSET $4;

-- name: CountAuditForResource :one
SELECT count(*) FROM audit_log
WHERE resource = $1 AND resource_id = $2;

-- name: ListAudit :many
-- The audit viewer (docs/04-RBAC.md §6), filterable by actor, resource and date
-- range. NULL means "no filter" for each dimension.
SELECT * FROM audit_log
WHERE (sqlc.narg(actor_id)::uuid IS NULL OR actor_id = sqlc.narg(actor_id))
  AND (sqlc.narg(resource)::text IS NULL OR resource = sqlc.narg(resource))
  AND (sqlc.narg(resource_id)::uuid IS NULL OR resource_id = sqlc.narg(resource_id))
  AND (sqlc.narg(from_ts)::timestamptz IS NULL OR occurred_at >= sqlc.narg(from_ts))
  AND (sqlc.narg(to_ts)::timestamptz IS NULL OR occurred_at <= sqlc.narg(to_ts))
ORDER BY occurred_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: CountAudit :one
SELECT count(*) FROM audit_log
WHERE (sqlc.narg(actor_id)::uuid IS NULL OR actor_id = sqlc.narg(actor_id))
  AND (sqlc.narg(resource)::text IS NULL OR resource = sqlc.narg(resource))
  AND (sqlc.narg(resource_id)::uuid IS NULL OR resource_id = sqlc.narg(resource_id))
  AND (sqlc.narg(from_ts)::timestamptz IS NULL OR occurred_at >= sqlc.narg(from_ts))
  AND (sqlc.narg(to_ts)::timestamptz IS NULL OR occurred_at <= sqlc.narg(to_ts));
