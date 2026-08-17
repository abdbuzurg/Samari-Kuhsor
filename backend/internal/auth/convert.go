package auth

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Conversions between Go values and the pgtype wrappers sqlc generates. Kept in
// one place so no caller reaches for pgtype directly and gets Valid wrong — a
// zero-value pgtype with Valid=false silently writes NULL.

func timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// interval converts a Go duration to a Postgres interval. Postgres intervals are
// (months, days, microseconds); a duration has no calendar component, so it maps
// cleanly onto microseconds alone.
func interval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
