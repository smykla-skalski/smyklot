package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func TestPendingCICheckMaintenancePermissionEndsAfterDrain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		desired  storage.PendingCIMode
		draining bool
		want     bool
	}{
		{name: "checks desired", desired: storage.PendingCIModeChecks, want: true},
		{name: "labels draining", desired: storage.PendingCIModeLabels, draining: true, want: true},
		{name: "labels drained", desired: storage.PendingCIModeLabels, want: false},
	}
	for _, test := range tests {
		if got := pendingCIMustMaintainChecks(test.desired, test.draining); got != test.want {
			t.Errorf("%s: maintain checks = %t, want %t", test.name, got, test.want)
		}
	}
}

func TestPendingCIPolicyBlockDoesNotRetryTheFullSweep(t *testing.T) {
	t.Parallel()
	store, target, repository, now := pendingCIGateTestStore(t)
	target.Permissions = map[string]string{
		"checks":       "write",
		"merge_queues": "read",
		"statuses":     "read",
	}
	reconciler := &pendingCIGateReconciler{store: store, now: func() time.Time { return now }}
	if err := reconciler.Reconcile(
		t.Context(), nil, target, repository, nil, true,
	); err != nil {
		t.Fatalf("persisted policy blocker requested a retry: %v", err)
	}
	gate, err := store.GetPendingCIRepositoryGate(t.Context(), repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Readiness != storage.PendingCIBlocked ||
		gate.Reason != "administration write approval is missing" {
		t.Fatalf("policy gate = %#v", gate)
	}
}

func TestPendingCIChecksRequireMergeQueueReadPermission(t *testing.T) {
	t.Parallel()
	store, target, repository, now := pendingCIGateTestStore(t)
	delete(target.Permissions, "merge_queues")
	reconciler := &pendingCIGateReconciler{store: store, now: func() time.Time { return now }}
	if err := reconciler.Reconcile(
		t.Context(), nil, target, repository, nil, true,
	); err != nil {
		t.Fatalf("persisted permission blocker requested a retry: %v", err)
	}
	gate, err := store.GetPendingCIRepositoryGate(t.Context(), repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Readiness != storage.PendingCIBlocked ||
		gate.Reason != "merge queues read approval is missing" {
		t.Fatalf("merge-queue permission gate = %#v", gate)
	}
}

func TestPendingCIChecksRequireCommitStatusReadPermission(t *testing.T) {
	t.Parallel()
	store, target, repository, now := pendingCIGateTestStore(t)
	delete(target.Permissions, "statuses")
	reconciler := &pendingCIGateReconciler{store: store, now: func() time.Time { return now }}
	if err := reconciler.Reconcile(
		t.Context(), nil, target, repository, nil, true,
	); err != nil {
		t.Fatalf("persisted permission blocker requested a retry: %v", err)
	}
	gate, err := store.GetPendingCIRepositoryGate(t.Context(), repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Readiness != storage.PendingCIBlocked ||
		gate.Reason != "commit statuses read approval is missing" {
		t.Fatalf("commit-status permission gate = %#v", gate)
	}
}

func TestInactiveGateRemovesOwnedRulesetBeforeArtifactCleanup(t *testing.T) {
	t.Parallel()
	store, target, repository, now := pendingCIGateTestStore(t)
	armed, err := store.Arm(t.Context(), pendingci.ArmRequest{
		TargetID: target.ID, InstallationID: 77,
		RepositoryID: repository.ID, RepositoryFullName: repository.FullName,
		PullRequest: 42, HeadSHA: "cleanup-head", BaseBranch: "main",
		MergeMethod: pendingci.MergeMethodSquash, RequiredChecksOnly: true,
		Requester: "operator", SourceCommentID: 42,
		SourceRevision: now.Format(time.RFC3339Nano), SourceSequence: 1, SourceOrder: 42,
		Label: "smyklot:pending:ci:squash:required", RequestedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Finish(t.Context(), pendingci.FinishRequest{
		ID: armed.Request.ID, ExpectedRevision: armed.Request.Revision,
		Lifecycle: pendingci.LifecycleCancelled, Reason: "service stood down",
		FinishedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	gate, err := store.GetPendingCIRepositoryGate(t.Context(), repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	rulesetID, appID := int64(91), int64(17)
	_, err = store.UpdatePendingCIRepositoryGate(t.Context(), storage.PendingCIGateChange{
		RepositoryID: repository.ID, ExpectedRevision: gate.Revision,
		EffectiveMode: storage.PendingCIEffectiveChecks, Readiness: storage.PendingCIReady,
		Reason: pendingCIChecksReadyReason, AppID: &appID, RulesetID: &rulesetID,
		RulesetFingerprint: "owned", ObservedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	deletes := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/repos/owner/repo/rulesets/91" {
			t.Errorf("unexpected GitHub request %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected request", http.StatusNotFound)

			return
		}
		deletes++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer api.Close()
	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pendingCIGateReconciler{store: store, now: func() time.Time { return now }}
	if err := reconciler.Reconcile(
		t.Context(), client, target, repository, nil, false,
	); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 {
		t.Fatalf("owned ruleset deletes = %d, want 1", deletes)
	}
	gate, err = store.GetPendingCIRepositoryGate(t.Context(), repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Readiness != storage.PendingCIDraining || gate.RulesetID != nil {
		t.Fatalf("inactive draining gate = %#v", gate)
	}
}

func pendingCIGateTestStore(
	t *testing.T,
) (storage.Store, storage.Target, storage.Repository, time.Time) {
	t.Helper()
	store, err := open.Store(t.Context(), filepath.Join(t.TempDir(), "gate.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	err = store.ReconcileInstallation(t.Context(), storage.InstallationSnapshot{
		TargetID: "installation:77", InstallationID: "77", Kind: storage.TargetOrganization,
		Account: storage.Account{
			ID: "account:77", Provider: "github", SubjectID: "77",
			Login: "owner", UpdatedAt: now,
		},
		Repositories: []storage.RepositorySnapshot{{
			ID: "repository-20", Name: "repo", FullName: "owner/repo", DefaultBranch: "main",
		}},
		Permissions: map[string]string{
			"checks": "write", "administration": "write", "merge_queues": "read",
			"statuses": "read",
		},
		SyncedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.GetTarget(t.Context(), "installation:77")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := store.GetRepository(t.Context(), target.ID, "repository-20")
	if err != nil {
		t.Fatal(err)
	}

	return store, target, repository, now
}

func TestMergeQueueGuardIgnoresNonEnforcingRulesets(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/graphql":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"repository": map[string]any{"mergeQueue": nil}},
			})
		case "/repos/owner/repo/rulesets":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": int64(1), "target": pendingCIRulesetBranch, "enforcement": "evaluate"},
				{"id": int64(2), "target": pendingCIRulesetBranch, "enforcement": "disabled"},
				{"id": int64(3), "target": "tag", "enforcement": pendingCIRulesetActive},
			})
		default:
			t.Errorf("non-enforcing ruleset was read: %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureNoMergeQueue(
		t.Context(), client, "owner", "repo", "main", nil,
		storage.DefaultPendingCIBranchPatterns(),
	); err != nil {
		t.Fatalf("non-enforcing merge queues blocked checks: %v", err)
	}
}

func TestMergeQueueGuardChecksOpenPullRequestBases(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			t.Errorf("unexpected GitHub request %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected request", http.StatusNotFound)

			return
		}
		var request struct {
			Variables map[string]string `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		branch := request.Variables["branch"]
		seen[branch] = true
		var queue any
		if branch == "release/1" {
			queue = map[string]any{"id": "MQ_release"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"repository": map[string]any{"mergeQueue": queue}},
		})
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	prs := []map[string]interface{}{{
		"number": float64(7),
		"head":   map[string]interface{}{"sha": "head"},
		"base":   map[string]interface{}{"ref": "release/1"},
	}}
	if err := ensureNoMergeQueue(
		t.Context(), client, "owner", "repo", "main", prs,
		storage.PendingCIBranchPatterns{Include: []string{"~ALL"}},
	); err == nil {
		t.Fatal("classic merge queue on an open pull request base was accepted")
	}
	if !seen["main"] || !seen["release/1"] {
		t.Fatalf("checked merge queue branches = %#v", seen)
	}
}

func TestMergeQueueGuardIgnoresExcludedPullRequestBases(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			t.Fatalf("unexpected GitHub request %s %s", r.Method, r.URL.RequestURI())
		}
		var request struct {
			Variables map[string]string `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Variables["branch"] != "main" {
			t.Fatalf("excluded branch queue queried: %q", request.Variables["branch"])
		}
		_, _ = w.Write([]byte(`{"data":{"repository":{"mergeQueue":null}}}`))
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	prs := []map[string]interface{}{{
		"number": float64(7),
		"head":   map[string]interface{}{"sha": "head"},
		"base":   map[string]interface{}{"ref": "release/1"},
	}}
	if err := ensureNoMergeQueue(
		t.Context(), client, "owner", "repo", "main", prs,
		storage.DefaultPendingCIBranchPatterns(),
	); err != nil {
		t.Fatalf("excluded branch queue blocked checks mode: %v", err)
	}
}

func TestInactiveModeChecksOpenNonDefaultBases(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := strings.Contains(r.URL.Path, "release/1")
		switch {
		case strings.Contains(r.URL.Path, "/protection/required_status_checks") && release:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"contexts": []string{},
				"checks":   []map[string]any{{"context": storage.PendingCICheckName, "app_id": 17}},
			})
		case strings.Contains(r.URL.Path, "/protection/required_status_checks"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"contexts": []string{}, "checks": []map[string]any{},
			})
		case strings.Contains(r.URL.Path, "/rules/branches/"):
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Errorf("unexpected GitHub request %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	prs := []map[string]interface{}{{
		"number": float64(7),
		"head":   map[string]interface{}{"sha": "head"},
		"base":   map[string]interface{}{"ref": "release/1"},
	}}
	if err := ensureNoPendingCIRequiredContextOnBranches(
		t.Context(), client, "owner", "repo", "main", prs,
	); err == nil {
		t.Fatal("inactive mode accepted a required Smyklot check on an open non-default base")
	}
}

func TestLabelModeDetectsInheritedSmyklotRequirementOutsideDefaultBranch(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/rulesets":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": int64(91), "name": "release", "target": pendingCIRulesetBranch,
				"enforcement": pendingCIRulesetActive, "source_type": "Organization",
			}})
		case "/repos/owner/repo/rulesets/91":
			if r.URL.Query().Get("includes_parents") != "true" {
				t.Error("inherited ruleset was read without its parent")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": int64(91), "name": "release", "target": pendingCIRulesetBranch,
				"enforcement": pendingCIRulesetActive,
				"conditions": map[string]any{"ref_name": map[string]any{
					"include": []string{"refs/heads/release/*"}, "exclude": []string{},
				}},
				"rules": []map[string]any{{
					"type": "required_status_checks",
					"parameters": map[string]any{"required_status_checks": []map[string]any{{
						"context": storage.PendingCICheckName, "integration_id": int64(17),
					}}},
				}},
			})
		default:
			t.Errorf("unexpected GitHub request %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureNoPendingCIRequiredRulesets(
		t.Context(), client, "owner", "repo",
	); err == nil {
		t.Fatal("label mode accepted an inherited Smyklot requirement")
	}
}

func TestPullRequestOpenedWakesGateReconciliation(t *testing.T) {
	t.Parallel()
	srv := &server{pendingCIGateChanged: make(chan struct{}, 1)}
	err := srv.applyPendingCINotification(
		context.Background(),
		&webhook.PendingCINotification{
			Event: webhook.EventPullRequest, Action: "opened",
		},
		"delivery-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-srv.pendingCIGateChanged:
	default:
		t.Fatal("pull_request.opened did not wake pending CI gate reconciliation")
	}
}

func TestPendingCIBranchIncludedUsesRawRulesetRefs(t *testing.T) {
	t.Parallel()
	patterns := storage.PendingCIBranchPatterns{
		Include: []string{
			"~DEFAULT_BRANCH", "refs/heads/release/*", "refs/heads/releases/**/*",
			"refs/heads/stable/[!0-9]*",
		},
		Exclude: []string{
			"refs/heads/release/private-*", "refs/heads/stable/[!a-z]*",
		},
	}
	tests := []struct {
		branch string
		want   bool
	}{
		{branch: "main", want: true},
		{branch: "release/1.2", want: true},
		{branch: "release/private-1.2", want: false},
		{branch: "releases/2026/q3/hotfix", want: true},
		{branch: "feature/checks", want: false},
		{branch: "stable/ready", want: true},
		{branch: "stable/1-hotfix", want: false},
		{branch: "stable/_internal", want: false},
	}
	for _, test := range tests {
		if got := pendingCIBranchIncluded(test.branch, "main", patterns); got != test.want {
			t.Errorf("branch %q included = %t, want %t", test.branch, got, test.want)
		}
	}
}

func TestGitHubRefPatternMatchesFnmatchPathnameSemantics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pattern string
		ref     string
		want    bool
	}{
		{pattern: "refs/heads/**", ref: "refs/heads/a", want: true},
		{pattern: "refs/heads/**", ref: "refs/heads/a/b", want: false},
		{pattern: "refs/heads/**/*", ref: "refs/heads/a/b", want: true},
		{pattern: "refs/heads/*", ref: "refs/heads/.hidden", want: false},
		{pattern: "refs/heads/**/x", ref: "refs/heads/.a/x", want: false},
		{pattern: "refs/heads/.*/x", ref: "refs/heads/.a/x", want: true},
	}
	for _, test := range tests {
		if got := githubRefPatternMatches(test.pattern, test.ref); got != test.want {
			t.Errorf("pattern %q ref %q = %t, want %t", test.pattern, test.ref, got, test.want)
		}
	}
}

