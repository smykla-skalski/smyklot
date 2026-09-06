package configsync

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestConnectionStatusInvalidatesEverySavedSettingsSource(t *testing.T) {
	for _, test := range []struct {
		repositoryID string
		syncOnly     bool
	}{{"", false}, {"", true}, {"github:repository:11", false}, {"github:repository:11", true}} {
		repositoryID, syncOnly := test.repositoryID, test.syncOnly
		t.Run(repositoryID+"/"+map[bool]string{false: "settings", true: "sync"}[syncOnly], func(t *testing.T) {
			engine := Engine{Store: engineStore(t)}
			snapshot, err := engine.Snapshot(t.Context(), "workspace", repositoryID)
			if err != nil {
				t.Fatal(err)
			}
			panel, err := snapshot.JSON()
			if err != nil {
				t.Fatal(err)
			}
			storeConnection(t, engine, repositoryID, Connection{Version: 1, Status: StatusReady, Base: present(string(panel))})
			before, err := engine.ReadStatus(t.Context(), "workspace", repositoryID)
			if err != nil || before.Status != StatusReady || before.LastCheck == nil || !before.LastCheck.SettingsCurrent {
				t.Fatalf("checked settings = %+v (%v)", before, err)
			}
			saveStatusSettingsChange(t, engine, snapshot, syncOnly)
			after, err := engine.ReadStatus(t.Context(), "workspace", repositoryID)
			if err != nil || after.Status != StatusPending || after.LastCheck == nil || after.LastCheck.SettingsCurrent ||
				after.LastCheck.Status != StatusReady || !after.LastCheck.CheckedAt.Equal(before.LastCheck.CheckedAt) {
				t.Fatalf("new save falsely claimed checked = %+v (%v)", after, err)
			}
		})
	}
}

func saveStatusSettingsChange(t *testing.T, engine Engine, snapshot PanelSnapshot, syncOnly bool) {
	t.Helper()
	request := storage.SaveInstallationSettingsRequest{TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC()}
	if syncOnly {
		if snapshot.Repository == nil {
			request.SyncConfigs = []storage.InstallationSyncConfigChange{{Kind: orgsync.KindLabels, Enabled: true, Document: []byte(`{"labels":[]}`)}}
		} else {
			request.SyncOverrides = []storage.InstallationSyncOverrideChange{{RepositoryID: "github:repository:11", Kind: orgsync.KindLabels, Enabled: new(false), Document: []byte(`{}`)}}
		}
	} else if snapshot.Repository == nil {
		request.Target = &storage.InstallationTargetSettingsChange{
			ConfigFileSyncEnabled: true, ExpectedRevision: snapshot.OwnerRevision(), ConfigPatch: config.Patch{CommandPrefix: new("/new ")},
		}
	} else {
		request.Repositories = []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "github:repository:11", ConfigFileSyncEnabled: true, ExpectedRevision: snapshot.OwnerRevision(), ConfigPatch: config.Patch{CommandPrefix: new("/new ")},
		}}
	}
	if _, err := engine.Store.SaveInstallationSettings(t.Context(), request); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionStatusKeepsBlockedObservationButHidesPrivateState(t *testing.T) {
	engine := Engine{Store: engineStore(t)}
	storeConnection(t, engine, "github:repository:11", Connection{
		Version: 1, Status: StatusBlocked, Problem: "conflicting_edits", Comparison: "private-comparison",
		ConflictCount: 1, ConflictPaths: [][]string{{"command_prefix"}},
		Resolution: &ResolutionChoice{Side: ResolutionPanel, Comparison: "private-comparison"},
		Proposal:   &Proposal{Number: 42, URL: "https://github.com/acme/web/pull/42", Commit: "private-commit", Digest: "private-digest"},
	})
	before, _ := engine.Store.GetConfigFileState(t.Context(), "workspace", "github:repository:11")
	answer, err := engine.ReadStatus(t.Context(), "workspace", "github:repository:11")
	if err != nil || answer.Status != StatusBlocked || answer.LastCheck == nil || answer.LastCheck.ConflictCount != 1 ||
		answer.LastCheck.Problem != "conflicting_edits" || answer.LastCheck.Proposal == nil || answer.LastCheck.Proposal.Number != 42 {
		t.Fatalf("blocked status = %+v (%v)", answer, err)
	}
	encoded, _ := json.Marshal(answer)
	for _, hidden := range []string{"private-", "\"base\"", "\"inputs\"", "\"resolution\"", "\"comparison\""} {
		if strings.Contains(string(encoded), hidden) {
			t.Fatalf("status exposed private state: %s", encoded)
		}
	}
	after, _ := engine.Store.GetConfigFileState(t.Context(), "workspace", "github:repository:11")
	if before.Revision != after.Revision || string(before.Document) != string(after.Document) {
		t.Fatal("reading status mutated the connection")
	}
}

func TestConnectionStatusRequiresFreshCheckForLegacyOrReactivatedState(t *testing.T) {
	engine := Engine{Store: engineStore(t)}
	snapshot, _ := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
	panel, _ := snapshot.JSON()
	legacy, _ := json.Marshal(Connection{Version: 1, Status: StatusReady, Base: present(string(panel))})
	_, err := engine.Store.SaveConfigFileState(t.Context(), storage.ConfigFileStateChange{
		TargetID: "workspace", RepositoryID: "github:repository:11", OwnerRevision: snapshot.OwnerRevision(), SyncRevisions: snapshot.SyncRevisions(),
		Document: legacy, ChangedAt: time.Now().UTC(), Initialized: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := engine.ReadStatus(t.Context(), "workspace", "github:repository:11")
	if err != nil || answer.Status != StatusPending || answer.LastCheck == nil || answer.LastCheck.SettingsCurrent {
		t.Fatalf("legacy observation claimed current = %+v (%v)", answer, err)
	}
	for _, enabled := range []bool{false, true} {
		snapshot, _ = engine.Snapshot(t.Context(), "workspace", "github:repository:11")
		_, err = engine.Store.SaveInstallationSettings(t.Context(), storage.SaveInstallationSettingsRequest{
			TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC(),
			Repositories: []storage.InstallationRepositorySettingsChange{{RepositoryID: "github:repository:11", ConfigFileSyncEnabled: enabled, ExpectedRevision: snapshot.OwnerRevision()}},
		})
		if err != nil {
			t.Fatal(err)
		}
		answer, err = engine.ReadStatus(t.Context(), "workspace", "github:repository:11")
		want := StatusOff
		if enabled {
			want = StatusPending
		}
		if err != nil || answer.Enabled != enabled || answer.Status != want {
			t.Fatalf("activation state = %+v (%v)", answer, err)
		}
	}
}
