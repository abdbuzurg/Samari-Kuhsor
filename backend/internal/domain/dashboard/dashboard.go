// Package dashboard assembles Панель управления.
//
// docs/05-MODULES.md §2. Built last, because it aggregates every module beneath
// it — a panel cannot summarise a module that does not exist.
//
// Two rules govern everything here.
//
// First: 05-MODULES.md:70 — "Do not fabricate figures, and do not carry the
// prototype's sample numbers into production." On opening day the factory has
// produced nothing and sold nothing, so every figure below is zero. That is the
// correct answer, and a dashboard showing 2 480 000 c. of revenue for a factory
// that has made nothing is a lie the client would reasonably act on.
//
// Second: every panel is filtered by the viewer's permissions. A dashboard is
// the one screen that would otherwise leak every module at once — a warehouse
// operator with no CRM grant must not learn the month's revenue from the landing
// page they cannot avoid seeing.
package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/alerts"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Period is the window the revenue panel covers. The prototype's switcher reads
// Сегодня / Неделя / Месяц / Квартал.
type Period string

const (
	PeriodDay     Period = "day"
	PeriodWeek    Period = "week"
	PeriodMonth   Period = "month"
	PeriodQuarter Period = "quarter"
)

// Days converts a period to its window. An unrecognised value falls back to a
// month rather than erroring: this is a display window, and refusing to render
// the dashboard over a bad query parameter helps nobody.
func (p Period) Days() int32 {
	switch p {
	case PeriodDay:
		return 1
	case PeriodWeek:
		return 7
	case PeriodQuarter:
		return 90
	default:
		return 30
	}
}

func ParsePeriod(s string) Period {
	switch Period(s) {
	case PeriodDay, PeriodWeek, PeriodMonth, PeriodQuarter:
		return Period(s)
	default:
		return PeriodMonth
	}
}

// Snapshot is everything the dashboard shows, with each panel present only when
// the viewer may read the module behind it.
type Snapshot struct {
	Period Period

	Sales      *SalesPanel
	Stock      *StockPanel
	Quality    *QualityPanel
	Production *ProductionPanel
	Pipeline   []PipelineStage
	StockRows  []StockRow
	Recent     []RecentOrder
	Feed       []FeedEntry
	Revenue    []RevenuePoint
}

type SalesPanel struct {
	Revenue    decimal.Decimal
	OrderCount int
	OpenOrders int
	OverduePOs int
}

type StockPanel struct {
	Value        decimal.Decimal
	BelowMinimum int
}

type QualityPanel struct {
	Quarantined int
}

type ProductionPanel struct {
	GoodQty    decimal.Decimal
	ScrapQty   decimal.Decimal
	PlannedQty decimal.Decimal
}

// StockRow is one line of the Запасы panel.
//
// `Detail` is assembled here rather than in the handler because what identifies
// a position differs by what it is: a finished good is known by its batch, a raw
// material by where it sits. The panel has one line of room for whichever it is.
type StockRow struct {
	SKU      string
	Name     string
	Detail   string
	OnHand   decimal.Decimal
	UOM      string
	Expiring bool
	Low      bool
}

type PipelineStage struct {
	Stage  string
	Count  int
	Amount decimal.Decimal
}

type RecentOrder struct {
	ID           string
	SONo         string
	CustomerName string
	Status       string
	Total        decimal.Decimal
	CreatedAt    string
}

type FeedEntry struct {
	ID         string
	Action     string
	Resource   string
	ResourceID string
	ActorName  string
	OccurredAt string
}

