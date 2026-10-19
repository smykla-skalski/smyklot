package gate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/pkg/githubapp"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func TestRepeatedCIWebhookReusesAnAppliedBaseline(t *testing.T) {
	t.Parallel()
	store, err := open.Store(t.Context(), filepath.Join(t.TempDir(), "baseline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Now().UTC()
	targetID, repositoryID := storage.InstallationID(77), storage.RepositoryID(20)
	err = store.ReconcileInstallation(t.Context(), storage.InstallationSnapshot{
		TargetID: targetID, InstallationID: "77", Kind: storage.TargetOrganization,
		Account:      storage.Account{ID: "account:77", Provider: "github", SubjectID: "77", Login: "owner", UpdatedAt: now},
		Repositories: []storage.RepositorySnapshot{{ID: repositoryID, Name: "repo", FullName: "owner/repo", DefaultBranch: "main"}},
		Permissions:  map[string]string{"checks": "write"}, SyncedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.SaveInstallationSettings(t.Context(), storage.SaveInstallationSettingsRequest{
		TargetID: targetID, ActorAccountID: "account:77", ChangedAt: now,
		Target: &storage.InstallationTargetSettingsChange{RepositoryDefaultEnabled: true, ExpectedRevision: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	var reads atomic.Int64
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/installations/77/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "installation-token", "expires_at": now.Add(time.Hour)})
		case "/repos/owner/repo/pulls/42":
			reads.Add(1)
			_, _ = w.Write([]byte(`{"state":"closed","head":{"sha":"new-head"},"base":{"ref":"main"}}`))
		default:
			t.Errorf("unexpected GitHub request: %s", r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer api.Close()
	tokens, err := githubapp.NewTokenStore("17", githubtest.AppPrivateKey(), api.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	checks := &Checks{store: store, tokens: tokens, appID: 17, apiBaseURL: api.URL, now: func() time.Time { return now }}
	desired := baselineDesired()
	digest, err := checkDigest(desired)
	if err != nil {
		t.Fatal(err)
	}
	slot, err := store.EnsureCheckSlot(t.Context(), pendingci.EnsureCheckSlotRequest{
		TargetID: targetID, InstallationID: 77, RepositoryID: repositoryID,
		RepositoryFullName: "owner/repo", PullRequest: 42, HeadSHA: "head", AppID: 17,
		Name: storage.PendingCICheckName, ExternalID: "baseline-head", DesiredStatus: desired.Status,
		DesiredConclusion: desired.Conclusion, DesiredTitle: desired.Title,
		DesiredSummary: desired.Summary, DesiredDigest: digest, ChangedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ApplyCheckSlot(t.Context(), pendingci.ApplyCheckSlotRequest{
		ID: slot.ID, ExpectedRevision: slot.Revision, CheckRunID: 701, AppliedDigest: digest, AppliedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	gate := &Gate{store: store, Checks: checks, panelled: true, apiBaseURL: api.URL, tokens: tokens, coordinator: bot.NewCoordinator()}
	notification := &pendingci.Notification{
		Event: webhook.EventCheckRun, Source: webhook.Source{InstallationID: 77, Repository: webhook.Repository{ID: 20}},
		Signals: []pendingci.Signal{{Kind: pendingci.SignalWakePullRequest, PullRequest: 42, HeadSHA: "head", EventKey: "same-head"}},
	}
	for range 10 {
		if err := gate.HandleWebhook(t.Context(), notification, "delivery"); err != nil {
			t.Fatal(err)
		}
	}
	if reads.Load() != 0 {
		t.Fatalf("repeated CI hints made %d pull request reads", reads.Load())
	}
	notification.Signals[0].HeadSHA = "new-head"
	if err := gate.HandleWebhook(t.Context(), notification, "new-delivery"); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 1 {
		t.Fatalf("a new head made %d pull request reads, want 1", reads.Load())
	}
}
