-- Commerce and administration: procurement, logistics, sales, inquiries, HR,
-- equipment, documents. docs/02-SCHEMA.md §8.
--
-- Grouped into one migration rather than seven because these tables reference
-- each other, and seven migrations that must be applied in order are seven
-- chances to apply them out of order.

-- +goose Up

-- ---------------------------------------------------------------------------
-- Procurement
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE suppliers (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  name       text        NOT NULL,
  tax_id     text,
  contact    text,
  region     text,
  rating     integer     CHECK (rating IS NULL OR rating BETWEEN 1 AND 5),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer     NOT NULL DEFAULT 1,
  created_by uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX suppliers_name_idx ON suppliers (name) WHERE deleted_at IS NULL;
CREATE TRIGGER suppliers_touch BEFORE UPDATE ON suppliers FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE purchase_orders (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  po_no       text        NOT NULL,
  supplier_id uuid        NOT NULL REFERENCES suppliers(id),
  expected_at date,
  status      text        NOT NULL DEFAULT 'draft' CHECK (status IN
                ('draft','approval','confirmed','in_transit','receiving','closed','cancelled')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer     NOT NULL DEFAULT 1,
  created_by  uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX purchase_orders_no_key ON purchase_orders (po_no) WHERE deleted_at IS NULL;
CREATE INDEX purchase_orders_status_idx ON purchase_orders (status) WHERE deleted_at IS NULL;
CREATE TRIGGER purchase_orders_touch BEFORE UPDATE ON purchase_orders FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE purchase_order_lines (
  id         uuid          PRIMARY KEY DEFAULT uuidv7(),
  po_id      uuid          NOT NULL REFERENCES purchase_orders(id),
  item_id    uuid          NOT NULL REFERENCES items(id),
  qty        numeric(14,3) NOT NULL CHECK (qty > 0),
  unit_price numeric(14,2) NOT NULL CHECK (unit_price >= 0),
  created_at timestamptz   NOT NULL DEFAULT now(),
  updated_at timestamptz   NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer       NOT NULL DEFAULT 1,
  created_by uuid          REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX purchase_order_lines_po_idx ON purchase_order_lines (po_id) WHERE deleted_at IS NULL;
CREATE TRIGGER purchase_order_lines_touch BEFORE UPDATE ON purchase_order_lines FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE goods_receipts (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  po_id       uuid        NOT NULL REFERENCES purchase_orders(id),
  location_id uuid        NOT NULL REFERENCES locations(id),
  received_at timestamptz NOT NULL DEFAULT now(),
  received_by uuid        REFERENCES users(id),
  note        text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer     NOT NULL DEFAULT 1,
  created_by  uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX goods_receipts_po_idx ON goods_receipts (po_id) WHERE deleted_at IS NULL;
CREATE TRIGGER goods_receipts_touch BEFORE UPDATE ON goods_receipts FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE goods_receipt_lines (
  id          uuid          PRIMARY KEY DEFAULT uuidv7(),
  receipt_id  uuid          NOT NULL REFERENCES goods_receipts(id),
  po_line_id  uuid          NOT NULL REFERENCES purchase_order_lines(id),
  qty         numeric(14,3) NOT NULL CHECK (qty > 0),
  created_at  timestamptz   NOT NULL DEFAULT now(),
  updated_at  timestamptz   NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer       NOT NULL DEFAULT 1,
  created_by  uuid          REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX goods_receipt_lines_receipt_idx ON goods_receipt_lines (receipt_id) WHERE deleted_at IS NULL;
CREATE TRIGGER goods_receipt_lines_touch BEFORE UPDATE ON goods_receipt_lines FOR EACH ROW EXECUTE FUNCTION touch_row();

-- ---------------------------------------------------------------------------
-- Logistics
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE vehicles (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  plate      text        NOT NULL,
  model      text,
  capacity   numeric(14,3),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer     NOT NULL DEFAULT 1,
  created_by uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX vehicles_plate_key ON vehicles (plate) WHERE deleted_at IS NULL;
CREATE TRIGGER vehicles_touch BEFORE UPDATE ON vehicles FOR EACH ROW EXECUTE FUNCTION touch_row();

-- ---------------------------------------------------------------------------
-- HR — contains personal data (docs/05-MODULES.md:224)
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE departments (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  name       text        NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer     NOT NULL DEFAULT 1,
  created_by uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE TRIGGER departments_touch BEFORE UPDATE ON departments FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE positions (
  id            uuid        PRIMARY KEY DEFAULT uuidv7(),
  department_id uuid        NOT NULL REFERENCES departments(id),
  title         text        NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  deleted_at    timestamptz,
  version       integer     NOT NULL DEFAULT 1,
  created_by    uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE TRIGGER positions_touch BEFORE UPDATE ON positions FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE employees (
  id             uuid        PRIMARY KEY DEFAULT uuidv7(),
  full_name      text        NOT NULL,
  position_id    uuid        REFERENCES positions(id),
  shift          text        CHECK (shift IS NULL OR shift IN ('day','night','rotating')),
  hired_on       date,
  contract_until date,
  status         text        NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','on_leave','suspended','terminated')),
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  deleted_at     timestamptz,
  version        integer     NOT NULL DEFAULT 1,
  created_by     uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX employees_contract_idx ON employees (contract_until)
  WHERE deleted_at IS NULL AND contract_until IS NOT NULL;
CREATE TRIGGER employees_touch BEFORE UPDATE ON employees FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE drivers (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  employee_id uuid        NOT NULL REFERENCES employees(id),
  licence_no  text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer     NOT NULL DEFAULT 1,
  created_by  uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE TRIGGER drivers_touch BEFORE UPDATE ON drivers FOR EACH ROW EXECUTE FUNCTION touch_row();

-- ---------------------------------------------------------------------------
-- Sales and CRM
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE customers (
  id            uuid        PRIMARY KEY DEFAULT uuidv7(),
  name          text        NOT NULL,
  customer_type text,
  region        text,       -- Душанбе, Худжанд, Хорог, Бохтар
  contact       text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  deleted_at    timestamptz,
  version       integer     NOT NULL DEFAULT 1,
  created_by    uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX customers_name_idx ON customers (name) WHERE deleted_at IS NULL;
CREATE TRIGGER customers_touch BEFORE UPDATE ON customers FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- docs/07-IMPLEMENTATION-PLAN.md C6/I16 — referenced by the detail view but
-- missing from 02-SCHEMA.md.
CREATE TABLE contacts (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  customer_id uuid        NOT NULL REFERENCES customers(id),
  full_name   text        NOT NULL,
  role        text,
  email       text,
  phone       text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer     NOT NULL DEFAULT 1,
  created_by  uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX contacts_customer_idx ON contacts (customer_id) WHERE deleted_at IS NULL;
CREATE TRIGGER contacts_touch BEFORE UPDATE ON contacts FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- Written by the PUBLIC WEBSITE through the same backend (docs/02-SCHEMA.md:370).
-- Every submission returns its reference_no to the visitor — a ToR requirement.
CREATE TABLE inquiries (
  id           uuid        PRIMARY KEY DEFAULT uuidv7(),
  reference_no text        NOT NULL,
  -- Prefix by type: WR- wholesale, CF- contact, DA- distributor,
  -- CP- complaint, JB- job.
  inquiry_type text        NOT NULL CHECK (inquiry_type IN
                 ('wholesale','contact','distributor','complaint','job')),
  name         text        NOT NULL,
  company      text,
  contact      text        NOT NULL,
  message      text,
  -- Complaints link to a batch so the ToR's complaint→traceability workflow is
  -- possible (docs/05-MODULES.md:166).
  batch_id     uuid        REFERENCES batches(id),
  status       text        NOT NULL DEFAULT 'new' CHECK (status IN ('new','lead_created','closed')),
  source_ip    inet,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz,
  version      integer     NOT NULL DEFAULT 1,
  created_by   uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX inquiries_reference_key ON inquiries (reference_no) WHERE deleted_at IS NULL;
CREATE INDEX inquiries_status_idx ON inquiries (status, created_at DESC) WHERE deleted_at IS NULL;
CREATE TRIGGER inquiries_touch BEFORE UPDATE ON inquiries FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE leads (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  customer_id uuid        REFERENCES customers(id),
  inquiry_id  uuid        REFERENCES inquiries(id),
  source      text,
  status      text        NOT NULL DEFAULT 'new'
              CHECK (status IN ('new','negotiation','quoted','won','lost')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer     NOT NULL DEFAULT 1,
  created_by  uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX leads_status_idx ON leads (status) WHERE deleted_at IS NULL;
CREATE TRIGGER leads_touch BEFORE UPDATE ON leads FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE deals (
  id             uuid          PRIMARY KEY DEFAULT uuidv7(),
  customer_id    uuid          NOT NULL REFERENCES customers(id),
  amount         numeric(14,2),
  stage          text          NOT NULL DEFAULT 'new'
                 CHECK (stage IN ('new','negotiation','quoted','won','lost')),
  owner_id       uuid          REFERENCES users(id),
  expected_close date,
  created_at     timestamptz   NOT NULL DEFAULT now(),
  updated_at     timestamptz   NOT NULL DEFAULT now(),
  deleted_at     timestamptz,
  version        integer       NOT NULL DEFAULT 1,
  created_by     uuid          REFERENCES users(id)
);
-- +goose StatementEnd
CREATE TRIGGER deals_touch BEFORE UPDATE ON deals FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- History as append-only events, mirroring batch_status_events. docs/07 I16:
-- consistency with the house pattern is worth more than the table costs.
CREATE TABLE deal_stage_events (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  deal_id     uuid        NOT NULL REFERENCES deals(id),
  from_stage  text,
  to_stage    text        NOT NULL,
  changed_by  uuid        NOT NULL REFERENCES users(id),
  note        text,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  created_at  timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd
CREATE INDEX deal_stage_events_deal_idx ON deal_stage_events (deal_id, occurred_at DESC);

-- +goose StatementBegin
CREATE TABLE sales_orders (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  so_no       text        NOT NULL,
  customer_id uuid        NOT NULL REFERENCES customers(id),
  ordered_on  date,
  status      text        NOT NULL DEFAULT 'draft'
              CHECK (status IN ('draft','confirmed','picking','shipped','closed','cancelled')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer     NOT NULL DEFAULT 1,
  created_by  uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX sales_orders_no_key ON sales_orders (so_no) WHERE deleted_at IS NULL;
CREATE TRIGGER sales_orders_touch BEFORE UPDATE ON sales_orders FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE sales_order_lines (
  id             uuid          PRIMARY KEY DEFAULT uuidv7(),
  sales_order_id uuid          NOT NULL REFERENCES sales_orders(id),
  item_id        uuid          NOT NULL REFERENCES items(id),
  -- The batch must be 'released' before the order is confirmed
  -- (docs/02-SCHEMA.md:346). Enforced in the domain: a CHECK cannot see another
  -- table's status.
  batch_id       uuid          REFERENCES batches(id),
  qty            numeric(14,3) NOT NULL CHECK (qty > 0),
  unit_price     numeric(14,2) NOT NULL CHECK (unit_price >= 0),
  created_at     timestamptz   NOT NULL DEFAULT now(),
  updated_at     timestamptz   NOT NULL DEFAULT now(),
  deleted_at     timestamptz,
  version        integer       NOT NULL DEFAULT 1,
  created_by     uuid          REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX sales_order_lines_order_idx ON sales_order_lines (sales_order_id) WHERE deleted_at IS NULL;
CREATE TRIGGER sales_order_lines_touch BEFORE UPDATE ON sales_order_lines FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE shipments (
  id             uuid          PRIMARY KEY DEFAULT uuidv7(),
  trip_no        text          NOT NULL,
  route_from     text,
  route_to       text,
  driver_id      uuid          REFERENCES drivers(id),
  vehicle_id     uuid          REFERENCES vehicles(id),
  -- An operational cost, NOT a finance record (docs/02-SCHEMA.md:334). Finance is
  -- deferred (D2) and this must not become the thin end of it.
  transport_cost numeric(14,2),
  status         text          NOT NULL DEFAULT 'planned'
                 CHECK (status IN ('planned','loading','in_transit','delivered','delayed','cancelled')),
  departed_at    timestamptz,
  delivered_at   timestamptz,
  created_at     timestamptz   NOT NULL DEFAULT now(),
  updated_at     timestamptz   NOT NULL DEFAULT now(),
  deleted_at     timestamptz,
  version        integer       NOT NULL DEFAULT 1,
  created_by     uuid          REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX shipments_trip_key ON shipments (trip_no) WHERE deleted_at IS NULL;
CREATE INDEX shipments_status_idx ON shipments (status) WHERE deleted_at IS NULL;
CREATE TRIGGER shipments_touch BEFORE UPDATE ON shipments FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE shipment_lines (
  id          uuid          PRIMARY KEY DEFAULT uuidv7(),
  shipment_id uuid          NOT NULL REFERENCES shipments(id),
  item_id     uuid          NOT NULL REFERENCES items(id),
  -- Must be 'released'. Enforced server-side in the domain (docs/05-MODULES.md:212).
  batch_id    uuid          NOT NULL REFERENCES batches(id),
  qty         numeric(14,3) NOT NULL CHECK (qty > 0),
  created_at  timestamptz   NOT NULL DEFAULT now(),
  updated_at  timestamptz   NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer       NOT NULL DEFAULT 1,
  created_by  uuid          REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX shipment_lines_shipment_idx ON shipment_lines (shipment_id) WHERE deleted_at IS NULL;
CREATE TRIGGER shipment_lines_touch BEFORE UPDATE ON shipment_lines FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- docs/07-IMPLEMENTATION-PLAN.md C6/I16 — the CRM KPI "Просроченные задачи".
CREATE TABLE tasks (
  id           uuid        PRIMARY KEY DEFAULT uuidv7(),
  title        text        NOT NULL,
  assignee_id  uuid        REFERENCES users(id),
  due_on       date,
  status       text        NOT NULL DEFAULT 'open' CHECK (status IN ('open','done','cancelled')),
  related_type text,
  related_id   uuid,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz,
  version      integer     NOT NULL DEFAULT 1,
  created_by   uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX tasks_due_idx ON tasks (due_on) WHERE deleted_at IS NULL AND status = 'open';
CREATE TRIGGER tasks_touch BEFORE UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION touch_row();

-- ---------------------------------------------------------------------------
-- Equipment
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE assets (
  id              uuid        PRIMARY KEY DEFAULT uuidv7(),
  asset_no        text        NOT NULL,   -- 'EQ-047'
  name            text        NOT NULL,
  asset_type      text,
  line            text,
  commissioned_on date,
  warranty_until  date,
  status          text        NOT NULL DEFAULT 'running'
                  CHECK (status IN ('running','maintenance_due','broken','retired')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  deleted_at      timestamptz,
  version         integer     NOT NULL DEFAULT 1,
  created_by      uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX assets_no_key ON assets (asset_no) WHERE deleted_at IS NULL;
CREATE TRIGGER assets_touch BEFORE UPDATE ON assets FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE maintenance_events (
  id           uuid        PRIMARY KEY DEFAULT uuidv7(),
  asset_id     uuid        NOT NULL REFERENCES assets(id),
  event_type   text,
  performed_at timestamptz,
  next_due_on  date,
  notes        text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz,
  version      integer     NOT NULL DEFAULT 1,
  created_by   uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX maintenance_events_asset_idx ON maintenance_events (asset_id) WHERE deleted_at IS NULL;
CREATE INDEX maintenance_events_due_idx ON maintenance_events (next_due_on)
  WHERE deleted_at IS NULL AND next_due_on IS NOT NULL;
CREATE TRIGGER maintenance_events_touch BEFORE UPDATE ON maintenance_events FOR EACH ROW EXECUTE FUNCTION touch_row();

-- ---------------------------------------------------------------------------
-- Documents — controlled, versioned, never deleted
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE documents (
  id           uuid        PRIMARY KEY DEFAULT uuidv7(),
  doc_no       text        NOT NULL,
  title        text        NOT NULL,
  doc_type     text,
  owner_id     uuid        REFERENCES users(id),
  valid_until  date,
  status       text        NOT NULL DEFAULT 'draft' CHECK (status IN
                 ('draft','approval','active','expiring','expired','archived')),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz,
  version      integer     NOT NULL DEFAULT 1,
  created_by   uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX documents_no_key ON documents (doc_no) WHERE deleted_at IS NULL;
CREATE INDEX documents_valid_idx ON documents (valid_until)
  WHERE deleted_at IS NULL AND valid_until IS NOT NULL;
CREATE TRIGGER documents_touch BEFORE UPDATE ON documents FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- Superseded versions are RETAINED, never deleted — the archive-rather-than-delete
-- rule from the ToR (docs/05-MODULES.md:247). file_path points into the
-- documents/ upload tree, which is unreachable by any static path (I17).
CREATE TABLE document_versions (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  document_id uuid        NOT NULL REFERENCES documents(id),
  version_no  integer     NOT NULL,
  file_path   text        NOT NULL,
  mime_type   text,
  size_bytes  bigint,
  uploaded_by uuid        REFERENCES users(id),
  uploaded_at timestamptz NOT NULL DEFAULT now(),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer     NOT NULL DEFAULT 1,
  created_by  uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX document_versions_key
  ON document_versions (document_id, version_no) WHERE deleted_at IS NULL;
CREATE TRIGGER document_versions_touch BEFORE UPDATE ON document_versions FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose Down

DROP TABLE IF EXISTS document_versions;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS maintenance_events;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS shipment_lines;
DROP TABLE IF EXISTS shipments;
DROP TABLE IF EXISTS sales_order_lines;
DROP TABLE IF EXISTS sales_orders;
DROP TABLE IF EXISTS deal_stage_events;
DROP TABLE IF EXISTS deals;
DROP TABLE IF EXISTS leads;
DROP TABLE IF EXISTS inquiries;
DROP TABLE IF EXISTS contacts;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS drivers;
DROP TABLE IF EXISTS employees;
DROP TABLE IF EXISTS positions;
DROP TABLE IF EXISTS departments;
DROP TABLE IF EXISTS vehicles;
DROP TABLE IF EXISTS goods_receipt_lines;
DROP TABLE IF EXISTS goods_receipts;
DROP TABLE IF EXISTS purchase_order_lines;
DROP TABLE IF EXISTS purchase_orders;
DROP TABLE IF EXISTS suppliers;
