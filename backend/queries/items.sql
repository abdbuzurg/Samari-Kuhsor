-- Product master queries.
--
-- Minimal for now: T03 wires sqlc and proves generation. The full Товары query
-- set — list with search and filters, detail with translations, packaging units
-- and price history — arrives with T11, which is the reference slice.
--
-- Note there is no query here that reads or writes a stock quantity. There is no
-- such column; stock is derived from the movement ledger (CLAUDE.md §4.2, T16).

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
