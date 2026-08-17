// Package cms implements the website's content management — docs/05-MODULES.md §15.
//
// Not in the prototype. It is a new build, and it is the reason the website is a
// Next.js application rather than static files: the client edits their own copy.
//
// The workflow ladder is the whole design:
//
//	Черновик → Техническая проверка → Языковая проверка → Утверждено → Опубликовано
//
// Two rungs need cms:approve — Утверждено and Опубликовано. The reason is the
// same one that gates releasing a batch: publishing is a claim the company makes
// in public, and someone with authority signs it. The first two rungs are
// review, not authority, so anyone with cms:manage may move a draft along.
//
// A step BACK down the ladder never needs approve. Refusing to approve something
// is not itself an act of approval, and requiring authority to send a draft back
// for correction would mean a reviewer who spots a mistake cannot report it.
package cms

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

const Resource = rbac.CMS

// The ladder — migrations/00004_cms_notifications.sql:39.
const (
	StatusDraft           = "draft"
	StatusTechnicalReview = "technical_review"
	StatusLanguageReview  = "language_review"
	StatusApproved        = "approved"
	StatusPublished       = "published"
)

// Entity types for content_workflow_events.
const (
	EntityPage = "content_page"
	EntityNews = "news_post"
)

// Locales the CMS edits. Matches the CHECK constraint on every translation table
// and docs/07-IMPLEMENTATION-PLAN.md C2 — `tg`, never `tj`.
var Locales = []string{"ru", "tg", "en"}

// Transition is one legal move on the ladder.
type Transition struct {
	From, To        string
	RequiresApprove bool
}

// ladder is the complete matrix. Anything absent is illegal.
//
// Forward moves are one rung at a time: a draft cannot jump to published, which
// is what makes the two reviews mean anything. Backward moves may skip rungs —
// a language reviewer who finds a factual error sends it all the way back to the
// author, not to the technical reviewer who already passed it.
var ladder = []Transition{
	{StatusDraft, StatusTechnicalReview, false},
	{StatusTechnicalReview, StatusLanguageReview, false},
	{StatusLanguageReview, StatusApproved, true},
	{StatusApproved, StatusPublished, true},

	// Back down. Never needs approve — see the package comment.
	{StatusTechnicalReview, StatusDraft, false},
	{StatusLanguageReview, StatusDraft, false},
	{StatusLanguageReview, StatusTechnicalReview, false},
	{StatusApproved, StatusDraft, false},
	{StatusApproved, StatusLanguageReview, false},

	// Unpublishing IS an act of authority: it removes something the company has
	// already said in public, and the client should know who did it.
	{StatusPublished, StatusApproved, true},
	{StatusPublished, StatusDraft, true},
}

func Lookup(from, to string) (Transition, bool) {
	for _, t := range ladder {
		if t.From == from && t.To == to {
			return t, true
		}
	}
	return Transition{}, false
}

// Ladder returns the whole matrix, for tests and documentation.
func Ladder() []Transition { return append([]Transition(nil), ladder...) }

// AllowedFrom projects the ladder onto one status and one permission, so the
// buttons and the rules share a single definition.
func AllowedFrom(status string, hasApprove bool) []string {
	out := make([]string, 0, len(ladder))
	for _, t := range ladder {
		if t.From != status || (t.RequiresApprove && !hasApprove) {
			continue
		}
		out = append(out, t.To)
	}
	return out
}

// IsPublic reports whether content in this status is visible on the website.
//
// Exactly one status qualifies. `approved` deliberately does not: it means the
// content has cleared review, not that the client has decided to release it, and
// conflating the two would publish things the moment a reviewer finished.
func IsPublic(status string) bool { return status == StatusPublished }

var (
	NewsSortSpec = common.SortSpec{
		Allowed:     []string{"slug", "published_on", "status"},
		Default:     "published_on",
		DefaultDesc: true,
	}
	MediaSortSpec = common.SortSpec{
		Allowed:     []string{"file_path", "created_at"},
		Default:     "created_at",
		DefaultDesc: true,
	}
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ---------------------------------------------------------------------------
// Pages
// ---------------------------------------------------------------------------

func (s *Service) Pages(ctx context.Context, status *string) ([]db.ListContentPagesRow, error) {
	rows, err := db.New(s.pool).ListContentPages(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("cms: pages: %w", err)
	}
	return rows, nil
}

func (s *Service) Page(ctx context.Context, id uuid.UUID) (db.ContentPage, error) {
	page, err := db.New(s.pool).GetContentPage(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ContentPage{}, common.NotFound()
		}
		return db.ContentPage{}, fmt.Errorf("cms: page: %w", err)
	}
	return page, nil
}

