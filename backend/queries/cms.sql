-- CMS — docs/05-MODULES.md §15.
--
-- The public site renders only `published`; the CRM can preview any state. That
-- asymmetry is the module's whole reason for existing, so every read here is
-- explicit about which side it serves.

-- ---------------------------------------------------------------------------
-- Pages and blocks
-- ---------------------------------------------------------------------------

-- name: ListContentPages :many
SELECT p.*,
  (SELECT count(*) FROM content_blocks b
    WHERE b.page_id = p.id AND b.deleted_at IS NULL)::int AS block_count
FROM content_pages p
WHERE p.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR p.status = sqlc.narg(status))
ORDER BY p.key;

-- name: GetContentPage :one
SELECT * FROM content_pages WHERE id = $1 AND deleted_at IS NULL;

-- name: GetContentPageByKey :one
SELECT * FROM content_pages WHERE key = $1 AND deleted_at IS NULL;

-- name: CreateContentPage :one
INSERT INTO content_pages (key, created_by) VALUES ($1, $2) RETURNING *;

-- name: TransitionContentPage :one
-- Guarded on the current status, so a concurrent move cannot slip between the
-- caller's read and this write.
UPDATE content_pages
SET status = sqlc.arg(to_status)::text,
    published_at = CASE WHEN sqlc.arg(to_status)::text = 'published' THEN now() ELSE published_at END,
    published_by = CASE WHEN sqlc.arg(to_status)::text = 'published' THEN sqlc.narg(actor)::uuid ELSE published_by END
WHERE id = $1 AND deleted_at IS NULL AND status = sqlc.arg(from_status)::text
RETURNING *;

-- name: ListContentBlocks :many
-- Every block with its translation in one locale. LEFT JOIN, so a block whose
-- translation is missing still appears — an editor cannot fill in a gap they
-- cannot see.
SELECT b.*, t.heading, t.body, t.cta_label
FROM content_blocks b
LEFT JOIN content_block_translations t
  ON t.block_id = b.id AND t.locale = sqlc.arg(locale)::text AND t.deleted_at IS NULL
WHERE b.page_id = $1 AND b.deleted_at IS NULL
ORDER BY b.sort_order, b.block_key;

-- name: UpsertContentBlock :one
INSERT INTO content_blocks (page_id, block_key, sort_order, created_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (page_id, block_key) WHERE deleted_at IS NULL
DO UPDATE SET sort_order = EXCLUDED.sort_order
RETURNING *;

-- name: UpsertBlockTranslation :one
INSERT INTO content_block_translations (block_id, locale, heading, body, cta_label, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (block_id, locale) WHERE deleted_at IS NULL
DO UPDATE SET heading = EXCLUDED.heading, body = EXCLUDED.body, cta_label = EXCLUDED.cta_label
RETURNING *;

-- ---------------------------------------------------------------------------
-- News
-- ---------------------------------------------------------------------------

-- name: ListNewsPosts :many
SELECT n.*, COALESCE(tr.title, ru.title) AS title
FROM news_posts n
LEFT JOIN news_post_translations tr
  ON tr.post_id = n.id AND tr.locale = sqlc.arg(locale)::text AND tr.deleted_at IS NULL
LEFT JOIN news_post_translations ru
  ON ru.post_id = n.id AND ru.locale = 'ru' AND ru.deleted_at IS NULL
WHERE n.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR n.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(n.slug)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(tr.title, ru.title, ''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY n.published_on DESC NULLS FIRST, n.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountNewsPosts :one
SELECT count(*) FROM news_posts n
LEFT JOIN news_post_translations tr
  ON tr.post_id = n.id AND tr.locale = sqlc.arg(locale)::text AND tr.deleted_at IS NULL
LEFT JOIN news_post_translations ru
  ON ru.post_id = n.id AND ru.locale = 'ru' AND ru.deleted_at IS NULL
WHERE n.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR n.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(n.slug)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(tr.title, ru.title, ''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: GetNewsPost :one
SELECT * FROM news_posts WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateNewsPost :one
INSERT INTO news_posts (slug, category, published_on, created_by)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: TransitionNewsPost :one
UPDATE news_posts SET status = sqlc.arg(to_status)::text
WHERE id = $1 AND deleted_at IS NULL AND status = sqlc.arg(from_status)::text
RETURNING *;

-- name: ListNewsTranslations :many
SELECT * FROM news_post_translations
WHERE post_id = $1 AND deleted_at IS NULL ORDER BY locale;

-- name: UpsertNewsTranslation :one
INSERT INTO news_post_translations (post_id, locale, title, excerpt, body, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (post_id, locale) WHERE deleted_at IS NULL
DO UPDATE SET title = EXCLUDED.title, excerpt = EXCLUDED.excerpt, body = EXCLUDED.body
RETURNING *;

-- ---------------------------------------------------------------------------
-- Workflow history
-- ---------------------------------------------------------------------------

-- name: InsertWorkflowEvent :one
-- Immutable evidence of who moved what, and when. No deleted_at and no version:
-- there is nothing here to edit.
INSERT INTO content_workflow_events (entity_type, entity_id, from_status, to_status, actor_id, comment)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: ListWorkflowEvents :many
SELECT e.*, u.full_name AS actor_name
FROM content_workflow_events e
JOIN users u ON u.id = e.actor_id
WHERE e.entity_type = $1 AND e.entity_id = $2
ORDER BY e.occurred_at DESC;

-- ---------------------------------------------------------------------------
-- Media
-- ---------------------------------------------------------------------------

-- name: ListMedia :many
SELECT * FROM media WHERE deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(file_path)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(alt_ru,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%')
ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: CountMedia :one
SELECT count(*) FROM media WHERE deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(file_path)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR unaccent(lower(COALESCE(alt_ru,''))) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%');

-- name: CreateMedia :one
INSERT INTO media (file_path, mime_type, width, height, size_bytes, alt_ru, alt_tg, alt_en, uploaded_by, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9) RETURNING *;

-- name: UpdateMediaAlt :one
UPDATE media SET alt_ru = $2, alt_tg = $3, alt_en = $4
WHERE id = $1 AND deleted_at IS NULL AND version = $5 RETURNING *;
