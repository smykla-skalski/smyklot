package configsync

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func engineStore(t *testing.T) storage.Store {
	t.Helper()
	ctx, now := context.Background(), time.Now().UTC()
	store, err := open.Store(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	account := storage.Account{ID: "owner", Provider: "github", SubjectID: "1", Login: "acme", DisplayName: "Acme", UpdatedAt: now}
	err = store.ReconcileInstallation(ctx, storage.InstallationSnapshot{
		TargetID: "workspace", InstallationID: "100", Kind: storage.TargetOrganization,
		Account: account, SyncedAt: now, Permissions: map[string]string{"contents": "write", "pull_requests": "write"},
		Ownership: storage.OwnershipSnapshot{
			Source: storage.OwnershipSourceOrganizationAdmin, Status: storage.OwnershipStatusFresh,
			Owners: []storage.Account{account}, SyncedAt: now,
		},
		Repositories: []storage.RepositorySnapshot{
			{ID: "repo", Name: "web", FullName: "acme/web", DefaultBranch: "main"},
			{ID: "config-repo", Name: ".github", FullName: "acme/.github", DefaultBranch: "main"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
		TargetID: "workspace", ActorAccountID: account.ID, ChangedAt: now,
		Target:       &storage.InstallationTargetSettingsChange{ConfigFileSyncEnabled: true, ExpectedRevision: 1},
		Repositories: []storage.InstallationRepositorySettingsChange{{RepositoryID: "repo", ConfigFileSyncEnabled: true, ExpectedRevision: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func engineFileCalls(path, content string) []remoteCall {
	entries, _ := json.Marshal([]any{remoteEntry(path, content)})
	return append(remoteReadCalls(string(entries)), contentCall(path, content))
}

func storeConnection(t *testing.T, engine Engine, repositoryID string, connection Connection) {
	t.Helper()
	ctx := context.Background()
	snapshot, err := engine.Snapshot(ctx, "workspace", repositoryID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := engine.Store.GetConfigFileState(ctx, "workspace", repositoryID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = engine.save(ctx, snapshot, stored, connection)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEngineImportsAFileThenRereadsBeforeAdvancingBaseline(t *testing.T) {
	store := engineStore(t)
	engine := Engine{Store: store}
	content := "command_prefix='/file '\n[panel]\nversion=1\nscope='repository'\n[panel.settings]\nenabled=false\n"
	calls := append(engineFileCalls(".smyklot.toml", content), engineFileCalls(".smyklot.toml", content)...)
	connection, err := engine.Run(context.Background(), scriptedRemote(t, calls...), "workspace", "repo")
	if err != nil || connection.Status != "ready" || !connection.Base.Exists {
		t.Fatalf("file import = %+v (%v)", connection, err)
	}
	repository, err := store.GetRepository(context.Background(), "workspace", "repo")
	if err != nil || repository.Revision != 3 || repository.ConfigPatch.CommandPrefix == nil ||
		*repository.ConfigPatch.CommandPrefix != "/file " || repository.EnabledOverride == nil || *repository.EnabledOverride {
		t.Fatalf("imported repository = %+v (%v)", repository, err)
	}
	audit, err := store.ListAudit(context.Background(), "workspace", storage.AuditPageRequest{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range audit.Items {
		if item.Action == "configuration_file.imported" && strings.Contains(item.Summary, ".smyklot.toml at "+remoteHead) {
			found = true
		}
	}
	if !found {
		t.Fatal("import lost its immutable source provenance")
	}
}

func TestEngineSurfacesConflictsWithoutImportOrPublication(t *testing.T) {
	store := engineStore(t)
	ctx, engine := context.Background(), Engine{Store: store}
	_, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
		TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC(),
		Repositories: []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "repo", ExpectedRevision: 2, ConfigFileSyncEnabled: true,
			ConfigPatch: config.Patch{CommandPrefix: new("/panel ")},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := engineFileCalls(".smyklot.toml", "command_prefix='/file '\n")
	connection, err := engine.Run(ctx, scriptedRemote(t, calls...), "workspace", "repo")
	if err != nil || connection.Problem != "conflicting_edits" || connection.ConflictCount != 1 || connection.Base.Exists {
		t.Fatalf("conflict = %+v (%v)", connection, err)
	}
	connection.Resolution = &ResolutionChoice{Side: "file", Comparison: connection.Comparison}
	storeConnection(t, engine, "repo", connection)
	calls = append(engineFileCalls(".smyklot.toml", "command_prefix='/file '\n"), engineFileCalls(".smyklot.toml", "command_prefix='/file '\n")...)
	connection, err = engine.Run(ctx, scriptedRemote(t, calls...), "workspace", "repo")
	if err != nil || connection.Status != "ready" || connection.Resolution != nil {
		t.Fatalf("resolved import did not converge = %+v (%v)", connection, err)
	}
}

func TestEngineRejectsAResolutionAfterRemoteSettingsChange(t *testing.T) {
	engine := conflictPreviewEngine(t)
	snapshot, err := engine.Snapshot(context.Background(), "workspace", "repo")
	if err != nil {
		t.Fatal(err)
	}
	panel, _ := snapshot.JSON()
	file, err := ReadFileSource(config.FormatTOML, []byte("command_prefix='/old '\n"), config.PanelFileRepository)
	if err != nil {
		t.Fatal(err)
	}
	comparison, _ := comparisonKey(ReconcileInput{Panel: panel, File: file.Snapshot})
	connection := Connection{Version: 1, Resolution: &ResolutionChoice{Side: "file", Comparison: comparison}}
	storeConnection(t, engine, "repo", connection)
	connection, err = engine.Run(context.Background(), scriptedRemote(t, engineFileCalls(".smyklot.toml", "command_prefix='/new '\n")...), "workspace", "repo")
	if err != nil || connection.Problem != "stale_resolution" || connection.Base.Exists {
		t.Fatalf("stale choice = %+v (%v)", connection, err)
	}
}

func TestEngineDoesNotTreatFileDeletionAsEmptySettings(t *testing.T) {
	engine := Engine{Store: engineStore(t)}
	snapshot, _ := engine.Snapshot(context.Background(), "workspace", "repo")
	panel, _ := snapshot.JSON()
	storeConnection(t, engine, "repo", Connection{Version: 1, Base: Snapshot{Exists: true, Document: panel}})
	connection, err := engine.Run(context.Background(), scriptedRemote(t, remoteReadCalls(`[]`)...), "workspace", "repo")
	if err != nil || connection.Problem != "file_removed" || !connection.Base.Exists {
		t.Fatalf("removed file = %+v (%v)", connection, err)
	}
}

func TestEngineCannotPublishAfterConcurrentOptOut(t *testing.T) {
	store := engineStore(t)
	engine := Engine{Store: &optOutConnectionStore{ConnectionStore: store}}
	calls := append(remoteReadCalls(`[]`), prepareEngineCalls()...)
	_, err := engine.Run(context.Background(), scriptedRemote(t, calls...), "workspace", "repo")
	if !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("publication continued after its durable intent was rejected: %v", err)
	}
}

type optOutConnectionStore struct{ ConnectionStore }

func (store *optOutConnectionStore) SaveConfigFileState(ctx context.Context, change storage.ConfigFileStateChange) (storage.ConfigFileState, error) {
	_, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
		TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC(),
		Repositories: []storage.InstallationRepositorySettingsChange{{RepositoryID: "repo", ExpectedRevision: change.OwnerRevision}},
	})
	if err != nil {
		return storage.ConfigFileState{}, err
	}
	return store.ConnectionStore.SaveConfigFileState(ctx, change)
}

func prepareEngineCalls() []remoteCall {
	return []remoteCall{
		{method: "GET", path: proposalRefPath(), status: 404, answer: `{}`},
		{method: "GET", path: "/repos/acme/web/pulls", answer: `[]`},
		{method: "GET", path: "/repos/acme/web/git/commits/" + remoteHead, answer: `{"tree":{"sha":"` + strings.Repeat("b", 40) + `"}}`},
		{method: "POST", path: "/repos/acme/web/git/blobs", answer: `{"sha":"` + strings.Repeat("c", 40) + `"}`},
		{method: "POST", path: "/repos/acme/web/git/trees", answer: `{"sha":"` + strings.Repeat("d", 40) + `"}`},
		{method: "POST", path: "/repos/acme/web/git/commits", answer: `{"sha":"` + strings.Repeat("e", 40) + `"}`},
	}
}
