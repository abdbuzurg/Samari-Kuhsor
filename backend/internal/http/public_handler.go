package http

import (
	"net/http"
	"strconv"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/http/common"
)

// The public surface — docs/03-API-CONTRACT.md §9.
//
// Four endpoints, all read-only except the enquiry form. Every one of them is
// still behind the service key: "public" means "no user session", not "open to
// the internet". The Go API publishes no host port at all in production, so the
// only way to reach these is through our own website container (I8/I18).

// publicLocale reads and validates the ?locale parameter.
//
// An unrecognised value falls back to Russian rather than erroring. This is a
// public page: a stale link carrying ?locale=tj must render the site, not a
// validation failure a visitor cannot act on.
func publicLocale(r *http.Request) string {
	switch r.URL.Query().Get("locale") {
	case "tg":
		return "tg"
	case "en":
		return "en"
	default:
		return "ru"
	}
}

func (s *Server) handlePublicProducts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.Items.PublicList(r.Context(), publicLocale(r))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.PublicProduct, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.PublicProduct{
			ID: row.ID.String(), SKU: row.Sku, Name: row.Name,
			Description: row.Description, Category: row.Category,
		})
	}
	common.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (s *Server) handlePublicProduct(w http.ResponseWriter, r *http.Request) {
	sku := chiURLParam(r, "sku")
	row, err := s.svc.Items.PublicOne(r.Context(), publicLocale(r), sku)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, map[string]any{"data": api.PublicProduct{
		ID: row.ID.String(), SKU: row.Sku, Name: row.Name,
		Description: row.Description, Category: row.Category,
	}})
}

func (s *Server) handlePublicNews(w http.ResponseWriter, r *http.Request) {
	limit := int32(6)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 50 {
			limit = int32(n)
		}
	}
	rows, err := s.svc.Items.PublicNews(r.Context(), publicLocale(r), limit)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.PublicNewsItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.PublicNewsItem{
			ID: row.ID.String(), Slug: row.Slug, Category: row.Category,
			PublishedOn: common.Date(row.PublishedOn),
			Title:       row.Title, Excerpt: row.Excerpt,
		})
	}
	common.JSON(w, http.StatusOK, map[string]any{"data": out})
}
