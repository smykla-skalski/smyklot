package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/configsync"
	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/internal/orgsync"
	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestConfigFilePanelResolutionUsesFreshGitHubReadAndAtomicStore(t *testing.T) {
	service, remote := configFilePanelHarness(t)
	preview, err := service.PreviewConfigurationFile(t.Context(), "workspace", "repo")
	if err != nil || preview.ReviewToken == "" || len(preview.Choices) != 2 {
		t.Fatalf("fresh preview = %+v (%v)", preview, err)
	}
	request := adminpanel.ConfigFileResolutionRequest{
		TargetID: "workspace", RepositoryID: "repo", ReviewToken: preview.ReviewToken,
		Side: "file", ActorAccountID: "owner",
	}
	remote.mu.Lock()
	remote.head = strings.Repeat("b", 40)
	remote.mu.Unlock()
	if err := service.ResolveConfigurationFile(t.Context(), request); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("changed GitHub commit must reject old choice: %v", err)
	}
	state, err := service.store.GetConfigFileState(t.Context(), "workspace", "repo")
	if err != nil || state.Revision != 0 {
		t.Fatalf("stale choice was persisted: %+v (%v)", state, err)
	}
	preview, err = service.PreviewConfigurationFile(t.Context(), "workspace", "repo")
	if err != nil || preview.ReviewToken == request.ReviewToken {
		t.Fatalf("new commit did not produce a new review: %+v (%v)", preview, err)
	}
	request.ReviewToken = preview.ReviewToken
	if err := service.ResolveConfigurationFile(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	state, err = service.store.GetConfigFileState(t.Context(), "workspace", "repo")
	if err != nil || state.Revision != 1 {
		t.Fatalf("fresh choice was not persisted: %+v (%v)", state, err)
	}
	connection, err := configsync.DecodeConnection(state, config.PanelFileRepository)
	if err != nil || connection.Resolution == nil || connection.Resolution.Side != "file" || connection.Status != configsync.StatusPending {
		t.Fatalf("pending connection = %+v (%v)", connection, err)
	}
	repository, err := service.store.GetRepository(t.Context(), "workspace", "repo")
	if err != nil || repository.Revision != 2 || repository.ConfigPatch.CommandPrefix == nil || *repository.ConfigPatch.CommandPrefix != "/panel " {
		t.Fatalf("HTTP choice applied settings before the worker: %+v (%v)", repository, err)
	}
	if err := service.ResolveConfigurationFile(t.Context(), request); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("a repeated choice must not write twice: %v", err)
	}
	assertConfigFileResolutionAudit(t, service)
}

