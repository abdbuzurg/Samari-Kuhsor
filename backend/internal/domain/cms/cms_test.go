package cms_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/domain/cms"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// CMS — docs/05-MODULES.md §15.
//
// The ladder is the module. Everything worth testing is about who may move
// content up it, what may not be edited once it is at the top, and the one rule
// that stops a half-translated post going live in three languages.

type fixture struct {
	pool  *pgxpool.Pool
	svc   *cms.Service
	actor uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testsupport.NewDB(t)
	f := fixture{pool: pool, svc: cms.NewService(pool)}
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ('editor@samari-kuhsor.tj','Редактор','x') RETURNING id`).Scan(&f.actor); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fixture) page(t *testing.T, key string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO content_pages (key) VALUES ($1) RETURNING id`, key).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f fixture) post(t *testing.T, slug string) uuid.UUID {
	t.Helper()
	p, err := f.svc.CreateNews(context.Background(), f.actor, cms.NewsInput{Slug: slug})
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

// climb moves an entity up the ladder to `target`, granting approve throughout.
func (f fixture) climb(t *testing.T, entity string, id uuid.UUID, target string) {
	t.Helper()
	route := []string{
		cms.StatusTechnicalReview, cms.StatusLanguageReview,
		cms.StatusApproved, cms.StatusPublished,
	}
	for _, step := range route {
		if _, err := f.svc.Transition(context.Background(), f.actor, cms.TransitionInput{
			EntityType: entity, EntityID: id, To: step, HasApprove: true,
		}); err != nil {
			t.Fatalf("climbing to %s: %v", step, err)
		}
		if step == target {
			return
		}
	}
}

func ptr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// The ladder
// ---------------------------------------------------------------------------

func TestLadderIsExhaustive(t *testing.T) {
	t.Parallel()

	all := []string{
		cms.StatusDraft, cms.StatusTechnicalReview, cms.StatusLanguageReview,
		cms.StatusApproved, cms.StatusPublished,
	}
	legal := map[string]bool{}
	for _, tr := range cms.Ladder() {
		legal[tr.From+"->"+tr.To] = true
	}

	// Every pair is checked, so a transition added without thought fails here.
	for _, from := range all {
		for _, to := range all {
			_, ok := cms.Lookup(from, to)
			if ok != legal[from+"->"+to] {
				t.Errorf("%s->%s: Lookup says %v, ladder says %v", from, to, ok, legal[from+"->"+to])
			}
			if from == to && ok {
				t.Errorf("%s->%s is a no-op but is declared legal", from, to)
			}
		}
	}

	// Forward moves are one rung at a time. A draft that could jump straight to
	// published would make both reviews decorative.
	for _, skip := range [][2]string{
		{cms.StatusDraft, cms.StatusLanguageReview},
		{cms.StatusDraft, cms.StatusApproved},
		{cms.StatusDraft, cms.StatusPublished},
		{cms.StatusTechnicalReview, cms.StatusApproved},
		{cms.StatusTechnicalReview, cms.StatusPublished},
		{cms.StatusLanguageReview, cms.StatusPublished},
	} {
		if _, ok := cms.Lookup(skip[0], skip[1]); ok {
			t.Errorf("%s->%s skips a rung of the ladder", skip[0], skip[1])
		}
	}
}

// Утверждено and Опубликовано need cms:approve; the two review rungs do not.
// Publishing is a claim the company makes in public and someone signs it; a
// review is not an act of authority.
func TestOnlyApprovalAndPublicationRequireApprove(t *testing.T) {
	t.Parallel()

	needsApprove := map[string]bool{}
	for _, tr := range cms.Ladder() {
		needsApprove[tr.From+"->"+tr.To] = tr.RequiresApprove
	}

	for move, want := range map[string]bool{
		"draft->technical_review":           false,
		"technical_review->language_review": false,
		"language_review->approved":         true,
		"approved->published":               true,
		// Back down never needs authority: refusing to approve is not itself an
		// act of approval, and a reviewer who spots a mistake must be able to
		// report it.
		"technical_review->draft":           false,
		"language_review->draft":            false,
		"language_review->technical_review": false,
		"approved->draft":                   false,
		"approved->language_review":         false,
		// Unpublishing IS authority: it removes something already said in public.
		"published->approved": true,
		"published->draft":    true,
	} {
		if got := needsApprove[move]; got != want {
			t.Errorf("%s requiresApprove = %v, want %v", move, got, want)
		}
	}
}

func TestApprovingWithoutTheGrantIsRefused(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	id := f.page(t, "home")

	// The two review rungs need no authority.
	for _, step := range []string{cms.StatusTechnicalReview, cms.StatusLanguageReview} {
		if _, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
			EntityType: cms.EntityPage, EntityID: id, To: step,
		}); err != nil {
			t.Fatalf("moving to %s without approve: %v", step, err)
		}
	}

	_, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
		EntityType: cms.EntityPage, EntityID: id, To: cms.StatusApproved,
	})
	if err == nil {
		t.Fatal("content was approved without cms:approve")
	}
	if code := common.AsError(err).Code; code != common.CodeForbidden {
		t.Errorf("code = %s, want forbidden", code)
	}

	if _, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
		EntityType: cms.EntityPage, EntityID: id, To: cms.StatusApproved, HasApprove: true,
	}); err != nil {
		t.Fatalf("approving with the grant: %v", err)
	}
}

