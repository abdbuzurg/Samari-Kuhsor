package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Passwords are argon2id (docs/03-API-CONTRACT.md:191).
//
// Hashes are stored in the PHC string format, which carries the parameters used
// to produce them. That matters because the cost parameters below will be raised
// as hardware improves, and existing hashes must keep verifying — a hash that
// hard-codes today's parameters somewhere else is a hash that cannot be migrated.

// Argon2 cost parameters. The RFC 9106 second recommended option: 64 MiB, three
// passes. Comfortable on the single Dushanbe box for a system with a few dozen
// staff logging in, and far above the point where offline cracking is cheap.
var (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024 // KiB
	argonThreads uint8  = threads()
	argonKeyLen  uint32 = 32
	saltLen             = 16
)

func threads() uint8 {
	// More parallelism buys little and costs scheduling on a shared box.
	return uint8(max(1, min(runtime.NumCPU(), 4)))
}

var (
	// ErrMalformedHash means the stored hash could not be parsed. Treated as a
	// verification failure by callers, never as a successful login.
	ErrMalformedHash = errors.New("auth: malformed password hash")
	// ErrIncompatibleVersion means the hash was produced by a newer argon2.
	ErrIncompatibleVersion = errors.New("auth: incompatible argon2 version")
)

// HashPassword returns a PHC-format argon2id hash with a fresh random salt.
func HashPassword(plain string) (string, error) {
	return hashWith(plain, argonTime, argonMemory, argonThreads)
}

// hashWith is HashPassword with explicit cost parameters. Separated so the cost
// can be raised over time and so verification of hashes made under older
// parameters stays exercised by tests.
func hashWith(plain string, t, memory uint32, threads uint8) (string, error) {
	if plain == "" {
		return "", errors.New("auth: password must not be empty")
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: read salt: %w", err)
	}
	key := argon2.IDKey([]byte(plain), salt, t, memory, threads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memory, t, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether plain matches the encoded hash.
//
// The comparison is constant-time. Errors are returned separately from the
// boolean so callers can distinguish "wrong password" from "this hash is corrupt"
// — the first should increment the failure counter, the second should not.
func VerifyPassword(encoded, plain string) (bool, error) {
	params, salt, want, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(plain), salt, params.time, params.memory, params.threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, ErrMalformedHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	if version != argon2.Version {
		return argonParams{}, nil, nil, ErrIncompatibleVersion
	}

	var p argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return argonParams{}, nil, nil, ErrMalformedHash
	}

	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	return p, salt, key, nil
}

// NeedsRehash reports whether a stored hash was produced with weaker parameters
// than the current ones, so it can be upgraded transparently on next login.
func NeedsRehash(encoded string) bool {
	p, _, _, err := decodeHash(encoded)
	if err != nil {
		return true // unparseable: replace it
	}
	return p.memory < argonMemory || p.time < argonTime
}
