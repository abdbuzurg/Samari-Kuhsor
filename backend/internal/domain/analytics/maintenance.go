package analytics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
)

// MaintenanceResult is what one run did, and what the audit row records.
type MaintenanceResult struct {
	DaysRolledUp    int
	RowsDeleted     int
	OldestSurviving *time.Time
}

// Maintain rolls completed days up and then deletes anything past the retention
// window (docs/01-DECISIONS.md D12).
//
// The order matters and is not an implementation detail: rolling up BEFORE
// deleting is the only reason ninety-day-old history survives at all. Reverse
// them and the counts go with the rows.
//
// One transaction. A run that rolled up and then failed to delete would leave
// the window silently unenforced while looking like it had worked.
//
// This is the one place in the system that writes an audit row for a DELETION
// rather than for a mutation, which is the other half of D12's carve-out:
// ingestion writes nothing, so the trail that proves who released a batch is not
// buried under web traffic, but the deletion itself stays provable.
func (s *Service) Maintain(ctx context.Context) (MaintenanceResult, error) {
	var res MaintenanceResult

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("analytics: maintain: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	days, err := q.AnalyticsDaysNeedingRollup(ctx)
	if err != nil {
		return res, fmt.Errorf("analytics: days: %w", err)
	}
	for _, day := range days {
		if _, err := q.RollUpAnalyticsDay(ctx, day); err != nil {
			return res, fmt.Errorf("analytics: roll up %v: %w", day.Time, err)
		}
		res.DaysRolledUp++
	}

	cutoff := s.now().Add(-Retention)
	deleted, err := q.DeleteAnalyticsEventsBefore(ctx,
		pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		return res, fmt.Errorf("analytics: delete: %w", err)
	}
	res.RowsDeleted = int(deleted)

	oldest, err := q.OldestAnalyticsEvent(ctx)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return res, fmt.Errorf("analytics: oldest: %w", err)
	}
	var oldestDate pgtype.Date
	if oldest.Valid {
		t := oldest.Time
		res.OldestSurviving = &t
		oldestDate = pgtype.Date{Time: t, Valid: true}
	}

	if _, err := q.RecordAnalyticsMaintenanceRun(ctx, db.RecordAnalyticsMaintenanceRunParams{
		DaysRolledUp:    int32(res.DaysRolledUp),
		RowsDeleted:     int32(res.RowsDeleted),
		OldestSurviving: oldestDate,
	}); err != nil {
		return res, fmt.Errorf("analytics: record run: %w", err)
	}

	// Actor is nil: nobody ran this, a timer did. The audit viewer renders that
	// as «Система», which is truthful — inventing a user would put a name
	// against an action no person took.
	if err := audit.Record(ctx, tx, audit.Entry{
		Action: audit.ActionDelete, Resource: Resource,
		After: map[string]any{
			"days_rolled_up": res.DaysRolledUp,
			"rows_deleted":   res.RowsDeleted,
			"retention_days": int(Retention.Hours() / 24),
		},
	}); err != nil {
		return res, err
	}

	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("analytics: maintain: commit: %w", err)
	}
	return res, nil
}

// StaleAfter is how long the platform tolerates maintenance not having run
// before it says so out loud.
const StaleAfter = 48 * time.Hour

// MaintenanceStale reports whether the last successful run is older than
// StaleAfter, and when it was.
//
// The API calls this at boot and logs a warning. Without it a ticker that dies
// silently is indistinguishable from one that is working, and the ninety-day
// window becomes an assertion in a privacy policy rather than a fact about the
// database.
func (s *Service) MaintenanceStale(ctx context.Context) (stale bool, last *time.Time, err error) {
	run, err := db.New(s.pool).LastAnalyticsMaintenanceRun(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Never run. On a fresh install that is expected rather than wrong,
			// so the caller decides how loudly to say it.
			return true, nil, nil
		}
		return false, nil, fmt.Errorf("analytics: last run: %w", err)
	}
	when := run.RanAt.Time
	return s.now().Sub(when) > StaleAfter, &when, nil
}
