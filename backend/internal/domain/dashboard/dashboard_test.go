package dashboard_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/domain/dashboard"
	"github.com/qoim/samari/backend/internal/rbac"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// Панель управления.
//
// Two things can go wrong here and both are serious.
//
// The first is fabrication: 05-MODULES.md:70 forbids carrying the prototype's
// sample figures into production. A dashboard showing revenue for a factory that
// has produced nothing is a lie the client would act on.
//
// The second is leakage. The dashboard is the landing page — the one screen a
// user cannot avoid — so a panel rendered without checking permission hands every
// module's headline figure to everyone who can log in.

type fixture struct {
	pool *pgxpool.Pool
	svc  *dashboard.Service
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testsupport.NewDB(t)
	return fixture{pool: pool, svc: dashboard.NewService(pool)}
}

func perms(list ...string) rbac.Set { return rbac.NewSet(list) }

// On opening day the factory has produced nothing and sold nothing. Every figure
// is zero, and zero is the truth.
func TestAnEmptySystemReportsZeroAndNotSampleData(t *testing.T) {
	t.Parallel()
	f := setup(t)

	snap, err := f.svc.Build(context.Background(),
		perms("dashboard:read", "crm:read", "inventory:read", "quality:read",
			"production:read", "procurement:read"),
		dashboard.PeriodMonth)
	if err != nil {
		t.Fatal(err)
	}

	if snap.Sales == nil {
		t.Fatal("the sales panel is missing for a viewer who may read CRM")
	}
	if !snap.Sales.Revenue.IsZero() {
		t.Errorf("revenue = %s on an empty system", snap.Sales.Revenue)
	}
	if snap.Sales.OrderCount != 0 || snap.Sales.OpenOrders != 0 {
		t.Errorf("orders = %d/%d on an empty system",
			snap.Sales.OrderCount, snap.Sales.OpenOrders)
	}
	if snap.Stock == nil || !snap.Stock.Value.IsZero() {
		t.Errorf("stock value = %v on an empty system", snap.Stock)
	}
	if snap.Quality == nil || snap.Quality.Quarantined != 0 {
		t.Errorf("quarantined = %v on an empty system", snap.Quality)
	}
	if len(snap.Recent) != 0 || len(snap.Pipeline) != 0 {
		t.Errorf("%d recent orders and %d pipeline stages materialised from nothing",
			len(snap.Recent), len(snap.Pipeline))
	}
}

// The leak this prevents: a warehouse operator opens the landing page they cannot
// avoid and learns the month's revenue.
func TestEachPanelRequiresItsOwnModulePermission(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		perms rbac.Set
		want  func(dashboard.Snapshot) error
	}{
		{
			name:  "warehouse operator sees stock and nothing else",
			perms: perms("dashboard:read", "inventory:read"),
			want: func(s dashboard.Snapshot) error {
				if s.Stock == nil {
					return errf("stock panel missing")
				}
				if s.Sales != nil {
					return errf("revenue was shown to a viewer with no CRM grant")
				}
				if s.Quality != nil {
					return errf("quarantine count was shown with no quality grant")
				}
				if s.Production != nil {
					return errf("production output was shown with no production grant")
				}
				return nil
			},
		},
		{
			name:  "sales manager sees revenue but not the shop floor",
			perms: perms("dashboard:read", "crm:read"),
			want: func(s dashboard.Snapshot) error {
				if s.Sales == nil {
					return errf("sales panel missing")
				}
				if s.Stock != nil {
					return errf("stock value was shown with no inventory grant")
				}
				if s.Production != nil {
					return errf("production output was shown with no production grant")
				}
				return nil
			},
		},
		{
			name:  "a viewer with only dashboard:read sees no panel at all",
			perms: perms("dashboard:read"),
			want: func(s dashboard.Snapshot) error {
				if s.Sales != nil || s.Stock != nil || s.Quality != nil || s.Production != nil {
					return errf("a panel rendered for a viewer with no module grants")
				}
				return nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap, err := f.svc.Build(ctx, tc.perms, dashboard.PeriodMonth)
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.want(snap); err != nil {
				t.Error(err)
			}
		})
	}
}

