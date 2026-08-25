package panel

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestRootSchedulePoliciesDistinguishDeploymentDefaults(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	current, err := harness.store.GetEffectiveQueuePolicy(
		t.Context(), workqueue.KindSyncScan, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	current.Cadence = 12 * time.Hour
	if _, err := harness.store.SaveQueuePolicy(t.Context(), workqueue.PolicyChange{
		Kind: current.Kind, Enabled: current.Enabled, Cadence: current.Cadence,
		ProfileID: current.ProfileID, DefaultPriority: current.DefaultPriority,
		RetryDelay: current.RetryDelay, Retention: current.Retention,
		ApprovalTTL: current.ApprovalTTL, Configuration: current.Configuration,
		ExpectedRevision: current.Revision, ActorID: "github:test:user:1",
		ChangedAt: harness.now,
	}); err != nil {
		t.Fatal(err)
	}

	response := harness.request(
		t, http.MethodGet, "/panel/api/v1/root/job-policies", nil, session,
	)
	requireResponse(t, response, "root queue policies", http.StatusOK)
	var result struct {
		PolicySet schedulePolicySet `json:"policy_set"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	requireSchedulePolicyCollections(t, result.PolicySet)
	deployment := queuePolicyFromSet(t, result.PolicySet.DeploymentDefaults, workqueue.KindSyncScan)
	effective := queuePolicyFromSet(t, result.PolicySet.Effective, workqueue.KindSyncScan)
	if deployment.Cadence != 6*time.Hour || effective.Cadence != 12*time.Hour {
		t.Fatalf("deployment = %s, effective = %s", deployment.Cadence, effective.Cadence)
	}
}

func TestInstallationSchedulesEncodeEmptyOverridesAsArray(t *testing.T) {
	harness := newPanelHarness(t, "root")
	session := harness.signIn(t)
	response := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/schedules",
		nil,
		session,
	)
	requireResponse(t, response, "installation queue policies", http.StatusOK)
	var result struct {
		PolicySet schedulePolicySet `json:"policies"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	requireSchedulePolicyCollections(t, result.PolicySet)
}

func requireSchedulePolicyCollections(t *testing.T, set schedulePolicySet) {
	t.Helper()
	if set.Current == nil || set.DeploymentDefaults == nil || set.Overrides == nil ||
		set.Effective == nil {
		t.Fatalf("schedule policy set contains a null collection: %#v", set)
	}
}

func queuePolicyFromSet(t *testing.T, policies []workqueue.Policy, kind workqueue.Kind) workqueue.Policy {
	t.Helper()
	for _, policy := range policies {
		if policy.Kind == kind {
			return policy
		}
	}
	t.Fatalf("missing policy %s", kind)

	return workqueue.Policy{}
}

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
