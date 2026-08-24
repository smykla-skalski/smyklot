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

func TestValidScheduleRequestInputRequiresRecurringCadence(t *testing.T) {
	t.Parallel()

	profileID := workqueue.AlwaysOpenProfileID
	input := scheduleRequestInput{
		Kind:            workqueue.KindSyncScan,
		BaseRevision:    1,
		ProfileID:       &profileID,
		CadenceSeconds:  0,
		DefaultPriority: workqueue.PriorityNormal,
		Reason:          "Run during installation hours",
	}
	if validScheduleRequestInput(input) {
		t.Fatal("recurring schedule request accepted a zero cadence")
	}

	input.Kind = workqueue.KindPendingCI
	if !validScheduleRequestInput(input) {
		t.Fatal("event-driven schedule request rejected a zero cadence")
	}
}
