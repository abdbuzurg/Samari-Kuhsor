-- CMS and notifications. docs/02-SCHEMA.md §9 and docs/07-IMPLEMENTATION-PLAN.md I15.

-- +goose Up

-- ---------------------------------------------------------------------------
-- CMS — the CRM edits this, the website reads it
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE media (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  file_path   text        NOT NULL,
  mime_type   text        NOT NULL,
  width       integer,
  height      integer,
  size_bytes  bigint,
  -- One of the two deliberate exceptions to the sibling-translation-table rule
  -- (docs/02-SCHEMA.md:56): short fixed-cardinality labels.
  alt_ru      text,
  alt_tg      text,
  alt_en      text,
  uploaded_by uuid        REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer     NOT NULL DEFAULT 1,
  created_by  uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE TRIGGER media_touch BEFORE UPDATE ON media FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- The publishing ladder is a ToR requirement (docs/02-SCHEMA.md:421):
--   draft → technical_review → language_review → approved → published
CREATE TABLE content_pages (
  id           uuid        PRIMARY KEY DEFAULT uuidv7(),
  key          text        NOT NULL,   -- 'home','products','production','contacts'
  status       text        NOT NULL DEFAULT 'draft' CHECK (status IN
                 ('draft','technical_review','language_review','approved','published')),
  published_at timestamptz,
  published_by uuid        REFERENCES users(id),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz,
  version      integer     NOT NULL DEFAULT 1,
  created_by   uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX content_pages_key_key ON content_pages (key) WHERE deleted_at IS NULL;
CREATE TRIGGER content_pages_touch BEFORE UPDATE ON content_pages FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE content_blocks (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  page_id    uuid        NOT NULL REFERENCES content_pages(id),
  block_key  text        NOT NULL,   -- 'hero','trust_strip','eco','cta'
  sort_order integer     NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer     NOT NULL DEFAULT 1,
  created_by uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX content_blocks_key ON content_blocks (page_id, block_key) WHERE deleted_at IS NULL;
CREATE TRIGGER content_blocks_touch BEFORE UPDATE ON content_blocks FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE content_block_translations (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  block_id   uuid        NOT NULL REFERENCES content_blocks(id),
  locale     text        NOT NULL CHECK (locale IN ('ru','tg','en')),
  heading    text,
  body       text,
  cta_label  text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer     NOT NULL DEFAULT 1,
  created_by uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX content_block_translations_key
  ON content_block_translations (block_id, locale) WHERE deleted_at IS NULL;
CREATE TRIGGER content_block_translations_touch BEFORE UPDATE ON content_block_translations
  FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE news_posts (
  id             uuid        PRIMARY KEY DEFAULT uuidv7(),
  slug           text        NOT NULL,
  category       text,
  published_on   date,
  cover_media_id uuid        REFERENCES media(id),
  status         text        NOT NULL DEFAULT 'draft' CHECK (status IN
                   ('draft','technical_review','language_review','approved','published')),
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  deleted_at     timestamptz,
  version        integer     NOT NULL DEFAULT 1,
  created_by     uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX news_posts_slug_key ON news_posts (slug) WHERE deleted_at IS NULL;
CREATE TRIGGER news_posts_touch BEFORE UPDATE ON news_posts FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE news_post_translations (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  post_id    uuid        NOT NULL REFERENCES news_posts(id),
  locale     text        NOT NULL CHECK (locale IN ('ru','tg','en')),
  title      text        NOT NULL,
  excerpt    text,
  body       text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  version    integer     NOT NULL DEFAULT 1,
  created_by uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE UNIQUE INDEX news_post_translations_key
  ON news_post_translations (post_id, locale) WHERE deleted_at IS NULL;
CREATE TRIGGER news_post_translations_touch BEFORE UPDATE ON news_post_translations
  FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
-- Append-only, like batch_status_events: a workflow decision is a record of who
-- moved content forward, and it is not amended afterwards.
CREATE TABLE content_workflow_events (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  entity_type text        NOT NULL CHECK (entity_type IN ('content_page','news_post')),
  entity_id   uuid        NOT NULL,
  from_status text,
  to_status   text        NOT NULL,
  actor_id    uuid        NOT NULL REFERENCES users(id),
  comment     text,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  created_at  timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd
CREATE INDEX content_workflow_events_entity_idx
  ON content_workflow_events (entity_type, entity_id, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- Notifications — only the THREE discrete events (docs/07 I15)
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
-- The seven STANDING conditions (low stock, expiring batch, PO awaiting approval,
-- overdue delivery, expiring document, expiring contract, maintenance due) are
-- NOT stored. They are live queries: self-healing, no reconciliation, and no risk
-- of alarming the factory about a problem that was solved days ago.
--
-- Only these three are events that happened at a point in time and cannot be
-- re-derived: a new inquiry, a batch entering quarantine, a batch rejected.
CREATE TABLE notifications (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  kind        text        NOT NULL CHECK (kind IN
                ('inquiry_received','batch_quarantined','batch_rejected')),
  -- The module key. Visibility is resolved at READ time against the viewer's
  -- permissions, so a notification is broadcast by resource rather than addressed
  -- to a user (docs/05-MODULES.md:294).
  resource    text        NOT NULL,
  resource_id uuid,
  level       text        NOT NULL CHECK (level IN ('ok','warn','danger','info','neutral')),
  title       text        NOT NULL,
  body        text,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer     NOT NULL DEFAULT 1,
  created_by  uuid        REFERENCES users(id)
);
-- +goose StatementEnd
CREATE INDEX notifications_occurred_idx ON notifications (occurred_at DESC) WHERE deleted_at IS NULL;
CREATE TRIGGER notifications_touch BEFORE UPDATE ON notifications FOR EACH ROW EXECUTE FUNCTION touch_row();

-- +goose StatementBegin
CREATE TABLE notification_reads (
  id              uuid        PRIMARY KEY DEFAULT uuidv7(),
  notification_id uuid        NOT NULL REFERENCES notifications(id),
  user_id         uuid        NOT NULL REFERENCES users(id),
  read_at         timestamptz NOT NULL DEFAULT now(),
  created_at      timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd
CREATE UNIQUE INDEX notification_reads_key ON notification_reads (notification_id, user_id);

-- +goose Down

DROP TABLE IF EXISTS notification_reads;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS content_workflow_events;
DROP TABLE IF EXISTS news_post_translations;
DROP TABLE IF EXISTS news_posts;
DROP TABLE IF EXISTS content_block_translations;
DROP TABLE IF EXISTS content_blocks;
DROP TABLE IF EXISTS content_pages;
DROP TABLE IF EXISTS media;