func (s *Service) Blocks(ctx context.Context, pageID uuid.UUID, locale string) ([]db.ListContentBlocksRow, error) {
	if !validLocale(locale) {
		return nil, common.Validation(common.FieldError{
			Field: "locale", Code: "invalid", Message: "Недопустимый язык",
		})
	}
	rows, err := db.New(s.pool).ListContentBlocks(ctx, db.ListContentBlocksParams{
		PageID: pageID, Locale: locale,
	})
	if err != nil {
		return nil, fmt.Errorf("cms: blocks: %w", err)
	}
	return rows, nil
}

// BlockInput is one block's copy in one locale.
type BlockInput struct {
	PageID    uuid.UUID
	BlockKey  string
	SortOrder int32
	Locale    string
	Heading   *string
	Body      *string
	CTALabel  *string
}

// SaveBlock upserts a block and its translation.
//
// Editing content on a PUBLISHED page is refused. The alternative — letting an
// edit land live with no review — would make the whole ladder decorative: anyone
// with cms:manage could rewrite an approved page without passing a single rung.
// Move it back to draft first; that step is recorded and attributable.
func (s *Service) SaveBlock(ctx context.Context, actor uuid.UUID, in BlockInput) (db.ContentBlockTranslation, error) {
	if !validLocale(in.Locale) {
		return db.ContentBlockTranslation{}, common.Validation(common.FieldError{
			Field: "locale", Code: "invalid", Message: "Недопустимый язык",
		})
	}
	if strings.TrimSpace(in.BlockKey) == "" {
		return db.ContentBlockTranslation{}, common.Validation(common.FieldError{
			Field: "block_key", Code: "required", Message: "Укажите ключ блока",
		})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.ContentBlockTranslation{}, fmt.Errorf("cms: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	page, err := q.GetContentPage(ctx, in.PageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ContentBlockTranslation{}, common.Validation(common.FieldError{
				Field: "page_id", Code: "not_found", Message: "Страница не найдена",
			})
		}
		return db.ContentBlockTranslation{}, fmt.Errorf("cms: load page: %w", err)
	}
	if page.Status == StatusPublished {
		return db.ContentBlockTranslation{}, common.BusinessRule(
			"Нельзя редактировать опубликованную страницу. Верните её в черновик.")
	}

	block, err := q.UpsertContentBlock(ctx, db.UpsertContentBlockParams{
		PageID: in.PageID, BlockKey: in.BlockKey, SortOrder: in.SortOrder,
		CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.ContentBlockTranslation{}, fmt.Errorf("cms: upsert block: %w", err)
	}

	translation, err := q.UpsertBlockTranslation(ctx, db.UpsertBlockTranslationParams{
		BlockID: block.ID, Locale: in.Locale,
		Heading: in.Heading, Body: in.Body, CtaLabel: in.CTALabel,
		CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.ContentBlockTranslation{}, fmt.Errorf("cms: upsert translation: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate,
		Resource: Resource, ResourceID: audit.Target(in.PageID),
		After: map[string]any{
			"page": page.Key, "block": in.BlockKey, "locale": in.Locale,
		},
	}); err != nil {
		return db.ContentBlockTranslation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.ContentBlockTranslation{}, fmt.Errorf("cms: commit: %w", err)
	}
	return translation, nil
}

// ---------------------------------------------------------------------------
// The ladder
// ---------------------------------------------------------------------------

type TransitionInput struct {
	EntityType string
	EntityID   uuid.UUID
	To         string
	Comment    *string
	HasApprove bool
}

// Transition moves a page or a post along the ladder.
//
// One function for both, because the ladder is the same and duplicating it would
// let the two drift — at which point "published" would mean something different
// on a page than on a post.
func (s *Service) Transition(ctx context.Context, actor uuid.UUID, in TransitionInput) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("cms: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	var current string
	switch in.EntityType {
	case EntityPage:
		page, err := q.GetContentPage(ctx, in.EntityID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", common.NotFound()
			}
			return "", fmt.Errorf("cms: load page: %w", err)
		}
		current = page.Status
	case EntityNews:
		post, err := q.GetNewsPost(ctx, in.EntityID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", common.NotFound()
			}
			return "", fmt.Errorf("cms: load post: %w", err)
		}
		current = post.Status
	default:
		return "", common.Validation(common.FieldError{
			Field: "entity_type", Code: "invalid", Message: "Недопустимый тип объекта",
		})
	}

	rule, legal := Lookup(current, in.To)
	if !legal {
		return "", common.BusinessRule(fmt.Sprintf(
			"Недопустимый переход: из «%s» в «%s».", current, in.To))
	}
	if rule.RequiresApprove && !in.HasApprove {
		return "", common.Forbidden()
	}

	// A publish with an untranslated locale would put a Russian string on the
	// Tajik site. The three locales ship together (D10), so all three must exist
	// before anything goes live.
	if in.To == StatusPublished && in.EntityType == EntityNews {
		translations, err := q.ListNewsTranslations(ctx, in.EntityID)
		if err != nil {
			return "", fmt.Errorf("cms: translations: %w", err)
		}
		if missing := missingLocales(translations); len(missing) > 0 {
			return "", common.BusinessRule(fmt.Sprintf(
				"Нельзя опубликовать: нет перевода для языков: %s.", strings.Join(missing, ", ")))
		}
	}

	switch in.EntityType {
	case EntityPage:
		if _, err := q.TransitionContentPage(ctx, db.TransitionContentPageParams{
			ID: in.EntityID, FromStatus: current, ToStatus: in.To,
			Actor: uuid.NullUUID{UUID: actor, Valid: true},
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", common.BusinessRule(
					"Статус изменился другим пользователем. Обновите страницу.")
			}
			return "", fmt.Errorf("cms: transition page: %w", err)
		}
	case EntityNews:
		if _, err := q.TransitionNewsPost(ctx, db.TransitionNewsPostParams{
			ID: in.EntityID, FromStatus: current, ToStatus: in.To,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", common.BusinessRule(
					"Статус изменился другим пользователем. Обновите страницу.")
			}
			return "", fmt.Errorf("cms: transition post: %w", err)
		}
	}

	if _, err := q.InsertWorkflowEvent(ctx, db.InsertWorkflowEventParams{
		EntityType: in.EntityType, EntityID: in.EntityID,
		FromStatus: &current, ToStatus: in.To,
		ActorID: actor, Comment: in.Comment,
	}); err != nil {
		return "", fmt.Errorf("cms: workflow event: %w", err)
	}

	action := audit.ActionUpdate
	if rule.RequiresApprove {
		action = audit.ActionApprove
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: action,
		Resource: Resource, ResourceID: audit.Target(in.EntityID),
		Before: map[string]any{"status": current},
		After:  map[string]any{"status": in.To, "entity_type": in.EntityType},
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("cms: commit: %w", err)
	}
	return in.To, nil
}