func TestIllegalTransitionsAreRefused(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	id := f.page(t, "products")

	if _, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
		EntityType: cms.EntityPage, EntityID: id, To: cms.StatusPublished, HasApprove: true,
	}); err == nil {
		t.Error("a draft was published without passing through review")
	}
	if _, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
		EntityType: "invoice", EntityID: id, To: cms.StatusDraft,
	}); err == nil {
		t.Error("an unknown entity type was accepted")
	}
	if _, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
		EntityType: cms.EntityPage, EntityID: uuid.New(), To: cms.StatusTechnicalReview,
	}); err == nil {
		t.Error("an unknown page was transitioned")
	}
}

func TestEveryTransitionIsRecordedAndAudited(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	id := f.page(t, "production")

	if _, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
		EntityType: cms.EntityPage, EntityID: id,
		To: cms.StatusTechnicalReview, Comment: ptr("Готово к проверке"),
	}); err != nil {
		t.Fatal(err)
	}

	history, err := f.svc.History(ctx, cms.EntityPage, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("%d workflow events, want 1", len(history))
	}
	if history[0].ToStatus != cms.StatusTechnicalReview {
		t.Errorf("to_status = %s", history[0].ToStatus)
	}
	if history[0].ActorName != "Редактор" {
		t.Errorf("actor = %s, want the editor's name", history[0].ActorName)
	}
	if history[0].Comment == nil || *history[0].Comment != "Готово к проверке" {
		t.Errorf("comment = %v", history[0].Comment)
	}

	// And the audit trail records it as `update` — a review is not an approval.
	testsupport.AssertAudited(t, f.pool, "cms", id, "update")

	// Approving is audited as `approve`. The verb is the whole reason the audit
	// write lives in the domain rather than in a trigger.
	if _, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
		EntityType: cms.EntityPage, EntityID: id, To: cms.StatusLanguageReview,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
		EntityType: cms.EntityPage, EntityID: id, To: cms.StatusApproved, HasApprove: true,
	}); err != nil {
		t.Fatal(err)
	}
	testsupport.AssertAudited(t, f.pool, "cms", id, "approve")
}

