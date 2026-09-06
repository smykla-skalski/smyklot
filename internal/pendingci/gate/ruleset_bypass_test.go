package gate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

const pendingCIRulesetWithBypasses = `{
	"name":"Smyklot: merge after CI","target":"branch","enforcement":"active",
	"conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},
	"bypass_actors":[
		{"actor_id":17,"actor_type":"Integration","bypass_mode":"always"},
		{"actor_id":23,"actor_type":"Team","bypass_mode":"pull_request"},
		{"actor_id":29,"actor_type":"Integration","bypass_mode":"exempt"}
	],
	"rules":[{"type":"required_status_checks","parameters":{
		"strict_required_status_checks_policy":false,"do_not_enforce_on_create":true,
		"required_status_checks":[{"context":"Smyklot / merge after CI","integration_id":17}]
	}}]
}`

const pendingCIRulesetListing = `[
	{"id":91,"name":"Smyklot: merge after CI","target":"branch",
	"enforcement":"active","source_type":"Repository"}
]`

func TestPendingCIRulesetPreservesExistingBypassesUntilConfigured(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		ownedID *int64
	}{
		{name: "recorded ownership", ownedID: new(int64(91))},
		{name: "adoption without recorded ownership"},
		{name: "adoption after recorded ruleset disappeared", ownedID: new(int64(90))},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, writes := pendingCIRulesetAPI(t, pendingCIRulesetListing, pendingCIRulesetWithBypasses)
			desired := ruleset(storage.DefaultPendingCIBranchPatterns(), 17)
			gate := storage.PendingCIRepositoryGate{RulesetID: test.ownedID}
			for range 2 {
				id, err := reconcilePendingCIRuleset(t.Context(), client, "owner", "repo", gate, desired)
				if err != nil || id != 91 {
					t.Fatalf("reconcile = %d, %v", id, err)
				}
				gate.RulesetID = &id
			}
			select {
			case body := <-writes:
				t.Fatalf("matching ruleset with configured bypasses was rewritten: %s", body)
			default:
			}
		})
	}
}

func TestPendingCIRulesetRepairsPolicyWithoutDroppingBypasses(t *testing.T) {
	t.Parallel()
	client, writes := pendingCIRulesetAPI(t, pendingCIRulesetListing, pendingCIRulesetWithBypasses)
	desired := ruleset(storage.PendingCIBranchPatterns{Include: []string{"refs/heads/release/*"}}, 42)
	gate := storage.PendingCIRepositoryGate{RulesetID: new(int64(91))}
	for range 2 {
		id, err := reconcilePendingCIRuleset(t.Context(), client, "owner", "repo", gate, desired)
		if err != nil || id != 91 {
			t.Fatalf("reconcile = %d, %v", id, err)
		}
	}
	var body []byte
	select {
	case body = <-writes:
	default:
		t.Fatal("changed branch policy and app binding were not applied")
	}
	select {
	case extra := <-writes:
		t.Fatalf("second reconciliation was not idempotent: %s", extra)
	default:
	}
	var original, updated map[string]json.RawMessage
	if err := json.Unmarshal([]byte(pendingCIRulesetWithBypasses), &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err := json.Unmarshal(original["bypass_actors"], &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(updated["bypass_actors"], &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bypass actors changed: got %s, want %s", updated["bypass_actors"], original["bypass_actors"])
	}
	actual, err := client.GetRepositoryRuleset(t.Context(), "owner", "repo", 91)
	if err != nil {
		t.Fatal(err)
	}
	actual.BypassActors = nil
	if !samePendingCIRuleset(actual, desired) {
		t.Fatalf("managed policy was not repaired: %#v", actual)
	}
}

func TestPendingCIRulesetBypassesDoNotAllowConflictingAdoption(t *testing.T) {
	t.Parallel()
	client, writes := pendingCIRulesetAPI(t, pendingCIRulesetListing, pendingCIRulesetWithBypasses)
	desired := ruleset(storage.DefaultPendingCIBranchPatterns(), 42)
	id, err := reconcilePendingCIRuleset(
		t.Context(), client, "owner", "repo", storage.PendingCIRepositoryGate{}, desired,
	)
	if err == nil || id != 0 {
		t.Fatalf("adopted a ruleset bound to another app: %d, %v", id, err)
	}
	select {
	case body := <-writes:
		t.Fatalf("conflicting ruleset was changed: %s", body)
	default:
	}
}

func TestPendingCIRulesetCreationDoesNotGrantBypasses(t *testing.T) {
	t.Parallel()
	client, writes := pendingCIRulesetAPI(t, "[]", "{}")
	id, err := reconcilePendingCIRuleset(
		t.Context(), client, "owner", "repo", storage.PendingCIRepositoryGate{},
		ruleset(storage.DefaultPendingCIBranchPatterns(), 17),
	)
	if err != nil || id != 91 {
		t.Fatalf("create = %d, %v", id, err)
	}
	select {
	case body := <-writes:
		var created struct {
			BypassActors []json.RawMessage `json:"bypass_actors"`
		}
		if err := json.Unmarshal(body, &created); err != nil {
			t.Fatal(err)
		}
		if len(created.BypassActors) != 0 {
			t.Fatalf("new ruleset granted bypasses: %s", body)
		}
	default:
		t.Fatal("ruleset was not created")
	}
}

func pendingCIRulesetAPI(t *testing.T, listing, current string) (*github.Client, <-chan []byte) {
	t.Helper()
	var mutex sync.Mutex
	writes := make(chan []byte, 8)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /repos/owner/repo/rulesets":
			_, _ = io.WriteString(w, listing)
		case "GET /repos/owner/repo/rulesets/91":
			_, _ = io.WriteString(w, current)
		case "POST /repos/owner/repo/rulesets", "PUT /repos/owner/repo/rulesets/91":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				http.Error(w, "unreadable body", http.StatusBadRequest)
				return
			}
			writes <- body
			current = string(body)
			_, _ = fmt.Fprint(w, `{"id":91}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	t.Cleanup(api.Close)
	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	return client, writes
}
