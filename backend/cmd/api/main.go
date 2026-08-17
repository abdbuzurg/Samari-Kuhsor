// Command api is the Samari Kuhsor HTTP API.
//
// It is the only process that opens a database connection. Both Next.js apps
// reach it through their own BFF route handlers; the browser never calls it
// directly. See docs/03-API-CONTRACT.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/auth"
	"github.com/qoim/samari/backend/internal/domain/items"
	samarihttp "github.com/qoim/samari/backend/internal/http"
)

// tlsMode mirrors docs/07-IMPLEMENTATION-PLAN.md I24. The session cookie's
// Secure flag is derived from it and is never set by hand.
type tlsMode string

const (
	tlsOff      tlsMode = "off"      // plain HTTP; cookie NOT Secure
	tlsInternal tlsMode = "internal" // Caddy's own CA; cookie Secure
	tlsAuto     tlsMode = "auto"     // Let's Encrypt; cookie Secure
)

// cookieSecure reports whether the session cookie carries the Secure attribute.
//
// Browsers refuse to send a Secure cookie over plain HTTP, so during the
// IP-and-plain-HTTP client-testing phase (I25) this must be false or login fails
// silently — the cookie is set and then never returned.
func (m tlsMode) cookieSecure() bool { return m != tlsOff }

func (m tlsMode) valid() bool {
	switch m {
	case tlsOff, tlsInternal, tlsAuto:
		return true
	}
	return false
}

// validateEnv enforces the boot guard from I24: going live insecure must be
// impossible, not merely discouraged.
func validateEnv(appEnv string, mode tlsMode) error {
	if !mode.valid() {
		return fmt.Errorf("TLS_MODE must be one of off|internal|auto, got %q", mode)
	}
	if appEnv == "production" && mode != tlsAuto {
		return fmt.Errorf("refusing to start: APP_ENV=production requires TLS_MODE=auto, got %q", mode)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	appEnv := envOr("APP_ENV", "development")
	mode := tlsMode(envOr("TLS_MODE", "off"))
	addr := envOr("LISTEN_ADDR", ":8080")

	if err := validateEnv(appEnv, mode); err != nil {
		return err
	}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return errors.New("DB_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := newPool(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	srv, err := samarihttp.NewServer(
		auth.NewService(pool, auth.DefaultConfig()),
		items.NewService(pool),
		samarihttp.Config{ServiceKey: os.Getenv("SERVICE_KEY")},
	)
	if err != nil {
		// docs/04-RBAC.md:123 — an undeclared route means we do not serve at all.
		return fmt.Errorf("route declaration check failed: %w", err)
	}

	slog.Info("samari api",
		"app_env", appEnv,
		"tls_mode", mode,
		"cookie_secure", mode.cookieSecure(),
		"addr", addr,
	)
	// Print the whole access surface on every boot, so what is guarded and what is
	// deliberately public is visible without reading the router.
	for _, d := range srv.Declarations() {
		slog.Info("route", "declaration", d.String())
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
}

func newPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DB_URL: %w", err)
	}
	// Money and quantities are shopspring decimals end to end (CLAUDE.md §4.6).
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		pgxdecimal.Register(c.TypeMap())
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
