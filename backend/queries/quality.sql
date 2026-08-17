-- Качество и безопасность — docs/02-SCHEMA.md §7. The regulatory heart.
--
-- batches.status is changed ONLY through here, and only by a batch_status_events
-- row that names the deciding user. That table is the evidence trail behind the
-- website's laboratory-control claim.

-- name: CreateQualityTest :one
INSERT INTO quality_tests (batch_id, test_type, result_value, passed, inspector_id, notes, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $5)
RETURNING *;

-- name: ListQualityTests :many
SELECT qt.*, u.full_name AS inspector_name
FROM quality_tests qt
LEFT JOIN users u ON u.id = qt.inspector_id
WHERE qt.batch_id = $1 AND qt.deleted_at IS NULL
ORDER BY qt.tested_at DESC;

-- name: ListQualityTestsPaged :many
SELECT sqlc.embed(qt), b.batch_no, i.sku, b.status AS batch_status,
       COALESCE(u.full_name, '') AS inspector_name
FROM quality_tests qt
JOIN batches b ON b.id = qt.batch_id
JOIN items i ON i.id = b.item_id
LEFT JOIN users u ON u.id = qt.inspector_id
WHERE qt.deleted_at IS NULL
  AND (sqlc.narg(batch_status)::text IS NULL OR b.status = sqlc.narg(batch_status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(b.batch_no)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
       OR unaccent(lower(i.sku)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%')
ORDER BY qt.tested_at DESC
LIMIT $1 OFFSET $2;

-- name: CountQualityTestsPaged :one
SELECT count(*) FROM quality_tests qt
JOIN batches b ON b.id = qt.batch_id
JOIN items i ON i.id = b.item_id
WHERE qt.deleted_at IS NULL
  AND (sqlc.narg(batch_status)::text IS NULL OR b.status = sqlc.narg(batch_status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(b.batch_no)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
       OR unaccent(lower(i.sku)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%');

-- name: TransitionBatchStatus :one
-- Guarded on the CURRENT status, so an illegal transition cannot slip through a
-- read-then-write race. Zero rows means the batch moved underneath us.
UPDATE batches SET status = sqlc.arg(to_status)
WHERE id = $1 AND status = sqlc.arg(from_status) AND deleted_at IS NULL
RETURNING *;

-- name: InsertBatchStatusEvent :one
-- Append-only and immutable: no deleted_at, no version. This row is the evidence.
INSERT INTO batch_status_events (batch_id, from_status, to_status, decided_by, reason)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListBatchStatusEvents :many
SELECT e.*, u.full_name AS decided_by_name
FROM batch_status_events e
JOIN users u ON u.id = e.decided_by
WHERE e.batch_id = $1
ORDER BY e.occurred_at DESC;

-- name: CountBatchesByStatus :one
SELECT count(*)::int FROM batches WHERE status = $1 AND deleted_at IS NULL;

-- name: CountFailedTests :one
SELECT count(*)::int FROM quality_tests WHERE passed = false AND deleted_at IS NULL;
