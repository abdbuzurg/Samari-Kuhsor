package main

import "testing"

// The Secure attribute on the session cookie is derived from TLS_MODE and never
// set by hand (docs/07-IMPLEMENTATION-PLAN.md I24). Browsers refuse to send a
// Secure cookie over plain HTTP, so getting this wrong makes login fail silently.
func TestTLSModeCookieSecure(t *testing.T) {
	cases := []struct {
		mode tlsMode
		want bool
	}{
		{tlsOff, false},
		{tlsInternal, true},
		{tlsAuto, true},
	}
	for _, c := range cases {
		if got := c.mode.cookieSecure(); got != c.want {
			t.Errorf("tlsMode(%q).cookieSecure() = %v, want %v", c.mode, got, c.want)
		}
	}
}

// I25: the launch cookie configuration must be proven in the suite, because the
// client-testing deployment runs plain HTTP and never exercises it.
func TestProductionCookieIsSecure(t *testing.T) {
	const prodMode = tlsAuto
	if err := validateEnv("production", prodMode); err != nil {
		t.Fatalf("production + TLS_MODE=auto must be allowed, got %v", err)
	}
	if !prodMode.cookieSecure() {
		t.Fatal("production must issue Secure cookies")
	}
}

func TestValidateEnv(t *testing.T) {
	cases := []struct {
		name    string
		appEnv  string
		mode    tlsMode
		wantErr bool
	}{
		{"dev plain http is fine", "development", tlsOff, false},
		{"dev self-signed is fine", "development", tlsInternal, false},
		{"dev auto is fine", "development", tlsAuto, false},
		{"staging plain http is fine", "staging", tlsOff, false},

		// The boot guard. Going live insecure must be impossible.
		{"production refuses plain http", "production", tlsOff, true},
		{"production refuses self-signed", "production", tlsInternal, true},
		{"production accepts auto", "production", tlsAuto, false},

		{"unknown mode rejected", "development", tlsMode("https"), true},
		{"empty mode rejected", "development", tlsMode(""), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateEnv(c.appEnv, c.mode)
			if (err != nil) != c.wantErr {
				t.Errorf("validateEnv(%q, %q) error = %v, wantErr %v", c.appEnv, c.mode, err, c.wantErr)
			}
		})
	}
}

func TestEnvOr(t *testing.T) {
	if got := envOr("SAMARI_DEFINITELY_UNSET_VAR", "fallback"); got != "fallback" {
		t.Errorf("envOr fallback = %q, want %q", got, "fallback")
	}
	t.Setenv("SAMARI_TEST_VAR", "set")
	if got := envOr("SAMARI_TEST_VAR", "fallback"); got != "set" {
		t.Errorf("envOr = %q, want %q", got, "set")
	}
	// An empty variable must fall back, not yield "".
	t.Setenv("SAMARI_TEST_EMPTY", "")
	if got := envOr("SAMARI_TEST_EMPTY", "fallback"); got != "fallback" {
		t.Errorf("envOr on empty = %q, want %q", got, "fallback")
	}
}
