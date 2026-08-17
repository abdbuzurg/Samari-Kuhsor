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

// Pending() existed so the seven standing conditions could not look healthy while
// silently counting nothing. All seven are now attached, so it must be empty — and
// this assertion is what stops a future condition being added with a nil query and
// reporting a permanent zero.
func TestNoConditionIsStillUnattached(t *testing.T) {
	t.Parallel()
	if pending := alerts.Pending(); len(pending) != 0 {
		t.Errorf("conditions with no query attached: %v", pending)
	}
}

// Every kind is either a persisted event or a derived condition — never both, and
// never neither. The split is the whole design (I15); a kind that falls outside it
// would be counted twice or not at all.
func TestEveryKindIsExactlyOneSpecies(t *testing.T) {
	t.Parallel()
	derived := make(map[alerts.Kind]bool)
	for _, c := range alerts.ConditionKinds() {
		derived[c] = true
	}
	var persisted, standing int
	for _, k := range alerts.Kinds() {
		switch {
		case alerts.IsPersisted(k) && derived[k]:
			t.Errorf("%s is both a persisted event and a derived condition", k)
		case alerts.IsPersisted(k):
			persisted++
		case derived[k]:
			standing++
		default:
			t.Errorf("%s is neither persisted nor derived — it will never appear", k)
		}
	}
	if persisted != 3 || standing != 7 {
		t.Errorf("%d persisted / %d standing, want 3 / 7 (I15)", persisted, standing)
	}
}
