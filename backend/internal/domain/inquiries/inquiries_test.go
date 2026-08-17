package inquiries_test

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/domain/inquiries"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// Интеграция с сайтом. This is the only module that accepts unauthenticated
// input, so it is tested from that angle: what a stranger can send, how often,
// and what comes back.

type fixture struct {
	pool  *pgxpool.Pool
	svc   *inquiries.Service
	actor uuid.UUID
	batch uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testsupport.NewDB(t)
	ctx := context.Background()

	f := fixture{pool: pool, svc: inquiries.NewService(pool, inquiries.DefaultRateLimit())}

	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ('sales@samari-kuhsor.tj', 'Менеджер', 'x') RETURNING id`).Scan(&f.actor); err != nil {
		t.Fatal(err)
	}
	var item uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom) VALUES ('APJ-1000','finished_good','bottle')
		RETURNING id`).Scan(&item); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO batches (batch_no, item_id) VALUES ('B-2617', $1) RETURNING id`,
		item).Scan(&f.batch); err != nil {
		t.Fatal(err)
	}
	return f
}

func ip(s string) *netip.Addr { a := netip.MustParseAddr(s); return &a }

// docs/05-MODULES.md:160 — every submission returns a reference number, and the
// prefix identifies the type. These strings appear on the visitor's receipt and
// in QOIM's own correspondence, so they are fixed.
func TestEachTypeGetsItsDocumentedPrefix(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	want := map[string]string{
		inquiries.TypeWholesale:   "WR-",
		inquiries.TypeContact:     "CF-",
		inquiries.TypeDistributor: "DA-",
		inquiries.TypeJob:         "JB-",
	}
	for kind, prefix := range want {
		got, err := f.svc.Submit(ctx, inquiries.SubmitInput{
			Type: kind, Name: "Тест", Contact: "+992 000 00 00",
		})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if !strings.HasPrefix(got.ReferenceNo, prefix) {
			t.Errorf("%s → %s, want the %s prefix", kind, got.ReferenceNo, prefix)
		}
	}

	// Complaints need a batch, so they are submitted separately.
	complaint, err := f.svc.Submit(ctx, inquiries.SubmitInput{
		Type: inquiries.TypeComplaint, Name: "Тест", Contact: "x",
		BatchID: uuid.NullUUID{UUID: f.batch, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(complaint.ReferenceNo, "CP-") {
		t.Errorf("complaint → %s, want CP-", complaint.ReferenceNo)
	}

	// And the map covers every type, so adding one cannot silently produce a
	// reference with no prefix.
	for _, kind := range inquiries.Types() {
		if inquiries.Prefixes[kind] == "" {
			t.Errorf("type %s has no prefix", kind)
		}
	}
}

// Reference numbers are the visitor's only receipt. Two visitors sharing one
// would make the client's correspondence ambiguous.
func TestReferenceNumbersAreUniqueUnderConcurrency(t *testing.T) {
	t.Parallel()
	f := setup(t)

	const n = 12
	var wg sync.WaitGroup
	refs := make(chan string, n)
	errs := make(chan error, n)

	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inq, err := f.svc.Submit(context.Background(), inquiries.SubmitInput{
				Type: inquiries.TypeWholesale, Name: "Тест", Contact: "x",
			})
			if err != nil {
				errs <- err
				return
			}
			refs <- inq.ReferenceNo
		}()
	}
	wg.Wait()
	close(refs)
	close(errs)

	for err := range errs {
		t.Errorf("submission failed: %v", err)
	}
	seen := map[string]bool{}
	for r := range refs {
		if seen[r] {
			t.Errorf("duplicate reference number %s", r)
		}
		seen[r] = true
	}
	if len(seen) != n {
		t.Errorf("%d distinct references for %d submissions", len(seen), n)
	}
}

// docs/05-MODULES.md:166 — complaints must link to a batch so the ToR's
// complaint → traceability workflow has an entry point.
func TestComplaintRequiresAValidBatch(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	if _, err := f.svc.Submit(ctx, inquiries.SubmitInput{
		Type: inquiries.TypeComplaint, Name: "Покупатель", Contact: "x",
	}); err == nil {
		t.Error("a complaint with no batch was accepted")
	}

	if _, err := f.svc.Submit(ctx, inquiries.SubmitInput{
		Type: inquiries.TypeComplaint, Name: "Покупатель", Contact: "x",
		BatchID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}); err == nil {
		t.Error("a complaint naming an unknown batch was accepted")
	}

	// Other types do not need one.
	if _, err := f.svc.Submit(ctx, inquiries.SubmitInput{
		Type: inquiries.TypeContact, Name: "Гость", Contact: "x",
	}); err != nil {
		t.Errorf("a contact enquiry was refused for having no batch: %v", err)
	}
}

// docs/03-API-CONTRACT.md:249 — rate limit by IP. This is a public endpoint on a
// server with no WAF in front of it.
func TestRateLimitByIP(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	svc := inquiries.NewService(pool, inquiries.RateLimit{Max: 3, Lookback: time.Hour})
	ctx := context.Background()

	submit := func(addr string) error {
		_, err := svc.Submit(ctx, inquiries.SubmitInput{
			Type: inquiries.TypeContact, Name: "Тест", Contact: "x", IP: ip(addr),
		})
		return err
	}

	for i := range 3 {
		if err := submit("203.0.113.7"); err != nil {
			t.Fatalf("submission %d was refused: %v", i+1, err)
		}
	}
	err := submit("203.0.113.7")
	if err == nil {
		t.Fatal("the rate limit did not apply")
	}
	if code := common.AsError(err).Code; code != common.CodeRateLimited {
		t.Errorf("code = %s, want rate_limited (429)", code)
	}

	// A different visitor is unaffected — the limit is per IP, not global, or one
	// nuisance would silence every genuine enquiry.
	if err := submit("203.0.113.99"); err != nil {
		t.Errorf("a different IP was rate-limited: %v", err)
	}
}

func TestSubmissionValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	long := strings.Repeat("а", 5001)
	cases := map[string]inquiries.SubmitInput{
		"unknown type": {Type: "spam", Name: "Т", Contact: "x"},
		"no name":      {Type: inquiries.TypeContact, Contact: "x"},
		"blank name":   {Type: inquiries.TypeContact, Name: "   ", Contact: "x"},
		"no contact":   {Type: inquiries.TypeContact, Name: "Т"},
		// Bounded so a public endpoint cannot be used to store arbitrary volumes.
		"message too long": {Type: inquiries.TypeContact, Name: "Т", Contact: "x", Message: &long},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.svc.Submit(ctx, in); err == nil {
				t.Error("accepted")
			}
		})
	}
}

// The submitter is a member of the public, not a user of this system. Recording
// no actor is truthful; inventing one would put a name against an action nobody
// in the company took.
func TestPublicSubmissionIsAuditedWithNoActor(t *testing.T) {
	t.Parallel()
	f := setup(t)

	inq, err := f.svc.Submit(context.Background(), inquiries.SubmitInput{
		Type: inquiries.TypeContact, Name: "Гость", Contact: "x", IP: ip("203.0.113.7"),
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := testsupport.AssertAudited(t, f.pool, "inquiries", inq.ID, "create")
	if entry.ActorID.Valid {
		t.Errorf("a public submission was audited against user %s", entry.ActorID.UUID)
	}
}

// docs/05-MODULES.md:164 — conversion carries the reference number across, so the
// trail from website to order is unbroken.
func TestConvertToLeadCarriesTheReference(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	inq, err := f.svc.Submit(ctx, inquiries.SubmitInput{
		Type: inquiries.TypeWholesale, Name: "Ориён Маркет", Contact: "+992 000 00 00",
	})
	if err != nil {
		t.Fatal(err)
	}

	lead, err := f.svc.ConvertToLead(ctx, f.actor, inq.ID)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if lead.Source == nil || *lead.Source != inq.ReferenceNo {
		t.Errorf("lead source = %v, want %s", lead.Source, inq.ReferenceNo)
	}

	after, err := f.svc.Get(ctx, inq.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != inquiries.StatusLeadCreated {
		t.Errorf("status = %s, want lead_created", after.Status)
	}

	// Converting twice would create two leads chasing one enquiry.
	if _, err := f.svc.ConvertToLead(ctx, f.actor, inq.ID); err == nil {
		t.Error("the enquiry was converted twice")
	}
}
