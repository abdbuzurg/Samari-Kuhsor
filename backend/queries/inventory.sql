-- Склад и запасы — the append-only ledger. docs/02-SCHEMA.md §5.
--
-- Note what is absent from this entire file: any UPDATE of qty_delta, and any
-- query that sets a quantity. Corrections are compensating INSERTs. The original
-- row is evidence and is never edited (docs/02-SCHEMA.md:240).

-- name: CreateLocation :one
INSERT INTO locations (code, name, zone, created_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetLocationByCode :one
SELECT * FROM locations WHERE code = $1 AND deleted_at IS NULL;

-- name: GetLocationByID :one
SELECT * FROM locations WHERE id = $1 AND deleted_at IS NULL;

-- name: ListLocations :many
SELECT * FROM locations WHERE deleted_at IS NULL ORDER BY zone, code;

-- name: GetQuarantineLocation :one
-- Production output lands here and only a quality decision moves it out (§7).
SELECT * FROM locations
WHERE zone = 'quarantine' AND deleted_at IS NULL
ORDER BY code
LIMIT 1;

-- ---------------------------------------------------------------------------
-- Movements
-- ---------------------------------------------------------------------------

-- name: InsertStockMovement :one
-- The only way stock changes. qty_delta is signed; there is no other writer.
INSERT INTO stock_movements (
  item_id, batch_id, location_id, qty_delta, reason, ref_type, ref_id, note, occurred_at, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE(sqlc.narg(occurred_at)::timestamptz, now()), $9)
RETURNING *;

-- name: LockStockPosition :exec
-- Serialises writers against one (item, batch, location) for the rest of the
-- transaction (docs/07-IMPLEMENTATION-PLAN.md I6).
--
-- Taken only when posting a NEGATIVE delta: two receipts cannot oversell, so
-- locking them would serialise the warehouse for nothing. The lock key hashes
-- the position; batch_id is nullable, so it is coalesced to a fixed sentinel
-- rather than left to hash NULL.
SELECT pg_advisory_xact_lock(
  hashtextextended(
    $1::text || ':' || COALESCE($2::text, '-') || ':' || $3::text, 0));

-- name: GetPositionBalance :one
-- The exact balance for one position. Read INSIDE the advisory lock before
-- posting a negative delta — a value read before the lock is already stale.
SELECT COALESCE(SUM(qty_delta), 0)::numeric AS on_hand
FROM stock_movements
WHERE item_id = $1
  AND batch_id IS NOT DISTINCT FROM sqlc.narg(batch_id)::uuid
  AND location_id = $2
  AND deleted_at IS NULL;

-- name: ListMovementsForPosition :many
-- The movement ledger for one position, with a running balance — the detail view
-- (docs/05-MODULES.md:115).
SELECT
  m.*,
  SUM(m.qty_delta) OVER (ORDER BY m.occurred_at, m.id) AS running_balance
FROM stock_movements m
WHERE m.item_id = $1
  AND m.batch_id IS NOT DISTINCT FROM sqlc.narg(batch_id)::uuid
  AND m.location_id = $2
  AND m.deleted_at IS NULL
ORDER BY m.occurred_at DESC, m.id DESC
LIMIT $3 OFFSET $4;

-- name: ListMovements :many
SELECT m.*, i.sku, l.code AS location_code
FROM stock_movements m
JOIN items i ON i.id = m.item_id
JOIN locations l ON l.id = m.location_id
WHERE m.deleted_at IS NULL
  AND (sqlc.narg(item_id)::uuid IS NULL OR m.item_id = sqlc.narg(item_id))
  AND (sqlc.narg(reason)::text IS NULL OR m.reason = sqlc.narg(reason))
  AND (sqlc.narg(ref_id)::uuid IS NULL OR m.ref_id = sqlc.narg(ref_id))
ORDER BY m.occurred_at DESC, m.id DESC
LIMIT $1 OFFSET $2;

-- ---------------------------------------------------------------------------
-- Balances — derived, never stored
-- ---------------------------------------------------------------------------

-- name: ListStockBalances :many
-- The Склад list view. Every quantity here is a SUM computed at read time.
--
-- Positions that have netted to zero are excluded: a row reading "0" is not a
-- stock position, it is the absence of one, and showing it would fill the
-- warehouse list with everything that ever passed through.
SELECT
  sqlc.embed(b),
  i.sku,
  COALESCE(tr.name, i.sku) AS item_name,
  i.base_uom,
  i.min_qty,
  l.code AS location_code,
  l.zone AS location_zone,
  bt.batch_no,
  bt.expires_on,
  bt.status AS batch_status
FROM stock_balances b
JOIN items i ON i.id = b.item_id AND i.deleted_at IS NULL
JOIN locations l ON l.id = b.location_id AND l.deleted_at IS NULL
LEFT JOIN item_translations tr
  ON tr.item_id = i.id AND tr.locale = 'ru' AND tr.deleted_at IS NULL
LEFT JOIN batches bt ON bt.id = b.batch_id
WHERE b.on_hand <> 0
  AND (sqlc.narg(item_id)::uuid IS NULL OR b.item_id = sqlc.narg(item_id))
  AND (sqlc.narg(zone)::text IS NULL OR l.zone = sqlc.narg(zone))
  AND (
    sqlc.narg(q)::text IS NULL
    OR unaccent(lower(i.sku)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
    OR unaccent(lower(COALESCE(tr.name, ''))) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
    OR unaccent(lower(COALESCE(bt.batch_no, ''))) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
  )
ORDER BY i.sku, l.code, bt.batch_no NULLS FIRST
LIMIT $1 OFFSET $2;

-- name: CountStockBalances :one
SELECT count(*)
FROM stock_balances b
JOIN items i ON i.id = b.item_id AND i.deleted_at IS NULL
JOIN locations l ON l.id = b.location_id AND l.deleted_at IS NULL
LEFT JOIN item_translations tr
  ON tr.item_id = i.id AND tr.locale = 'ru' AND tr.deleted_at IS NULL
LEFT JOIN batches bt ON bt.id = b.batch_id
WHERE b.on_hand <> 0
  AND (sqlc.narg(item_id)::uuid IS NULL OR b.item_id = sqlc.narg(item_id))
  AND (sqlc.narg(zone)::text IS NULL OR l.zone = sqlc.narg(zone))
  AND (
    sqlc.narg(q)::text IS NULL
    OR unaccent(lower(i.sku)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
    OR unaccent(lower(COALESCE(tr.name, ''))) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
    OR unaccent(lower(COALESCE(bt.batch_no, ''))) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
  );

-- name: GetItemTotalOnHand :one
-- Total across every location and batch for one item. Used by the low-stock
-- alert, which compares it against items.min_qty.
SELECT COALESCE(SUM(on_hand), 0)::numeric AS on_hand
FROM stock_balances WHERE item_id = $1;

-- ---------------------------------------------------------------------------
-- Alerts — derived standing conditions (docs/07-IMPLEMENTATION-PLAN.md I15)
-- ---------------------------------------------------------------------------

-- name: CountItemsBelowMinimum :one
-- Self-healing by construction: a goods receipt pushes the total back above the
-- threshold and the alert disappears with no retraction logic.
SELECT count(*)::int FROM (
  SELECT i.id
  FROM items i
  JOIN stock_balances b ON b.item_id = i.id
  WHERE i.deleted_at IS NULL AND i.min_qty IS NOT NULL
  GROUP BY i.id, i.min_qty
  HAVING SUM(b.on_hand) < i.min_qty
) low;

-- name: CountBatchesExpiringWithin :one
SELECT count(*)::int FROM batches
WHERE deleted_at IS NULL
  AND expires_on IS NOT NULL
  AND expires_on <= CURRENT_DATE + sqlc.arg(days)::int
  AND status <> 'rejected';

-- name: ListLowStockItems :many
SELECT i.id, i.sku, i.min_qty, SUM(b.on_hand)::numeric AS on_hand
FROM items i
JOIN stock_balances b ON b.item_id = i.id
WHERE i.deleted_at IS NULL AND i.min_qty IS NOT NULL
GROUP BY i.id, i.sku, i.min_qty
HAVING SUM(b.on_hand) < i.min_qty
ORDER BY i.sku;
