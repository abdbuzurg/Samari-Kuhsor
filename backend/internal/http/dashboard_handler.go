package http

import (
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/domain/dashboard"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Панель управления — docs/05-MODULES.md §2.

// stockRowStatus grades one panel line.
//
// Below minimum outranks expiring: a stockout stops the line today, an expiry is
// a date in the future. Only a position that is neither is green — green means
// healthy, never merely present (CLAUDE.md §5).
func stockRowStatus(r dashboard.StockRow) api.Status {
	switch {
	case r.Low:
		return api.Status{Key: "below_minimum", Label: "Низкий остаток", Level: string(common.LevelDanger)}
	case r.Expiring:
		return api.Status{Key: "expiring", Label: "Истекает", Level: string(common.LevelWarn)}
	default:
		return api.Status{Key: "ok", Label: "В норме", Level: string(common.LevelOK)}
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	period := dashboard.ParsePeriod(r.URL.Query().Get("period"))
	snap, err := s.svc.Dashboard.Build(r.Context(), rbac.NewSet(ident.Permissions), period)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	out := api.Dashboard{
		Period:    string(snap.Period),
		Pipeline:  make([]api.DashboardStage, 0, len(snap.Pipeline)),
		Recent:    make([]api.DashboardOrder, 0, len(snap.Recent)),
		Feed:      make([]api.DashboardEvent, 0, len(snap.Feed)),
		Revenue:   make([]api.DashboardRevenue, 0, len(snap.Revenue)),
		StockRows: make([]api.DashboardStockRow, 0, len(snap.StockRows)),
	}
	for _, r := range snap.StockRows {
		out.StockRows = append(out.StockRows, api.DashboardStockRow{
			SKU: r.SKU, Name: r.Name, Detail: r.Detail,
			OnHand: r.OnHand.String(), UOM: r.UOM,
			Status: stockRowStatus(r),
		})
	}
	if p := snap.Sales; p != nil {
		out.Sales = &api.DashboardSales{
			Revenue: p.Revenue.StringFixed(2), OrderCount: p.OrderCount,
			OpenOrders: p.OpenOrders, OverduePOs: p.OverduePOs,
		}
	}
	if p := snap.Stock; p != nil {
		out.Stock = &api.DashboardStock{
			Value: p.Value.StringFixed(2), BelowMinimum: p.BelowMinimum,
		}
	}
	if p := snap.Quality; p != nil {
		out.Quality = &api.DashboardQuality{Quarantined: p.Quarantined}
	}
	if p := snap.Production; p != nil {
		out.Production = &api.DashboardProduction{
			GoodQty: p.GoodQty.String(), ScrapQty: p.ScrapQty.String(),
			PlannedQty: p.PlannedQty.String(),
			Progress:   moProgress(p.GoodQty, p.PlannedQty),
		}
	}
	for _, st := range snap.Pipeline {
		out.Pipeline = append(out.Pipeline, api.DashboardStage{
			Stage: dealStage(st.Stage), Count: st.Count, Amount: st.Amount.StringFixed(2),
		})
	}
	for _, o := range snap.Recent {
		out.Recent = append(out.Recent, api.DashboardOrder{
			ID: o.ID, SONo: o.SONo, CustomerName: o.CustomerName,
			Total: o.Total.StringFixed(2), Status: soStatus(o.Status),
			CreatedAt: o.CreatedAt,
		})
	}
	for _, e := range snap.Feed {
		out.Feed = append(out.Feed, api.DashboardEvent{
			ID: e.ID, Action: e.Action, Resource: e.Resource,
			ResourceID: e.ResourceID, ActorName: e.ActorName, OccurredAt: e.OccurredAt,
		})
	}
	for _, p := range snap.Revenue {
		out.Revenue = append(out.Revenue, api.DashboardRevenue{
			Day: p.Day, Revenue: p.Revenue.StringFixed(2), OrderCount: p.OrderCount,
		})
	}
	common.JSON(w, http.StatusOK, out)
}

var _ = decimal.Zero
