package crm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/http/common"

	"github.com/qoim/samari/backend/internal/domain/crm"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// R12 — CRM и продажи, the customer side.
//
// The stage ladder gets exhaustive matrix coverage for the same reason the batch
// transition matrix does (CLAUDE.md §7): it is the rule that decides whether a
// deal's history can be rewritten.

func fixture(t *testing.T) (*crm.Service, *pgxpool.Pool, uuid.UUID) {
	t.Helper()
	pool := testsupport.NewDB(t)
	svc := crm.NewService(pool)

	var actor uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password_hash, full_name, is_active)
		VALUES ('crm@samari-kuhsor.tj', '$argon2id$fake', 'Тест', true)
		RETURNING id`).Scan(&actor)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return svc, pool, actor
}

func customer(t *testing.T, svc *crm.Service, actor uuid.UUID) uuid.UUID {
	t.Helper()
	c, err := svc.CreateCustomer(context.Background(), actor, crm.CustomerInput{
		Name: "ООО «Ориён Савдо»", Region: "Душанбе", Type: "distributor",
	})
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	return c.ID
}

// ---------------------------------------------------------------------------
// The stage matrix
// ---------------------------------------------------------------------------

// Every from/to pair, legal and illegal. Anything absent from the matrix is
// illegal, and won/lost are terminal.
func TestLegalStageMoveCoversEveryPair(t *testing.T) {
	t.Parallel()

	legal := map[string]map[string]bool{}
	for _, from := range crm.Stages {
		legal[from] = map[string]bool{}
		for _, to := range crm.Stages {
			// Open stages may move to any OTHER stage. Closed stages may not move
			// at all: a reopened deal makes every conversion figure provisional.
			closed := from == crm.StageWon || from == crm.StageLost
			legal[from][to] = !closed && from != to
		}
	}

	for _, from := range crm.Stages {
		for _, to := range crm.Stages {
			got := crm.LegalStageMove(from, to)
			if want := legal[from][to]; got != want {
				t.Errorf("LegalStageMove(%q, %q) = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestUnknownStageIsNeverLegal(t *testing.T) {
	t.Parallel()
	for _, from := range crm.Stages {
		if crm.LegalStageMove(from, "proposal") {
			// 'proposal' and 'qualification' appeared in an old dashboard mapping
			// and are not in the CHECK constraint. They must not become legal by
			// accident.
			t.Errorf("move to unknown stage %q from %q was allowed", "proposal", from)
		}
	}
}

// AllowedFrom is what the UI renders. It must project exactly the same matrix
// the domain enforces, or the interface offers a button that fails.
func TestAllowedFromProjectsTheSameMatrix(t *testing.T) {
	t.Parallel()
	for _, from := range crm.Stages {
		for _, to := range crm.AllowedFrom(from) {
			if !crm.LegalStageMove(from, to) {
				t.Errorf("AllowedFrom(%q) offered %q, which the domain refuses", from, to)
			}
		}
		for _, to := range crm.Stages {
			if crm.LegalStageMove(from, to) && !contains(crm.AllowedFrom(from), to) {
				t.Errorf("AllowedFrom(%q) omitted %q, which is legal", from, to)
			}
		}
	}
}

func TestClosedDealsOfferNothing(t *testing.T) {
	t.Parallel()
	for _, closed := range []string{crm.StageWon, crm.StageLost} {
		if got := crm.AllowedFrom(closed); len(got) != 0 {
			t.Errorf("AllowedFrom(%q) = %v, want empty — closed deals are terminal", closed, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Moving a stage
// ---------------------------------------------------------------------------

func TestMoveStageWritesAnImmutableEventInTheSameTransaction(t *testing.T) {
	t.Parallel()
	svc, _, actor := fixture(t)
	ctx := context.Background()
	cust := customer(t, svc, actor)

	deal, err := svc.CreateDeal(ctx, actor, crm.DealInput{CustomerID: cust})
	if err != nil {
		t.Fatalf("create deal: %v", err)
	}

	// Creation already records the opening rung, so the history is never empty.
	events, err := svc.StageEvents(ctx, deal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ToStage != crm.StageNew {
		t.Fatalf("opening event = %+v, want one event to 'new'", events)
	}

	note := "Клиент запросил КП"
	if _, err := svc.MoveStage(ctx, actor, crm.StageInput{
		DealID: deal.ID, To: crm.StageQuoted, Note: &note,
	}); err != nil {
		t.Fatalf("move stage: %v", err)
	}

	events, err = svc.StageEvents(ctx, deal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	// Newest first.
	if events[0].ToStage != crm.StageQuoted {
		t.Errorf("latest event to = %q, want quoted", events[0].ToStage)
	}
	if events[0].FromStage == nil || *events[0].FromStage != crm.StageNew {
		t.Errorf("latest event from = %v, want new", events[0].FromStage)
	}
	if events[0].Note == nil || *events[0].Note != note {
		t.Errorf("note = %v, want %q", events[0].Note, note)
	}
}

func TestMoveStageRefusesAnIllegalMove(t *testing.T) {
	t.Parallel()
	svc, _, actor := fixture(t)
	ctx := context.Background()
	cust := customer(t, svc, actor)

	deal, err := svc.CreateDeal(ctx, actor, crm.DealInput{CustomerID: cust})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MoveStage(ctx, actor, crm.StageInput{
		DealID: deal.ID, To: crm.StageNew,
	}); err == nil {
		t.Error("moving a deal to the stage it is already in was allowed")
	}
}

func TestAClosedDealCannotBeReopened(t *testing.T) {
	t.Parallel()
	svc, _, actor := fixture(t)
	ctx := context.Background()
	cust := customer(t, svc, actor)

	deal, err := svc.CreateDeal(ctx, actor, crm.DealInput{CustomerID: cust})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MoveStage(ctx, actor, crm.StageInput{
		DealID: deal.ID, To: crm.StageWon,
	}); err != nil {
		t.Fatalf("close deal: %v", err)
	}

	_, err = svc.MoveStage(ctx, actor, crm.StageInput{
		DealID: deal.ID, To: crm.StageNegotiation,
	})
	if err == nil {
		t.Fatal("a won deal was reopened; every conversion figure is now provisional")
	}
	if !strings.Contains(err.Error(), "закрыта") {
		t.Errorf("error = %v, want it to say the deal is closed", err)
	}
}

// A deal that slips backwards is an ordinary Tuesday. Refusing it would only
// teach people to lie to the system.
func TestADealMaySlipBackwards(t *testing.T) {
	t.Parallel()
	svc, _, actor := fixture(t)
	ctx := context.Background()
	cust := customer(t, svc, actor)

	deal, err := svc.CreateDeal(ctx, actor, crm.DealInput{CustomerID: cust})
	if err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{crm.StageQuoted, crm.StageNegotiation} {
		if _, err := svc.MoveStage(ctx, actor, crm.StageInput{DealID: deal.ID, To: to}); err != nil {
			t.Fatalf("move to %s: %v", to, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Validation and audit
// ---------------------------------------------------------------------------

func TestCustomerRegionIsAClosedList(t *testing.T) {
	t.Parallel()
	svc, _, actor := fixture(t)

	_, err := svc.CreateCustomer(context.Background(), actor, crm.CustomerInput{
		Name: "Тест", Region: "Марс",
	})
	if err == nil {
		// A free-text region makes the CRM's own Регион column meaningless
		// within a month.
		t.Fatal("an unknown region was accepted")
	}
}

func TestCustomerRequiresAName(t *testing.T) {
	t.Parallel()
	svc, _, actor := fixture(t)
	if _, err := svc.CreateCustomer(context.Background(), actor, crm.CustomerInput{
		Name: "   ",
	}); err == nil {
		t.Fatal("a blank customer name was accepted")
	}
}

func TestEveryMutationWritesAnAuditRow(t *testing.T) {
	t.Parallel()
	svc, pool, actor := fixture(t)
	ctx := context.Background()

	cust := customer(t, svc, actor)
	deal, err := svc.CreateDeal(ctx, actor, crm.DealInput{CustomerID: cust})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MoveStage(ctx, actor, crm.StageInput{
		DealID: deal.ID, To: crm.StageWon,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateContact(ctx, actor, crm.ContactInput{
		CustomerID: cust, FullName: "Ф. Юсупов",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTask(ctx, actor, crm.TaskInput{Title: "Перезвонить"}); err != nil {
		t.Fatal(err)
	}

	// CLAUDE.md §4.5 — a regulatory requirement, not a nicety.
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE resource = 'crm'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 5 {
		t.Errorf("audit rows = %d, want one per mutation (customer, deal, stage, contact, task)", n)
	}
}

func TestADealNeedsAnExistingCustomer(t *testing.T) {
	t.Parallel()
	svc, _, actor := fixture(t)
	if _, err := svc.CreateDeal(context.Background(), actor, crm.DealInput{
		CustomerID: uuid.New(),
	}); err == nil {
		t.Fatal("a deal was created against a customer that does not exist")
	}
}

// ---------------------------------------------------------------------------
// KPIs
// ---------------------------------------------------------------------------

// Conversion is null before anything closes, never "0". On a sales dashboard
// "0% conversion" and "nothing has closed yet" read very differently.
func TestConversionIsNullUntilSomethingCloses(t *testing.T) {
	t.Parallel()
	svc, _, actor := fixture(t)
	ctx := context.Background()
	cust := customer(t, svc, actor)

	if _, err := svc.CreateDeal(ctx, actor, crm.DealInput{CustomerID: cust}); err != nil {
		t.Fatal(err)
	}
	kpis, err := svc.KPIs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kpis.Conversion != nil {
		t.Errorf("conversion = %q with nothing decided, want null", *kpis.Conversion)
	}
	if kpis.OpenDeals != 1 {
		t.Errorf("open deals = %d, want 1", kpis.OpenDeals)
	}

	won, err := svc.CreateDeal(ctx, actor, crm.DealInput{CustomerID: cust})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MoveStage(ctx, actor, crm.StageInput{DealID: won.ID, To: crm.StageWon}); err != nil {
		t.Fatal(err)
	}

	kpis, err = svc.KPIs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kpis.Conversion == nil || *kpis.Conversion != "100.0" {
		t.Errorf("conversion = %v, want 100.0 (one won, one decided)", kpis.Conversion)
	}
	// The won deal left the pipeline.
	if kpis.OpenDeals != 1 {
		t.Errorf("open deals = %d, want 1", kpis.OpenDeals)
	}
}

func contains(h []string, n string) bool {
	for _, s := range h {
		if s == n {
			return true
		}
	}
	return false
}

// A customer with no leads must still list.
//
// The first version of ListCustomers cast max(status) to text without a
// COALESCE, which made sqlc type the column non-nullable; every customer without
// a lead then failed to scan and the whole register 500'd. No unit test caught
// it because none listed a customer — it took a live request.
func TestCustomersListWhenTheyHaveNoLeadsOrDeals(t *testing.T) {
	t.Parallel()
	svc, _, actor := fixture(t)
	ctx := context.Background()

	if _, err := svc.CreateCustomer(ctx, actor, crm.CustomerInput{
		Name: "Магазин без лидов", Region: "Хорог",
	}); err != nil {
		t.Fatal(err)
	}

	rows, total, err := svc.Customers(ctx, common.Params{Page: 1, PerPage: 25}, nil)
	if err != nil {
		t.Fatalf("listing a customer with no leads failed: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("got %d rows (total %d), want 1", len(rows), total)
	}
	if rows[0].LeadStatus != "" {
		t.Errorf("lead_status = %q, want empty for a customer with no leads", rows[0].LeadStatus)
	}
	if !rows[0].OpenAmount.IsZero() {
		t.Errorf("open_amount = %s, want 0", rows[0].OpenAmount)
	}
}

// And one WITH a lead and an open deal, so the aggregate columns are exercised
// in both directions.
func TestCustomerRowCarriesOpenDealCountAndValue(t *testing.T) {
	t.Parallel()
	svc, pool, actor := fixture(t)
	ctx := context.Background()
	cust := customer(t, svc, actor)

	amount := decimal.RequireFromString("48000.00")
	if _, err := svc.CreateDeal(ctx, actor, crm.DealInput{
		CustomerID: cust, Amount: decimal.NullDecimal{Decimal: amount, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO leads (customer_id, source, status, created_by)
		VALUES ($1, 'Сайт', 'negotiation', $2)`, cust, actor); err != nil {
		t.Fatal(err)
	}

	rows, _, err := svc.Customers(ctx, common.Params{Page: 1, PerPage: 25}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].OpenDeals != 1 {
		t.Errorf("open_deals = %d, want 1", rows[0].OpenDeals)
	}
	if !rows[0].OpenAmount.Equal(amount) {
		t.Errorf("open_amount = %s, want %s", rows[0].OpenAmount, amount)
	}
	if rows[0].LeadStatus != "negotiation" {
		t.Errorf("lead_status = %q, want negotiation", rows[0].LeadStatus)
	}
}
