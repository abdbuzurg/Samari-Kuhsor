-- Производство — docs/02-SCHEMA.md §6.
--
-- Actual output, yield and downtime are SUMS over production_entries, never
-- columns on the order. There is deliberately no query here that writes them.

-- name: CreateManufacturingOrder :one
INSERT INTO manufacturing_orders (mo_no, item_id, batch_id, line, planned_qty, scheduled_for, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetManufacturingOrder :one
SELECT * FROM manufacturing_orders WHERE id = $1 AND deleted_at IS NULL;

-- name: ListManufacturingOrders :many
SELECT sqlc.embed(mo), i.sku, b.batch_no,
  -- The list shows the Russian product name, not the SKU alone: an operator on
  -- the line reads «Сок яблочный 1 л», not APJ-1000. Falls back to the SKU so a
  -- product whose translation is missing still renders something identifiable
  -- rather than an empty cell.
  COALESCE(tr.name, i.sku) AS item_name,
  COALESCE((SELECT SUM(good_qty) FROM production_entries e
             WHERE e.mo_id = mo.id AND e.deleted_at IS NULL), 0)::numeric AS good_qty,
  COALESCE((SELECT SUM(scrap_qty) FROM production_entries e
             WHERE e.mo_id = mo.id AND e.deleted_at IS NULL), 0)::numeric AS scrap_qty
FROM manufacturing_orders mo
JOIN items i ON i.id = mo.item_id
LEFT JOIN batches b ON b.id = mo.batch_id
LEFT JOIN item_translations tr
  ON tr.item_id = i.id AND tr.locale = 'ru' AND tr.deleted_at IS NULL
WHERE mo.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR mo.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(mo.mo_no)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
       OR unaccent(lower(i.sku)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
       OR unaccent(lower(COALESCE(tr.name, ''))) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%')
ORDER BY mo.scheduled_for DESC NULLS LAST, mo.mo_no DESC
LIMIT $1 OFFSET $2;

-- name: CountManufacturingOrders :one
SELECT count(*) FROM manufacturing_orders mo
JOIN items i ON i.id = mo.item_id
LEFT JOIN item_translations tr
  ON tr.item_id = i.id AND tr.locale = 'ru' AND tr.deleted_at IS NULL
WHERE mo.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR mo.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(mo.mo_no)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
       OR unaccent(lower(i.sku)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
       OR unaccent(lower(COALESCE(tr.name, ''))) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%');

-- name: SetManufacturingOrderStatus :one
UPDATE manufacturing_orders SET status = $2
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: InsertProductionEntry :one
INSERT INTO production_entries (mo_id, good_qty, scrap_qty, downtime_min, note, recorded_by, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $6)
RETURNING *;

-- name: ListProductionEntries :many
SELECT * FROM production_entries
WHERE mo_id = $1 AND deleted_at IS NULL
ORDER BY recorded_at DESC;

-- name: GetProductionTotals :one
-- Yield is a computation, not a column (docs/02-SCHEMA.md:274).
SELECT
  COALESCE(SUM(good_qty), 0)::numeric     AS good_qty,
  COALESCE(SUM(scrap_qty), 0)::numeric    AS scrap_qty,
  COALESCE(SUM(downtime_min), 0)::bigint  AS downtime_min,
  count(*)::bigint                        AS entry_count
FROM production_entries
WHERE mo_id = $1 AND deleted_at IS NULL;
