package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	t.Parallel()
	const plain = "правильный-пароль-42"

	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// The hash must not contain the password, in any form.
	if strings.Contains(hash, plain) {
		t.Fatal("hash contains the plaintext password")
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("hash is not argon2id: %q", hash)
	}

	ok, err := VerifyPassword(hash, plain)
	if err != nil || !ok {
		t.Fatalf("correct password did not verify: ok=%v err=%v", ok, err)
	}

	ok, err = VerifyPassword(hash, plain+"x")
	if err != nil {
		t.Fatalf("VerifyPassword on wrong password errored: %v", err)
	}
	if ok {
		t.Fatal("wrong password verified")
	}
}

// A fresh salt per hash is what stops a stolen database being attacked with one
// precomputed table for all users.
func TestHashIsSaltedPerCall(t *testing.T) {
	t.Parallel()
	a, err := HashPassword("same")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("same")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two hashes of the same password are identical — the salt is not random")
	}
	for _, h := range []string{a, b} {
		ok, err := VerifyPassword(h, "same")
		if err != nil || !ok {
			t.Fatalf("hash %q failed to verify: ok=%v err=%v", h, ok, err)
		}
	}
}

func TestEmptyPasswordRejected(t *testing.T) {
	t.Parallel()
	if _, err := HashPassword(""); err == nil {
		t.Fatal("empty password was hashed; it must be rejected")
	}
}

// A corrupt hash must be an error, never a silent "false". Callers distinguish
// them: a wrong password increments the lockout counter, a corrupt hash must not.
func TestMalformedHashIsAnError(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"empty":           "",
		"not phc":         "hunter2",
		"wrong algorithm": "$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"truncated":       "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA",
		"bad base64 salt": "$argon2id$v=19$m=65536,t=3,p=4$!!!!$aGFzaA",
		"bad params":      "$argon2id$v=19$memory=65536$c2FsdA$aGFzaA",
		"future version":  "$argon2id$v=99$m=65536,t=3,p=4$c2FsdA$aGFzaA",
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			ok, err := VerifyPassword(h, "anything")
			if err == nil {
				t.Fatal("expected an error for a malformed hash")
			}
			if ok {
				t.Fatal("malformed hash verified as correct")
			}
		})
	}
}

func TestNeedsRehash(t *testing.T) {
	t.Parallel()
	current, err := HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if NeedsRehash(current) {
		t.Error("a hash produced with the current parameters should not need rehashing")
	}
	// Weaker than the current cost: must be upgraded on next login.
	weak := "$argon2id$v=19$m=1024,t=1,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGE"
	if !NeedsRehash(weak) {
		t.Error("a hash with weaker parameters should need rehashing")
	}
	if !NeedsRehash("garbage") {
		t.Error("an unparseable hash should need rehashing")
	}
}

// The stored parameters, not today's constants, must drive verification —
// otherwise raising the cost would invalidate every existing password.
func TestVerifyUsesParametersFromTheHash(t *testing.T) {
	t.Parallel()
	weakHash, err := hashWith("legacy-password", 1, 8*1024, 1)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword(weakHash, "legacy-password")
	if err != nil {
		t.Fatalf("verifying a legacy hash errored: %v", err)
	}
	if !ok {
		t.Fatal("a hash made with older parameters no longer verifies — raising the cost would lock every user out")
	}
}
