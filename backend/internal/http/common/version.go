package common

import (
	"encoding/json"
	"io"
	"net/http"
)

// Optimistic concurrency (docs/03-API-CONTRACT.md §7).
//
// Every mutable resource returns `version`. A PATCH must send the version it read;
// if the stored version differs, the response is 409 version_conflict.
//
// This is cheap now and it is the mechanism phase-2 Khorog synchronisation will
// rely on (D6) — which is why it is enforced uniformly rather than per module.

// Versioned is embedded in every PATCH request body.
type Versioned struct {
	Version *int32 `json:"version"`
}

// RequireVersion extracts the version a PATCH must carry.
//
// A missing version is a validation error, never a silent overwrite. Treating
// "absent" as "whatever is current" would make the guard opt-in, and the one
// client that forgets it is the one that clobbers a colleague's edit.
func RequireVersion(v Versioned) (int32, error) {
	if v.Version == nil {
		return 0, Validation(FieldError{
			Field: "version", Code: "required",
			Message: "Не указана версия записи",
		})
	}
	return *v.Version, nil
}

// CheckVersion compares the version a client sent against the stored one.
func CheckVersion(sent, stored int32) error {
	if sent != stored {
		return VersionConflict(stored)
	}
	return nil
}

// DecodeJSON reads a JSON request body into dst.
//
// Unknown fields are rejected. A typo in a field name would otherwise be accepted
// silently and the update would appear to succeed while changing nothing — the
// most confusing possible failure for whoever is entering data on the factory
// floor.
func DecodeJSON(r *http.Request, dst any) error {
	const maxBody = 1 << 20 // 1 MiB; uploads have their own path

	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return Validation(FieldError{
			Field: "body", Code: "invalid_json",
			Message: "Некорректный формат запроса",
		}).WithCause(err)
	}
	// Exactly one JSON value, so a trailing object cannot smuggle past the decoder.
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return Validation(FieldError{
			Field: "body", Code: "invalid_json",
			Message: "Некорректный формат запроса",
		})
	}
	return nil
}