// The overdue-deliveries figure sits inside the sales panel but belongs to
// procurement, so it needs its own check — otherwise reading CRM would be enough
// to learn how late the supply chain is.
func TestOverdueDeliveriesNeedsProcurementNotCRM(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	var supplier uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO suppliers (name) VALUES ('Ориён Агро') RETURNING id`).Scan(&supplier); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO purchase_orders (po_no, supplier_id, expected_at, status)
		VALUES ('PO-000001', $1, CURRENT_DATE - 10, 'in_transit')`, supplier); err != nil {
		t.Fatal(err)
	}

	withProcurement, err := f.svc.Build(ctx,
		perms("dashboard:read", "crm:read", "procurement:read"), dashboard.PeriodMonth)
	if err != nil {
		t.Fatal(err)
	}
	if withProcurement.Sales.OverduePOs != 1 {
		t.Errorf("overdue = %d, want 1", withProcurement.Sales.OverduePOs)
	}

	withoutProcurement, err := f.svc.Build(ctx,
		perms("dashboard:read", "crm:read"), dashboard.PeriodMonth)
	if err != nil {
		t.Fatal(err)
	}
	if withoutProcurement.Sales.OverduePOs != 0 {
		t.Errorf("overdue = %d for a viewer with no procurement grant",
			withoutProcurement.Sales.OverduePOs)
	}
}

