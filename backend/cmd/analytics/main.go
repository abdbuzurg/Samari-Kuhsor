// Command analytics runs website-statistics maintenance on demand.
//
//	analytics maintain
//
// The API already runs this daily in-process (docs/01-DECISIONS.md D12). This
// binary exists for the cases a ticker cannot cover: catching up after the box
// was off, verifying retention by hand, and running it under a debugger.
//
// It shares the API image, like /app/seed.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/domain/analytics"
)

func main() {
	if err := run(); err != nil {
		slog.Error("analytics maintenance failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 || os.Args[1] != "maintain" {
		return fmt.Errorf("usage: analytics maintain")
	}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return fmt.Errorf("DB_URL is required")
	}

	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return fmt.Errorf("parse DB_URL: %w", err)
	}
	cfg.AfterConnect = func(_ context.Context, c *pgx.Conn) error {
		pgxdecimal.Register(c.TypeMap())
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	svcCfg := analytics.DefaultConfig()
	svcCfg.IPSalt = os.Getenv("ANALYTICS_IP_SALT")

	res, err := analytics.NewService(pool, svcCfg).Maintain(ctx)
	if err != nil {
		return err
	}

	oldest := "none"
	if res.OldestSurviving != nil {
		oldest = res.OldestSurviving.Format("2006-01-02")
	}
	slog.Info("analytics maintenance complete",
		"days_rolled_up", res.DaysRolledUp,
		"rows_deleted", res.RowsDeleted,
		"oldest_surviving", oldest,
		"retention_days", int(analytics.Retention.Hours()/24))
	return nil
}
