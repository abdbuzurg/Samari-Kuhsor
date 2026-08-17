-- Товары и цены — the product master. docs/05-MODULES.md §4.
--
-- This is the reference slice: every module after it copies these patterns, so
-- the shapes here are deliberate.
--
-- Note what is absent: any query that reads or writes a stock quantity. There is
-- no such column. Stock is derived from the movement ledger (CLAUDE.md §4.2), and
-- the UI must never offer "set stock to X" (docs/05-MODULES.md:112).

-- name: GetItemByID :one
SELECT * FROM items WHERE id = $1 AND deleted_at IS NULL;

-- name: GetItemBySKU :one
SELECT * FROM items WHERE sku = $1 AND deleted_at IS NULL;

-- name: CountItems :one
SELECT count(*) FROM items WHERE deleted_at IS NULL;

-- name: CountItemsByStatus :one
SELECT count(*) FROM items WHERE status = $1 AND deleted_at IS NULL;

-- name: ListItemTranslations :many
SELECT * FROM item_translations
WHERE item_id = $1 AND deleted_at IS NULL
ORDER BY locale;

-- ---------------------------------------------------------------------------
-- List — docs/03-API-CONTRACT.md §5
-- ---------------------------------------------------------------------------

-- name: ListItems :many
-- Returns the item rows and one display label. Packaging codes and current
-- prices are fetched for the whole page by the two queries below rather than
-- joined here.
--
-- A LEFT JOIN LATERAL for the price reads better but sqlc infers its columns as
-- NON-NULL, so an item with no price fails to scan at runtime — a bug that only
-- appears once someone creates a product before pricing it, which is exactly the
-- normal order of work. Three round trips per page, constant in page size, is
-- the honest trade.
--
-- Sorting is expressed as CASE branches rather than string interpolation. The
-- field is already whitelisted in Go, but building an ORDER BY by concatenation
-- is how whitelists get bypassed later, when someone adds a field and forgets.
SELECT
  sqlc.embed(i),
  -- Requested locale, falling back to Russian, then to the SKU. An item with no
  -- translation at all must not render as a blank row; its SKU is the one label
  -- that always exists.
  COALESCE(tr.name, tr_ru.name, i.sku) AS display_name
FROM items i
LEFT JOIN item_translations tr
  ON tr.item_id = i.id AND tr.locale = sqlc.arg(locale)::text AND tr.deleted_at IS NULL
LEFT JOIN item_translations tr_ru
  ON tr_ru.item_id = i.id AND tr_ru.locale = 'ru' AND tr_ru.deleted_at IS NULL
