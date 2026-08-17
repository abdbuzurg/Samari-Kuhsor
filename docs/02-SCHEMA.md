# 02 — Data Model

Authoritative schema for the Samari Kuhsor platform. Postgres. Migrations with goose, queries with
sqlc.

If an implementation detail is not covered here, follow the conventions in §1 rather than inventing
a new pattern.

---

## 1. Universal conventions

Every table, without exception:

```sql
id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),  -- prefer UUIDv7 if available
created_at  timestamptz NOT NULL DEFAULT now(),
updated_at  timestamptz NOT NULL DEFAULT now(),
deleted_at  timestamptz NULL,                                   -- tombstone; never hard-delete
version     integer     NOT NULL DEFAULT 1,                     -- increments on every update
created_by  uuid        NULL REFERENCES users(id)
```

Rules:

- **Never `SERIAL` or `bigserial`.** UUID keys exist so a second instance can create rows without
  collisions when sync arrives in phase 2.
- **Never hard-delete.** Set `deleted_at`. All reads filter `WHERE deleted_at IS NULL`.
- **Never store a computed balance.** See §5. There is no `quantity_on_hand` column anywhere.
- **Money:** `numeric(14,2)`. Base currency Somoni (TJS). Never float, never integer cents.
- **Quantities:** `numeric(14,3)`.
- **Enumerations** are Postgres `text` with a `CHECK` constraint, not native enums — they are
  easier to extend under migration.
- Partial unique indexes account for tombstones:
  `CREATE UNIQUE INDEX ... ON items (sku) WHERE deleted_at IS NULL;`

### Multilingual content

Translatable content lives in a sibling table, never in the parent row.

```sql
CREATE TABLE item_translations (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  item_id     uuid NOT NULL REFERENCES items(id),
  locale      text NOT NULL CHECK (locale IN ('ru','tg','en')),
  name        text NOT NULL,
  description text,
  ...
  UNIQUE (item_id, locale)
);
```

The same pattern applies to `content_block_translations` and `news_post_translations`. Falling back
to `ru` when a locale row is missing is the frontend's job.

**Two deliberate exceptions**, both short fixed-cardinality labels where a sibling table costs more
than it earns: `roles (name_ru, name_tg, name_en)` and `media (alt_ru, alt_tg, alt_en)`. Do not
extend this exception to anything else.

---

## 2. Identity and access

```sql
users (
  email            text NOT NULL,          -- unique where deleted_at IS NULL
  full_name        text NOT NULL,
  password_hash    text NOT NULL,          -- argon2id
  is_active        boolean NOT NULL DEFAULT true,
  last_login_at    timestamptz,
  failed_attempts  integer NOT NULL DEFAULT 0,
  locked_until     timestamptz
)

sessions (
  user_id     uuid NOT NULL REFERENCES users(id),
  token_hash  text NOT NULL,               -- store the hash, never the token
  expires_at  timestamptz NOT NULL,
  revoked_at  timestamptz,
  ip          inet,
  user_agent  text
)

roles (
  key          text NOT NULL,              -- 'admin', 'director', 'warehouse', ...
  name_ru      text NOT NULL,
  name_tg      text NOT NULL,
  name_en      text NOT NULL,
  is_system    boolean NOT NULL DEFAULT false  -- seed roles; cannot be deleted
)

role_permissions (
  role_id   uuid NOT NULL REFERENCES roles(id),
  resource  text NOT NULL,   -- module key, see 04-RBAC.md
  action    text NOT NULL CHECK (action IN ('read','manage','approve')),
  UNIQUE (role_id, resource, action)
)

user_roles (
  user_id  uuid NOT NULL REFERENCES users(id),
  role_id  uuid NOT NULL REFERENCES roles(id),
  UNIQUE (user_id, role_id)
)
```

A user's effective permissions are the union across all their roles.

### Audit log

```sql
audit_log (
  actor_id      uuid REFERENCES users(id),
  action        text NOT NULL,        -- 'create' | 'update' | 'delete' | 'approve' | 'login' | ...
  resource      text NOT NULL,        -- module key
  resource_id   uuid,
  before        jsonb,
  after         jsonb,
  ip            inet,
  occurred_at   timestamptz NOT NULL DEFAULT now()
)
```