type RevenuePoint struct {
	Day        string
	Revenue    decimal.Decimal
	OrderCount int
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Build assembles the snapshot for one viewer.
//
// Each panel is guarded on the module it summarises, so a permission the user
// does not hold produces a nil panel rather than a zero — the frontend renders
// nothing at all rather than "0 c." for a figure the user is not entitled to.
// Those two look identical on a screen and mean completely different things.
func (s *Service) Build(ctx context.Context, perms rbac.Set, period Period) (Snapshot, error) {
	q := db.New(s.pool)
	out := Snapshot{Period: period}
	days := period.Days()

	if perms.CanRead(rbac.CRM) {
		totals, err := q.DashboardSalesTotals(ctx, days)
		if err != nil {
			return out, fmt.Errorf("dashboard: sales: %w", err)
		}
		open, err := q.DashboardOpenOrders(ctx)
		if err != nil {
			return out, fmt.Errorf("dashboard: open orders: %w", err)
		}
		panel := &SalesPanel{
			Revenue: totals.Revenue, OrderCount: int(totals.OrderCount), OpenOrders: int(open),
		}
		// Overdue deliveries belong to procurement, so the figure only appears for
		// someone who may read it — the panel is still shown either way.
		if perms.CanRead(rbac.Procurement) {
			overdue, err := q.DashboardOverduePurchases(ctx)
			if err != nil {
				return out, fmt.Errorf("dashboard: overdue: %w", err)
			}
			panel.OverduePOs = int(overdue)
		}
		out.Sales = panel

		recent, err := q.DashboardRecentOrders(ctx, 6)
		if err != nil {
			return out, fmt.Errorf("dashboard: recent orders: %w", err)
		}
		for _, r := range recent {
			out.Recent = append(out.Recent, RecentOrder{
				ID: r.ID.String(), SONo: r.SoNo, CustomerName: r.CustomerName,
				Status: r.Status, Total: r.Total,
				CreatedAt: r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
			})
		}

		revenue, err := q.DashboardRevenueByDay(ctx, days)
		if err != nil {
			return out, fmt.Errorf("dashboard: revenue series: %w", err)
		}
		for _, p := range revenue {
			out.Revenue = append(out.Revenue, RevenuePoint{
				Day: p.Day.Time.Format("2006-01-02"), Revenue: p.Revenue,
				OrderCount: int(p.OrderCount),
			})
		}

		stages, err := q.DashboardPipeline(ctx)
		if err != nil {
			return out, fmt.Errorf("dashboard: pipeline: %w", err)
		}
		for _, st := range stages {
			out.Pipeline = append(out.Pipeline, PipelineStage{
				Stage: st.Stage, Count: int(st.DealCount), Amount: st.Amount,
			})
		}
	}

	if perms.CanRead(rbac.Inventory) {
		value, err := q.DashboardStockValue(ctx)
		if err != nil {
			return out, fmt.Errorf("dashboard: stock value: %w", err)
		}
		low, err := q.CountItemsBelowMinimum(ctx)
		if err != nil {
			return out, fmt.Errorf("dashboard: low stock: %w", err)
		}
		out.Stock = &StockPanel{Value: value, BelowMinimum: int(low)}

		rows, err := q.DashboardStockRows(ctx, 5)
		if err != nil {
			return out, fmt.Errorf("dashboard: stock rows: %w", err)
		}
		for _, r := range rows {
			row := StockRow{SKU: r.Sku, Name: r.ItemName, OnHand: r.OnHand, UOM: r.BaseUom}
			row.Low = r.MinQty.Valid && r.OnHand.LessThan(r.MinQty.Decimal)
			// Expiring uses the same 30-day horizon as the alerts service, so the
			// panel and the bell cannot disagree about what "скоро истекает" means.
			if r.ExpiresOn.Valid {
				row.Expiring = !r.ExpiresOn.Time.After(time.Now().AddDate(0, 0, alerts.ExpiryWindow))
			}
			switch {
			case r.BatchNo != nil:
				row.Detail = "Партия " + *r.BatchNo
			default:
				row.Detail = r.LocationCode
			}
			out.StockRows = append(out.StockRows, row)
		}
	}

	if perms.CanRead(rbac.Quality) {
		n, err := q.DashboardQuarantinedBatches(ctx)
		if err != nil {
			return out, fmt.Errorf("dashboard: quarantine: %w", err)
		}
		out.Quality = &QualityPanel{Quarantined: int(n)}
	}

	if perms.CanRead(rbac.Production) {
		today, err := q.DashboardProductionToday(ctx)
		if err != nil {
			return out, fmt.Errorf("dashboard: production: %w", err)
		}
		out.Production = &ProductionPanel{
			GoodQty: today.GoodQty, ScrapQty: today.ScrapQty, PlannedQty: today.PlannedQty,
		}
	}

	// Лента событий, read from the audit log rather than a parallel feed table —
	// the audit trail is already the record of everything that happened.
	// Restricted to the viewer's readable resources, so it cannot become a
	// side channel around the panel guards above.
	readable := perms.ReadableResources()
	if len(readable) > 0 {
		entries, err := q.DashboardRecentAudit(ctx, db.DashboardRecentAuditParams{
			Resources: readable, Limit: 12,
		})
		if err != nil {
			return out, fmt.Errorf("dashboard: feed: %w", err)
		}
		for _, e := range entries {
			entry := FeedEntry{
				ID: e.ID.String(), Action: e.Action, Resource: e.Resource,
				OccurredAt: e.OccurredAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
			}
			if e.ResourceID.Valid {
				entry.ResourceID = e.ResourceID.UUID.String()
			}
			if e.ActorName != nil {
				entry.ActorName = *e.ActorName
			}
			out.Feed = append(out.Feed, entry)
		}
	}

	return out, nil
}
