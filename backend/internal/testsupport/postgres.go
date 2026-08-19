// Package testsupport provides the integration-test database harness.
//
// CLAUDE.md §7 requires every handler to be tested against a real Postgres. Doing
// that naively — one container per test — is unusably slow across twelve modules.
// The shape used here (docs/07-IMPLEMENTATION-PLAN.md I10):
//
//	one postgres:18 container per test binary
//	  → migrations applied once into a template database
//	    → CREATE DATABASE … TEMPLATE per test
//
// Cloning a template is a file copy inside Postgres, so per-test setup is a few
// milliseconds while each test still gets a genuinely isolated database.
//
// Per-test transaction rollback would be marginally faster and was rejected: the
// domain layer opens its own transactions for audit writes and advisory locks
// (I4, I6), and nesting those inside a test-owned transaction would misrepresent
// exactly the behaviour most in need of honest testing.
package testsupport

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/qoim/samari/backend/migrations"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver, for goose
)

const (
	// Must match the production server major version. docs/07 C5/I7: uuidv7() is
	// native in 18 and the platform depends on it as a column default.
	postgresImage = "postgres:18-alpine"

	templateDB = "samari_template"
	adminDB    = "postgres"

	dbUser = "samari"
	dbPass = "test_only"
)

var (
	once      sync.Once
	setupErr  error
	adminPool *pgxpool.Pool
	baseDSN   string // DSN with a %s placeholder for the database name
	dbCounter atomic.Int64
)

// dsn builds a connection string for a named database on the shared container.
func dsn(database string) string { return fmt.Sprintf(baseDSN, database) }

// start brings up the container and builds the template database. Runs once per
// test binary; every NewDB call after that is a cheap clone.
func start(ctx context.Context) error {
	container, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase(adminDB),
		tcpostgres.WithUsername(dbUser),
		tcpostgres.WithPassword(dbPass),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		return fmt.Errorf("start postgres container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return fmt.Errorf("container host: %w", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return fmt.Errorf("container port: %w", err)
	}
	baseDSN = fmt.Sprintf("postgres://%s:%s@%s:%s/%%s?sslmode=disable",
		dbUser, dbPass, host, port.Port())

	if adminPool, err = newPool(ctx, adminDB); err != nil {
		return fmt.Errorf("admin pool: %w", err)
	}

	// Build the template. goose drives database/sql, so use the stdlib adapter
	// here rather than the pgx-native pool the application uses.
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+templateDB); err != nil {
		return fmt.Errorf("create template database: %w", err)
	}
	sqlDB, err := sql.Open("pgx", dsn(templateDB))
	if err != nil {
		return fmt.Errorf("open template for migration: %w", err)
	}
	defer sqlDB.Close()

	// The SAME embedded filesystem the API migrates from, deliberately. Reading
	// these from disk here would create a second code path: a migration the embed
	// pattern failed to pick up would still apply in tests and silently not apply
	// in production, which is the worst possible direction for that bug to run.
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		return fmt.Errorf("migrate template: %w", err)
	}

	// CREATE DATABASE … TEMPLATE refuses to run while any session is connected to
	// the template, so this close is load-bearing, not tidiness.
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close template connection: %w", err)
	}
	return nil
}

func newPool(ctx context.Context, database string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn(database))
	if err != nil {
		return nil, err
	}
	// Money and quantities are shopspring decimals end to end (CLAUDE.md §4.6).
	// Without this the driver has no codec for them and every numeric read fails.
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		pgxdecimal.Register(c.TypeMap())
		return nil
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// NewDB returns an isolated database cloned from the migrated template. The
// database is dropped when the test finishes.
func NewDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	once.Do(func() { setupErr = start(ctx) })
	if setupErr != nil {
		t.Fatalf("test database harness: %v", setupErr)
	}

	name := fmt.Sprintf("samari_test_%d", dbCounter.Add(1))
	if _, err := adminPool.Exec(ctx,
		fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, templateDB)); err != nil {
		t.Fatalf("clone template into %s: %v", name, err)
	}

	pool, err := newPool(ctx, name)
	if err != nil {
		t.Fatalf("connect to %s: %v", name, err)
	}

	t.Cleanup(func() {
		pool.Close()
		// WITH (FORCE) terminates stragglers; a leaked connection would otherwise
		// leave the database behind and slowly fill the container's disk.
		if _, err := adminPool.Exec(context.Background(),
			fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", name)); err != nil {
			t.Logf("warning: could not drop %s: %v", name, err)
		}
	})

	return pool
}
