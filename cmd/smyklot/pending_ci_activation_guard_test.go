package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestPendingCICheckModeRequiresTheExactBaseBranchContext(t *testing.T) {
	t.Parallel()

	api := httptest.NewServer(pendingCIActivationGuardHandler(t))
	defer api.Close()

	client, err := github.NewClient("installation-token", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	store, err := open.Store(t.Context(), filepath.Join(t.TempDir(), "activation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	err = store.ReconcileInstallation(t.Context(), storage.InstallationSnapshot{
		TargetID: "installation:77", InstallationID: "77",
		Kind: storage.TargetOrganization,
		Account: storage.Account{
			ID: "account:77", Provider: "github", SubjectID: "77",
			Login: "owner", UpdatedAt: now,
		},
		Repositories: []storage.RepositorySnapshot{{
			ID: "repository-20", Name: "repo", FullName: "owner/repo", DefaultBranch: "main",
		}},
		SyncedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	gate, err := store.GetPendingCIRepositoryGate(t.Context(), "repository-20")
	if err != nil {
		t.Fatal(err)
	}
	appID := int64(17)
	_, err = store.UpdatePendingCIRepositoryGate(t.Context(), storage.PendingCIGateChange{
		RepositoryID: "repository-20", ExpectedRevision: gate.Revision,
		EffectiveMode: storage.PendingCIEffectiveChecks, Readiness: storage.PendingCIReady,
		Reason: "Checks and required context are ready", AppID: &appID, ObservedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	guard := githubPendingCIActivationGuard{
		server:       &server{store: store, panel: &adminpanel.Server{}},
		client:       client,
		targetID:     "installation:77",
		repositoryID: "repository-20",
		owner:        "owner",
		repository:   "repo",
	}
	mode, err := guard.PendingCIMode(t.Context(), "release")
	if err != nil {
		t.Fatal(err)
	}
	if mode != storage.PendingCIModeChecks {
		t.Fatalf("release mode = %q, want checks", mode)
	}
	if _, err := guard.PendingCIMode(t.Context(), "feature"); err == nil {
		t.Fatal("check mode activated on a base branch without the Smyklot context")
	}
	if _, err := guard.PendingCIMode(t.Context(), "queued"); err == nil {
		t.Fatal("check mode activated on a base branch with a merge queue")
	}
	eligible, err := guard.repositoryAllowsActivation(t.Context(), "release")
	if err != nil {
		t.Fatal(err)
	}
	if eligible {
		t.Fatal("check mode activated outside the configured branch patterns")
	}
	repositorySettings, err := store.GetRepository(
		t.Context(), "installation:77", "repository-20",
	)
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	_, err = store.UpdateRepositorySettings(t.Context(), storage.RepositorySettingsChange{
		TargetID: "installation:77", RepositoryID: "repository-20",
		ActorAccountID: "account:77", EnabledOverride: &disabled,
		ExpectedRevision: repositorySettings.Revision, ChangedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := guard.AllowsActivation(
		t.Context(), pendingci.ArtifactCheck, "release", false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("check mode activated after the repository was disabled")
	}
}

func pendingCIActivationGuardHandler(t *testing.T) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/graphql":
			var request struct {
				Variables map[string]string `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			var queue any
			if request.Variables["branch"] == "queued" {
				queue = map[string]any{"id": "MQ_queued"}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"repository": map[string]any{"mergeQueue": queue}},
			})
		case "/repos/owner/repo/branches/release/protection/required_status_checks",
			"/repos/owner/repo/branches/feature/protection/required_status_checks":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Branch not protected"}`))
		case "/repos/owner/repo/rules/branches/release":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"type": "required_status_checks",
				"parameters": map[string]any{
					"required_status_checks": []map[string]any{{
						"context": storage.PendingCICheckName, "integration_id": int64(17),
					}},
				},
			}})
		case "/repos/owner/repo/rules/branches/feature":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected GitHub request %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	})
}

func TestRequiredOnlyExcludesOnlySmyklotsOwnedRequirement(t *testing.T) {
	t.Parallel()
	appID := int64(17)
	otherAppID := int64(18)
	required := []github.RequiredCheck{
		{Context: storage.PendingCICheckName, AppID: &appID},
		{Context: storage.PendingCICheckName, AppID: &otherAppID},
		{Context: "build"},
	}

	external := pendingCIExternalRequiredChecks(
		required,
		storage.PendingCICheckName,
		appID,
	)
	if len(external) != 2 || external[0].AppID == nil ||
		*external[0].AppID != otherAppID || external[1].Context != "build" {
		t.Fatalf("external required checks = %#v", external)
	}
}
