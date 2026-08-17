package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/cms"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// CMS — docs/05-MODULES.md §15.

// cmsStatus renders a rung of the ladder.
//
// Only `published` is green. `approved` is info, not ok: it means review is
// finished, not that the content is live, and colouring it as healthy would let
// an editor believe the job was done.
func cmsStatus(status string) api.Status {
	switch status {
	case cms.StatusTechnicalReview:
		return api.Status{Key: status, Label: "Техническая проверка", Level: string(common.LevelWarn)}
	case cms.StatusLanguageReview:
		return api.Status{Key: status, Label: "Языковая проверка", Level: string(common.LevelWarn)}
	case cms.StatusApproved:
		return api.Status{Key: status, Label: "Утверждено", Level: string(common.LevelInfo)}
	case cms.StatusPublished:
		return api.Status{Key: status, Label: "Опубликовано", Level: string(common.LevelOK)}
	default:
		return api.Status{Key: status, Label: "Черновик", Level: string(common.LevelNeutral)}
	}
}

// cmsLocale reads the ?locale parameter, defaulting to Russian.
func cmsLocale(r *http.Request) string {
	locale := r.URL.Query().Get("locale")
	for _, l := range cms.Locales {
		if l == locale {
			return locale
		}
	}
	return "ru"
}

func (s *Server) hasCMSApprove(r *http.Request) bool {
	ident, ok := identityFrom(r)
	if !ok {
		return false
	}
	return rbac.NewSet(ident.Permissions).Can(rbac.CMS, rbac.Approve)
}

// ---------------------------------------------------------------------------
// Pages
// ---------------------------------------------------------------------------

