package workqueue

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParsePendingCITimingUsesConfiguredValues(t *testing.T) {
	t.Parallel()
	timing, err := ParsePendingCITiming(json.RawMessage(`{
		"active_check_seconds":60,"no_check_grace_seconds":120,
		"defer_after_seconds":180,"deferred_check_seconds":240,
		"passing_quiet_seconds":15
	}`), PendingCITiming{})
	if err != nil {
		t.Fatal(err)
	}
	if timing.ActiveInterval != time.Minute || timing.DiscoveryGrace != 2*time.Minute ||
		timing.DeferAfter != 3*time.Minute || timing.DeferredInterval != 4*time.Minute ||
		timing.PassingQuiet != 15*time.Second {
		t.Fatalf("unexpected timing: %#v", timing)
	}
}

func TestPolicyConfigurationRejectsInvalidJobTiming(t *testing.T) {
	t.Parallel()
	if err := ValidatePolicyConfiguration(
		KindPendingCI,
		json.RawMessage(`{"active_check_seconds":0}`),
	); err == nil {
		t.Fatal("zero pending-CI interval was accepted")
	}
	if err := ValidatePolicyConfiguration(
		KindWebhookDelivery,
		json.RawMessage(`{"max_delay_seconds":30,"max_attempts":0}`),
	); err == nil {
		t.Fatal("zero webhook attempt budget was accepted")
	}
}
