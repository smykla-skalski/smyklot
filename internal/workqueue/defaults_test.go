package workqueue

import (
	"strings"
	"testing"
	"time"
)

func TestDeploymentPoliciesPreserveLegacyTiming(t *testing.T) {
	t.Parallel()

	policies := DeploymentPolicies(DeploymentDefaults{
		PollInterval:         90 * time.Second,
		PendingCIQuietPeriod: 45 * time.Second,
		PathIndexInterval:    20 * time.Minute,
	})
	if len(policies) != len(Kinds())-1 {
		t.Fatalf("deployment policy count = %d", len(policies))
	}
	if policyFromSet(t, policies, KindReactionScan).Cadence != 90*time.Second {
		t.Fatal("reaction cadence did not follow the deployment poll interval")
	}
	if policyFromSet(t, policies, KindPathRefresh).Cadence != 20*time.Minute {
		t.Fatal("path cadence did not follow the deployment path interval")
	}
	if policyFromSet(t, policies, KindSyncScan).Cadence != 6*time.Hour {
		t.Fatal("sync cadence did not preserve its independent recheck interval")
	}
	wantQuiet := `"passing_quiet_seconds":45`
	if configuration := string(policyFromSet(t, policies, KindPendingCI).Configuration); !strings.Contains(configuration, wantQuiet) {
		t.Fatalf("pending CI configuration = %s", configuration)
	}
}

func TestDeploymentPoliciesMapEverySweepWithoutZeroCadence(t *testing.T) {
	t.Parallel()

	policies := DeploymentPolicies(DeploymentDefaults{
		PollInterval:         1500 * time.Millisecond,
		PendingCIQuietPeriod: 30 * time.Second,
	})
	path := policyFromSet(t, policies, KindPathRefresh)
	if !path.Enabled || path.Cadence != 2*time.Second {
		t.Fatalf("every-sweep path policy = %#v", path)
	}

	disabled := DeploymentPolicies(DeploymentDefaults{
		PendingCIQuietPeriod: 30 * time.Second,
	})
	for _, kind := range []Kind{KindReactionScan, KindPendingCIGate, KindConfigMigration, KindPathRefresh} {
		policy := policyFromSet(t, disabled, kind)
		if policy.Enabled || policy.Cadence != 0 {
			t.Fatalf("disabled %s policy = %#v", kind, policy)
		}
	}
}

func policyFromSet(t *testing.T, policies []Policy, kind Kind) Policy {
	t.Helper()
	for _, policy := range policies {
		if policy.Kind == kind {
			return policy
		}
	}
	t.Fatalf("missing policy %s", kind)

	return Policy{}
}