WHERE i.deleted_at IS NULL
  AND (sqlc.narg(item_type)::text IS NULL OR i.item_type = sqlc.narg(item_type))
  AND (sqlc.narg(status)::text IS NULL OR i.status = sqlc.narg(status))
  AND (sqlc.narg(category)::text IS NULL OR i.category = sqlc.narg(category))
  -- Case-insensitive and unaccented, per docs/03-API-CONTRACT.md:136. Searches
  -- the SKU and EVERY locale's name, so a Tajik-speaking operator finds a product
  -- by its Tajik name even while the list renders in Russian.
  AND (
    sqlc.narg(q)::text IS NULL
    OR unaccent(lower(i.sku)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
    OR EXISTS (
      SELECT 1 FROM item_translations s
       WHERE s.item_id = i.id AND s.deleted_at IS NULL
         AND unaccent(lower(s.name)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
    )
  )
ORDER BY
  (CASE WHEN sqlc.arg(sort_field)::text = 'sku'        AND NOT sqlc.arg(sort_desc)::bool THEN i.sku END) ASC,
  (CASE WHEN sqlc.arg(sort_field)::text = 'sku'        AND     sqlc.arg(sort_desc)::bool THEN i.sku END) DESC,
  (CASE WHEN sqlc.arg(sort_field)::text = 'category'   AND NOT sqlc.arg(sort_desc)::bool THEN i.category END) ASC,
  (CASE WHEN sqlc.arg(sort_field)::text = 'category'   AND     sqlc.arg(sort_desc)::bool THEN i.category END) DESC,
  (CASE WHEN sqlc.arg(sort_field)::text = 'status'     AND NOT sqlc.arg(sort_desc)::bool THEN i.status END) ASC,
  (CASE WHEN sqlc.arg(sort_field)::text = 'status'     AND     sqlc.arg(sort_desc)::bool THEN i.status END) DESC,
  (CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::bool THEN i.created_at END) ASC,
  (CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND     sqlc.arg(sort_desc)::bool THEN i.created_at END) DESC,
  -- Always a tiebreaker on id, so paging is deterministic when sort keys tie
  -- (docs/03-API-CONTRACT.md:139). Without it a client silently sees one row
  -- twice and misses another.
  i.id ASC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: ListPackagingUnitsForItems :many
-- One query for a whole page, not one per row.
SELECT * FROM packaging_units
WHERE item_id = ANY(sqlc.arg(item_ids)::uuid[]) AND deleted_at IS NULL
ORDER BY item_id, qty_in_base, code;

-- name: ListCurrentPricesForItems :many
-- The price in force today for each of the given items. DISTINCT ON keeps one
-- row per item; the ORDER BY decides which one.
SELECT DISTINCT ON (item_id) *
FROM item_prices
WHERE item_id = ANY(sqlc.arg(item_ids)::uuid[]) AND deleted_at IS NULL
  AND valid_from <= CURRENT_DATE
  AND (valid_to IS NULL OR valid_to >= CURRENT_DATE)
ORDER BY item_id, valid_from DESC, id DESC;

-- name: ListTranslationsForItems :many
SELECT * FROM item_translations
WHERE item_id = ANY(sqlc.arg(item_ids)::uuid[]) AND deleted_at IS NULL
ORDER BY item_id, locale;

-- name: CountListItems :one
-- Must apply exactly the same filters as ListItems, or the pagination metadata
-- describes a different collection from the one returned.
SELECT count(*)
FROM items i
WHERE i.deleted_at IS NULL
  AND (sqlc.narg(item_type)::text IS NULL OR i.item_type = sqlc.narg(item_type))
  AND (sqlc.narg(status)::text IS NULL OR i.status = sqlc.narg(status))
  AND (sqlc.narg(category)::text IS NULL OR i.category = sqlc.narg(category))
  AND (
    sqlc.narg(q)::text IS NULL
    OR unaccent(lower(i.sku)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
    OR EXISTS (
      SELECT 1 FROM item_translations s
       WHERE s.item_id = i.id AND s.deleted_at IS NULL
         AND unaccent(lower(s.name)) LIKE '%' || unaccent(lower(sqlc.narg(q))) || '%'
    )
  );

-- ---------------------------------------------------------------------------
-- Detail
-- ---------------------------------------------------------------------------

-- name: ListPackagingUnits :many
SELECT * FROM packaging_units
WHERE item_id = $1 AND deleted_at IS NULL
ORDER BY qty_in_base, code;

-- name: ListItemPrices :many
-- Full price history, newest first — the detail view shows it (docs/05-MODULES.md:90).
SELECT * FROM item_prices
WHERE item_id = $1 AND deleted_at IS NULL
ORDER BY valid_from DESC, id DESC;

-- name: GetCurrentItemPrice :one
SELECT * FROM item_prices
WHERE item_id = $1 AND deleted_at IS NULL
  AND valid_from <= CURRENT_DATE
  AND (valid_to IS NULL OR valid_to >= CURRENT_DATE)
ORDER BY valid_from DESC, id DESC
LIMIT 1;

-- name: ListBatchesForItem :many
-- Related records on the detail view (docs/05-MODULES.md §2).
SELECT * FROM batches
WHERE item_id = $1 AND deleted_at IS NULL
ORDER BY produced_on DESC NULLS LAST, batch_no DESC
LIMIT $2;

-- ---------------------------------------------------------------------------
-- Mutations
-- ---------------------------------------------------------------------------

-- name: CreateItem :one
INSERT INTO items (sku, item_type, category, base_uom, shelf_life_days, min_qty, status, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateItem :one
-- The version guard is in the WHERE clause, not a prior read: checking then
-- writing in two statements leaves a window where another request commits in
-- between. No rows returned means the version was stale.
UPDATE items SET
  category        = $2,
  base_uom        = $3,
  shelf_life_days = $4,
  min_qty         = $5,
  status          = $6
WHERE id = $1 AND version = sqlc.arg(expected_version) AND deleted_at IS NULL
RETURNING *;

-- name: TombstoneItem :one
-- No hard deletes (CLAUDE.md §4.3). Also version-guarded: deleting a record
-- someone else just edited should fail the same way updating it would.
UPDATE items SET deleted_at = now()
WHERE id = $1 AND version = sqlc.arg(expected_version) AND deleted_at IS NULL
RETURNING *;

-- name: UpsertItemTranslation :one
INSERT INTO item_translations (
  item_id, locale, name, description, ingredients, nutrition,
  storage_conditions, after_opening, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (item_id, locale) WHERE deleted_at IS NULL
DO UPDATE SET
  name               = EXCLUDED.name,
  description        = EXCLUDED.description,
  ingredients        = EXCLUDED.ingredients,
  nutrition          = EXCLUDED.nutrition,
  storage_conditions = EXCLUDED.storage_conditions,
  after_opening      = EXCLUDED.after_opening
RETURNING *;

-- name: CreatePackagingUnit :one
INSERT INTO packaging_units (item_id, code, qty_in_base, barcode, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: TombstonePackagingUnit :exec
UPDATE packaging_units SET deleted_at = now()
WHERE id = $1 AND item_id = $2 AND deleted_at IS NULL;

-- name: CreateItemPrice :one
INSERT INTO item_prices (item_id, currency, amount, valid_from, valid_to, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: CloseOpenItemPrices :exec
-- A new price supersedes the open one rather than replacing it: price history is
-- evidence, and the detail view shows it. Closes the day before the new price
-- starts so the two never overlap.
UPDATE item_prices
SET valid_to = sqlc.arg(new_valid_from)::date - 1
WHERE item_id = $1 AND deleted_at IS NULL AND valid_to IS NULL
  AND valid_from < sqlc.arg(new_valid_from)::date;

-- ---------------------------------------------------------------------------
-- Batches and QR — docs/01-DECISIONS.md D11
-- ---------------------------------------------------------------------------

-- name: GetBatchByID :one
SELECT * FROM batches WHERE id = $1 AND deleted_at IS NULL;

-- name: GetBatchByNo :one
SELECT * FROM batches WHERE batch_no = $1 AND deleted_at IS NULL;

-- name: CreateBatch :one
INSERT INTO batches (batch_no, item_id, produced_on, expires_on, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: IssueBatchQR :one
-- Writes the payload and stamps the issue time. Guarded on qr_payload IS NULL:
-- a wrapper is printed against the issued code, so re-issuing a different one
-- silently invalidates wrappers that may already be in production. Zero rows
-- means it was already issued.
UPDATE batches
SET qr_payload = $2, qr_issued_at = now()
WHERE id = $1 AND deleted_at IS NULL AND qr_payload IS NULL
RETURNING *;

-- name: ListBatchesForQRExport :many
-- Batches whose codes go to the printer. NULL filters mean "no filter".
SELECT sqlc.embed(b), i.sku, COALESCE(tr.name, i.sku) AS item_name
FROM batches b
JOIN items i ON i.id = b.item_id AND i.deleted_at IS NULL
LEFT JOIN item_translations tr
  ON tr.item_id = i.id AND tr.locale = 'ru' AND tr.deleted_at IS NULL
WHERE b.deleted_at IS NULL
  AND b.qr_payload IS NOT NULL
  AND (sqlc.narg(item_id)::uuid IS NULL OR b.item_id = sqlc.narg(item_id))
  AND (sqlc.narg(issued_after)::timestamptz IS NULL OR b.qr_issued_at >= sqlc.narg(issued_after))
ORDER BY b.batch_no;

-- name: CountBatchesAwaitingQR :one
SELECT count(*) FROM batches WHERE deleted_at IS NULL AND qr_payload IS NULL;

-- ---------------------------------------------------------------------------
-- Public site — docs/03-API-CONTRACT.md §9
-- ---------------------------------------------------------------------------
--
-- These read the catalogue for an anonymous visitor. What they deliberately do
-- NOT select is as important as what they do: no cost price, no supplier, no
-- stock, no internal status. A public endpoint that joins one table too many is
-- how a competitor learns the margin.

-- name: ListPublicProducts :many
-- Only ACTIVE finished goods. A draft product is one the client is still
-- editing, and publishing it would put unapproved copy on the public site.
SELECT
  i.id, i.sku,
  COALESCE(tr.name, ru.name, i.sku) AS name,
  COALESCE(tr.description, ru.description) AS description,
  i.category
FROM items i
LEFT JOIN item_translations tr
  ON tr.item_id = i.id AND tr.locale = sqlc.arg(locale)::text AND tr.deleted_at IS NULL
LEFT JOIN item_translations ru
  ON ru.item_id = i.id AND ru.locale = 'ru' AND ru.deleted_at IS NULL
WHERE i.deleted_at IS NULL
  AND i.item_type = 'finished_good'
  AND i.status = 'active'
ORDER BY i.sku;

-- name: GetPublicProduct :one
SELECT
  i.id, i.sku,
  COALESCE(tr.name, ru.name, i.sku) AS name,
  COALESCE(tr.description, ru.description) AS description,
  i.category, i.base_uom, i.shelf_life_days
FROM items i
LEFT JOIN item_translations tr
  ON tr.item_id = i.id AND tr.locale = sqlc.arg(locale)::text AND tr.deleted_at IS NULL
LEFT JOIN item_translations ru
  ON ru.item_id = i.id AND ru.locale = 'ru' AND ru.deleted_at IS NULL
WHERE i.deleted_at IS NULL
  AND i.item_type = 'finished_good'
  AND i.status = 'active'
  AND i.sku = sqlc.arg(sku)::text;
