package analytics

import (
	"context"
	"log/slog"
	"time"
)

// MaintenanceHour is when the daily run happens, in Asia/Dushanbe. An hour after
// deploy/backup.sh's 02:00, so a restored backup captures a database whose
// retention has already been applied rather than one mid-sweep.
const MaintenanceHour = 3

// RunDaily blocks until ctx is cancelled, running Maintain once a day at
// MaintenanceHour Dushanbe time.
//
// In-process rather than cron, deliberately (docs/01-DECISIONS.md D12).
// deploy/backup.sh documents a crontab line that has never been installed, and a
// retention promise that depends on an un-run manual step is not a promise. This
// is a single long-lived process on one box; there is no replica set to
// coordinate and nothing to forget.
//
// It runs once at start too. A machine that was off for a week must catch up
// when it comes back, not wait for 03:00.
func (s *Service) RunDaily(ctx context.Context, log *slog.Logger) {
	loc, err := time.LoadLocation("Asia/Dushanbe")
	if err != nil {
		// A container without tzdata should not disable retention. UTC is four
		// or five hours off, which for a nightly sweep is immaterial.
		log.Warn("analytics: Asia/Dushanbe unavailable, scheduling in UTC", "error", err)
		loc = time.UTC
	}

	s.runOnce(ctx, log)

	for {
		wait := untilNext(s.now().In(loc), MaintenanceHour)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.runOnce(ctx, log)
		}
	}
}

func (s *Service) runOnce(ctx context.Context, log *slog.Logger) {
	res, err := s.Maintain(ctx)
	if err != nil {
		// Never fatal. Analytics maintenance failing must not take the API down
		// with it — but it must be loud, because a silent failure is how a
		// ninety-day window becomes an indefinite one.
		log.Error("analytics: maintenance failed", "error", err)
		return
	}
	log.Info("analytics: maintenance complete",
		"days_rolled_up", res.DaysRolledUp,
		"rows_deleted", res.RowsDeleted,
		"retention_days", int(Retention.Hours()/24))
}

// untilNext returns how long until the next occurrence of hour.
func untilNext(now time.Time, hour int) time.Duration {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(now)
}

// WarnIfStale logs when maintenance has not run recently.
//
// Called at boot. Without it, a ticker that died is indistinguishable from one
// that is working, and nobody would discover the retention window had lapsed
// until somebody asked how long visitor data is kept.
func (s *Service) WarnIfStale(ctx context.Context, log *slog.Logger) {
	stale, last, err := s.MaintenanceStale(ctx)
	if err != nil {
		log.Warn("analytics: could not read the last maintenance run", "error", err)
		return
	}
	if !stale {
		return
	}
	if last == nil {
		// Expected on a fresh install; the run below fixes it within seconds.
		log.Info("analytics: maintenance has never run")
		return
	}
	log.Warn("analytics: maintenance is overdue — the retention window may not be enforced",
		"last_run", last.Format(time.RFC3339),
		"stale_after_hours", int(StaleAfter.Hours()))
}
