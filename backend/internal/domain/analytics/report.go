package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/qoim/samari/backend/internal/db"
)

// Report is what the two dashboard panels render.
//
// Everything here reads analytics_daily and never the raw table. The raw rows
// are gone after ninety days and the panels must keep working — a chart that
// silently loses its history at the retention boundary would be worse than no
// chart, because nobody would notice.
type Report struct {
	// Visits is distinct sessions, not events. ProductVisits is how many of them
	// looked at a product at all.
	Visits        int
	ProductVisits int
	Products      []ProductRow
	Links         []LinkRow
}

// ProductRow is one line of «Что смотрят на сайте». Ranked by Visits: ten views
// in one session are one visit (docs/01-DECISIONS.md D12).
type ProductRow struct {
	SKU    string
	Name   string
	Visits int
	Views  int
}

// LinkRow is one line of «Популярные ссылки». Only `cta` and `outbound` reach
// here; nav and footer are captured but never shown.
type LinkRow struct {
	Target   string
	Category string
	Visits   int
	Clicks   int
}

// Since maps a dashboard period onto a start date. The period switch already
// exists on the dashboard, so analytics reuses it rather than inventing a second
// date control.
func Since(now time.Time, period string) time.Time {
	switch period {
	case "day":
		return now.AddDate(0, 0, -1)
	case "week":
		return now.AddDate(0, 0, -7)
	case "quarter":
		return now.AddDate(0, -3, 0)
	default: // month
		return now.AddDate(0, -1, 0)
	}
}

// Report reads both panels for one period and locale.
func (s *Service) Report(ctx context.Context, period, locale string, limit int32) (Report, error) {
	var out Report
	q := db.New(s.pool)
	since := pgtype.Date{Time: Since(s.now(), period), Valid: true}

	totals, err := q.AnalyticsVisitTotals(ctx, since)
	if err != nil {
		return out, fmt.Errorf("analytics: totals: %w", err)
	}
	out.Visits = int(totals.Visits)
	out.ProductVisits = int(totals.ProductVisits)

	products, err := q.TopProductsByVisits(ctx, db.TopProductsByVisitsParams{
		Limit: limit, Since: since, Locale: locale,
	})
	if err != nil {
		return out, fmt.Errorf("analytics: products: %w", err)
	}
	for _, p := range products {
		out.Products = append(out.Products, ProductRow{
			SKU: p.Sku, Name: p.Name, Visits: int(p.Visits), Views: int(p.Views),
		})
	}

	links, err := q.TopLinksByVisits(ctx, db.TopLinksByVisitsParams{Limit: limit, Since: since})
	if err != nil {
		return out, fmt.Errorf("analytics: links: %w", err)
	}
	for _, l := range links {
		out.Links = append(out.Links, LinkRow{
			Target: l.Target, Category: l.Category, Visits: int(l.Visits), Clicks: int(l.Clicks),
		})
	}

	return out, nil
}