`audit_log` is append-only and exempt from the tombstone convention. **Every mutation writes a
row.** Index on `(resource, resource_id, occurred_at DESC)` and `(actor_id, occurred_at DESC)`.

---

## 3. Product master

```sql
items (                             -- one table for everything the company stocks
  sku          text NOT NULL,       -- unique where not deleted
  item_type    text NOT NULL CHECK (item_type IN ('finished_good','raw_material','packaging')),
  category     text,                -- 'juice' | 'jam' | 'paste' | 'water' | ...
  base_uom     text NOT NULL,       -- 'bottle' | 'jar' | 'kg' | 'l' | 'pcs'
  shelf_life_days integer,
  min_qty      numeric(14,3),       -- reorder threshold; drives the low-stock alerts in §5
  is_active    boolean NOT NULL DEFAULT true,
  status       text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','archived'))
)
-- `status = 'active'` IS the website publication state. A finished good appears in the public
-- catalogue when and only when it is active. There is no separate publication flag.

item_translations (item_id, locale, name, description, ingredients,
                   nutrition, storage_conditions, after_opening)

packaging_units (                   -- a case is a unit, not a product
  item_id        uuid NOT NULL REFERENCES items(id),
  code           text NOT NULL,     -- 'BOTTLE', 'CASE24', 'PALLET'
  qty_in_base    numeric(14,3) NOT NULL,
  barcode        text,              -- EAN-13, pending register Q4
  UNIQUE (item_id, code)
)

item_prices (
  item_id     uuid NOT NULL REFERENCES items(id),
  currency    text NOT NULL DEFAULT 'TJS',
  amount      numeric(14,2) NOT NULL,
  valid_from  date NOT NULL,
  valid_to    date
)
```

### Seed — the five real products

Only these five finished goods exist. See `01-DECISIONS.md` D8.

| SKU | `category` | `base_uom` | RU name |
|---|---|---|---|
| `APJ-1000` | juice | bottle | Яблочный сок прямого отжима, 1 000 мл, стекло |
| `APR-220` | jam | jar | Абрикосовый джем, 212–228 мл, стекло |
| `TOM-500` | paste | jar | Томатная паста, 500 мл, стекло |
| `WAT-500` | water | bottle | Негазированная питьевая вода 0,5 л, ПЭТ |
| `WAT-1000` | water | bottle | Негазированная питьевая вода 1 л, ПЭТ |

Compositions, nutritional values, shelf life and water classification are **`уточняется`** until
the client's recipes are approved and lab-verified. The client set this rule explicitly: the system
must not publish unverified claims. Leave these fields null and render the placeholder.

---

## 4. Batches and traceability

```sql
batches (
  batch_no       text NOT NULL,     -- 'B-2617', unique where not deleted
  item_id        uuid NOT NULL REFERENCES items(id),
  produced_on    date,
  expires_on     date,
  qr_payload     text,              -- generated in the CRM, handed to the wrapper printer
  qr_issued_at   timestamptz,
  status         text NOT NULL DEFAULT 'in_production'
                 CHECK (status IN ('in_production','quarantine','released','rejected'))
)
```

`batches.status` is the traceability spine. It is changed **only** by quality events (§7), never
directly by production or warehouse staff.

---

## 5. Inventory — append-only ledger

**This is the most important table in the system.** There is no stock balance column anywhere.

```sql
locations (
  code       text NOT NULL,         -- 'A-12', 'C-07', 'D-01'
  name       text NOT NULL,
  zone       text NOT NULL CHECK (zone IN
             ('finished_goods','raw','packaging','quarantine'))  -- mirrors items.item_type
)

stock_movements (
  item_id       uuid NOT NULL REFERENCES items(id),
  batch_id      uuid REFERENCES batches(id),
  location_id   uuid NOT NULL REFERENCES locations(id),
  qty_delta     numeric(14,3) NOT NULL,   -- signed: +receipt, −issue. NEVER an absolute total.
  reason        text NOT NULL CHECK (reason IN
                  ('goods_receipt','production_output','material_issue','sale','transfer',
                   'adjustment','scrap','return')),
  ref_type      text,                     -- 'purchase_order' | 'manufacturing_order' | ...
  ref_id        uuid,
  occurred_at   timestamptz NOT NULL DEFAULT now()
)
```

