package testsupport_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/testsupport"
)

// The harness is load-bearing for every integration test in the codebase, so it
// gets tested itself. If isolation is broken here, twelve modules' worth of tests
// become quietly meaningless.

func TestTemplateCarriesTheSchema(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)

	var n int
	err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name <> 'goose_db_version'`).Scan(&n)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}
	// Asserted as a floor rather than an exact count: this test exists to prove
	// the clone carries the schema at all, and pinning the number would make
	// every new migration fail here for no reason.
	if n < 11 {
		t.Fatalf("cloned database has %d tables; migration 00001 alone creates 11", n)
	}

	// The tables the harness's own fixtures depend on must be present.
	for _, table := range []string{"users", "roles", "items", "batches", "audit_log", "stock_movements"} {
		var exists bool
		if err := pool.QueryRow(context.Background(), `
			SELECT EXISTS (SELECT 1 FROM information_schema.tables
			               WHERE table_schema='public' AND table_name=$1)`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("the cloned database is missing %s", table)
		}
	}
}

// The property the whole design rests on: two tests mutating the same table must
// not see each other. If this fails, every test in the suite is suspect.
func TestDatabasesAreIsolated(t *testing.T) {
	t.Parallel()

	insert := func(t *testing.T, pool *pgxpool.Pool, email string) {
		t.Helper()
		_, err := pool.Exec(context.Background(),
			`INSERT INTO users (email, full_name, password_hash) VALUES ($1, 'X', 'x')`, email)
		if err != nil {
			t.Fatalf("insert %s: %v", email, err)
		}
	}
	count := func(t *testing.T, pool *pgxpool.Pool) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&n); err != nil {
			t.Fatalf("count users: %v", err)
		}
		return n
	}

	a := testsupport.NewDB(t)
	b := testsupport.NewDB(t)

	insert(t, a, "a@example.com")
	insert(t, a, "a2@example.com")
	insert(t, b, "b@example.com")

	if got := count(t, a); got != 2 {
		t.Errorf("database A sees %d users, expected 2", got)
	}
	if got := count(t, b); got != 1 {
		t.Errorf("database B sees %d users, expected 1 — isolation is broken", got)
	}
}

// Same again, but concurrently: the clone-per-test approach must hold when the
// suite runs in parallel, which is how it will actually be run.
func TestConcurrentDatabasesAreIsolated(t *testing.T) {
	t.Parallel()

	const workers = 6
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pool := testsupport.NewDB(t)
			for j := 0; j <= i; j++ { // worker i inserts i+1 rows
				_, err := pool.Exec(context.Background(),
					`INSERT INTO users (email, full_name, password_hash) VALUES ($1, 'X', 'x')`,
					uuid.NewString()+"@example.com")
				if err != nil {
					errs <- err
					return
				}
			}
			var n int
			if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&n); err != nil {
				errs <- err
				return
			}
			if n != i+1 {
				t.Errorf("worker %d sees %d users, expected %d", i, n, i+1)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("worker failed: %v", err)
	}
}

// Money and quantities are shopspring decimals end to end. Without the codec
// registered in AfterConnect, every numeric read fails — and it would fail late,
// inside an unrelated module's test.
func TestDecimalCodecIsRegistered(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	ctx := context.Background()

	var itemID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom, min_qty)
		VALUES ('RAW-SUG-50', 'raw_material', 'kg', 12.345) RETURNING id`).Scan(&itemID)
	if err != nil {
		t.Fatalf("insert item: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO item_prices (item_id, amount, valid_from)
		VALUES ($1, 18.50, '2026-09-09')`, itemID); err != nil {
		t.Fatalf("insert price: %v", err)
	}

	var amount, minQty string
	err = pool.QueryRow(ctx, `
		SELECT p.amount::text, i.min_qty::text
		FROM item_prices p JOIN items i ON i.id = p.item_id
		WHERE p.item_id = $1`, itemID).Scan(&amount, &minQty)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	// Exact, not approximate: numeric(14,2) and numeric(14,3) keep their scale.
	if amount != "18.50" {
		t.Errorf("money round-tripped as %q, expected %q", amount, "18.50")
	}
	if minQty != "12.345" {
		t.Errorf("quantity round-tripped as %q, expected %q", minQty, "12.345")
	}
}

func TestAssertAuditedFindsAnEntry(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	ctx := context.Background()

	var actorID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ('inspector@example.com', 'Inspector', 'x') RETURNING id`).Scan(&actorID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	resourceID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, resource, resource_id, before, after)
		VALUES ($1, 'approve', 'quality', $2, '{"status":"quarantine"}', '{"status":"released"}')`,
		actorID, resourceID)
	if err != nil {
		t.Fatalf("insert audit row: %v", err)
	}

	entry := testsupport.AssertAudited(t, pool, "quality", resourceID, "approve")
	if entry.ActorID.UUID != actorID {
		t.Errorf("actor = %s, expected %s", entry.ActorID.UUID, actorID)
	}
	if !strings.Contains(string(entry.After), "released") {
		t.Errorf("after = %s, expected it to record the new status", entry.After)
	}
	testsupport.AssertNotAudited(t, pool, "quality", uuid.New())
}

// The helper is only useful if it actually fails when a mutation was not audited.
// A silently-passing assertion would be worse than none, so prove it fails.
func TestAssertAuditedFailsWhenMissing(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)

	fake := run(func(tb testsupport.TB) {
		testsupport.AssertAudited(tb, pool, "items", uuid.New(), "create")
	})

	if !fake.failed {
		t.Fatal("AssertAudited passed with no audit row present — the mandatory audit gate does not work")
	}
	if !strings.Contains(fake.msg, "every mutation must write one") {
		t.Errorf("failure message was %q; it should say why this matters", fake.msg)
	}
}

func TestAssertAuditedFailsOnDuplicate(t *testing.T) {
	t.Parallel()
	pool := testsupport.NewDB(t)
	id := uuid.New()

	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(context.Background(), `
			INSERT INTO audit_log (action, resource, resource_id)
			VALUES ('create', 'items', $1)`, id); err != nil {
			t.Fatalf("insert audit row: %v", err)
		}
	}

	fake := run(func(tb testsupport.TB) {
		testsupport.AssertAudited(tb, pool, "items", id, "create")
	})
	if !fake.failed {
		t.Fatal("AssertAudited accepted two rows; a duplicate means the mutation ran twice")
	}
	if !strings.Contains(fake.msg, "expected exactly one") {
		t.Errorf("failure message was %q", fake.msg)
	}
}

// recorder stands in for *testing.T so an assertion helper's own failure path can
// be exercised. Fatalf panics with a sentinel, which run recovers — mirroring the
// Goexit that the real Fatalf performs, without killing the test.
type recorder struct {
	failed bool
	msg    string
}

func (r *recorder) Helper() {}

func (r *recorder) Fatalf(format string, args ...any) {
	r.failed = true
	r.msg = fmt.Sprintf(format, args...)
	panic(sentinel{})
}

type sentinel struct{}

func run(fn func(tb testsupport.TB)) (r *recorder) {
	r = &recorder{}
	defer func() {
		if p := recover(); p != nil {
			if _, ok := p.(sentinel); !ok {
				panic(p)
			}
		}
	}()
	fn(r)
	return r
}
