package http

import (
	"net/http"
	"net/netip"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/domain/analytics"
	"github.com/qoim/samari/backend/internal/http/common"
)

// Аналитика сайта — docs/01-DECISIONS.md D12.

// handleAnalyticsCollect is the second unauthenticated write in the system, and
// unlike the first it takes traffic on every click rather than every form.
//
// It answers 204 for everything: a valid batch, a batch of nonsense, a malformed
// body. Three reasons, all deliberate:
//
//   - Validation feedback on an anonymous endpoint is a probing oracle. A caller
//     who learns WHICH of their forged SKUs was rejected has been handed a
//     catalogue enumeration tool.
//   - A beacon must never surface an error into the page. sendBeacon gives the
//     browser no way to react anyway, so a status code is for our logs, not the
//     visitor's experience.
//   - Analytics failing must never look like the site failing.
//
// The rate limit is the one exception, and only because a 429 lets an honest
// client back off rather than hammering.
func (s *Server) handleAnalyticsCollect(w http.ResponseWriter, r *http.Request) {
	var req api.AnalyticsBatchRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		// Not a validation error to the caller. Malformed telemetry is dropped.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	events := make([]analytics.Event, 0, len(req.Events))
	for _, e := range req.Events {
		events = append(events, analytics.Event{
			Kind: e.Kind, Target: e.Target, Source: e.Source,
			Category: e.Category, Locale: e.Locale, SKU: e.SKU,
		})
	}

	_, err := s.svc.Analytics.Ingest(r.Context(), analytics.Batch{
		SessionID: req.SessionID,
		// clientIP reads the forwarded header explicitly, and only because
		// nothing but the BFF can reach this port (I8/I18). It is hashed with a
		// salt before storage and never written down raw.
		IP:     ipString(clientIP(r)),
		Events: events,
	})
	if err != nil {
		// Rate limits and database faults both surface through the normal error
		// path — a 429 lets an honest client back off, and a 500 belongs in our
		// logs. Everything BELOW an error, including every dropped event, is a
		// 204 with no explanation.
		common.Fail(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleAnalyticsReport serves both dashboard panels.
//
// Guarded on analytics:read, which only admin and director hold. Reads
// analytics_daily rather than the raw table, so the panels keep working after
// the ninety-day window has emptied it.
func (s *Server) handleAnalyticsReport(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}
	locale := r.URL.Query().Get("locale")
	if locale == "" {
		locale = "ru"
	}

	report, err := s.svc.Analytics.Report(r.Context(), period, locale, 8)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	out := api.AnalyticsReport{
		Period:        period,
		Visits:        int32(report.Visits),
		ProductVisits: int32(report.ProductVisits),
		Products:      make([]api.AnalyticsProductRow, 0, len(report.Products)),
		Links:         make([]api.AnalyticsLinkRow, 0, len(report.Links)),
	}
	for _, p := range report.Products {
		out.Products = append(out.Products, api.AnalyticsProductRow{
			SKU: p.SKU, Name: p.Name, Visits: int32(p.Visits), Views: int32(p.Views),
		})
	}
	for _, l := range report.Links {
		out.Links = append(out.Links, api.AnalyticsLinkRow{
			Target: l.Target, Category: l.Category,
			Visits: int32(l.Visits), Clicks: int32(l.Clicks),
		})
	}
	common.JSON(w, http.StatusOK, out)
}

// ipString renders the address for hashing. An unparseable or absent address
// hashes as the empty string, which buckets every such caller together — the
// correct failure mode for a rate limit, since it is stricter rather than looser.
func ipString(addr *netip.Addr) string {
	if addr == nil {
		return ""
	}
	return addr.String()
}
