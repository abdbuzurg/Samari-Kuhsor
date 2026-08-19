package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	// Registers the "pgx" database/sql driver. Imported for that side effect only:
	// goose needs a database/sql handle, and using pgx's own adapter means there
	// is no second driver to keep in step with the pool's.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/qoim/samari/backend/migrations"
)

// migrateAdvisoryLock is an arbitrary but FIXED key. Any process that might
// migrate this database must use the same one; two different keys would let two
// migrations run concurrently while both believing they held the lock.
const migrateAdvisoryLock int64 = 7_3_1_9_2026

// applyMigrations brings the schema up to date before the server starts serving.
//
// Migrating on boot rather than from an operator's workstation is a deliberate
// choice for this deployment: there is ONE server, ONE api container, and no
// rolling restarts. The failure it prevents is the realistic one — a deploy that
// updates the code and forgets the schema, or gets half-applied because somebody's
// laptop lost its connection mid-command.
//
// It is guarded three ways:
//
//   - A Postgres advisory lock, so if a second process ever does start
//     concurrently it waits rather than racing. goose's own table locking does
//     not cover the gap between reading the version and applying.
//   - Failure is fatal. A server that starts against a schema it does not
//     understand will fail later, further from the cause, and possibly after
//     writing something.
//   - MIGRATE_ON_START=false opts out entirely, for the case where an operator
//     wants to inspect a migration before it runs.
func applyMigrations(ctx context.Context, dbURL string) error {
	// goose needs a database/sql handle; pgx's stdlib adapter provides one over
	// the same driver the pool uses, so there is no second driver to keep in step.
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("migrate: open: %w", err)
	}
	defer db.Close()

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate: connect: %w", err)
	}
	defer conn.Close()

	// Blocks until acquired. A concurrent starter waits for the migration to
	// finish rather than attempting its own.
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrateAdvisoryLock); err != nil {
		return fmt.Errorf("migrate: lock: %w", err)
	}
	defer func() {
		if _, err := conn.ExecContext(context.WithoutCancel(ctx),
			"SELECT pg_advisory_unlock($1)", migrateAdvisoryLock); err != nil {
			slog.Error("migrate: could not release advisory lock", "error", err)
		}
	}()

	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrate: dialect: %w", err)
	}

	before, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("migrate: current version: %w", err)
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}
	after, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("migrate: new version: %w", err)
	}

	if after == before {
		slog.Info("migrations up to date", "version", after)
	} else {
		slog.Info("migrations applied", "from", before, "to", after)
	}
	return nil
}
