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

func TestEveryKindIsBounded(t *testing.T) {
	t.Parallel()

	for _, kind := range Kinds() {
		retention := Retention(kind)
		if retention <= 0 || retention > EventRetention {
			t.Fatalf("%s retention = %s", kind, retention)
		}
	}
	if Retention(KindReactionScan) != RoutineRetention {
		t.Fatal("a repository sweep is kept as long as work somebody asked for")
	}
	if Retention(KindWebhookDelivery) != EventRetention {
		t.Fatal("a delivery is kept as briefly as a repository sweep")
	}

	policies := DeploymentPolicies(DeploymentDefaults{
		PollInterval:         5 * time.Minute,
		PendingCIQuietPeriod: 30 * time.Second,
	})
	for _, policy := range policies {
		if policy.Retention == nil || *policy.Retention != Retention(policy.Kind) {
			t.Fatalf("%s deployment retention = %v", policy.Kind, policy.Retention)
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