// The event feed reads the audit log, which records every mutation in every
// module. Unfiltered it would be a side channel around every panel guard above.
func TestTheEventFeedIsFilteredByReadableResources(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	var actor uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ('a@samari-kuhsor.tj','Актор','x') RETURNING id`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{"crm", "inventory", "quality"} {
		if _, err := f.pool.Exec(ctx, `
			INSERT INTO audit_log (actor_id, action, resource, resource_id)
			VALUES ($1, 'create', $2, gen_random_uuid())`, actor, resource); err != nil {
			t.Fatal(err)
		}
	}

	snap, err := f.svc.Build(ctx, perms("dashboard:read", "inventory:read"), dashboard.PeriodMonth)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Feed) != 1 {
		t.Fatalf("%d feed entries for an inventory-only viewer, want 1", len(snap.Feed))
	}
	if snap.Feed[0].Resource != rbac.Inventory {
		t.Errorf("the feed leaked a %s event", snap.Feed[0].Resource)
	}
	if snap.Feed[0].ActorName != "Актор" {
		t.Errorf("actor name = %q, want the user's name", snap.Feed[0].ActorName)
	}

	// A viewer with no read grants gets nothing, and the query is not issued.
	none, err := f.svc.Build(ctx, rbac.Set{}, dashboard.PeriodMonth)
	if err != nil {
		t.Fatal(err)
	}
	if len(none.Feed) != 0 {
		t.Errorf("%d feed entries for a viewer with no permissions", len(none.Feed))
	}
}

// Revenue counts confirmed orders onwards. A draft is a quotation; counting it
// would overstate the month with business nobody has agreed to.
func TestDraftAndCancelledOrdersAreNotRevenue(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	var customer, item uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO customers (name) VALUES ('Маркет Хорог') RETURNING id`).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom) VALUES ('APJ-1000','finished_good','bottle')
		RETURNING id`).Scan(&item); err != nil {
		t.Fatal(err)
	}

	for _, o := range []struct{ no, status, qty string }{
		{"SO-0001", "draft", "100"},
		{"SO-0002", "cancelled", "200"},
		{"SO-0003", "confirmed", "10"},
		{"SO-0004", "shipped", "5"},
	} {
		var soID uuid.UUID
		if err := f.pool.QueryRow(ctx, `
			INSERT INTO sales_orders (so_no, customer_id, status)
			VALUES ($1,$2,$3) RETURNING id`, o.no, customer, o.status).Scan(&soID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `
			INSERT INTO sales_order_lines (sales_order_id, item_id, qty, unit_price)
			VALUES ($1,$2,$3::numeric,'10.00')`, soID, item, o.qty); err != nil {
			t.Fatal(err)
		}
	}

	snap, err := f.svc.Build(ctx, perms("dashboard:read", "crm:read"), dashboard.PeriodMonth)
	if err != nil {
		t.Fatal(err)
	}
	// Only SO-0003 (10 × 10.00) and SO-0004 (5 × 10.00) count. Asserted through
	// StringFixed(2) because that is what goes on the wire — decimal.String()
	// trims trailing zeros, so "150" and "150.00" are the same value.
	if got := snap.Sales.Revenue.StringFixed(2); got != "150.00" {
		t.Errorf("revenue = %s, want 150.00 — drafts and cancellations were counted", got)
	}
	if snap.Sales.OrderCount != 2 {
		t.Errorf("order count = %d, want 2", snap.Sales.OrderCount)
	}
	// Open orders is a live figure and ignores the window: `confirmed` counts,
	// `shipped` has left the building.
	if snap.Sales.OpenOrders != 1 {
		t.Errorf("open orders = %d, want 1", snap.Sales.OpenOrders)
	}
	// And every order appears in the recent list, drafts included: that panel
	// answers "what came in", not "what did we earn".
	if len(snap.Recent) != 4 {
		t.Errorf("%d recent orders, want all 4", len(snap.Recent))
	}
}

// A day with no orders must appear as zero rather than being skipped: a gap in
// the axis reads as a spike when the points either side are joined up.
func TestTheRevenueSeriesHasNoGaps(t *testing.T) {
	t.Parallel()
	f := setup(t)

	snap, err := f.svc.Build(context.Background(),
		perms("dashboard:read", "crm:read"), dashboard.PeriodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Revenue) != 7 {
		t.Fatalf("%d points for a 7-day window", len(snap.Revenue))
	}
	for _, p := range snap.Revenue {
		if !p.Revenue.IsZero() {
			t.Errorf("%s reports %s on an empty system", p.Day, p.Revenue)
		}
	}
}

func TestPeriodWindows(t *testing.T) {
	t.Parallel()
	cases := map[string]int32{
		"day": 1, "week": 7, "month": 30, "quarter": 90,
	}
	for name, want := range cases {
		if got := dashboard.ParsePeriod(name).Days(); got != want {
			t.Errorf("%s = %d days, want %d", name, got, want)
		}
	}
	// An unrecognised period falls back to a month. Refusing to render the
	// landing page because of a bad query parameter helps nobody.
	if got := dashboard.ParsePeriod("fortnight"); got != dashboard.PeriodMonth {
		t.Errorf("unknown period = %s, want month", got)
	}
	if got := dashboard.ParsePeriod(""); got != dashboard.PeriodMonth {
		t.Errorf("empty period = %s, want month", got)
	}
}

// Stock is valued at what it cost, not at what it might sell for: unsold stock
// is money spent, not money earned.
func TestStockIsValuedAtPurchasePrice(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	var item, loc, supplier, po uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom) VALUES ('SUG-25','raw_material','kg')
		RETURNING id`).Scan(&item); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO locations (code, name, zone) VALUES ('RAW-1','Сырьё','raw')
		RETURNING id`).Scan(&loc); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO suppliers (name) VALUES ('Поставщик') RETURNING id`).Scan(&supplier); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO purchase_orders (po_no, supplier_id) VALUES ('PO-1',$1) RETURNING id`,
		supplier).Scan(&po); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO purchase_order_lines (po_id, item_id, qty, unit_price)
		VALUES ($1,$2,'100','7.50')`, po, item); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO stock_movements (item_id, location_id, qty_delta, reason)
		VALUES ($1,$2,'40','goods_receipt')`, item, loc); err != nil {
		t.Fatal(err)
	}

	snap, err := f.svc.Build(ctx, perms("dashboard:read", "inventory:read"), dashboard.PeriodMonth)
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Stock.Value.StringFixed(2); got != "300.00" {
		t.Errorf("stock value = %s, want 300.00 (40 kg at 7.50)", got)
	}
}

// An item that has never been purchased has no price to value it at. It must
// contribute zero rather than making the whole figure NULL — one unpriced item
// would otherwise blank the entire panel.
func TestUnpricedStockDoesNotBlankTheValuation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	var item, loc uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom) VALUES ('MISC-1','raw_material','kg')
		RETURNING id`).Scan(&item); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO locations (code, name, zone) VALUES ('RAW-2','Сырьё','raw')
		RETURNING id`).Scan(&loc); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO stock_movements (item_id, location_id, qty_delta, reason)
		VALUES ($1,$2,'40','goods_receipt')`, item, loc); err != nil {
		t.Fatal(err)
	}

	snap, err := f.svc.Build(ctx, perms("dashboard:read", "inventory:read"), dashboard.PeriodMonth)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Stock == nil {
		t.Fatal("the stock panel vanished because one item had no purchase price")
	}
	if !snap.Stock.Value.IsZero() {
		t.Errorf("value = %s, want 0.00", snap.Stock.Value)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
func errf(s string) error         { return errString(s) }
