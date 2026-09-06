package configsync

import (
	"errors"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestEngineFollowsLiveRepositoryIdentityAndDefaultBranch(t *testing.T) {
	engine := Engine{Store: engineStore(t)}
	snapshot, err := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
	if err != nil {
		t.Fatal(err)
	}
	document, err := snapshot.Document()
	if err != nil {
		t.Fatal(err)
	}
	content, err := config.RenderFileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	// The catalog still names acme/web on main. That name can now belong to
	// another repository, so no request may use it to resolve this connection.
	calls := []remoteCall{{method: "GET", path: "/repositories/11", answer: `{"id":11,"name":"renamed","full_name":"acme/renamed","owner":{"login":"acme"},"default_branch":"release"}`}}
	for _, call := range engineFileCalls(".smyklot.toml", string(content))[1:] {
		if call.path == "/repositories/11" {
			call.answer = calls[0].answer
		}
		call.path = strings.ReplaceAll(call.path, "/acme/web/", "/acme/renamed/")
		call.path = strings.ReplaceAll(call.path, "/heads/main", "/heads/release")
		calls = append(calls, call)
	}
	connection, err := engine.Run(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if err != nil || connection.Status != StatusReady {
		t.Fatalf("live repository sync = %+v (%v)", connection, err)
	}
}

func TestPublicationRejectsRepositoryChangesBeforeAnyWrite(t *testing.T) {
	for _, response := range []string{
		`{"id":11,"name":"renamed","full_name":"acme/renamed","owner":{"login":"acme"},"default_branch":"main"}`,
		`{"id":11,"name":"web","full_name":"acme/web","owner":{"login":"acme"},"default_branch":"release"}`,
	} {
		client := scriptedRemote(t, remoteCall{method: "GET", path: "/repositories/11", answer: response})
		_, err := PublishProposal(t.Context(), client, remoteLocation(), readyProposal())
		var blocked *BlockedError
		if !errors.As(err, &blocked) || blocked.Code != "source_changed" {
			t.Fatalf("stale location must stop publication: %v", err)
		}
	}
}

func TestPreviewRejectsDefaultBranchChangeDuringFileRead(t *testing.T) {
	engine := conflictPreviewEngine(t)
	calls := previewFileCalls(".smyklot.toml", previewFile)
	calls[len(calls)-1].answer = `{"id":11,"name":"web","full_name":"acme/web","owner":{"login":"acme"},"default_branch":"release"}`
	_, err := engine.Preview(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("preview crossed a default-branch change: %v", err)
	}
}

func TestEngineRejectsNameReuseDuringFileReadBeforeImport(t *testing.T) {
	engine := Engine{Store: engineStore(t)}
	calls := engineFileCalls(".smyklot.toml", "command_prefix='/wrong-repository '\n")
	calls[len(calls)-1].answer = `{"id":11,"name":"renamed","full_name":"acme/renamed","owner":{"login":"acme"},"default_branch":"main"}`
	_, again, err := engine.runOnce(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if err != nil || !again {
		t.Fatalf("moving identity must retry a fresh read: again=%t, error=%v", again, err)
	}
	repository, err := engine.Store.GetRepository(t.Context(), "workspace", "github:repository:11")
	if err != nil || repository.Revision != 2 || repository.ConfigPatch.CommandPrefix != nil {
		t.Fatalf("import crossed repository identities: %+v (%v)", repository, err)
	}
}

func TestWorkspaceSourceCannotFollowARenamedGitHubRepository(t *testing.T) {
	engine := Engine{Store: engineStore(t)}
	snapshot, _ := engine.Snapshot(t.Context(), "workspace", "")
	saveStatusSettingsChange(t, engine, snapshot, false)
	client := scriptedRemote(t, remoteCall{method: "GET", path: "/repositories/12", answer: `{"id":12,"name":"archive","full_name":"acme/archive","owner":{"login":"acme"},"default_branch":"main"}`})
	connection, err := engine.Run(t.Context(), client, "workspace", "")
	if err != nil || connection.Status != StatusBlocked || connection.Problem != "workspace_repository_renamed" {
		t.Fatalf("renamed workspace source = %+v (%v)", connection, err)
	}
}
