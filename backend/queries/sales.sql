-- CRM и продажи, Логистика. Both enforce the released-batch rule.

-- name: CreateSalesOrder :one
INSERT INTO sales_orders (so_no, customer_id, ordered_on, created_by)
VALUES ($1,$2,$3,$4) RETURNING *;

-- name: GetSalesOrder :one
SELECT * FROM sales_orders WHERE id=$1 AND deleted_at IS NULL;

-- name: SetSalesOrderStatus :one
UPDATE sales_orders SET status=sqlc.arg(to_status)
WHERE id=$1 AND status=sqlc.arg(from_status) AND deleted_at IS NULL RETURNING *;

-- name: CreateSalesOrderLine :one
INSERT INTO sales_order_lines (sales_order_id, item_id, batch_id, qty, unit_price, created_by)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: ListSalesOrderLines :many
SELECT l.*, i.sku, COALESCE(tr.name, i.sku) AS item_name, b.batch_no, b.status AS batch_status
FROM sales_order_lines l
JOIN items i ON i.id=l.item_id
LEFT JOIN batches b ON b.id=l.batch_id
LEFT JOIN item_translations tr
  ON tr.item_id = i.id AND tr.locale = 'ru' AND tr.deleted_at IS NULL
WHERE l.sales_order_id=$1 AND l.deleted_at IS NULL ORDER BY i.sku;

-- name: ListSalesOrders :many
SELECT sqlc.embed(so), c.name AS customer_name,
  COALESCE((SELECT SUM(l.qty*l.unit_price) FROM sales_order_lines l
            WHERE l.sales_order_id=so.id AND l.deleted_at IS NULL),0)::numeric AS total_amount
FROM sales_orders so JOIN customers c ON c.id=so.customer_id
WHERE so.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR so.status=sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL OR unaccent(lower(so.so_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(c.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY so.ordered_on DESC NULLS LAST, so.so_no DESC LIMIT $1 OFFSET $2;

-- name: CountSalesOrders :one
SELECT count(*) FROM sales_orders so JOIN customers c ON c.id=so.customer_id
WHERE so.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR so.status=sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL OR unaccent(lower(so.so_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(c.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: SumConfirmedSalesRevenue :one
-- The dashboard's Выручка. Sourced from CONFIRMED sales orders only
-- (docs/05-MODULES.md:65) — there is no finance module to ask.
SELECT COALESCE(SUM(l.qty*l.unit_price),0)::numeric
FROM sales_orders so JOIN sales_order_lines l ON l.sales_order_id=so.id
WHERE so.deleted_at IS NULL AND l.deleted_at IS NULL
  AND so.status IN ('confirmed','picking','shipped','closed');

-- name: CreateShipment :one
INSERT INTO shipments (trip_no, route_from, route_to, driver_id, vehicle_id, transport_cost, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *;

-- name: GetShipment :one
SELECT * FROM shipments WHERE id=$1 AND deleted_at IS NULL;

-- name: SetShipmentStatus :one
UPDATE shipments SET status=sqlc.arg(to_status)
WHERE id=$1 AND status=sqlc.arg(from_status) AND deleted_at IS NULL RETURNING *;

-- name: CreateShipmentLine :one
INSERT INTO shipment_lines (shipment_id, item_id, batch_id, qty, created_by)
VALUES ($1,$2,$3,$4,$5) RETURNING *;

-- name: ListShipmentLines :many
SELECT l.*, i.sku, COALESCE(tr.name, i.sku) AS item_name, b.batch_no, b.status AS batch_status
FROM shipment_lines l JOIN items i ON i.id=l.item_id JOIN batches b ON b.id=l.batch_id
LEFT JOIN item_translations tr
  ON tr.item_id = i.id AND tr.locale = 'ru' AND tr.deleted_at IS NULL
WHERE l.shipment_id=$1 AND l.deleted_at IS NULL ORDER BY i.sku;

-- name: ListShipments :many
SELECT sqlc.embed(s), e.full_name AS driver_name, v.plate AS vehicle_plate
FROM shipments s
LEFT JOIN employees e ON e.id=s.driver_id
LEFT JOIN vehicles v ON v.id=s.vehicle_id
WHERE s.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR s.status=sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL OR unaccent(lower(s.trip_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY s.created_at DESC LIMIT $1 OFFSET $2;

-- name: CountShipments :one
SELECT count(*) FROM shipments WHERE deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR status=sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL OR unaccent(lower(trip_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: CountOverdueTasks :one
SELECT count(*)::int FROM tasks
WHERE deleted_at IS NULL AND status='open' AND due_on < CURRENT_DATE;
