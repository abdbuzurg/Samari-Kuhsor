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

-- name: GetBatchWithItem :one
-- The traceability header: the batch plus enough of the product to name it.
SELECT sqlc.embed(b), i.sku, COALESCE(tr.name, i.sku) AS item_name
FROM batches b
JOIN items i ON i.id = b.item_id
LEFT JOIN item_translations tr
  ON tr.item_id = i.id AND tr.locale = 'ru' AND tr.deleted_at IS NULL
WHERE b.id = $1 AND b.deleted_at IS NULL;

-- name: ListBatchesForQuality :many
-- Качество list view. Ordered so the batches that need a decision come first:
-- quarantine before released, newest before oldest.
SELECT sqlc.embed(b), i.sku, COALESCE(tr.name, i.sku) AS item_name,
  (SELECT count(*) FROM quality_tests t
    WHERE t.batch_id=b.id AND t.deleted_at IS NULL)::int AS test_count,
  (SELECT count(*) FROM quality_tests t
    WHERE t.batch_id=b.id AND t.deleted_at IS NULL AND t.passed IS FALSE)::int AS failed_count
FROM batches b
JOIN items i ON i.id=b.item_id
LEFT JOIN item_translations tr
  ON tr.item_id=i.id AND tr.locale='ru' AND tr.deleted_at IS NULL
WHERE b.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR b.status=sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(b.batch_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(i.sku)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(tr.name,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY
  -- Quarantine first: it is the only status that is waiting on a human.
  CASE b.status WHEN 'quarantine' THEN 0 WHEN 'in_production' THEN 1 ELSE 2 END,
  b.produced_on DESC NULLS LAST, b.batch_no DESC
LIMIT $1 OFFSET $2;

-- name: CountBatchesForQuality :one
SELECT count(*) FROM batches b
JOIN items i ON i.id=b.item_id
LEFT JOIN item_translations tr
  ON tr.item_id=i.id AND tr.locale='ru' AND tr.deleted_at IS NULL
WHERE b.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR b.status=sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(b.batch_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(i.sku)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(tr.name,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');
