package panel

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/configsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestConfigFileStatusRoutesShareScopeAuthorization(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	for _, prefix := range []string{"/panel/api/v1/targets/", "/panel/api/v1/root/workspaces/"} {
		for _, suffix := range []string{"/config-file", "/repositories/repository-20/config-file"} {
			path := prefix + "github:installation:10" + suffix
			requireResponse(t, harness.request(t, http.MethodGet, path, nil, nil), "anonymous file status", http.StatusUnauthorized)
			answer := harness.request(t, http.MethodGet, path, nil, session)
			requireResponse(t, answer, "disabled file status", http.StatusOK, `"enabled":false`, `"available":true`, `"status":"off"`)
		}
		wrongRepository := prefix + "github:installation:10/repositories/another-repository/config-file"
		requireResponse(t, harness.request(t, http.MethodGet, wrongRepository, nil, session), "foreign repository", http.StatusNotFound)
		missing := prefix + "another-workspace/config-file"
		requireResponse(t, harness.request(t, http.MethodGet, missing, nil, session), "foreign workspace", http.StatusNotFound)
	}
}

func TestConfigFileStatusServesCachedObservationWithoutPrivateDocuments(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	const targetID = "github:installation:10"
	_, err := harness.store.SaveInstallationSettings(t.Context(), storage.SaveInstallationSettingsRequest{
		TargetID: targetID, ActorAccountID: "github:test:user:1", ChangedAt: harness.now,
		Repositories: []storage.InstallationRepositorySettingsChange{{RepositoryID: "repository-20", ConfigFileSyncEnabled: true, ExpectedRevision: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := configsync.Engine{Store: harness.store}
	snapshot, err := engine.Snapshot(t.Context(), targetID, "repository-20")
	if err != nil {
		t.Fatal(err)
	}
	panel, err := snapshot.JSON()
	if err != nil {
		t.Fatal(err)
	}
	document, err := json.Marshal(configsync.Connection{
		Version: 1, Status: configsync.StatusReady, Path: ".smyklot.toml", Comparison: "private-comparison",
		Base:   configsync.Snapshot{Exists: true, Document: panel},
		Inputs: &configsync.InputRevisions{Owner: snapshot.OwnerRevision(), Sync: snapshot.SyncRevisions()},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = harness.store.SaveConfigFileState(t.Context(), storage.ConfigFileStateChange{
		TargetID: targetID, RepositoryID: "repository-20", OwnerRevision: snapshot.OwnerRevision(), SyncRevisions: snapshot.SyncRevisions(),
		Document: document, ChangedAt: harness.now, Initialized: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := harness.request(t, http.MethodGet, "/panel/api/v1/targets/"+targetID+"/repositories/repository-20/config-file", nil, session)
	requireResponse(t, response, "checked file status", http.StatusOK, `"settings_current":true`, `"path":".smyklot.toml"`)
	var answer configsync.ConnectionStatus
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil || answer.LastCheck == nil ||
		answer.Status != configsync.StatusReady || !answer.LastCheck.CheckedAt.Equal(harness.now) {
		t.Fatalf("file status = %+v (%v)", answer, err)
	}
	var wire map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	assertWireNames(t, "configuration file status", wire)
	for _, key := range []string{"base", "comparison", "inputs", "resolution"} {
		if _, exposed := wire["last_check"].(map[string]any)[key]; exposed {
			t.Fatalf("status leaked %s", key)
		}
	}
}
