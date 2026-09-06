package configsync

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestEngineWorkspaceImportsUseTheSeparateOrganizationFile(t *testing.T) {
	store := engineStore(t)
	engine, ctx := Engine{Store: store}, context.Background()
	snapshot, err := engine.Snapshot(ctx, "workspace", "")
	if err != nil {
		t.Fatal(err)
	}
	document, err := snapshot.Document()
	if err != nil {
		t.Fatal(err)
	}
	document.CommandPrefix = new("/organization ")
	content, err := config.RenderFileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	read := engineFileCalls(WorkspaceFilePath, string(content))
	for index := range read {
		read[index].path = strings.Replace(read[index].path, "/repos/acme/web/", "/repos/acme/.github/", 1)
		read[index] = workspaceIdentityCall(read[index])
	}
	calls := slices.Concat(read, read)
	connection, err := engine.Run(ctx, scriptedRemote(t, calls...), "workspace", "")
	if err != nil || connection.Status != StatusReady || connection.Path != WorkspaceFilePath {
		t.Fatalf("workspace import = %+v (%v)", connection, err)
	}
	target, _ := store.GetTarget(ctx, "workspace")
	repository, _ := store.GetRepository(ctx, "workspace", "github:repository:11")
	if target.ConfigPatch.CommandPrefix == nil || *target.ConfigPatch.CommandPrefix != "/organization " || repository.ConfigPatch.CommandPrefix != nil {
		t.Fatal("workspace import did not remain in its own settings scope")
	}
}

func TestEngineCanImportWithReadAccessButCannotPublishWithoutWriteAccess(t *testing.T) {
	for _, missingFile := range []bool{false, true} {
		t.Run(map[bool]string{false: "import", true: "publication"}[missingFile], func(t *testing.T) {
			engine := Engine{Store: readOnlyInstallationStore{engineStore(t)}}
			var calls []remoteCall
			if missingFile {
				calls = engineReadCalls(`[]`)
			} else {
				read := engineFileCalls(".smyklot.toml", "command_prefix='/file '\n")
				calls = slices.Concat(read, read)
			}
			connection, err := engine.Run(context.Background(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
			if err != nil {
				t.Fatal(err)
			}
			if missingFile && connection.Problem != "write_permission_missing" {
				t.Fatalf("permission block = %+v", connection)
			}
			if !missingFile && connection.Status != StatusReady {
				t.Fatalf("read-only installation did not import: %+v", connection)
			}
		})
	}
}

type readOnlyInstallationStore struct{ ConnectionStore }

func (store readOnlyInstallationStore) GetTarget(ctx context.Context, id string) (storage.Target, error) {
	target, err := store.ConnectionStore.GetTarget(ctx, id)
	target.Permissions = map[string]string{"contents": "read", "pull_requests": "read"}
	return target, err
}

func TestEngineOffAndBypassedStatesDoNotReadGitHub(t *testing.T) {
	for _, bypassed := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "bypassed"}[bypassed], func(t *testing.T) {
			store := engineStore(t)
			_, err := store.SaveInstallationSettings(context.Background(), storage.SaveInstallationSettingsRequest{
				TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC(),
				Repositories: []storage.InstallationRepositorySettingsChange{{
					RepositoryID: "github:repository:11", ExpectedRevision: 2, ConfigFileSyncEnabled: bypassed, IgnoreRepositoryFile: bypassed,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			connection, err := (Engine{Store: store}).Run(context.Background(), scriptedRemote(t), "workspace", "github:repository:11")
			if err != nil || (bypassed && connection.Problem != "file_disabled") || (!bypassed && connection.Status != StatusOff) {
				t.Fatalf("off/bypassed = %+v (%v)", connection, err)
			}
		})
	}
}

func TestEngineInvalidatesFileChoiceWhenTheFileIsRemoved(t *testing.T) {
	engine, ctx := Engine{Store: engineStore(t)}, context.Background()
	snapshot, _ := engine.Snapshot(ctx, "workspace", "github:repository:11")
	panel, _ := snapshot.JSON()
	file, err := ReadFileSource(config.FormatTOML, []byte("command_prefix='/before '\n"), config.PanelFileRepository)
	if err != nil {
		t.Fatal(err)
	}
	comparison, _ := comparisonKey(ReconcileInput{Panel: panel, File: file.Snapshot})
	storeConnection(t, engine, "github:repository:11", Connection{Version: 1, Resolution: &ResolutionChoice{Comparison: comparison, Side: "file"}})
	connection, err := engine.Run(ctx, scriptedRemote(t, engineReadCalls(`[]`)...), "workspace", "github:repository:11")
	if err != nil || connection.Problem != "stale_resolution" {
		t.Fatalf("deleted file left its old choice as a retrying failure: %+v (%v)", connection, err)
	}
}

func TestEngineInvalidDomainValuesReplaceReadyWithAnActionableProblem(t *testing.T) {
	for _, example := range []struct{ content, problem string }{
		{content: "command_prefix=''\n", problem: "invalid_settings"},
		{content: "allowed_commands=['unknown']\n", problem: "invalid_settings"},
		{content: "command_aliases={ship='unknown'}\n", problem: "invalid_settings"},
		{content: "[panel]\nversion=1\nscope='repository'\n[panel.settings]\nfile_index_interval='100ms'\n", problem: "invalid_file"},
		{content: "[panel]\nversion=1\nscope='repository'\n[panel.settings]\nfile_index_interval='192h'\n", problem: "invalid_settings"},
		{content: "[panel]\nversion=1\nscope='repository'\n[panel.sync.files]\ndocument='{\"merges\":[{\"path\":\"missing.json\",\"overrides\":{\"a\":1}}]}'\n", problem: "invalid_settings"},
	} {
		t.Run(example.content, func(t *testing.T) {
			store := engineStore(t)
			engine, ctx := Engine{Store: store}, context.Background()
			snapshot, _ := engine.Snapshot(ctx, "workspace", "github:repository:11")
			baseline, _ := snapshot.JSON()
			storeConnection(t, engine, "github:repository:11", Connection{Version: 1, Status: StatusReady, Base: Snapshot{Exists: true, Document: baseline}})
			calls := engineFileCalls(".smyklot.toml", example.content)
			if example.problem == "invalid_file" {
				calls = calls[:len(calls)-1]
			}
			connection, err := engine.Run(ctx, scriptedRemote(t, calls...), "workspace", "github:repository:11")
			if err != nil || connection.Status != StatusBlocked || connection.Problem != example.problem || connection.Message == "" {
				t.Fatalf("invalid settings left a misleading state: %+v (%v)", connection, err)
			}
			after, _ := engine.Snapshot(ctx, "workspace", "github:repository:11")
			actual, _ := after.JSON()
			if !bytes.Equal(baseline, actual) || !bytes.Equal(connection.Base.Document, baseline) {
				t.Fatal("invalid import changed settings or advanced its baseline")
			}
			stored, _ := store.GetConfigFileState(ctx, "workspace", "github:repository:11")
			restored, err := DecodeConnection(stored, config.PanelFileRepository)
			if err != nil || restored.Status != StatusBlocked {
				t.Fatalf("failure was not persisted: %+v (%v)", restored, err)
			}
		})
	}
}
