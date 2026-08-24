-- CRM и продажи — customers, contacts, leads, deals and tasks.
--
-- The tables have existed since migration 00003 and were never queried. The
-- dashboard's Воронка продаж reads `deals` (inventory.sql:326), so the funnel has
-- rendered its empty state since the day it shipped.

-- name: ListCustomers :many
SELECT sqlc.embed(c),
  (SELECT count(*) FROM deals d
    WHERE d.customer_id = c.id AND d.deleted_at IS NULL
      AND d.stage NOT IN ('won','lost'))::int AS open_deals,
  (SELECT COALESCE(SUM(d.amount), 0) FROM deals d
    WHERE d.customer_id = c.id AND d.deleted_at IS NULL
      AND d.stage NOT IN ('won','lost'))::numeric(14,2) AS open_amount,
  -- COALESCE, not a bare cast: `::text` makes sqlc type the column
  -- non-nullable, and a customer with no leads then fails to scan. The empty
  -- string is the "no lead" case and the handler reads it as such.
  COALESCE((SELECT max(l.status) FROM leads l
    WHERE l.customer_id = c.id AND l.deleted_at IS NULL), '')::text AS lead_status
FROM customers c
WHERE c.deleted_at IS NULL
  AND (sqlc.narg(region)::text IS NULL OR c.region = sqlc.narg(region))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(c.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(c.contact,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(c.region,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY c.name
LIMIT $1 OFFSET $2;

-- name: CountCustomers :one
SELECT count(*) FROM customers c
WHERE c.deleted_at IS NULL
  AND (sqlc.narg(region)::text IS NULL OR c.region = sqlc.narg(region))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(c.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(c.contact,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(c.region,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: GetCustomer :one
SELECT * FROM customers WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateCustomer :one
UPDATE customers
SET name = $2, customer_type = $3, region = $4, contact = $5,
    updated_at = now(), version = version + 1
WHERE id = $1 AND deleted_at IS NULL AND version = $6
RETURNING *;

-- name: TombstoneCustomer :one
UPDATE customers SET deleted_at = now(), updated_at = now(), version = version + 1
WHERE id = $1 AND deleted_at IS NULL RETURNING *;

-- name: ListContactsForCustomer :many
SELECT * FROM contacts
WHERE customer_id = $1 AND deleted_at IS NULL
ORDER BY full_name;

-- name: CreateContact :one
INSERT INTO contacts (customer_id, full_name, role, email, phone, created_by)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: ListLeadsForCustomer :many
SELECT * FROM leads
WHERE customer_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: ListDeals :many
SELECT sqlc.embed(d), c.name AS customer_name, c.region AS customer_region,
  u.full_name AS owner_name
FROM deals d
JOIN customers c ON c.id = d.customer_id
LEFT JOIN users u ON u.id = d.owner_id
WHERE d.deleted_at IS NULL
  AND (sqlc.narg(stage)::text IS NULL OR d.stage = sqlc.narg(stage))
  AND (sqlc.narg(customer_id)::uuid IS NULL OR d.customer_id = sqlc.narg(customer_id))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(c.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(c.region,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY d.expected_close NULLS LAST, d.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountDeals :one
SELECT count(*) FROM deals d JOIN customers c ON c.id = d.customer_id
WHERE d.deleted_at IS NULL
  AND (sqlc.narg(stage)::text IS NULL OR d.stage = sqlc.narg(stage))
  AND (sqlc.narg(customer_id)::uuid IS NULL OR d.customer_id = sqlc.narg(customer_id))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(c.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(c.region,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: GetDeal :one
SELECT sqlc.embed(d), c.name AS customer_name, u.full_name AS owner_name
FROM deals d
JOIN customers c ON c.id = d.customer_id
LEFT JOIN users u ON u.id = d.owner_id
WHERE d.id = $1 AND d.deleted_at IS NULL;

-- name: CreateDeal :one
INSERT INTO deals (customer_id, amount, stage, owner_id, expected_close, created_by)
VALUES ($1, $2, COALESCE(sqlc.narg(stage)::text, 'new'), $3, $4, $5)
RETURNING *;

-- name: SetDealStage :one
UPDATE deals SET stage = $2, updated_at = now(), version = version + 1
WHERE id = $1 AND deleted_at IS NULL RETURNING *;

-- name: CreateDealStageEvent :one
-- Immutable evidence, exactly like batch_status_events: no version, no
-- deleted_at, so there is nothing here to edit.
INSERT INTO deal_stage_events (deal_id, from_stage, to_stage, changed_by, note)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ListDealStageEvents :many
SELECT e.*, u.full_name AS changed_by_name
FROM deal_stage_events e
LEFT JOIN users u ON u.id = e.changed_by
WHERE e.deal_id = $1
ORDER BY e.occurred_at DESC;

-- name: ListDealsForCustomer :many
SELECT sqlc.embed(d), u.full_name AS owner_name
FROM deals d LEFT JOIN users u ON u.id = d.owner_id
WHERE d.customer_id = $1 AND d.deleted_at IS NULL
ORDER BY d.created_at DESC;

-- name: ListSalesOrdersForCustomer :many
SELECT so.id, so.so_no, so.ordered_on, so.status,
  COALESCE((SELECT SUM(l.qty*l.unit_price) FROM sales_order_lines l
            WHERE l.sales_order_id = so.id AND l.deleted_at IS NULL), 0)::numeric(14,2) AS total
FROM sales_orders so
WHERE so.customer_id = $1 AND so.deleted_at IS NULL
ORDER BY so.ordered_on DESC NULLS LAST;

-- name: ListInquiriesForCustomer :many
-- An enquiry reaches a customer through the lead its conversion created.
SELECT i.id, i.reference_no, i.inquiry_type, i.status, i.created_at
FROM inquiries i
JOIN leads l ON l.inquiry_id = i.id AND l.deleted_at IS NULL
WHERE l.customer_id = $1 AND i.deleted_at IS NULL
ORDER BY i.created_at DESC;

-- name: ListTasks :many
SELECT t.*, u.full_name AS assignee_name
FROM tasks t LEFT JOIN users u ON u.id = t.assignee_id
WHERE t.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR t.status = sqlc.narg(status))
ORDER BY t.due_on NULLS LAST, t.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountTasks :one
SELECT count(*) FROM tasks
WHERE deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status));

-- name: CreateTask :one
INSERT INTO tasks (title, assignee_id, due_on, status, related_type, related_id, created_by)
VALUES ($1, $2, $3, 'open', $4, $5, $6) RETURNING *;

-- name: SetTaskStatus :one
UPDATE tasks SET status = $2, updated_at = now(), version = version + 1
WHERE id = $1 AND deleted_at IS NULL RETURNING *;

-- name: CountNewLeads :one
SELECT count(*) FROM leads WHERE status = 'new' AND deleted_at IS NULL;

-- name: CountOpenDeals :one
SELECT count(*) FROM deals
WHERE stage NOT IN ('won','lost') AND deleted_at IS NULL;

-- name: DealOutcomeCounts :one
-- Two counts rather than a computed rate.
--
-- The ratio is formed in Go so that "nothing has closed yet" can be NULL rather
-- than 0: on a sales dashboard those read very differently, and expressing that
-- in SQL means fighting sqlc's nullability inference through a cast.
SELECT
  count(*) FILTER (WHERE stage = 'won')::int AS won,
  count(*) FILTER (WHERE stage IN ('won','lost'))::int AS decided
FROM deals WHERE deleted_at IS NULL;