func (s *Server) handleListContentPages(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.CMS.Pages(r.Context(), optionalQuery(r, "status"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	hasApprove := s.hasCMSApprove(r)
	out := make([]api.ContentPage, 0, len(rows))
	for _, p := range rows {
		out = append(out, api.ContentPage{
			ID: p.ID.String(), Key: p.Key,
			BlockCount:         p.BlockCount,
			PublishedAt:        common.NullTimestamp(p.PublishedAt),
			Status:             cmsStatus(p.Status),
			Version:            p.Version,
			AllowedTransitions: cms.AllowedFrom(p.Status, hasApprove),
		})
	}
	common.JSON(w, http.StatusOK, out)
}

func (s *Server) handleListContentBlocks(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, err := s.svc.CMS.Blocks(r.Context(), id, cmsLocale(r))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	locale := cmsLocale(r)
	out := make([]api.ContentBlock, 0, len(rows))
	for _, b := range rows {
		out = append(out, api.ContentBlock{
			ID: b.ID.String(), BlockKey: b.BlockKey,
			SortOrder: b.SortOrder, Locale: locale,
			Heading: b.Heading, Body: b.Body, CTALabel: b.CtaLabel,
		})
	}
	common.JSON(w, http.StatusOK, out)
}

func (s *Server) handleSaveContentBlock(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var req api.ContentBlockWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	block, err := s.svc.CMS.SaveBlock(r.Context(), ident.User.ID, cms.BlockInput{
		PageID: id, BlockKey: req.BlockKey, SortOrder: req.SortOrder,
		Locale: req.Locale, Heading: req.Heading, Body: req.Body, CTALabel: req.CTALabel,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, api.ContentBlock{
		ID: block.BlockID.String(), BlockKey: req.BlockKey, SortOrder: req.SortOrder,
		Locale: block.Locale, Heading: block.Heading, Body: block.Body,
		CTALabel: block.CtaLabel,
	})
}

func (s *Server) handleTransitionContentPage(w http.ResponseWriter, r *http.Request) {
	s.transitionCMS(w, r, cms.EntityPage)
}

func (s *Server) handleTransitionNewsPost(w http.ResponseWriter, r *http.Request) {
	s.transitionCMS(w, r, cms.EntityNews)
}

// transitionCMS serves both entity types.
//
// One handler because the ladder is one ladder — duplicating it would let pages
// and posts drift, at which point "published" would mean two different things.
func (s *Server) transitionCMS(w http.ResponseWriter, r *http.Request, entityType string) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var req api.CMSTransitionRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	hasApprove := rbac.NewSet(ident.Permissions).Can(rbac.CMS, rbac.Approve)
	status, err := s.svc.CMS.Transition(r.Context(), ident.User.ID, cms.TransitionInput{
		EntityType: entityType, EntityID: id, To: req.To,
		Comment: req.Comment, HasApprove: hasApprove,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, map[string]any{
		"status":              cmsStatus(status),
		"allowed_transitions": cms.AllowedFrom(status, hasApprove),
	})
}

func (s *Server) handleContentHistory(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	entityType := cms.EntityPage
	if chi.RouteContext(r.Context()) != nil &&
		len(r.URL.Path) > 0 && containsSegment(r.URL.Path, "news") {
		entityType = cms.EntityNews
	}
	rows, err := s.svc.CMS.History(r.Context(), entityType, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.WorkflowEvent, 0, len(rows))
	for _, e := range rows {
		event := api.WorkflowEvent{
			ID:         e.ID.String(),
			ToStatus:   cmsStatus(e.ToStatus),
			ActorName:  e.ActorName,
			Comment:    e.Comment,
			OccurredAt: common.Timestamp(e.OccurredAt),
		}
		if e.FromStatus != nil {
			from := cmsStatus(*e.FromStatus)
			event.FromStatus = &from
		}
		out = append(out, event)
	}
	common.JSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// News
// ---------------------------------------------------------------------------

func (s *Server) handleListNewsPosts(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, cms.NewsSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.CMS.News(r.Context(), params, cmsLocale(r), optionalQuery(r, "status"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	hasApprove := s.hasCMSApprove(r)

	out := make([]api.NewsPost, 0, len(rows))
	for _, n := range rows {
		// The missing-translation list is computed per row so the editor sees the
		// gap in the list rather than discovering it when publication is refused.
		translations, err := s.svc.CMS.NewsTranslations(r.Context(), n.ID)
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		missing := missingLocaleNames(translations)
		out = append(out, api.NewsPost{
			ID: n.ID.String(), Slug: n.Slug, Title: &n.Title,
			Category:           n.Category,
			PublishedOn:        common.Date(n.PublishedOn),
			Status:             cmsStatus(n.Status),
			Version:            n.Version,
			MissingLocales:     missing,
			AllowedTransitions: cms.AllowedFrom(n.Status, hasApprove),
		})
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

func (s *Server) handleCreateNewsPost(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.NewsPostWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	publishedOn, err := parseDate(deref(req.PublishedOn), "published_on", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	post, err := s.svc.CMS.CreateNews(r.Context(), ident.User.ID, cms.NewsInput{
		Slug: req.Slug, Category: req.Category, PublishedOn: publishedOn,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.NewsPost{
		ID: post.ID.String(), Slug: post.Slug, Category: post.Category,
		PublishedOn: common.Date(post.PublishedOn),
		Status:      cmsStatus(post.Status), Version: post.Version,
		// A new post has no translations at all, which is the honest answer.
		MissingLocales:     append([]string(nil), cms.Locales...),
		AllowedTransitions: cms.AllowedFrom(post.Status, s.hasCMSApprove(r)),
	})
}

func (s *Server) handleListNewsTranslations(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, err := s.svc.CMS.NewsTranslations(r.Context(), id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.NewsTranslation, 0, len(rows))
	for _, t := range rows {
		out = append(out, api.NewsTranslation{
			Locale: t.Locale, Title: t.Title, Excerpt: t.Excerpt, Body: t.Body,
		})
	}
	common.JSON(w, http.StatusOK, out)
}

func (s *Server) handleSaveNewsTranslation(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var req api.NewsTranslationWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	translation, err := s.svc.CMS.SaveNewsTranslation(r.Context(), ident.User.ID,
		cms.NewsTranslationInput{
			PostID: id, Locale: req.Locale, Title: req.Title,
			Excerpt: req.Excerpt, Body: req.Body,
		})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, api.NewsTranslation{
		Locale: translation.Locale, Title: translation.Title,
		Excerpt: translation.Excerpt, Body: translation.Body,
	})
}

// ---------------------------------------------------------------------------
// Media
// ---------------------------------------------------------------------------

func (s *Server) handleListMedia(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, cms.MediaSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.CMS.Media(r.Context(), params)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.MediaItem, 0, len(rows))
	for _, m := range rows {
		out = append(out, mediaResponse(m))
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

func (s *Server) handleSetMediaAlt(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var req api.MediaAltWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	version, err := common.RequireVersion(common.Versioned{Version: req.Version})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	updated, err := s.svc.CMS.SetMediaAlt(r.Context(), ident.User.ID, cms.MediaAltInput{
		ID: id, AltRU: req.AltRU, AltTG: req.AltTG, AltEN: req.AltEN, Version: version,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, mediaResponse(updated))
}

func mediaResponse(m db.Medium) api.MediaItem {
	return api.MediaItem{
		ID: m.ID.String(), FilePath: m.FilePath, MimeType: m.MimeType,
		Width: m.Width, Height: m.Height, SizeBytes: m.SizeBytes,
		AltRU: m.AltRu, AltTG: m.AltTg, AltEN: m.AltEn, Version: m.Version,
	}
}

// missingLocaleNames names the locales with no usable translation, in the
// canonical order so the list is stable rather than map-ordered.
func missingLocaleNames(rows []db.NewsPostTranslation) []string {
	present := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.Title != "" {
			present[r.Locale] = true
		}
	}
	missing := make([]string, 0, len(cms.Locales))
	for _, l := range cms.Locales {
		if !present[l] {
			missing = append(missing, l)
		}
	}
	return missing
}

func containsSegment(path, segment string) bool {
	for _, part := range splitPath(path) {
		if part == segment {
			return true
		}
	}
	return false
}

func splitPath(path string) []string {
	var out []string
	current := ""
	for _, r := range path {
		if r == '/' {
			if current != "" {
				out = append(out, current)
			}
			current = ""
			continue
		}
		current += string(r)
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}
