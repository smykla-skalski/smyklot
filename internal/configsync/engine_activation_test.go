package configsync

import (
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestEngineReconnectRequiresAFreshReconciledBaseline(t *testing.T) {
	store := engineStore(t)
	engine, ctx := Engine{Store: store}, t.Context()
	snapshot, err := engine.Snapshot(ctx, "workspace", "github:repository:11")
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := snapshot.JSON()
	if err != nil {
		t.Fatal(err)
	}
	storeConnection(t, engine, "github:repository:11", Connection{
		Version: 1, Status: StatusReady, Base: Snapshot{Exists: true, Document: baseline},
	})
	for index, enabled := range []bool{false, true} {
		_, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC(),
			Repositories: []storage.InstallationRepositorySettingsChange{{
				RepositoryID: "github:repository:11", ConfigFileSyncEnabled: enabled, ExpectedRevision: int64(index + 2),
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	invalidCalls := engineFileCalls(".smyklot.toml", "allowed_commands = [")
	blocked, err := engine.Run(ctx, scriptedRemote(t, invalidCalls[:len(invalidCalls)-1]...), "workspace", "github:repository:11")
	if err != nil || blocked.Status != StatusBlocked {
		t.Fatalf("malformed reconnect = %+v (%v)", blocked, err)
	}
	stored, err := store.GetConfigFileState(ctx, "workspace", "github:repository:11")
	if err != nil || !stored.InitializationRequired {
		t.Fatalf("failed reconnect activated old baseline: %+v (%v)", stored, err)
	}
	content := "allowed_commands = ['approve']\n"
	calls := append(engineFileCalls(".smyklot.toml", content), engineFileCalls(".smyklot.toml", content)...)
	ready, err := engine.Run(ctx, scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if err != nil || ready.Status != StatusReady {
		t.Fatalf("corrected reconnect = %+v (%v)", ready, err)
	}
	stored, err = store.GetConfigFileState(ctx, "workspace", "github:repository:11")
	if err != nil || stored.InitializationRequired {
		t.Fatalf("reconciled reconnect still pending: %+v (%v)", stored, err)
	}
	repository, err := store.GetRepository(ctx, "workspace", "github:repository:11")
	allowed := repository.ConfigPatch.AllowedCommands
	if err != nil || allowed == nil || len(*allowed) != 1 || (*allowed)[0] != "approve" {
		t.Fatalf("activation lost file restrictions: %+v (%v)", repository.ConfigPatch, err)
	}
}
