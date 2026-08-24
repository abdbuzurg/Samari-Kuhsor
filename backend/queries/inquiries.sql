-- Интеграция с сайтом — docs/05-MODULES.md §8.
-- Written by the PUBLIC website through the same backend.

-- name: CreateInquiry :one
INSERT INTO inquiries (reference_no, inquiry_type, name, company, contact, message, batch_id, source_ip)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;

-- name: GetInquiry :one
SELECT * FROM inquiries WHERE id=$1 AND deleted_at IS NULL;

-- name: NextInquiryReference :one
-- Allocated from a per-type sequence (migration 00006). Contention-free: two
-- concurrent submissions can never receive the same number, which MAX()+1 could
-- not guarantee because it reads only committed rows.
SELECT next_inquiry_reference(sqlc.arg(prefix)::text) AS reference_no;

-- name: SetInquiryStatus :one
UPDATE inquiries SET status=$2 WHERE id=$1 AND deleted_at IS NULL RETURNING *;

-- name: ListInquiries :many
SELECT i.*, b.batch_no
FROM inquiries i LEFT JOIN batches b ON b.id=i.batch_id
WHERE i.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR i.status=sqlc.narg(status))
  AND (sqlc.narg(inquiry_type)::text IS NULL OR i.inquiry_type=sqlc.narg(inquiry_type))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(i.reference_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(i.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(i.company,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY i.created_at DESC LIMIT $1 OFFSET $2;

-- name: CountInquiries :one
SELECT count(*) FROM inquiries i WHERE i.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR i.status=sqlc.narg(status))
  AND (sqlc.narg(inquiry_type)::text IS NULL OR i.inquiry_type=sqlc.narg(inquiry_type))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(i.reference_no)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(i.name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(i.company,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: CountInquiriesToday :one
SELECT count(*)::int FROM inquiries
WHERE deleted_at IS NULL AND created_at >= CURRENT_DATE;

-- name: CountInquiriesSince :one
-- Rate limiting by IP (docs/03-API-CONTRACT.md:249).
SELECT count(*)::int FROM inquiries
WHERE source_ip = $1 AND created_at >= now() - sqlc.arg(lookback)::interval;

-- name: CreateCustomer :one
INSERT INTO customers (name, customer_type, region, contact, created_by)
VALUES ($1,$2,$3,$4,$5) RETURNING *;

-- name: CreateLead :one
INSERT INTO leads (customer_id, inquiry_id, source, created_by)
VALUES ($1,$2,$3,$4) RETURNING *;

-- name: GetLeadForInquiry :one
SELECT * FROM leads WHERE inquiry_id=$1 AND deleted_at IS NULL LIMIT 1;

-- name: GetInquiryDetail :one
-- The detail view's row. Mirrors ListInquiries so the list and the record it
-- opens carry the same shape; GetInquiry stays as it is because ConvertToLead
-- depends on its plain db.Inquiry return.
SELECT i.*, b.batch_no
FROM inquiries i LEFT JOIN batches b ON b.id=i.batch_id
WHERE i.id=$1 AND i.deleted_at IS NULL;