func assertConfigFileResolutionAudit(t *testing.T, service *server) {
	t.Helper()
	audit, err := service.store.ListAudit(t.Context(), "workspace", storage.AuditPageRequest{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range audit.Items {
		if item.Action == "configuration_file.resolved" && item.Actor.ID == "owner" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("resolution audits = %d", count)
	}
}

func TestConfigFilePanelReviewRespectsExclusionBeforeReadingGitHub(t *testing.T) {
	service, remote := configFilePanelHarness(t)
	coordinator := &configurationScopeCoordinator{stop: "repo"}
	service.pendingCICoordinator = coordinator
	if _, err := service.PreviewConfigurationFile(t.Context(), "workspace", "repo"); !errors.Is(err, context.Canceled) {
		t.Fatalf("preview bypassed exclusion: %v", err)
	}
	if err := service.ResolveConfigurationFile(t.Context(), adminpanel.ConfigFileResolutionRequest{
		TargetID: "workspace", RepositoryID: "repo", Side: "file", ReviewToken: strings.Repeat("a", 64), ActorAccountID: "owner",
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("resolution bypassed exclusion: %v", err)
	}
	remote.mu.Lock()
	defer remote.mu.Unlock()
	if remote.calls != 0 {
		t.Fatalf("blocked operation contacted GitHub %d times", remote.calls)
	}
}

func TestConfigFilePanelReviewDoesNotContactGitHubForDisabledOrForeignScopes(t *testing.T) {
	service, remote := configFilePanelHarness(t)
	_, err := service.store.SaveInstallationSettings(t.Context(), storage.SaveInstallationSettingsRequest{
		TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC(),
		Repositories: []storage.InstallationRepositorySettingsChange{{RepositoryID: "repo", ExpectedRevision: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, repo := range []string{"", "repo"} {
		preview, err := service.PreviewConfigurationFile(t.Context(), "workspace", repo)
		if err != nil || preview.Status != configsync.StatusOff {
			t.Fatalf("disabled scope %q = %+v (%v)", repo, preview, err)
		}
		err = service.ResolveConfigurationFile(t.Context(), adminpanel.ConfigFileResolutionRequest{
			TargetID: "workspace", RepositoryID: repo, ReviewToken: strings.Repeat("a", 64), Side: "file", ActorAccountID: "owner",
		})
		var blocked *configsync.BlockedError
		if !errors.As(err, &blocked) || blocked.Code != "sync_off" {
			t.Fatalf("disabled resolution = %v", err)
		}
	}
	if _, err := service.PreviewConfigurationFile(t.Context(), "workspace", "foreign"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("foreign repository preview = %v", err)
	}
	remote.mu.Lock()
	defer remote.mu.Unlock()
	if remote.calls != 0 {
		t.Fatalf("disabled/foreign scopes contacted GitHub %d times", remote.calls)
	}
}

func TestConfigFilePanelReviewDoesNotContactGitHubForUnavailableScopes(t *testing.T) {
	for _, scope := range []string{"workspace", "repository"} {
		t.Run(scope, func(t *testing.T) {
			service, remote := configFilePanelHarness(t)
			_, err := service.store.SaveInstallationSettings(t.Context(), storage.SaveInstallationSettingsRequest{
				TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC(),
				Target: &storage.InstallationTargetSettingsChange{ConfigFileSyncEnabled: true, ExpectedRevision: 1},
			})
			if err != nil {
				t.Fatal(err)
			}
			var snapshots []storage.InstallationSnapshot
			if scope == "repository" {
				snapshots = []storage.InstallationSnapshot{{
					TargetID: "workspace", InstallationID: "100", Kind: storage.TargetOrganization, SyncedAt: time.Now().UTC(),
					Account: storage.Account{ID: "owner", Provider: "github", SubjectID: "1", Login: "acme", UpdatedAt: time.Now().UTC()},
				}}
			}
			if err := service.store.ReconcileCatalog(t.Context(), snapshots); err != nil {
				t.Fatal(err)
			}
			repositories := []string{"repo"}
			if scope == "workspace" {
				repositories = append(repositories, "")
			}
			for _, repo := range repositories {
				assertConfigFileUnavailableReview(t, service, repo)
			}
			remote.mu.Lock()
			defer remote.mu.Unlock()
			if remote.calls != 0 {
				t.Fatalf("unavailable scope contacted GitHub %d times", remote.calls)
			}
		})
	}
}

func assertConfigFileUnavailableReview(t *testing.T, service *server, repositoryID string) {
	t.Helper()
	preview, err := service.PreviewConfigurationFile(t.Context(), "workspace", repositoryID)
	if err != nil || preview.Status != configsync.StatusOff {
		t.Fatalf("unavailable scope %q = %+v (%v)", repositoryID, preview, err)
	}
	err = service.ResolveConfigurationFile(t.Context(), adminpanel.ConfigFileResolutionRequest{
		TargetID: "workspace", RepositoryID: repositoryID, ReviewToken: strings.Repeat("a", 64), Side: "file", ActorAccountID: "owner",
	})
	var blocked *configsync.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != "sync_off" {
		t.Fatalf("unavailable resolution = %v", err)
	}
}

type configFilePanelRemote struct {
	mu    sync.Mutex
	head  string
	calls int
}

func (remote *configFilePanelRemote) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remote.mu.Lock()
		defer remote.mu.Unlock()
		remote.calls++
		w.Header().Set("Content-Type", "application/json")
		content := "command_prefix='/file '\nallow_self_approval=true\n"
		blob := orgsync.BlobID([]byte(content))
		var body any
		switch r.Method + " " + r.URL.Path {
		case "POST /app/installations/100/access_tokens":
			body = map[string]any{"token": "installation-token", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
		case "GET /repos/acme/web/git/ref/heads/main":
			body = map[string]any{"object": map[string]string{"sha": remote.head}}
		case "GET /repos/acme/web/git/trees/" + remote.head:
			body = map[string]any{"tree": []any{map[string]any{"path": ".smyklot.toml", "mode": "100644", "type": "blob", "sha": blob, "size": len(content)}}}
		case "GET /repos/acme/web/contents/.smyklot.toml":
			if r.URL.Query().Get("ref") != remote.head {
				t.Error("file read did not use the observed immutable commit")
			}
			body = map[string]any{"type": "file", "encoding": "base64", "sha": blob, "size": len(content), "content": base64.StdEncoding.EncodeToString([]byte(content))}
		default:
			t.Errorf("unexpected GitHub write or read: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	})
}

func configFilePanelHarness(t *testing.T) (*server, *configFilePanelRemote) {
	t.Helper()
	remote := &configFilePanelRemote{head: strings.Repeat("a", 40)}
	endpoint := httptest.NewServer(remote.handler(t))
	t.Cleanup(endpoint.Close)
	service, err := newServer(&serveConfig{
		database: t.TempDir() + "/panel.sqlite3", webhookPath: defaultWebhookPath, webhookSecret: []byte(testSecret),
		apiBaseURL: endpoint.URL, appClientID: "Iv1.test", appPrivateKey: githubtest.AppPrivateKey(),
		botConfig: config.Default(), logWriter: io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Error(err)
		}
	})
	now := time.Now().UTC()
	owner := storage.Account{ID: "owner", Provider: "github", SubjectID: "1", Login: "acme", UpdatedAt: now}
	err = service.store.ReconcileInstallation(t.Context(), storage.InstallationSnapshot{
		TargetID: "workspace", InstallationID: "100", Kind: storage.TargetOrganization, Account: owner, SyncedAt: now,
		Ownership:    storage.OwnershipSnapshot{Source: storage.OwnershipSourceOrganizationAdmin, Status: storage.OwnershipStatusFresh, Owners: []storage.Account{owner}, SyncedAt: now},
		Repositories: []storage.RepositorySnapshot{{ID: "repo", Name: "web", FullName: "acme/web", DefaultBranch: "main"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.store.SaveInstallationSettings(t.Context(), storage.SaveInstallationSettingsRequest{
		TargetID: "workspace", ActorAccountID: "owner", ChangedAt: now,
		Repositories: []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "repo", ConfigFileSyncEnabled: true, ExpectedRevision: 1,
			ConfigPatch: config.Patch{CommandPrefix: new("/panel "), QuietSuccess: new(false)},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, remote
}
