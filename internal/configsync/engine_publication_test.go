package configsync

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestEngineResumesDurableProposalAfterFinalStateWriteFails(t *testing.T) {
	store := engineStore(t)
	uncertain := &uncertainConnectionStore{ConnectionStore: store}
	engine, ctx := Engine{Store: uncertain}, context.Background()
	calls := append(engineReadCalls(`[]`), prepareEngineCalls()...)
	calls = append(calls, enginePublishCalls(false)...)
	_, err := engine.Run(ctx, scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if !errors.Is(err, errFinalStateWrite) {
		t.Fatalf("missing final state failure: %v", err)
	}
	stored, err := store.GetConfigFileState(ctx, "workspace", "github:repository:11")
	if err != nil {
		t.Fatal(err)
	}
	connection, err := DecodeConnection(stored, config.PanelFileRepository)
	if err != nil || connection.Status != "pending" || connection.Proposal == nil || connection.Base.Exists {
		t.Fatalf("durable proposal intent = %+v (%v)", connection, err)
	}
	// The retry must neither create git objects nor move refs or open another PR.
	calls = append(engineReadCalls(`[]`), enginePublishCalls(true)...)
	connection, err = engine.Run(ctx, scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if err != nil || connection.Status != "proposed" || connection.Proposal.Number != 42 || connection.Base.Exists {
		t.Fatalf("proposal retry = %+v (%v)", connection, err)
	}
	if connection.Path != ".smyklot.toml" {
		t.Errorf("new-file proposal did not expose its destination path: %q", connection.Path)
	}
	// Opening a PR is not agreement. Only a fresh observation on the default
	// branch can advance the baseline and mark this connection synchronized.
	snapshot, _ := engine.Snapshot(ctx, "workspace", "github:repository:11")
	document, _ := snapshot.Document()
	content, err := config.RenderFileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	mergedCalls := append(engineFileCalls(".smyklot.toml", string(content)), mergedProposalCalls()...)
	connection, err = engine.Run(ctx, scriptedRemote(t, mergedCalls...), "workspace", "github:repository:11")
	if err != nil || connection.Status != "ready" || !connection.Base.Exists {
		t.Fatalf("merged proposal did not converge: %+v (%v)", connection, err)
	}
}

var errFinalStateWrite = errors.New("test final state write failed")

type uncertainConnectionStore struct {
	ConnectionStore
	writes int
}

func (store *uncertainConnectionStore) SaveConfigFileState(ctx context.Context, change storage.ConfigFileStateChange) (storage.ConfigFileState, error) {
	store.writes++
	if store.writes == 2 {
		return storage.ConfigFileState{}, errFinalStateWrite
	}
	return store.ConnectionStore.SaveConfigFileState(ctx, change)
}

func enginePublishCalls(alreadyPublished bool) []remoteCall {
	calls := []remoteCall{repositoryIdentityCall(), {method: "GET", path: "/repos/acme/web/git/ref/heads/main", answer: `{"object":{"sha":"` + remoteHead + `"}}`}}
	pull := `{"number":42,"state":"open","html_url":"https://github.com/acme/web/pull/42"}`
	if alreadyPublished {
		return append(calls,
			remoteCall{method: "GET", path: proposalRefPath(), answer: `{"object":{"sha":"` + strings.Repeat("e", 40) + `"}}`},
			remoteCall{method: "GET", path: "/repos/acme/web/pulls", answer: `[` + pull + `]`})
	}
	return append(calls,
		remoteCall{method: "GET", path: proposalRefPath(), status: 404, answer: `{}`},
		remoteCall{method: "GET", path: "/repos/acme/web/pulls", answer: `[]`},
		remoteCall{method: "POST", path: "/repos/acme/web/git/refs", answer: `{}`},
		remoteCall{method: "POST", path: "/repos/acme/web/pulls", answer: pull})
}

func TestEngineRecordsIntentBeforeCreatingProposalReference(t *testing.T) {
	store := engineStore(t)
	engine := Engine{Store: store}
	calls := append(engineReadCalls(`[]`), prepareEngineCalls()...)
	publish := enginePublishCalls(false)
	publish[3].check = func(t *testing.T, _ *http.Request) {
		stored, err := store.GetConfigFileState(context.Background(), "workspace", "github:repository:11")
		if err != nil {
			t.Fatal(err)
		}
		connection, err := DecodeConnection(stored, config.PanelFileRepository)
		if err != nil || connection.Status != "pending" || connection.Proposal == nil || connection.Proposal.Commit != strings.Repeat("e", 40) {
			t.Errorf("branch was published before its intent was recorded: %+v (%v)", connection, err)
		}
	}
	calls = append(calls, publish...)
	if _, err := engine.Run(context.Background(), scriptedRemote(t, calls...), "workspace", "github:repository:11"); err != nil {
		t.Fatal(err)
	}
}

func TestEngineSettlesAPanelChoiceAfterItsProposalMerges(t *testing.T) {
	engine, ctx := Engine{Store: engineStore(t)}, context.Background()
	_, err := engine.Store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
		TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC(),
		Repositories: []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "github:repository:11", ExpectedRevision: 2, ConfigFileSyncEnabled: true,
			ConfigPatch: config.Patch{CommandPrefix: new("/panel ")},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := engine.Snapshot(ctx, "workspace", "github:repository:11")
	panel, _ := snapshot.JSON()
	file, err := ReadFileSource(config.FormatTOML, []byte("command_prefix='/file '\n"), config.PanelFileRepository)
	if err != nil {
		t.Fatal(err)
	}
	comparison, _ := comparisonKey(ReconcileInput{Panel: panel, File: file.Snapshot})
	storeConnection(t, engine, "github:repository:11", Connection{Version: 1, Resolution: &ResolutionChoice{Side: "panel", Comparison: comparison}})
	calls := append(engineFileCalls(".smyklot.toml", "command_prefix='/file '\n"), prepareEngineCalls()...)
	calls = append(calls, enginePublishCalls(false)...)
	connection, err := engine.Run(ctx, scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if err != nil || connection.Status != StatusProposed || connection.Base.Exists {
		t.Fatalf("resolved proposal = %+v (%v)", connection, err)
	}
	document, _ := snapshot.Document()
	content, err := config.RenderFileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	mergedCalls := append(engineFileCalls(".smyklot.toml", string(content)), mergedProposalCalls()...)
	connection, err = engine.Run(ctx, scriptedRemote(t, mergedCalls...), "workspace", "github:repository:11")
	if err != nil || connection.Status != StatusReady || !connection.Base.Exists || connection.Resolution != nil {
		t.Fatalf("merged choice was incorrectly reopened as a conflict: %+v (%v)", connection, err)
	}
}

func TestEnginePublishesLegacyMigrationEvenWhenSettingsAlreadyAgree(t *testing.T) {
	store := engineStore(t)
	engine := Engine{Store: store}
	calls := append(engineFileCalls(".github/smyklot.yaml", "{}\n"), prepareEngineCalls()...)
	calls = append(calls, enginePublishCalls(false)...)
	connection, err := engine.Run(context.Background(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if err != nil || connection.Status != "proposed" || connection.Base.Exists {
		t.Fatalf("legacy configuration was not proposed for migration: %+v (%v)", connection, err)
	}
}
