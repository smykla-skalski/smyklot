package pendingci

import (
	"testing"
	"time"
)

func TestDecideKeepsUnsafeCIArmed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		state ObservedState
	}{
		{name: "no checks", state: ObservedNoChecks},
		{name: "pending", state: ObservedPending},
		{name: "failing", state: ObservedFailing},
		{name: "indeterminate", state: ObservedIndeterminate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := policyRequest(now.Add(-2*time.Hour), test.state, "same")
			decision := mustDecide(t, request, policyObservation(now, test.state, "same"))
			if decision.Kind != DecisionReschedule || decision.Schedule != ScheduleDeferred {
				t.Fatalf("got %#v, want deferred reschedule", decision)
			}
			if decision.NextCheckAt != now.Add(6*time.Hour) {
				t.Fatalf("next check = %s, want six-hour fallback", decision.NextCheckAt)
			}
		})
	}
}

func TestDecideCancelsWhenAuthorizedHeadChanges(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	request := policyRequest(now.Add(-2*time.Hour), ObservedPending, "old")
	request.ArtifactKind = ArtifactLabel
	observation := policyObservation(now, ObservedNoChecks, "none")
	observation.HeadSHA = "new-head"
	decision := mustDecide(t, request, observation)
	if decision.Kind != DecisionFinish || decision.Lifecycle != LifecycleCancelled {
		t.Fatalf("got %#v, want exact-head cancellation", decision)
	}
}

func TestDecideCancelsWhenAuthorizedBaseChanges(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	request := policyRequest(now.Add(-2*time.Hour), ObservedPending, "old")
	request.ArtifactKind = ArtifactLabel
	observation := policyObservation(now, ObservedNoChecks, "none")
	observation.BaseBranch = "release"
	decision := mustDecide(t, request, observation)
	if decision.Kind != DecisionFinish || decision.Lifecycle != LifecycleCancelled {
		t.Fatalf("got %#v, want exact-base cancellation", decision)
	}
}

func TestDecideRequiresCheckReauthorizationWhenAuthorizedRevisionChanges(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	request := policyRequest(now.Add(-2*time.Hour), ObservedPending, "old")
	request.ArtifactKind = ArtifactCheck
	request.AuthorizationState = AuthorizationAuthorized
	observation := policyObservation(now, ObservedNoChecks, "none")
	observation.HeadSHA = "new-head"
	observation.BaseBranch = "release"

	decision := mustDecide(t, request, observation)
	if decision.Kind != DecisionReauthorize || decision.CandidateHeadSHA != "new-head" ||
		decision.CandidateBase != "release" {
		t.Fatalf("got %#v, want reauthorization for the observed revision", decision)
	}

	request.AuthorizationState = AuthorizationReauthorizationNeeded
	request.CandidateHeadSHA = "new-head"
	request.CandidateBaseBranch = "release"
	decision = mustDecide(t, request, observation)
	if decision.Kind != DecisionReschedule {
		t.Fatalf("got %#v, want the existing candidate left pending", decision)
	}
}

func TestDecideRetargetsReauthorizationWhenRevisionReturnsToAuthorizedHead(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	request := policyRequest(now.Add(-time.Hour), ObservedPending, "old")
	request.ArtifactKind = ArtifactCheck
	request.AuthorizationState = AuthorizationReauthorizationNeeded
	request.CandidateHeadSHA = "candidate-head"
	request.CandidateBaseBranch = "release"
	observation := policyObservation(now, ObservedPending, "current")

	decision := mustDecide(t, request, observation)
	if decision.Kind != DecisionReauthorize || decision.CandidateHeadSHA != request.HeadSHA ||
		decision.CandidateBase != request.BaseBranch {
		t.Fatalf("got %#v, want candidate retargeted to the current revision", decision)
	}
}

func TestDecideRequiresStableGreen(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	request := policyRequest(now.Add(-time.Minute), ObservedPending, "pending")
	first := mustDecide(t, request, policyObservation(now, ObservedPassing, "green"))
	if first.Kind != DecisionReschedule || first.NextCheckAt != now.Add(30*time.Second) {
		t.Fatalf("first green observation = %#v, want quiet-period reschedule", first)
	}
	request.LastObservedState = first.LastObservedState
	request.LastFingerprint = first.LastFingerprint
	request.LastProgressAt = first.LastProgressAt
	secondObservation := policyObservation(now.Add(30*time.Second), ObservedPassing, "green")
	second := mustDecide(t, request, secondObservation)
	if second.Kind != DecisionMerge || second.HeadSHA != request.HeadSHA {
		t.Fatalf("stable green observation = %#v, want exact-head merge", second)
	}
}

func TestDecideRequiresTwoGreenObservationsWithNoQuietDelay(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	request := policyRequest(now.Add(-time.Minute), ObservedPending, "pending")
	observation := policyObservation(now, ObservedPassing, "green")
	timing := Timing{
		ActiveInterval: 5 * time.Minute, DiscoveryGrace: 10 * time.Minute,
		DeferAfter: time.Hour, DeferredInterval: 6 * time.Hour, PassingQuiet: 0,
	}

	first, err := Decide(request, observation, timing)
	if err != nil {
		t.Fatal(err)
	}
	if first.Kind != DecisionReschedule || !first.NextCheckAt.Equal(now) {
		t.Fatalf("first green observation = %#v, want immediate second observation", first)
	}
	request.LastObservedState = first.LastObservedState
	request.LastFingerprint = first.LastFingerprint
	request.LastProgressAt = first.LastProgressAt
	second, err := Decide(request, observation, timing)
	if err != nil {
		t.Fatal(err)
	}
	if second.Kind != DecisionMerge {
		t.Fatalf("second green observation = %#v, want merge", second)
	}
}

func TestDecideCancelsTerminalPullRequestState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	for _, mutate := range []func(*Observation){
		func(observation *Observation) { observation.PullRequestOpen = false },
		func(observation *Observation) { observation.PendingLabelFound = false },
	} {
		observation := policyObservation(now, ObservedPending, "pending")
		mutate(&observation)
		decision := mustDecide(t, policyRequest(now, ObservedPending, "pending"), observation)
		if decision.Kind != DecisionFinish || decision.Lifecycle != LifecycleCancelled {
			t.Fatalf("got %#v, want terminal cancellation", decision)
		}
	}
}

func policyRequest(progressAt time.Time, state ObservedState, fingerprint string) Request {
	return Request{
		Lifecycle: LifecycleArmed, HeadSHA: "head", BaseBranch: "main",
		LastProgressAt:    progressAt,
		LastObservedState: string(state), LastFingerprint: fingerprint,
	}
}

func policyObservation(at time.Time, state ObservedState, fingerprint string) Observation {
	return Observation{
		HeadSHA: "head", BaseBranch: "main",
		PullRequestOpen: true, PendingLabelFound: true,
		State: state, Fingerprint: fingerprint, ObservedAt: at,
	}
}

func mustDecide(t *testing.T, request Request, observation Observation) Decision {
	t.Helper()
	decision, err := Decide(request, observation, Timing{
		ActiveInterval: 5 * time.Minute, DiscoveryGrace: 10 * time.Minute,
		DeferAfter: time.Hour, DeferredInterval: 6 * time.Hour, PassingQuiet: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	return decision
}
