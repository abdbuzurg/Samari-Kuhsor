package alerts_test

import (
	"testing"

	"github.com/qoim/samari/backend/internal/alerts"
)

// docs/05-MODULES.md §17 lists exactly ten triggers. If one is dropped during the
// build it must fail here, not go unnoticed until someone asks why the factory
// never got warned about an expiring certificate.
func TestAllTenTriggersAreAccountedFor(t *testing.T) {
	t.Parallel()
	if got := len(alerts.Kinds()); got != 10 {
		t.Errorf("%d triggers defined, docs/05-MODULES.md §17 lists 10", got)
	}
	seen := make(map[alerts.Kind]bool)
	for _, k := range alerts.Kinds() {
		if seen[k] {
			t.Errorf("duplicate trigger %s", k)
		}
		seen[k] = true
	}
}

// The seven derived conditions are known-unimplemented until their modules land.
// Pending() makes that gap explicit rather than letting the feed look healthy
// while silently reporting nothing.
func TestPendingConditionsAreDeclaredNotSilent(t *testing.T) {
	t.Parallel()
	pending := alerts.Pending()
	if len(pending) != 7 {
		t.Errorf("%d conditions pending, expected the 7 standing conditions", len(pending))
	}
	for kind, task := range pending {
		if task == "" {
			t.Errorf("%s is pending but names no task that will implement it", kind)
		}
	}
}