Balance is always derived:

```sql
SELECT item_id, batch_id, location_id, SUM(qty_delta) AS on_hand
FROM stock_movements
WHERE deleted_at IS NULL
GROUP BY item_id, batch_id, location_id;
```

Expose this as a materialised view `stock_balances`, refreshed on write or on a short schedule.
The materialised view is a cache; `stock_movements` is the truth.

**Corrections are compensating entries, never edits.** To fix a wrong receipt of 100, insert −100
with `reason = 'adjustment'`. Never update or tombstone the original.

A `transfer` is two rows: negative at source, positive at destination, sharing a `ref_id`.

Low-stock and expiry alerts come from `stock_balances` joined to `items.min_qty` and
`batches.expires_on`.

---

## 6. Production

```sql
manufacturing_orders (
  mo_no        text NOT NULL,         -- 'MO-0612'
  item_id      uuid NOT NULL REFERENCES items(id),
  batch_id     uuid REFERENCES batches(id),   -- 1:1 with the batch it produces
  line         text,                  -- 'Линия №1'
  planned_qty  numeric(14,3) NOT NULL,
  scheduled_for date,
  status       text NOT NULL DEFAULT 'planned'
               CHECK (status IN ('planned','in_progress','qc_hold','done','cancelled'))
)

production_entries (                  -- append-only, like stock
  mo_id        uuid NOT NULL REFERENCES manufacturing_orders(id),
  good_qty     numeric(14,3) NOT NULL DEFAULT 0,
  scrap_qty    numeric(14,3) NOT NULL DEFAULT 0,
  downtime_min integer NOT NULL DEFAULT 0,
  recorded_at  timestamptz NOT NULL DEFAULT now(),
  recorded_by  uuid REFERENCES users(id)
)
```

Actual output, yield and downtime are **sums over `production_entries`**, never columns on the
order. Confirmed output posts a `production_output` movement into `stock_movements` against a
quarantine location.

---

## 7. Quality

```sql
quality_tests (
  batch_id     uuid NOT NULL REFERENCES batches(id),
  test_type    text NOT NULL,      -- 'ph' | 'microbiology' | 'brix' | 'viscosity'
                                   -- | 'metal_detection' | 'organoleptic'
  result_value text,
  passed       boolean,
  inspector_id uuid REFERENCES users(id),
  tested_at    timestamptz NOT NULL DEFAULT now(),
  notes        text
)

batch_status_events (               -- append-only; drives batches.status
  batch_id     uuid NOT NULL REFERENCES batches(id),
  from_status  text,
  to_status    text NOT NULL,
  decided_by   uuid NOT NULL REFERENCES users(id),
  reason       text,
  occurred_at  timestamptz NOT NULL DEFAULT now()
)
```

Transition rules, enforced in Go and covered by exhaustive tests:

```
in_production → quarantine     automatic on production completion
quarantine    → released       requires quality:approve
quarantine    → rejected       requires quality:approve
released      → rejected       recall — requires quality:approve, reason mandatory
rejected      → (terminal)
```

A recall is modelled as `released → rejected` with a mandatory reason, not as a separate status.

**Only `released` batches may be sold or shipped.** Enforce this in the sales and logistics domains,
not only in the UI. Releasing writes to `audit_log` with the deciding user — this is the evidence
trail behind the website's laboratory-control claim.

---

## 8. Procurement, logistics, sales, HR, equipment, documents

Same conventions throughout; abbreviated because the patterns repeat.

