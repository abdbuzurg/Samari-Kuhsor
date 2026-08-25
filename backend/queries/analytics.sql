-- First-party website analytics (docs/01-DECISIONS.md D12).

-- name: CountAnalyticsEventsSince :one
-- The ingestion rate limit, counted per batch rather than per event because the
-- client buffers and flushes (D12). At click volume a per-event count would be
-- a scan; per batch it is the same order as the inquiry limiter.
SELECT count(*)::int FROM analytics_events
WHERE ip_hash = $1 AND occurred_at >= now() - sqlc.arg(lookback)::interval;

-- name: InsertAnalyticsEvent :exec
-- No RETURNING: nothing downstream needs the id, and the endpoint answers 204
-- regardless of what happened (D12).
INSERT INTO analytics_events
  (occurred_at, session_id, kind, target, item_id, source, category, locale, ip_hash)
VALUES (COALESCE(sqlc.narg(occurred_at)::timestamptz, now()), $1, $2, $3, $4, $5, $6, $7, $8);

-- name: ItemIDBySKU :one
-- Target validation. A product_view naming a SKU that is not in the catalogue is
-- dropped, which is what stops the ranking being forged: the worst a prober can
-- do is inflate something that genuinely exists.
SELECT id FROM items WHERE sku = $1 AND deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- The nightly rollup
-- ---------------------------------------------------------------------------

-- name: RollUpAnalyticsDay :execrows
-- Collapses one day into (kind, target, locale) with an event count and a
-- DISTINCT SESSION count. Idempotent — re-running a day overwrites it rather
-- than doubling it, which matters because the CLI exists so a missed day can be
-- caught up.
INSERT INTO analytics_daily
  (day, kind, target, item_id, category, locale, event_count, session_count)
SELECT
  sqlc.arg(day)::date,
  e.kind,
  e.target,
  -- One target maps to at most one item, so any non-null one will do. Postgres
  -- has no min(uuid), and array_agg with a FILTER is the idiomatic "pick one".
  (array_agg(e.item_id) FILTER (WHERE e.item_id IS NOT NULL))[1],
  min(e.category),
  e.locale,
  count(*)::int,
  count(DISTINCT e.session_id)::int
FROM analytics_events e
WHERE e.occurred_at >= sqlc.arg(day)::date
  AND e.occurred_at <  sqlc.arg(day)::date + INTERVAL '1 day'
GROUP BY e.kind, e.target, e.locale
ON CONFLICT (day, kind, target, locale) DO UPDATE
SET event_count   = EXCLUDED.event_count,
    session_count = EXCLUDED.session_count,
    item_id       = EXCLUDED.item_id,
    category      = EXCLUDED.category;

-- name: AnalyticsDaysNeedingRollup :many
-- Every day that has raw events, oldest first, excluding today — a day is only
-- rolled up once it is complete.
SELECT DISTINCT (occurred_at AT TIME ZONE 'UTC')::date AS day
FROM analytics_events
WHERE occurred_at < date_trunc('day', now())
ORDER BY day;

-- ---------------------------------------------------------------------------
-- Retention
-- ---------------------------------------------------------------------------

-- name: DeleteAnalyticsEventsBefore :execrows
-- A real DELETE, not a tombstone (D12). A soft-deleted row still contains the
-- session id.
DELETE FROM analytics_events WHERE occurred_at < sqlc.arg(cutoff)::timestamptz;

-- name: OldestAnalyticsEvent :one
SELECT min(occurred_at)::timestamptz FROM analytics_events;

-- name: RecordAnalyticsMaintenanceRun :one
INSERT INTO analytics_maintenance_runs (days_rolled_up, rows_deleted, oldest_surviving)
VALUES ($1, $2, $3) RETURNING *;

-- name: LastAnalyticsMaintenanceRun :one
-- Drives the boot warning: a ticker that dies silently must be distinguishable
-- from one that is working.
SELECT * FROM analytics_maintenance_runs ORDER BY ran_at DESC LIMIT 1;

-- ---------------------------------------------------------------------------
-- The dashboard panels
--
-- Both read analytics_daily, never the raw table: the raw rows are gone after 90
-- days and the panel must keep working. Ranked by SESSION count — ten views in
-- one session are one visit (D12).
-- ---------------------------------------------------------------------------

-- name: TopProductsByVisits :many
SELECT d.target AS sku,
       COALESCE(max(t.name), d.target)::text AS name,
       sum(d.session_count)::int AS visits,
       sum(d.event_count)::int   AS views
FROM analytics_daily d
LEFT JOIN item_translations t
  ON t.item_id = d.item_id AND t.locale = sqlc.arg(locale)::text AND t.deleted_at IS NULL
WHERE d.kind = 'product_view'
  AND d.day >= sqlc.arg(since)::date
GROUP BY d.target
ORDER BY visits DESC, views DESC
LIMIT $1;

-- name: TopLinksByVisits :many
-- `cta` and `outbound` only. Nav and footer are captured but never shown: they
-- always win on volume and tell the owner nothing (D12).
SELECT d.target,
       -- Cast for the same reason as migration 00005: sqlc types a bare
       -- aggregate as interface{}, unusable without an assertion per call site.
       COALESCE(max(d.category), 'cta')::text AS category,
       sum(d.session_count)::int AS visits,
       sum(d.event_count)::int   AS clicks
FROM analytics_daily d
WHERE d.kind = 'link_click'
  AND d.category IN ('cta', 'outbound')
  AND d.day >= sqlc.arg(since)::date
GROUP BY d.target
ORDER BY visits DESC, clicks DESC
LIMIT $1;

-- name: AnalyticsVisitTotals :one
-- The headline: how many visits, and how many of them looked at a product.
SELECT
  COALESCE(sum(d.session_count) FILTER (WHERE d.kind = 'page_view'), 0)::int    AS visits,
  COALESCE(sum(d.session_count) FILTER (WHERE d.kind = 'product_view'), 0)::int AS product_visits
FROM analytics_daily d
WHERE d.day >= sqlc.arg(since)::date;
