-- Base schema: conventions, identity, access control, audit, product master, batches.
--
-- Authority: docs/02-SCHEMA.md. Conventions in §1 apply to every table without
-- exception; audit_log is the single documented exemption (§2).
--
-- items and batches are pulled forward into the base migration because QR
-- generation depends on batches and QR is needed before production starts (D11).

-- +goose Up

-- +goose StatementBegin
-- Case-insensitive, unaccented search is required by docs/03-API-CONTRACT.md:136
-- for the `q` parameter every module's toolbar sends.
CREATE EXTENSION IF NOT EXISTS unaccent;
-- +goose StatementEnd

-- +goose StatementBegin
-- Every table carries updated_at and version, and version increments on every
-- update (CLAUDE.md §4.4). A trigger rather than a convention: a convention gets
-- forgotten in one of twelve modules and the optimistic-concurrency check in
-- docs/03-API-CONTRACT.md §7 silently stops protecting that table.
--
-- Unlike audit writes, this needs no vocabulary — it cannot tell an approval from
-- an edit and does not need to — so a trigger is the right tool here where it is
-- the wrong tool for audit_log (docs/07-IMPLEMENTATION-PLAN.md I4).
CREATE OR REPLACE FUNCTION touch_row() RETURNS trigger AS $$
BEGIN
  NEW.updated_at := now();
  -- Guard against a caller passing its own version: the stored value always wins.
  NEW.version := OLD.version + 1;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Identity
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE users (
  id              uuid        PRIMARY KEY DEFAULT uuidv7(),
  email           text        NOT NULL,
  full_name       text        NOT NULL,
  password_hash   text        NOT NULL,   -- argon2id
  is_active       boolean     NOT NULL DEFAULT true,
  last_login_at   timestamptz,
  failed_attempts integer     NOT NULL DEFAULT 0,
  locked_until    timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  deleted_at      timestamptz,
  version         integer     NOT NULL DEFAULT 1,
  created_by      uuid        REFERENCES users(id)
);
-- +goose StatementEnd

-- Partial, so a tombstoned account frees its address (docs/02-SCHEMA.md:34).
CREATE UNIQUE INDEX users_email_key ON users (lower(email)) WHERE deleted_at IS NULL;
CREATE TRIGGER users_touch BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE sessions (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  user_id    uuid        NOT NULL REFERENCES users(id),
  token_hash text        NOT NULL,   -- the token itself is never stored
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  ip         inet,
  user_agent text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer     NOT NULL DEFAULT 1,
  created_by uuid        REFERENCES users(id)
);
-- +goose StatementEnd

-- Not partial: a token hash must be globally unique for lookup to be unambiguous.
CREATE UNIQUE INDEX sessions_token_hash_key ON sessions (token_hash);
CREATE INDEX sessions_user_id_idx ON sessions (user_id) WHERE deleted_at IS NULL;
CREATE TRIGGER sessions_touch BEFORE UPDATE ON sessions FOR EACH ROW EXECUTE FUNCTION touch_row();

-- ---------------------------------------------------------------------------
-- Access control — docs/04-RBAC.md
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
-- roles is one of the two documented exceptions to the sibling-translation-table
-- rule: short, fixed-cardinality labels where a sibling table costs more than it
-- earns (docs/02-SCHEMA.md:56). Do not extend the exception.
CREATE TABLE roles (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  key        text        NOT NULL,   -- 'admin', 'director', 'warehouse', ...
  name_ru    text        NOT NULL,
  name_tg    text        NOT NULL,
  name_en    text        NOT NULL,
  is_system  boolean     NOT NULL DEFAULT false,  -- seed roles; editable, not deletable
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer     NOT NULL DEFAULT 1,
  created_by uuid        REFERENCES users(id)
);
-- +goose StatementEnd