```
suppliers            name, tax_id, contact, region, rating
purchase_orders      po_no, supplier_id, expected_at, status
                     ('draft','approval','confirmed','in_transit','receiving','closed','cancelled')
purchase_order_lines po_id, item_id, qty, unit_price
goods_receipts       po_id, received_at, received_by   → posts goods_receipt movements

shipments            trip_no, route_from, route_to, driver_id, vehicle_id,
                     transport_cost numeric(14,2),   -- operational cost, NOT a finance record
                     status ('planned','loading','in_transit','delivered','delayed','cancelled')
shipment_lines       shipment_id, item_id, batch_id, qty   -- batch must be 'released'
vehicles             plate, model, capacity
drivers              employee_id, licence_no

customers            name, customer_type, region, contact
leads                customer_id, inquiry_id NULL REFERENCES inquiries(id), source, status
                     ('new','negotiation','quoted','won','lost')
deals                customer_id, amount, stage, owner_id, expected_close
sales_orders         so_no, customer_id, ordered_on, status
                     ('draft','confirmed','picking','shipped','closed','cancelled')
sales_order_lines    sales_order_id, item_id, batch_id, qty, unit_price
                     -- batch must be 'released'; confirming an order posts 'sale' movements
inquiries            reference_no, inquiry_type, name, company, contact, message,
                     batch_id NULL REFERENCES batches(id),   -- complaints link to a batch
                     status
                     -- reference_no prefix by type: WR- wholesale, CF- contact,
                     -- DA- distributor, CP- complaint, JB- job
                     -- status: 'new' → 'lead_created' → 'closed'

departments          name
positions            department_id, title
employees            full_name, position_id, shift, hired_on, contract_until, status
                     -- shift: 'day' | 'night' | 'rotating'
                     -- status: 'active' | 'on_leave' | 'suspended' | 'terminated'

assets               asset_no, name, asset_type, line, commissioned_on,
                     warranty_until, status ('running','maintenance_due','broken','retired')
maintenance_events   asset_id, event_type, performed_at, next_due_on, notes

documents            doc_no, doc_type, owner_id, valid_until,
                     status ('draft','approval','active','expiring','expired','archived')
document_versions    document_id, version_no, file_path, uploaded_by, uploaded_at
```

`inquiries` is written by the **public website** through the same backend, and read by the CRM's
Интеграция с сайтом module. Every submission returns its `reference_no` to the visitor — a ToR
requirement.

---

## 9. CMS — the website content model

The CRM edits this; the website reads it. This is the thirteenth module and the reason `apps/web`
is a Next.js application rather than a static build.

```sql
content_pages (
  key           text NOT NULL,     -- 'home','products','production','contacts'
  status        text NOT NULL DEFAULT 'draft'
                CHECK (status IN ('draft','technical_review','language_review','approved','published')),
  published_at  timestamptz,
  published_by  uuid REFERENCES users(id)
)

content_blocks (
  page_id     uuid NOT NULL REFERENCES content_pages(id),
  block_key   text NOT NULL,       -- 'hero','trust_strip','eco','cta'
  sort_order  integer NOT NULL
)

content_block_translations (block_id, locale, heading, body, cta_label, ...)

news_posts (slug, category, published_on, cover_media_id, status /* same ladder */)
news_post_translations (post_id, locale, title, excerpt, body)

media (
  file_path   text NOT NULL,
  mime_type   text NOT NULL,
  width       integer,
  height      integer,
  alt_ru      text, alt_tg text, alt_en text,
  uploaded_by uuid REFERENCES users(id)
)

content_workflow_events (
  entity_type text NOT NULL,   -- 'content_page' | 'news_post'
  entity_id   uuid NOT NULL,
  from_status text,
  to_status   text NOT NULL,
  actor_id    uuid NOT NULL REFERENCES users(id),
  comment     text,
  occurred_at timestamptz NOT NULL DEFAULT now()
)
```

The publishing ladder is a ToR requirement:

```
draft → technical_review → language_review → approved → published
```

Moving to `approved` or `published` requires `cms:approve`. The public site renders only
`published` content; the CRM can preview any state.

---

## 10. Finance — stubs only

Tables may be created so nothing needs restructuring later, but **no finance logic or UI ships for
9 September**. See `01-DECISIONS.md` D2 and register question Q2.

```
budgets              department_id, period, amount
expense_requests     request_no, department_id, purpose, amount, status
invoices             invoice_no, customer_id, amount, due_on, status
cash_movements       append-only ledger, identical pattern to stock_movements
```

When finance is built, cash follows the **same ledger rule as stock**: movements, never balances.

---

## 11. Migration discipline

- Migrations are sequential and immutable once applied. To change something, write a new migration.
- Every migration has a working `-- +goose Down`.
- Seed data lives in a separate, idempotent seed command — not in migrations.
- After changing anything in `queries/`, regenerate with `sqlc generate` and commit the output.
  Never hand-edit generated code in `internal/db/`.
