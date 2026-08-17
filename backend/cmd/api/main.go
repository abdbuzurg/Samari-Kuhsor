// Command api is the Samari Kuhsor HTTP API.
//
// It is the only process that opens a database connection. Both Next.js apps
// reach it through their own BFF route handlers; the browser never calls it
// directly. See docs/03-API-CONTRACT.md.
package main

import (
	"fmt"
	"log/slog"
	"os"
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
	appEnv := envOr("APP_ENV", "development")
	mode := tlsMode(envOr("TLS_MODE", "off"))

	if err := validateEnv(appEnv, mode); err != nil {
		slog.Error("startup refused", "error", err)
		os.Exit(1)
	}

	slog.Info("samari api",
		"app_env", appEnv,
		"tls_mode", mode,
		"cookie_secure", mode.cookieSecure(),
	)
	slog.Info("no routes registered yet — see TASKS.md T05")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
