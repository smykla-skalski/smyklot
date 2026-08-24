package panel

import (
	"testing"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestInstallationScheduleProfileHidesGlobalImpact(t *testing.T) {
	t.Parallel()

	profile := installationScheduleProfile(workqueue.Profile{
		AffectedInstallations: 7,
		AffectedItems:         11,
		AffectedPolicies:      5,
	})
	if profile.AffectedInstallations != 0 || profile.AffectedItems != 0 ||
		profile.AffectedPolicies != 0 {
		t.Fatalf("installation profile exposed global impact: %#v", profile)
	}
}
