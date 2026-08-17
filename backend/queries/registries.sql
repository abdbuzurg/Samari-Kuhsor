-- Персонал, Оборудование и ТО, Документы — docs/05-MODULES.md §12, §13, §14.
--
-- These three are registries rather than transaction flows: rows are created,
-- kept current, and watched for expiry. What they share is a date that runs out
-- — a contract, a service interval, a certificate — which is why all three feed
-- standing conditions in the alerts service rather than firing events.

-- ---------------------------------------------------------------------------
-- Персонал
-- ---------------------------------------------------------------------------

-- name: ListEmployees :many
SELECT sqlc.embed(e), p.title AS position_title, d.name AS department_name
FROM employees e
LEFT JOIN positions p ON p.id = e.position_id AND p.deleted_at IS NULL
LEFT JOIN departments d ON d.id = p.department_id AND d.deleted_at IS NULL
WHERE e.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR e.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(e.full_name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(p.title,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY
  -- Contracts closest to expiry first: that is the column this list exists for.
  e.contract_until ASC NULLS LAST, e.full_name
LIMIT $1 OFFSET $2;

-- name: CountEmployees :one
SELECT count(*) FROM employees e
LEFT JOIN positions p ON p.id = e.position_id AND p.deleted_at IS NULL
WHERE e.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR e.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(e.full_name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(p.title,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: GetEmployee :one
SELECT sqlc.embed(e), p.title AS position_title, d.name AS department_name
FROM employees e
LEFT JOIN positions p ON p.id = e.position_id AND p.deleted_at IS NULL
LEFT JOIN departments d ON d.id = p.department_id AND d.deleted_at IS NULL
WHERE e.id = $1 AND e.deleted_at IS NULL;

-- name: CreateEmployee :one
INSERT INTO employees (full_name, position_id, shift, hired_on, contract_until, status, created_by)
VALUES ($1,$2,$3,$4,$5,COALESCE(sqlc.narg(status)::text,'active'),$6)
RETURNING *;

-- name: UpdateEmployee :one
UPDATE employees SET
  full_name = $2, position_id = $3, shift = $4,
  hired_on = $5, contract_until = $6, status = $7
WHERE id = $1 AND deleted_at IS NULL AND version = $8
RETURNING *;

-- name: TombstoneEmployee :one
UPDATE employees SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL AND version = $2
RETURNING *;

-- name: ListPositions :many
SELECT p.*, d.name AS department_name
FROM positions p
JOIN departments d ON d.id = p.department_id AND d.deleted_at IS NULL
WHERE p.deleted_at IS NULL ORDER BY d.name, p.title;

-- ---------------------------------------------------------------------------
-- Оборудование и ТО
-- ---------------------------------------------------------------------------

-- name: ListAssets :many
SELECT sqlc.embed(a),
  -- Cast for the same reason as migration 00005: sqlc types a bare aggregate as
  -- interface{}, which is unusable without a runtime assertion at every call.
  (SELECT max(m.next_due_on) FROM maintenance_events m
    WHERE m.asset_id = a.id AND m.deleted_at IS NULL)::date AS next_due_on,
  (SELECT max(m.performed_at) FROM maintenance_events m
    WHERE m.asset_id = a.id AND m.deleted_at IS NULL)::timestamptz AS last_service_at
FROM assets a
WHERE a.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR a.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(a.asset_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(a.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(a.line,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY a.asset_no
LIMIT $1 OFFSET $2;

-- name: CountAssets :one
SELECT count(*) FROM assets a
WHERE a.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR a.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(a.asset_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(a.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(a.line,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: GetAsset :one
SELECT * FROM assets WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateAsset :one
INSERT INTO assets (asset_no, name, asset_type, line, commissioned_on, warranty_until, status, created_by)
VALUES ($1,$2,$3,$4,$5,$6,COALESCE(sqlc.narg(status)::text,'running'),$7)
RETURNING *;

-- name: UpdateAsset :one
UPDATE assets SET
  asset_no = $2, name = $3, asset_type = $4, line = $5,
  commissioned_on = $6, warranty_until = $7, status = $8
WHERE id = $1 AND deleted_at IS NULL AND version = $9
RETURNING *;

-- name: TombstoneAsset :one
UPDATE assets SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL AND version = $2
RETURNING *;

-- name: ListMaintenanceEvents :many
-- Append-only in practice: a service record is what someone did on a date, and
-- there is no endpoint that edits one.
SELECT * FROM maintenance_events
WHERE asset_id = $1 AND deleted_at IS NULL
ORDER BY performed_at DESC NULLS LAST, created_at DESC;

-- name: CreateMaintenanceEvent :one
INSERT INTO maintenance_events (asset_id, event_type, performed_at, next_due_on, notes, created_by)
VALUES ($1,$2,COALESCE(sqlc.narg(performed_at)::timestamptz, now()),$3,$4,$5)
RETURNING *;

-- ---------------------------------------------------------------------------
-- Документы
-- ---------------------------------------------------------------------------

-- name: ListDocuments :many
SELECT d.*, u.full_name AS owner_name
FROM documents d
LEFT JOIN users u ON u.id = d.owner_id
WHERE d.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR d.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(d.doc_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(d.title)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY d.valid_until ASC NULLS LAST, d.doc_no
LIMIT $1 OFFSET $2;

-- name: CountDocuments :one
SELECT count(*) FROM documents d
WHERE d.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR d.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(d.doc_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(d.title)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: GetDocument :one
SELECT d.*, u.full_name AS owner_name
FROM documents d
LEFT JOIN users u ON u.id = d.owner_id
WHERE d.id = $1 AND d.deleted_at IS NULL;

-- name: CreateDocument :one
INSERT INTO documents (doc_no, title, doc_type, owner_id, valid_until, status, created_by)
VALUES ($1,$2,$3,$4,$5,COALESCE(sqlc.narg(status)::text,'draft'),$6)
RETURNING *;

-- name: UpdateDocument :one
UPDATE documents SET
  doc_no = $2, title = $3, doc_type = $4, owner_id = $5, valid_until = $6
WHERE id = $1 AND deleted_at IS NULL AND version = $7
RETURNING *;

-- name: TransitionDocument :one
-- Guarded on the current status, so a concurrent transition cannot slip between
-- the caller's read and this write.
UPDATE documents SET status = sqlc.arg(to_status)::text
WHERE id = $1 AND deleted_at IS NULL AND status = sqlc.arg(from_status)::text
RETURNING *;

-- name: TombstoneDocument :one
UPDATE documents SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL AND version = $2
RETURNING *;