func TestPendingCICheckRenewalKeepsOneGenerationSuffix(t *testing.T) {
	t.Parallel()
	slot := pendingci.CheckSlot{
		ExternalID: "smyklot:merge-after-ci:github:repository:20:abc:g2",
		Generation: 2,
	}
	if got := pendingCIRenewedExternalID(slot); got !=
		"smyklot:merge-after-ci:github:repository:20:abc:g3" {
		t.Fatalf("renewed external ID = %q", got)
	}
}

func TestPendingCIRulesetBindsTheStableContextToTheApp(t *testing.T) {
	t.Parallel()
	patterns := storage.DefaultPendingCIBranchPatterns()
	ruleset := pendingCIRuleset(patterns, 17)
	if ruleset.Name != storage.PendingCIRulesetName ||
		ruleset.Enforcement != pendingCIRulesetActive {
		t.Fatalf("ruleset identity = %#v", ruleset)
	}
	statusChecks := ruleset.Rules.RequiredStatusChecks
	if statusChecks == nil || len(statusChecks.Checks) != 1 {
		t.Fatalf("required checks = %#v", statusChecks)
	}
	check := statusChecks.Checks[0]
	if check.Context != storage.PendingCICheckName || check.IntegrationID != 17 {
		t.Fatalf("required check = %#v", check)
	}
	if !statusChecks.DoNotEnforceOnCreate {
		t.Fatal("ruleset must not block branch creation before a baseline can be written")
	}
}
