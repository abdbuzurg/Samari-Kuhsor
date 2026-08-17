-- +goose Up
-- Fix the inferred type of the derived balance columns.
--
-- stock_balances was declared as
--
--     SUM(qty_delta) AS on_hand, MAX(occurred_at) AS last_movement_at
--
-- Postgres evaluates that correctly — SUM over numeric is numeric — but sqlc's
-- view introspection does not carry the type through, and generated
-- `OnHand int64` / `LastMovementAt interface{}`.
--
-- int64 is not a rounding nuisance here, it is data loss in exactly the place
-- CLAUDE.md §4.7 chose numeric(14,3) to prevent: 0.750 kg of sugar scans as 0.
-- The domain's own BalanceOf was unaffected because GetPositionBalance casts
-- explicitly; the list and ledger views did not, which is why no existing test
-- caught it.
--
-- The casts below are redundant to Postgres and load-bearing for the code
-- generator. Applied as a new migration rather than an edit to 00002, because a
-- migration is never edited once applied.
--
-- DROP then CREATE rather than CREATE OR REPLACE: replacing a view cannot change
-- a column's declared type, and numeric → numeric(14,3) is exactly that change.
-- Nothing depends on this view, so the drop is safe; if anything ever does, this
-- migration will fail loudly rather than silently leaving the old definition.

DROP VIEW IF EXISTS stock_balances;

CREATE VIEW stock_balances AS
SELECT
  item_id,
  batch_id,
  location_id,
  SUM(qty_delta)::numeric(14,3)   AS on_hand,
  MAX(occurred_at)::timestamptz   AS last_movement_at
FROM stock_movements
WHERE deleted_at IS NULL
GROUP BY item_id, batch_id, location_id;

-- +goose Down
DROP VIEW IF EXISTS stock_balances;

CREATE VIEW stock_balances AS
SELECT
  item_id,
  batch_id,
  location_id,
  SUM(qty_delta)   AS on_hand,
  MAX(occurred_at) AS last_movement_at
FROM stock_movements
WHERE deleted_at IS NULL
GROUP BY item_id, batch_id, location_id;
