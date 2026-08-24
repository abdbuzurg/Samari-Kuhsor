// Command seed loads data into the database.
//
//	seed reference   production-safe, idempotent
//	seed demo        development only; refuses to run in production
//
// docs/07-IMPLEMENTATION-PLAN.md I22. The separation is not tidiness: this
// system's value is a traceability evidence trail, and demo batches with
// fabricated QC releases sitting in audit_log next to real ones would be a
// falsified regulatory record. No-hard-delete means tombstoning them later does
// not remove them.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/auth"
	"github.com/qoim/samari/backend/internal/seed"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: seed reference|demo")
	}
	mode := os.Args[1]

	appEnv := envOr("APP_ENV", "development")
	if mode == "demo" && appEnv == "production" {
		// A guard, not a convention. Exiting non-zero means a deploy script that
		// runs this by mistake fails loudly instead of quietly poisoning the
		// evidence trail.
		return fmt.Errorf("refusing to seed demo data: APP_ENV=production")
	}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return fmt.Errorf("DB_URL is required")
	}

	ctx := context.Background()
	pool, err := newPool(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch mode {
	case "reference":
		return runReference(ctx, pool)
	case "demo":
		return runDemo(ctx, pool)
	default:
		return fmt.Errorf("unknown mode %q; expected reference or demo", mode)
	}
}

// runDemo loads demonstration data across every module.
//
// Scheduled ahead of the client deploy on purpose: the first screen QOIM opens
// is their first impression of the system, and an empty table reads as broken to
// someone who has not been told it is new.
func runDemo(ctx context.Context, pool *pgxpool.Pool) error {
	res, err := seed.Demo(ctx, pool)
	if err != nil {
		return err
	}
	slog.Info("demo seed complete",
		"customers", res.Customers, "contacts", res.Contacts, "leads", res.Leads,
		"deals", res.Deals, "tasks", res.Tasks, "suppliers", res.Suppliers,
		"employees", res.Employees, "assets", res.Assets, "documents", res.Documents,
		"batches", res.Batches, "movements", res.Movements,
		"quality_tests", res.QualityTests, "orders", res.Orders,
		"inquiries", res.Inquiries, "shipments", res.Shipments)
	return nil
}

func runReference(ctx context.Context, pool *pgxpool.Pool) error {
	email := envOr("ADMIN_EMAIL", "admin@samari-kuhsor.tj")
	name := envOr("ADMIN_NAME", "Администратор")

	password := os.Getenv("ADMIN_PASSWORD")
	generated := false
	if password == "" {
		// Generated rather than defaulted: a well-known default password on a
		// system holding regulatory records is worse than no seed at all.
		password = randomPassword()
		generated = true
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	res, err := seed.Reference(ctx, pool, email, hash, name)
	if err != nil {
		return err
	}

	slog.Info("reference seed complete",
		"roles_created", res.RolesCreated,
		"permissions_created", res.PermissionsCreated,
		"items_created", res.ItemsCreated,
		"translations_created", res.TranslationsCreated,
		"packaging_created", res.PackagingCreated,
		"locations_created", res.LocationsCreated,
		"admin_created", res.AdminCreated,
	)

	if res.AdminCreated && generated {
		// Printed once, to stdout, never logged as structured output that might be
		// shipped somewhere. It cannot be recovered afterwards.
		fmt.Printf("\n  Administrator created\n    email:    %s\n    password: %s\n\n"+
			"  This password is shown once. Change it after first login.\n\n", email, password)
	}
	return nil
}

func randomPassword() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		panic("seed: cannot generate a password: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func newPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		pgxdecimal.Register(c.TypeMap())
		return nil
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
