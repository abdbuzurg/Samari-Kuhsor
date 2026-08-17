-- Закупки и поставщики — docs/05-MODULES.md §10.
-- Goods receipt posts goods_receipt movements: this is how raw material enters
-- inventory, and the two must happen in one transaction.

-- name: CreateSupplier :one
INSERT INTO suppliers (name, tax_id, contact, region, rating, created_by)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: GetSupplier :one
SELECT * FROM suppliers WHERE id=$1 AND deleted_at IS NULL;

-- name: ListSuppliers :many
SELECT * FROM suppliers WHERE deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL OR unaccent(lower(name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY name LIMIT $1 OFFSET $2;

-- name: CountSuppliers :one
SELECT count(*) FROM suppliers WHERE deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL OR unaccent(lower(name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: CreatePurchaseOrder :one
INSERT INTO purchase_orders (po_no, supplier_id, expected_at, created_by)
VALUES ($1,$2,$3,$4) RETURNING *;

-- name: GetPurchaseOrder :one
SELECT * FROM purchase_orders WHERE id=$1 AND deleted_at IS NULL;

-- name: SetPurchaseOrderStatus :one
-- Guarded on the current status so a concurrent transition cannot slip through.
UPDATE purchase_orders SET status=sqlc.arg(to_status)
WHERE id=$1 AND status=sqlc.arg(from_status) AND deleted_at IS NULL RETURNING *;

-- name: ListPurchaseOrders :many
SELECT sqlc.embed(po), s.name AS supplier_name,
  (SELECT count(*) FROM purchase_order_lines l WHERE l.po_id=po.id AND l.deleted_at IS NULL)::int AS line_count,
  COALESCE((SELECT SUM(l.qty*l.unit_price) FROM purchase_order_lines l
            WHERE l.po_id=po.id AND l.deleted_at IS NULL),0)::numeric AS total_amount
FROM purchase_orders po JOIN suppliers s ON s.id=po.supplier_id
WHERE po.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR po.status=sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL OR unaccent(lower(po.po_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(s.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY po.expected_at DESC NULLS LAST, po.po_no DESC LIMIT $1 OFFSET $2;

-- name: CountPurchaseOrders :one
SELECT count(*) FROM purchase_orders po JOIN suppliers s ON s.id=po.supplier_id
WHERE po.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR po.status=sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL OR unaccent(lower(po.po_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(s.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: CreatePurchaseOrderLine :one
INSERT INTO purchase_order_lines (po_id, item_id, qty, unit_price, created_by)
VALUES ($1,$2,$3,$4,$5) RETURNING *;

-- name: GetPurchaseOrderLine :one
SELECT * FROM purchase_order_lines WHERE id=$1 AND deleted_at IS NULL;

-- name: ListPurchaseOrderLines :many
SELECT l.*, i.sku,
  COALESCE((SELECT SUM(rl.qty) FROM goods_receipt_lines rl
            WHERE rl.po_line_id=l.id AND rl.deleted_at IS NULL),0)::numeric AS received_qty
FROM purchase_order_lines l JOIN items i ON i.id=l.item_id
WHERE l.po_id=$1 AND l.deleted_at IS NULL ORDER BY i.sku;

-- name: CreateGoodsReceipt :one
INSERT INTO goods_receipts (po_id, location_id, received_by, note, created_by)
VALUES ($1,$2,$3,$4,$3) RETURNING *;

-- name: CreateGoodsReceiptLine :one
INSERT INTO goods_receipt_lines (receipt_id, po_line_id, qty, created_by)
VALUES ($1,$2,$3,$4) RETURNING *;

-- name: ListGoodsReceipts :many
SELECT * FROM goods_receipts WHERE po_id=$1 AND deleted_at IS NULL ORDER BY received_at DESC;

-- name: SumReceivedForLine :one
SELECT COALESCE(SUM(qty),0)::numeric FROM goods_receipt_lines
WHERE po_line_id=$1 AND deleted_at IS NULL;

-- name: CountPurchaseOrdersAwaitingApproval :one
SELECT count(*)::int FROM purchase_orders
WHERE deleted_at IS NULL AND status IN ('draft','approval');

-- name: CountOverdueDeliveries :one
SELECT count(*)::int FROM purchase_orders
WHERE deleted_at IS NULL AND expected_at < CURRENT_DATE
  AND status IN ('confirmed','in_transit','receiving');
