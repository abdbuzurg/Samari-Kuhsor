-- The operating chain: inventory, production, quality.
--
-- docs/02-SCHEMA.md §5–§7. The most important object here is stock_movements,
-- and the most important thing about it is what is absent: there is no balance
-- column, anywhere, ever (CLAUDE.md §4.2).

-- +goose Up

-- +goose StatementBegin
CREATE TABLE locations (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  code       text        NOT NULL,   -- 'A-12', 'C-07', 'D-01'
  name       text        NOT NULL,
  -- Mirrors items.item_type, plus quarantine. Production output lands in a
  -- quarantine zone and only a quality decision moves it out (§7).
  zone       text        NOT NULL CHECK (zone IN ('finished_goods','raw','packaging','quarantine')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer     NOT NULL DEFAULT 1,
  created_by uuid        REFERENCES users(id)
);
-- +goose StatementEnd

CREATE UNIQUE INDEX locations_code_key ON locations (code) WHERE deleted_at IS NULL;
CREATE TRIGGER locations_touch BEFORE UPDATE ON locations FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- THE most important table in the system (docs/02-SCHEMA.md:204).
--
-- Append-only. qty_delta is SIGNED and is never an absolute total. To correct a
-- wrong receipt of 100, insert -100 with reason 'adjustment'; never update or
-- tombstone the original, because the original is evidence.
CREATE TABLE stock_movements (
  id          uuid          PRIMARY KEY DEFAULT uuidv7(),
  item_id     uuid          NOT NULL REFERENCES items(id),
  batch_id    uuid          REFERENCES batches(id),
  location_id uuid          NOT NULL REFERENCES locations(id),
  qty_delta   numeric(14,3) NOT NULL,
  reason      text          NOT NULL CHECK (reason IN
                ('goods_receipt','production_output','material_issue','sale',
                 'transfer','adjustment','scrap','return')),
  ref_type    text,   -- 'purchase_order' | 'manufacturing_order' | 'shipment' | ...
  ref_id      uuid,   -- a transfer's two rows share this
  note        text,
  occurred_at timestamptz   NOT NULL DEFAULT now(),
  created_at  timestamptz   NOT NULL DEFAULT now(),
  updated_at  timestamptz   NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer       NOT NULL DEFAULT 1,
  created_by  uuid          REFERENCES users(id),
  -- A zero movement records nothing and would only add noise to the ledger.
  CONSTRAINT stock_movements_nonzero CHECK (qty_delta <> 0)
);
-- +goose StatementEnd

-- Covering index for the balance view's GROUP BY.
CREATE INDEX stock_movements_position_idx
  ON stock_movements (item_id, batch_id, location_id) WHERE deleted_at IS NULL;
CREATE INDEX stock_movements_ref_idx ON stock_movements (ref_type, ref_id) WHERE deleted_at IS NULL;
CREATE INDEX stock_movements_occurred_idx ON stock_movements (occurred_at DESC) WHERE deleted_at IS NULL;
CREATE TRIGGER stock_movements_touch BEFORE UPDATE ON stock_movements
  FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- A PLAIN VIEW, not materialised (docs/07-IMPLEMENTATION-PLAN.md I5).
--
-- Always exact, no refresh machinery, no staleness. The materialised variant
-- would solve a problem this system does not have: a factory with five SKUs
-- generates movement counts in the low tens of thousands per year, and this
-- aggregate over that with the index above is sub-millisecond.
--
-- The name is preserved so a materialised swap later is invisible to callers.
CREATE VIEW stock_balances AS
SELECT
  item_id,
  batch_id,
  location_id,
  SUM(qty_delta) AS on_hand,
  MAX(occurred_at) AS last_movement_at
FROM stock_movements
WHERE deleted_at IS NULL
GROUP BY item_id, batch_id, location_id;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Production — docs/02-SCHEMA.md §6
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE manufacturing_orders (
  id            uuid          PRIMARY KEY DEFAULT uuidv7(),
  mo_no         text          NOT NULL,   -- 'MO-0612'
  item_id       uuid          NOT NULL REFERENCES items(id),
  -- 1:1 with the batch it produces (docs/05-MODULES.md:127).
  batch_id      uuid          REFERENCES batches(id),
  line          text,                     -- 'Линия №1'
  planned_qty   numeric(14,3) NOT NULL CHECK (planned_qty > 0),
  scheduled_for date,
  status        text          NOT NULL DEFAULT 'planned'
                CHECK (status IN ('planned','in_progress','qc_hold','done','cancelled')),
  created_at    timestamptz   NOT NULL DEFAULT now(),
  updated_at    timestamptz   NOT NULL DEFAULT now(),
  deleted_at    timestamptz,
  version       integer       NOT NULL DEFAULT 1,
  created_by    uuid          REFERENCES users(id)
);
-- +goose StatementEnd

CREATE UNIQUE INDEX manufacturing_orders_no_key ON manufacturing_orders (mo_no) WHERE deleted_at IS NULL;
-- Enforces the 1:1 in the schema rather than in prose: one batch cannot be the
-- output of two orders.
CREATE UNIQUE INDEX manufacturing_orders_batch_key
  ON manufacturing_orders (batch_id) WHERE deleted_at IS NULL AND batch_id IS NOT NULL;
CREATE TRIGGER manufacturing_orders_touch BEFORE UPDATE ON manufacturing_orders
  FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- Append-only, like stock. Actual output, yield and downtime are SUMS over these
-- rows, never columns on the order (docs/02-SCHEMA.md:274).
CREATE TABLE production_entries (
  id           uuid          PRIMARY KEY DEFAULT uuidv7(),
  mo_id        uuid          NOT NULL REFERENCES manufacturing_orders(id),
  good_qty     numeric(14,3) NOT NULL DEFAULT 0 CHECK (good_qty >= 0),
  scrap_qty    numeric(14,3) NOT NULL DEFAULT 0 CHECK (scrap_qty >= 0),
  downtime_min integer       NOT NULL DEFAULT 0 CHECK (downtime_min >= 0),
  note         text,
  recorded_at  timestamptz   NOT NULL DEFAULT now(),
  recorded_by  uuid          REFERENCES users(id),
  created_at   timestamptz   NOT NULL DEFAULT now(),
  updated_at   timestamptz   NOT NULL DEFAULT now(),
  deleted_at   timestamptz,
  version      integer       NOT NULL DEFAULT 1,
  created_by   uuid          REFERENCES users(id)
);
-- +goose StatementEnd

CREATE INDEX production_entries_mo_idx ON production_entries (mo_id) WHERE deleted_at IS NULL;
CREATE TRIGGER production_entries_touch BEFORE UPDATE ON production_entries
  FOR EACH ROW EXECUTE FUNCTION touch_row();

-- ---------------------------------------------------------------------------
-- Quality — docs/02-SCHEMA.md §7. The regulatory heart.
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE quality_tests (
  id           uuid        PRIMARY KEY DEFAULT uuidv7(),
  batch_id     uuid        NOT NULL REFERENCES batches(id),
  test_type    text        NOT NULL CHECK (test_type IN
                 ('ph','microbiology','brix','viscosity','metal_detection','organoleptic')),
  result_value text,
  passed       boolean,
  inspector_id uuid        REFERENCES users(id),
  tested_at    timestamptz NOT NULL DEFAULT now(),
  notes        text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz,
  version      integer     NOT NULL DEFAULT 1,
  created_by   uuid        REFERENCES users(id)
);
-- +goose StatementEnd

CREATE INDEX quality_tests_batch_idx ON quality_tests (batch_id, tested_at DESC) WHERE deleted_at IS NULL;
CREATE TRIGGER quality_tests_touch BEFORE UPDATE ON quality_tests
  FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- Append-only; drives batches.status. This table IS the evidence trail behind the
-- website's laboratory-control claim (docs/02-SCHEMA.md:318), so it carries no
-- tombstone and no version: an entry is never amended and never removed.
CREATE TABLE batch_status_events (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  batch_id    uuid        NOT NULL REFERENCES batches(id),
  from_status text,
  to_status   text        NOT NULL
              CHECK (to_status IN ('in_production','quarantine','released','rejected')),
  decided_by  uuid        NOT NULL REFERENCES users(id),
  reason      text,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  created_at  timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

CREATE INDEX batch_status_events_batch_idx ON batch_status_events (batch_id, occurred_at DESC);

-- +goose Down

DROP TABLE IF EXISTS batch_status_events;
DROP TABLE IF EXISTS quality_tests;
DROP TABLE IF EXISTS production_entries;
DROP TABLE IF EXISTS manufacturing_orders;
DROP VIEW IF EXISTS stock_balances;
DROP TABLE IF EXISTS stock_movements;
DROP TABLE IF EXISTS locations;
