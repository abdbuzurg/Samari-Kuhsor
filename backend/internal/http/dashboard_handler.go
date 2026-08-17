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

// dealStage maps a pipeline stage to its display level.
//
// Deliberately all neutral except the last: a deal being at "предложение" is not
// healthier than being at "квалификация", it is just further along. Colouring
// progress green would make the funnel read as a scorecard.
func dealStage(stage string) api.Status {
	switch stage {
	case "negotiation":
		return api.Status{Key: stage, Label: "Переговоры", Level: string(common.LevelInfo)}
	case "proposal":
		return api.Status{Key: stage, Label: "Предложение", Level: string(common.LevelInfo)}
	case "qualification":
		return api.Status{Key: stage, Label: "Квалификация", Level: string(common.LevelNeutral)}
	default:
		return api.Status{Key: stage, Label: stage, Level: string(common.LevelNeutral)}
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
		Period:   string(snap.Period),
		Pipeline: make([]api.DashboardStage, 0, len(snap.Pipeline)),
		Recent:   make([]api.DashboardOrder, 0, len(snap.Recent)),
		Feed:     make([]api.DashboardEvent, 0, len(snap.Feed)),
		Revenue:  make([]api.DashboardRevenue, 0, len(snap.Revenue)),
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
	common.JSON(w, http.StatusOK, map[string]any{"data": out})
}

var _ = decimal.Zero
