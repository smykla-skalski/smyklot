package gate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestConfiguredBypassPolicyConverges(t *testing.T) {
	t.Parallel()
	for _, allow := range []bool{false, true} {
		name := "deny"
		if allow {
			name = "allow selected"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client, writes := pendingCIRulesetAPI(t, pendingCIRulesetListing, pendingCIRulesetWithBypasses)
			desired := ruleset(storage.DefaultPendingCIBranchPatterns(), 17)
			applyBypassPolicy(&desired, &storage.PendingCIBypassPolicy{Allow: allow, Actors: []orgsync.RulesetBypassActor{
				{ActorID: 17, ActorType: "Integration", Mode: "always"},
				{ActorType: "OrganizationAdmin", Mode: "pull_request"},
				{ActorType: "DeployKey", Mode: "exempt"},
			}})
			gate := storage.PendingCIRepositoryGate{RulesetID: new(int64(91))}
			for range 2 {
				if _, err := reconcilePendingCIRuleset(t.Context(), client, "owner", "repo", gate, desired); err != nil {
					t.Fatal(err)
				}
			}
			assertOneBypassWrite(t, writes)
			actual, err := client.GetRepositoryRuleset(t.Context(), "owner", "repo", 91)
			if err != nil {
				t.Fatal(err)
			}
			if !samePendingCIRuleset(actual, desired) {
				t.Fatalf("actual bypasses = %+v, want %+v", actual.BypassActors, desired.BypassActors)
			}
		})
	}
}

func TestUnavailableBypassActorDoesNotFallBackToDroppingExceptions(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = fmt.Fprint(w, pendingCIRulesetWithBypasses)
		case http.MethodPut:
			var body struct {
				Actors []orgsync.RulesetBypassActor `json:"bypass_actors"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if len(body.Actors) != 1 || body.Actors[0].ActorID != 999 {
				t.Errorf("silently substituted exceptions: %+v", body.Actors)
			}
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = fmt.Fprint(w, `{"message":"Validation Failed","errors":[{"message":"Bypass actor is not available for this repository"}]}`)
		default:
			t.Errorf("unexpected fallback mutation: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(api.Close)
	client, err := github.NewClient("installation", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	desired := ruleset(storage.DefaultPendingCIBranchPatterns(), 17)
	applyBypassPolicy(&desired, &storage.PendingCIBypassPolicy{Allow: true, Actors: []orgsync.RulesetBypassActor{
		{ActorID: 999, ActorType: "Integration", Mode: "always"},
	}})
	_, err = reconcilePendingCIRuleset(t.Context(), client, "owner", "repo", storage.PendingCIRepositoryGate{RulesetID: new(int64(91))}, desired)
	if err == nil {
		t.Fatal("rejected exception was reported ready")
	}
	actual, err := client.GetRepositoryRuleset(t.Context(), "owner", "repo", 91)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual.BypassActors) != 3 || actual.BypassActors[0].ActorID != 17 {
		t.Fatalf("existing policy changed: %+v", actual.BypassActors)
	}
}

func TestBypassPolicyInheritanceAndFingerprint(t *testing.T) {
	t.Parallel()
	workspace := &storage.PendingCIBypassPolicy{Allow: true, Actors: []orgsync.RulesetBypassActor{{ActorID: 17, ActorType: "Integration", Mode: "always"}}}
	target := storage.Target{PendingCIBypassPolicyDefault: workspace}
	repository := storage.Repository{}
	if storage.EffectivePendingCIBypassPolicy(target, repository) != workspace {
		t.Fatal("repository did not inherit workspace exceptions")
	}
	repository.PendingCIBypassPolicyOverride = &storage.PendingCIBypassPolicy{Allow: false}
	policy := storage.EffectivePendingCIBypassPolicy(target, repository)
	if policy == nil || policy.Allow {
		t.Fatal("repository denial did not override workspace allowance")
	}
	before := ruleset(storage.DefaultPendingCIBranchPatterns(), 17)
	after := before
	applyBypassPolicy(&before, workspace)
	applyBypassPolicy(&after, policy)
	first, err := rulesetFingerprint(before)
	if err != nil {
		t.Fatal(err)
	}
	second, err := rulesetFingerprint(after)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("policy change did not invalidate readiness fingerprint")
	}
}

func assertOneBypassWrite(t *testing.T, writes <-chan []byte) {
	t.Helper()
	select {
	case <-writes:
	default:
		t.Fatal("explicit policy did not replace existing bypasses")
	}
	select {
	case extra := <-writes:
		t.Fatalf("policy did not converge: %s", extra)
	default:
	}
}