CREATE UNIQUE INDEX roles_key_key ON roles (key) WHERE deleted_at IS NULL;
CREATE TRIGGER roles_touch BEFORE UPDATE ON roles FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE role_permissions (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  role_id    uuid        NOT NULL REFERENCES roles(id),
  resource   text        NOT NULL,   -- module key, docs/04-RBAC.md §2
  action     text        NOT NULL CHECK (action IN ('read','manage','approve')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer     NOT NULL DEFAULT 1,
  created_by uuid        REFERENCES users(id)
);
-- +goose StatementEnd

CREATE UNIQUE INDEX role_permissions_key
  ON role_permissions (role_id, resource, action) WHERE deleted_at IS NULL;
CREATE TRIGGER role_permissions_touch BEFORE UPDATE ON role_permissions
  FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE user_roles (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  user_id    uuid        NOT NULL REFERENCES users(id),
  role_id    uuid        NOT NULL REFERENCES roles(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer     NOT NULL DEFAULT 1,
  created_by uuid        REFERENCES users(id)
);
-- +goose StatementEnd

CREATE UNIQUE INDEX user_roles_key ON user_roles (user_id, role_id) WHERE deleted_at IS NULL;
CREATE INDEX user_roles_role_id_idx ON user_roles (role_id) WHERE deleted_at IS NULL;
CREATE TRIGGER user_roles_touch BEFORE UPDATE ON user_roles
  FOR EACH ROW EXECUTE FUNCTION touch_row();

-- ---------------------------------------------------------------------------
-- Audit — the one documented exemption from the conventions
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
-- Append-only. No deleted_at, no version, no updated_at: an audit row is never
-- amended and never removed (docs/02-SCHEMA.md:123). Every mutation writes one,
-- inside the mutating transaction (docs/07-IMPLEMENTATION-PLAN.md I4).
CREATE TABLE audit_log (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  actor_id    uuid        REFERENCES users(id),
  action      text        NOT NULL,   -- create | update | delete | approve | login | ...
  resource    text        NOT NULL,   -- module key
  resource_id uuid,
  before      jsonb,
  after       jsonb,
  ip          inet,
  occurred_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

CREATE INDEX audit_log_resource_idx ON audit_log (resource, resource_id, occurred_at DESC);
CREATE INDEX audit_log_actor_idx    ON audit_log (actor_id, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- Product master — docs/02-SCHEMA.md §3
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
-- One table for everything the company stocks. Note the absence of any quantity
-- column: stock is derived from the movement ledger, never stored (CLAUDE.md §4.2).
CREATE TABLE items (
  id              uuid          PRIMARY KEY DEFAULT uuidv7(),
  sku             text          NOT NULL,
  item_type       text          NOT NULL CHECK (item_type IN ('finished_good','raw_material','packaging')),
  category        text,         -- 'juice' | 'jam' | 'paste' | 'water' | ...
  base_uom        text          NOT NULL,   -- 'bottle' | 'jar' | 'kg' | 'l' | 'pcs'
  shelf_life_days integer,
  min_qty         numeric(14,3),            -- reorder threshold, drives low-stock alerts
  is_active       boolean       NOT NULL DEFAULT true,
  -- status = 'active' IS the website publication state. A finished good appears in
  -- the public catalogue when and only when it is active. There is no separate flag.
  status          text          NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','archived')),
  created_at      timestamptz   NOT NULL DEFAULT now(),
  updated_at      timestamptz   NOT NULL DEFAULT now(),
  deleted_at      timestamptz,
  version         integer       NOT NULL DEFAULT 1,
  created_by      uuid          REFERENCES users(id)
);
-- +goose StatementEnd

CREATE UNIQUE INDEX items_sku_key ON items (sku) WHERE deleted_at IS NULL;
CREATE INDEX items_type_status_idx ON items (item_type, status) WHERE deleted_at IS NULL;
CREATE TRIGGER items_touch BEFORE UPDATE ON items FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- Translatable content lives in a sibling table, never in the parent row
-- (docs/02-SCHEMA.md:39). Falling back to 'ru' on a missing row is the frontend's job.
CREATE TABLE item_translations (
  id                 uuid        PRIMARY KEY DEFAULT uuidv7(),
  item_id            uuid        NOT NULL REFERENCES items(id),
  locale             text        NOT NULL CHECK (locale IN ('ru','tg','en')),
  name               text        NOT NULL,
  description        text,
  ingredients        text,       -- null renders 'уточняется' until lab-verified
  nutrition          text,
  storage_conditions text,
  after_opening      text,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  deleted_at         timestamptz,
  version            integer     NOT NULL DEFAULT 1,
  created_by         uuid        REFERENCES users(id)
);
-- +goose StatementEnd

CREATE UNIQUE INDEX item_translations_key
  ON item_translations (item_id, locale) WHERE deleted_at IS NULL;
CREATE TRIGGER item_translations_touch BEFORE UPDATE ON item_translations
  FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- A case is a unit, not a product. WAT-500 x 24 is a selling unit of WAT-500,
-- not a separate SKU (D8).
CREATE TABLE packaging_units (
  id          uuid          PRIMARY KEY DEFAULT uuidv7(),
  item_id     uuid          NOT NULL REFERENCES items(id),
  code        text          NOT NULL,   -- 'BOTTLE', 'CASE24', 'PALLET'
  qty_in_base numeric(14,3) NOT NULL,
  barcode     text,                     -- EAN-13; stays null pending register Q4
  created_at  timestamptz   NOT NULL DEFAULT now(),
  updated_at  timestamptz   NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer       NOT NULL DEFAULT 1,
  created_by  uuid          REFERENCES users(id)
);
-- +goose StatementEnd

CREATE UNIQUE INDEX packaging_units_key
  ON packaging_units (item_id, code) WHERE deleted_at IS NULL;
CREATE TRIGGER packaging_units_touch BEFORE UPDATE ON packaging_units
  FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- Money is numeric(14,2), base currency TJS, never float (CLAUDE.md §4.6).
CREATE TABLE item_prices (
  id         uuid          PRIMARY KEY DEFAULT uuidv7(),
  item_id    uuid          NOT NULL REFERENCES items(id),
  currency   text          NOT NULL DEFAULT 'TJS',
  amount     numeric(14,2) NOT NULL,
  valid_from date          NOT NULL,
  valid_to   date,
  created_at timestamptz   NOT NULL DEFAULT now(),
  updated_at timestamptz   NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer       NOT NULL DEFAULT 1,
  created_by uuid          REFERENCES users(id),
  CONSTRAINT item_prices_range_valid CHECK (valid_to IS NULL OR valid_to >= valid_from)
);
-- +goose StatementEnd

CREATE INDEX item_prices_item_idx ON item_prices (item_id, valid_from DESC) WHERE deleted_at IS NULL;
CREATE TRIGGER item_prices_touch BEFORE UPDATE ON item_prices
  FOR EACH ROW EXECUTE FUNCTION touch_row();

-- ---------------------------------------------------------------------------
-- Batches — the traceability spine, docs/02-SCHEMA.md §4
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
-- status is changed ONLY by quality events (§7), never directly by production or
-- warehouse staff. Enforcement lives in the domain layer and is covered by the
-- exhaustive transition matrix in T18.
CREATE TABLE batches (
  id           uuid        PRIMARY KEY DEFAULT uuidv7(),
  batch_no     text        NOT NULL,   -- 'B-2617'
  item_id      uuid        NOT NULL REFERENCES items(id),
  produced_on  date,
  expires_on   date,
  qr_payload   text,                   -- generated in the CRM, handed to the printer (D11)
  qr_issued_at timestamptz,
  status       text        NOT NULL DEFAULT 'in_production'
               CHECK (status IN ('in_production','quarantine','released','rejected')),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz,
  version      integer     NOT NULL DEFAULT 1,
  created_by   uuid        REFERENCES users(id),
  CONSTRAINT batches_expiry_after_production CHECK (expires_on IS NULL OR produced_on IS NULL OR expires_on >= produced_on)
);
-- +goose StatementEnd

CREATE UNIQUE INDEX batches_batch_no_key ON batches (batch_no) WHERE deleted_at IS NULL;
CREATE INDEX batches_item_status_idx ON batches (item_id, status) WHERE deleted_at IS NULL;
CREATE INDEX batches_expiry_idx ON batches (expires_on)
  WHERE deleted_at IS NULL AND expires_on IS NOT NULL;
CREATE TRIGGER batches_touch BEFORE UPDATE ON batches FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose Down

DROP TABLE IF EXISTS batches;
DROP TABLE IF EXISTS item_prices;
DROP TABLE IF EXISTS packaging_units;
DROP TABLE IF EXISTS item_translations;
DROP TABLE IF EXISTS items;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP FUNCTION IF EXISTS touch_row();
