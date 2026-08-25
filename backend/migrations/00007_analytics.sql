-- First-party website analytics. docs/01-DECISIONS.md D12.

-- +goose Up

-- ---------------------------------------------------------------------------
-- analytics_events — the raw log, 90 days, then HARD DELETED
--
-- Two deliberate departures from CLAUDE.md §4, both settled in D12 and both
-- limited to this file:
--
--   §4.3 "No hard deletes." This table deletes for real. A tombstoned row still
--   contains the session id, so `deleted_at` would satisfy the letter of the
--   rule while defeating its entire purpose. Retention here is a privacy
--   commitment, not a storage optimisation.
--
--   §4.5 "Every mutation writes to audit_log." Ingestion writes no audit row —
--   one per click would bury the trail that proves who released a batch under
--   web traffic within a week. The RETENTION JOB writes one per run instead, so
--   the deletion is provable and the clicks are not.
--
-- There is also no `version` and no `updated_at`: an event is a fact about a
-- moment and is never edited. No `created_by`: there is no actor, which is the
-- whole point.
-- ---------------------------------------------------------------------------
CREATE TABLE analytics_events (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  occurred_at timestamptz NOT NULL DEFAULT now(),

  -- Random, generated in the browser, held in sessionStorage and gone when the
  -- tab closes (D12). Pseudonymous personal data — which is why this table has
  -- a retention window at all, and why consent is required rather than polite.
  session_id  text        NOT NULL CHECK (length(session_id) BETWEEN 8 AND 64),

  kind        text        NOT NULL CHECK (kind IN
                ('page_view', 'product_view', 'link_click')),

  -- page_view: the route. product_view: the SKU. link_click: the href.
  target      text        NOT NULL CHECK (length(target) <= 512),

  -- Set for product_view and for a link_click attributed to a product (the
  -- modal's «Запросить цену» / «Подробнее»). Joining beats parsing a SKU out of
  -- `target`, and the FK means a forged product cannot be inserted at all.
  item_id     uuid        REFERENCES items(id),

  -- product_view only: which surface showed it.
  source      text        CHECK (source IS NULL OR source IN
                ('product_page', 'belt_modal')),

  -- link_click only. Everything is captured and classified; the dashboard shows
  -- `cta` and `outbound`, because nav and footer chrome always win on volume and
  -- tell the owner nothing (D12).
  category    text        CHECK (category IS NULL OR category IN
                ('cta', 'product', 'nav', 'footer', 'outbound')),

  locale      text        NOT NULL CHECK (locale IN ('ru', 'tg', 'en')),

  -- Salted SHA-256, never the raw address. Used only to rate-limit ingestion.
  -- Storing a raw IP against a form somebody deliberately submitted is
  -- defensible; storing one against browsing behaviour is a materially larger
  -- claim, and counting does not need it.
  ip_hash     text        NOT NULL CHECK (length(ip_hash) = 64)
);

-- The retention delete scans by age, so this is the index that has to exist.
CREATE INDEX analytics_events_occurred_idx ON analytics_events (occurred_at);

-- The rate limit counts recent rows for one hasher. Without this it is a table
-- scan per batch, which is the reason the inquiry limiter's design does not
-- transfer to click volume.
CREATE INDEX analytics_events_ip_idx ON analytics_events (ip_hash, occurred_at DESC);

-- The nightly rollup groups by kind within a day.
CREATE INDEX analytics_events_rollup_idx ON analytics_events (occurred_at, kind);

-- Counting VISITS rather than events means counting distinct sessions per
-- target — ten views in one session are one visit (D12).
CREATE INDEX analytics_events_session_idx ON analytics_events (kind, target, session_id);

-- ---------------------------------------------------------------------------
-- analytics_daily — the rollup, kept forever
--
-- No session_id, so there is nothing personal left once a day is rolled up.
-- That is what makes it safe to keep after the raw rows are deleted, and it is
-- why the rollup has to exist from the first version: delete raw without it and
-- the history goes too.
-- ---------------------------------------------------------------------------
CREATE TABLE analytics_daily (
  id            uuid        PRIMARY KEY DEFAULT uuidv7(),
  day           date        NOT NULL,
  kind          text        NOT NULL,
  target        text        NOT NULL,
  item_id       uuid        REFERENCES items(id),
  category      text,
  locale        text        NOT NULL,
  event_count   integer     NOT NULL CHECK (event_count >= 0),
  -- Distinct sessions. This is the number the dashboard ranks on.
  session_count integer     NOT NULL CHECK (session_count >= 0),
  created_at    timestamptz NOT NULL DEFAULT now()
);

-- One row per (day, kind, target, locale). The rollup is idempotent: re-running
-- a day overwrites it rather than doubling it, which matters because the CLI
-- exists precisely so a missed day can be caught up.
CREATE UNIQUE INDEX analytics_daily_key_idx
  ON analytics_daily (day, kind, target, locale);

CREATE INDEX analytics_daily_lookup_idx ON analytics_daily (kind, day DESC);

-- ---------------------------------------------------------------------------
-- analytics_maintenance_runs — proof the retention actually happened
--
-- The API warns at boot when the newest row here is older than 48 hours, so a
-- ticker that dies silently is distinguishable from one that is working. Without
-- this the 90-day window is an assertion in a privacy policy.
-- ---------------------------------------------------------------------------
CREATE TABLE analytics_maintenance_runs (
  id               uuid        PRIMARY KEY DEFAULT uuidv7(),
  ran_at           timestamptz NOT NULL DEFAULT now(),
  days_rolled_up   integer     NOT NULL,
  rows_deleted     integer     NOT NULL,
  oldest_surviving date
);

CREATE INDEX analytics_maintenance_runs_ran_idx
  ON analytics_maintenance_runs (ran_at DESC);

-- +goose Down
DROP TABLE IF EXISTS analytics_maintenance_runs;
DROP TABLE IF EXISTS analytics_daily;
DROP TABLE IF EXISTS analytics_events;