func TestAllowedFromMatchesWhatTheDomainEnforces(t *testing.T) {
	t.Parallel()

	// Without approve, an approved page may only be sent back.
	got := cms.AllowedFrom(cms.StatusApproved, false)
	for _, to := range got {
		if to == cms.StatusPublished {
			t.Error("AllowedFrom offered publication without approve")
		}
	}

	// Every destination the projection offers must actually be legal, or the UI
	// would render a button the domain then refuses.
	for _, from := range []string{
		cms.StatusDraft, cms.StatusTechnicalReview, cms.StatusLanguageReview,
		cms.StatusApproved, cms.StatusPublished,
	} {
		for _, hasApprove := range []bool{false, true} {
			for _, to := range cms.AllowedFrom(from, hasApprove) {
				rule, ok := cms.Lookup(from, to)
				if !ok {
					t.Errorf("AllowedFrom(%s, %v) offered an illegal move to %s", from, hasApprove, to)
				}
				if rule.RequiresApprove && !hasApprove {
					t.Errorf("AllowedFrom(%s, false) offered %s, which needs approve", from, to)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Published content is frozen
// ---------------------------------------------------------------------------

// The failure this prevents: anyone with cms:manage rewrites an approved page
// and the edit lands live without passing a single rung. That would make the
// whole ladder decorative.
func TestPublishedContentCannotBeEdited(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	id := f.page(t, "home")
	f.climb(t, cms.EntityPage, id, cms.StatusPublished)

	_, err := f.svc.SaveBlock(ctx, f.actor, cms.BlockInput{
		PageID: id, BlockKey: "hero", Locale: "ru", Heading: ptr("Новый заголовок"),
	})
	if err == nil {
		t.Fatal("a published page was edited in place")
	}
	if !strings.Contains(common.AsError(err).Message, "черновик") {
		t.Errorf("the refusal does not say how to proceed: %s", common.AsError(err).Message)
	}

	// Sending it back to draft makes it editable again — and that step is itself
	// recorded and attributable.
	if _, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
		EntityType: cms.EntityPage, EntityID: id, To: cms.StatusDraft, HasApprove: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SaveBlock(ctx, f.actor, cms.BlockInput{
		PageID: id, BlockKey: "hero", Locale: "ru", Heading: ptr("Новый заголовок"),
	}); err != nil {
		t.Errorf("a page returned to draft is still frozen: %v", err)
	}
}

func TestPublishedNewsCannotBeEdited(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	id := f.post(t, "zapusk-linii")
	for _, locale := range cms.Locales {
		if _, err := f.svc.SaveNewsTranslation(ctx, f.actor, cms.NewsTranslationInput{
			PostID: id, Locale: locale, Title: "Запуск линии",
		}); err != nil {
			t.Fatal(err)
		}
	}
	f.climb(t, cms.EntityNews, id, cms.StatusPublished)

	if _, err := f.svc.SaveNewsTranslation(ctx, f.actor, cms.NewsTranslationInput{
		PostID: id, Locale: "ru", Title: "Другой заголовок",
	}); err == nil {
		t.Error("a published news post was edited in place")
	}
}

// ---------------------------------------------------------------------------
// The three locales ship together
// ---------------------------------------------------------------------------

// D10: the platform ships ru/tg/en and all three are live at launch. Publishing a
// post translated only into Russian would put a Russian headline on the Tajik
// site, which is worse than the post not being there.
func TestPublishingNeedsAllThreeTranslations(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	id := f.post(t, "montazh-oborudovaniya")
	if _, err := f.svc.SaveNewsTranslation(ctx, f.actor, cms.NewsTranslationInput{
		PostID: id, Locale: "ru", Title: "Монтаж оборудования",
	}); err != nil {
		t.Fatal(err)
	}

	// Up to `approved` is fine — the check is at publication, so a post can move
	// through review while translations are still being written.
	for _, step := range []string{cms.StatusTechnicalReview, cms.StatusLanguageReview, cms.StatusApproved} {
		if _, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
			EntityType: cms.EntityNews, EntityID: id, To: step, HasApprove: true,
		}); err != nil {
			t.Fatalf("moving to %s: %v", step, err)
		}
	}

	_, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
		EntityType: cms.EntityNews, EntityID: id, To: cms.StatusPublished, HasApprove: true,
	})
	if err == nil {
		t.Fatal("a post with one translation was published")
	}
	msg := common.AsError(err).Message
	// The message names which languages are missing, so the editor knows what to
	// do rather than being told only that they cannot proceed.
	if !strings.Contains(msg, "tg") || !strings.Contains(msg, "en") {
		t.Errorf("the refusal does not name the missing languages: %s", msg)
	}

	for _, locale := range []string{"tg", "en"} {
		if _, err := f.svc.SaveNewsTranslation(ctx, f.actor, cms.NewsTranslationInput{
			PostID: id, Locale: locale, Title: "Монтаж оборудования",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.svc.Transition(ctx, f.actor, cms.TransitionInput{
		EntityType: cms.EntityNews, EntityID: id, To: cms.StatusPublished, HasApprove: true,
	}); err != nil {
		t.Errorf("publishing a fully translated post: %v", err)
	}
}

// A blank title is not a translation. Without this, saving an empty string in
// two locales would satisfy the publication check while putting empty headlines
// on two of the three sites.
func TestABlankTranslationDoesNotCount(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	id := f.post(t, "test-post")
	if _, err := f.svc.SaveNewsTranslation(ctx, f.actor, cms.NewsTranslationInput{
		PostID: id, Locale: "ru", Title: "   ",
	}); err == nil {
		t.Error("a blank title was accepted as a translation")
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func TestSlugValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	// A slug is a public URL segment. Cyrillic would percent-encode into
	// something unreadable wherever it is pasted, and a slash would produce a
	// nested path nobody intended.
	for name, slug := range map[string]string{
		"empty":     "",
		"cyrillic":  "запуск-линии",
		"slash":     "news/launch",
		"space":     "launch day",
		"uppercase": "Launch",
		"leading":   "-launch",
		"trailing":  "launch-",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.svc.CreateNews(ctx, f.actor, cms.NewsInput{Slug: slug}); err == nil {
				t.Errorf("accepted %q", slug)
			}
		})
	}

	if _, err := f.svc.CreateNews(ctx, f.actor, cms.NewsInput{Slug: "zapusk-2026"}); err != nil {
		t.Errorf("a valid slug was refused: %v", err)
	}
}

func TestLocaleValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	page := f.page(t, "contacts")

	// `tj` is a country TLD, not a language code. The schema's CHECK constraint
	// enforces `tg` (docs/07-IMPLEMENTATION-PLAN.md C2), and this refuses it
	// before it reaches the constraint so the error names the field.
	if _, err := f.svc.SaveBlock(ctx, f.actor, cms.BlockInput{
		PageID: page, BlockKey: "hero", Locale: "tj", Heading: ptr("x"),
	}); err == nil {
		t.Error("locale `tj` was accepted")
	}
	if _, err := f.svc.Blocks(ctx, page, "tj"); err == nil {
		t.Error("blocks were read for locale `tj`")
	}
	for _, locale := range cms.Locales {
		if _, err := f.svc.SaveBlock(ctx, f.actor, cms.BlockInput{
			PageID: page, BlockKey: "hero", Locale: locale, Heading: ptr("Заголовок"),
		}); err != nil {
			t.Errorf("locale %s was refused: %v", locale, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// A block with no translation still appears in the editor. An editor cannot fill
// in a gap they cannot see.
func TestBlocksWithNoTranslationStillAppear(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	page := f.page(t, "home")

	if _, err := f.svc.SaveBlock(ctx, f.actor, cms.BlockInput{
		PageID: page, BlockKey: "hero", Locale: "ru", Heading: ptr("Заголовок"),
	}); err != nil {
		t.Fatal(err)
	}

	blocks, err := f.svc.Blocks(ctx, page, "tg")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 {
		t.Fatalf("%d blocks for an untranslated locale, want 1", len(blocks))
	}
	if blocks[0].Heading != nil {
		t.Errorf("heading = %v for a locale with no translation", blocks[0].Heading)
	}
	if blocks[0].BlockKey != "hero" {
		t.Errorf("block key = %s", blocks[0].BlockKey)
	}
}

// Saving the same block twice updates it rather than creating a second one.
func TestSavingABlockTwiceUpdatesIt(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	page := f.page(t, "home")

	for _, heading := range []string{"Первый", "Второй"} {
		if _, err := f.svc.SaveBlock(ctx, f.actor, cms.BlockInput{
			PageID: page, BlockKey: "hero", Locale: "ru", Heading: ptr(heading),
		}); err != nil {
			t.Fatal(err)
		}
	}
	blocks, err := f.svc.Blocks(ctx, page, "ru")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 {
		t.Fatalf("%d blocks, want 1 — the upsert created a duplicate", len(blocks))
	}
	if blocks[0].Heading == nil || *blocks[0].Heading != "Второй" {
		t.Errorf("heading = %v, want the second value", blocks[0].Heading)
	}
}

// Exactly one status is public. `approved` means review is finished, not that
// the client has decided to release it.
func TestOnlyPublishedIsPublic(t *testing.T) {
	t.Parallel()
	for status, want := range map[string]bool{
		cms.StatusDraft:           false,
		cms.StatusTechnicalReview: false,
		cms.StatusLanguageReview:  false,
		cms.StatusApproved:        false,
		cms.StatusPublished:       true,
	} {
		if got := cms.IsPublic(status); got != want {
			t.Errorf("IsPublic(%s) = %v, want %v", status, got, want)
		}
	}
}

func TestMediaAltIsVersionGuarded(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	var id uuid.UUID
	var version int32
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO media (file_path, mime_type) VALUES ('hero.jpg','image/jpeg')
		RETURNING id, version`).Scan(&id, &version); err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.SetMediaAlt(ctx, f.actor, cms.MediaAltInput{
		ID: id, AltRU: ptr("Линия розлива"), Version: version,
	}); err != nil {
		t.Fatal(err)
	}
	// The second write carries the version the first superseded.
	_, err := f.svc.SetMediaAlt(ctx, f.actor, cms.MediaAltInput{
		ID: id, AltRU: ptr("Другое описание"), Version: version,
	})
	if err == nil {
		t.Fatal("a stale version was accepted")
	}
	if code := common.AsError(err).Code; code != common.CodeVersionConflict {
		t.Errorf("code = %s, want version_conflict", code)
	}
}