func (s *Service) History(ctx context.Context, entityType string, id uuid.UUID) ([]db.ListWorkflowEventsRow, error) {
	rows, err := db.New(s.pool).ListWorkflowEvents(ctx, db.ListWorkflowEventsParams{
		EntityType: entityType, EntityID: id,
	})
	if err != nil {
		return nil, fmt.Errorf("cms: history: %w", err)
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// News
// ---------------------------------------------------------------------------

type NewsInput struct {
	Slug        string
	Category    *string
	PublishedOn pgtype.Date
}

func (s *Service) CreateNews(ctx context.Context, actor uuid.UUID, in NewsInput) (db.NewsPost, error) {
	if strings.TrimSpace(in.Slug) == "" {
		return db.NewsPost{}, common.Validation(common.FieldError{
			Field: "slug", Code: "required", Message: "Укажите адрес публикации",
		})
	}
	// The slug is a public URL segment. Restricting it here rather than escaping
	// it later keeps the address readable and stops a title with a slash from
	// producing a nested path nobody intended.
	if !isSlug(in.Slug) {
		return db.NewsPost{}, common.Validation(common.FieldError{
			Field: "slug", Code: "invalid",
			Message: "Адрес может содержать только латинские буквы, цифры и дефис",
		})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.NewsPost{}, fmt.Errorf("cms: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	post, err := q.CreateNewsPost(ctx, db.CreateNewsPostParams{
		Slug: in.Slug, Category: in.Category, PublishedOn: in.PublishedOn,
		CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.NewsPost{}, fmt.Errorf("cms: create post: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate,
		Resource: Resource, ResourceID: audit.Target(post.ID),
		After: map[string]any{"slug": post.Slug, "status": post.Status},
	}); err != nil {
		return db.NewsPost{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.NewsPost{}, fmt.Errorf("cms: commit: %w", err)
	}
	return post, nil
}

type NewsTranslationInput struct {
	PostID  uuid.UUID
	Locale  string
	Title   string
	Excerpt *string
	Body    *string
}

func (s *Service) SaveNewsTranslation(ctx context.Context, actor uuid.UUID, in NewsTranslationInput) (db.NewsPostTranslation, error) {
	if !validLocale(in.Locale) {
		return db.NewsPostTranslation{}, common.Validation(common.FieldError{
			Field: "locale", Code: "invalid", Message: "Недопустимый язык",
		})
	}
	if strings.TrimSpace(in.Title) == "" {
		return db.NewsPostTranslation{}, common.Validation(common.FieldError{
			Field: "title", Code: "required", Message: "Укажите заголовок",
		})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.NewsPostTranslation{}, fmt.Errorf("cms: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	post, err := q.GetNewsPost(ctx, in.PostID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.NewsPostTranslation{}, common.NotFound()
		}
		return db.NewsPostTranslation{}, fmt.Errorf("cms: load post: %w", err)
	}
	if post.Status == StatusPublished {
		return db.NewsPostTranslation{}, common.BusinessRule(
			"Нельзя редактировать опубликованную новость. Верните её в черновик.")
	}

	translation, err := q.UpsertNewsTranslation(ctx, db.UpsertNewsTranslationParams{
		PostID: in.PostID, Locale: in.Locale, Title: in.Title,
		Excerpt: in.Excerpt, Body: in.Body, CreatedBy: audit.Actor(actor),
	})
	if err != nil {
		return db.NewsPostTranslation{}, fmt.Errorf("cms: upsert translation: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate,
		Resource: Resource, ResourceID: audit.Target(in.PostID),
		After: map[string]any{"slug": post.Slug, "locale": in.Locale},
	}); err != nil {
		return db.NewsPostTranslation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.NewsPostTranslation{}, fmt.Errorf("cms: commit: %w", err)
	}
	return translation, nil
}

func (s *Service) News(ctx context.Context, p common.Params, locale string, status *string) ([]db.ListNewsPostsRow, int64, error) {
	if !validLocale(locale) {
		locale = "ru"
	}
	q := db.New(s.pool)
	rows, err := q.ListNewsPosts(ctx, db.ListNewsPostsParams{
		Locale: locale, Status: status, Q: nilIfEmpty(p.Query),
		Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("cms: news: %w", err)
	}
	total, err := q.CountNewsPosts(ctx, db.CountNewsPostsParams{
		Locale: locale, Status: status, Q: nilIfEmpty(p.Query),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("cms: count news: %w", err)
	}
	return rows, total, nil
}

func (s *Service) NewsTranslations(ctx context.Context, postID uuid.UUID) ([]db.NewsPostTranslation, error) {
	rows, err := db.New(s.pool).ListNewsTranslations(ctx, postID)
	if err != nil {
		return nil, fmt.Errorf("cms: translations: %w", err)
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// Media
// ---------------------------------------------------------------------------

func (s *Service) Media(ctx context.Context, p common.Params) ([]db.Medium, int64, error) {
	q := db.New(s.pool)
	rows, err := q.ListMedia(ctx, db.ListMediaParams{
		Q: nilIfEmpty(p.Query), Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("cms: media: %w", err)
	}
	total, err := q.CountMedia(ctx, nilIfEmpty(p.Query))
	if err != nil {
		return nil, 0, fmt.Errorf("cms: count media: %w", err)
	}
	return rows, total, nil
}

type MediaAltInput struct {
	ID      uuid.UUID
	AltRU   *string
	AltTG   *string
	AltEN   *string
	Version int32
}

// SetMediaAlt updates alt text in all three locales.
//
// Alt text is not decoration: an image on a food producer's site with no
// description is unusable to a screen reader, and it is the one accessibility
// requirement a CMS can enforce rather than hope for.
func (s *Service) SetMediaAlt(ctx context.Context, actor uuid.UUID, in MediaAltInput) (db.Medium, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Medium{}, fmt.Errorf("cms: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	updated, err := q.UpdateMediaAlt(ctx, db.UpdateMediaAltParams{
		ID: in.ID, AltRu: in.AltRU, AltTg: in.AltTG, AltEn: in.AltEN, Version: in.Version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Medium{}, common.VersionConflict(in.Version)
		}
		return db.Medium{}, fmt.Errorf("cms: update alt: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate,
		Resource: Resource, ResourceID: audit.Target(in.ID),
		After: map[string]any{"file_path": updated.FilePath},
	}); err != nil {
		return db.Medium{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Medium{}, fmt.Errorf("cms: commit: %w", err)
	}
	return updated, nil
}

// ---------------------------------------------------------------------------

func validLocale(locale string) bool {
	for _, l := range Locales {
		if l == locale {
			return true
		}
	}
	return false
}

// missingLocales names the locales with no translation row, in Locales order so
// the message is stable rather than map-ordered.
func missingLocales(rows []db.NewsPostTranslation) []string {
	present := make(map[string]bool, len(rows))
	for _, r := range rows {
		if strings.TrimSpace(r.Title) != "" {
			present[r.Locale] = true
		}
	}
	var missing []string
	for _, l := range Locales {
		if !present[l] {
			missing = append(missing, l)
		}
	}
	return missing
}

// isSlug allows lowercase latin, digits and hyphens. Deliberately not Cyrillic:
// a slug is a URL segment, and a percent-encoded Cyrillic path is unreadable
// wherever it is pasted.
func isSlug(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return !strings.HasPrefix(s, "-") && !strings.HasSuffix(s, "-")
}
