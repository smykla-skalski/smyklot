package configsync

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestWorkspacePreviewUsesOnlyTheWorkspaceConfigurationPath(t *testing.T) {
	engine := Engine{Store: engineStore(t)}
	snapshot, _ := engine.Snapshot(t.Context(), "workspace", "")
	saveStatusSettingsChange(t, engine, snapshot, false)
	document, err := snapshot.Document()
	if err != nil {
		t.Fatal(err)
	}
	document.CommandPrefix = new("/file ")
	content, err := config.RenderFileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	calls := previewFileCalls(WorkspaceFilePath, string(content))
	for index := range calls {
		calls[index].path = strings.ReplaceAll(calls[index].path, "/acme/web/", "/acme/.github/")
		calls[index] = workspaceIdentityCall(calls[index])
	}
	preview, err := engine.Preview(t.Context(), scriptedRemote(t, calls...), "workspace", "")
	if err != nil || preview.Status != StatusBlocked || preview.Path != WorkspaceFilePath || len(preview.Choices) != 2 {
		t.Fatalf("workspace preview = %+v (%v)", preview, err)
	}
	for _, choice := range preview.Choices {
		if _, err := DecodeDocument(choice.Document, config.PanelFileWorkspace); err != nil {
			t.Fatalf("workspace choice lost scope: %v", err)
		}
	}
}

func TestPreviewDoesNotReadGitHubForDisabledOrBypassedConnections(t *testing.T) {
	for _, bypassed := range []bool{false, true} {
		t.Run(map[bool]string{true: "bypassed", false: "disabled"}[bypassed], func(t *testing.T) {
			engine := conflictPreviewEngine(t)
			_, err := engine.Store.SaveInstallationSettings(t.Context(), storage.SaveInstallationSettingsRequest{
				TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC(),
				Repositories: []storage.InstallationRepositorySettingsChange{{
					RepositoryID: "github:repository:11", ExpectedRevision: 3, ConfigFileSyncEnabled: bypassed, IgnoreRepositoryFile: bypassed,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			preview, err := engine.Preview(t.Context(), scriptedRemote(t), "workspace", "github:repository:11")
			want := "sync_off"
			if bypassed {
				want = "file_disabled"
			}
			if err != nil || preview.Problem != want || len(preview.Choices) != 0 || preview.ReviewToken != "" {
				t.Fatalf("disabled review = %+v (%v)", preview, err)
			}
		})
	}
}

func TestPreviewShowsInvalidFileAndStorageRangeWithoutWriting(t *testing.T) {
	for _, test := range []struct{ content, problem string }{
		{"not = [valid", "invalid_file"},
		{"[panel]\nversion=1\nscope='repository'\n[panel.settings]\nfile_index_interval='9000h'\n", "invalid_settings"},
	} {
		t.Run(test.problem, func(t *testing.T) {
			engine := Engine{Store: engineStore(t)}
			calls := engineFileCalls(".smyklot.toml", test.content)
			if test.problem == "invalid_file" {
				calls = calls[:len(calls)-1]
			}
			preview, err := engine.Preview(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
			if err != nil || preview.Status != StatusBlocked || preview.Problem != test.problem ||
				preview.Path != ".smyklot.toml" || preview.Head != remoteHead || preview.ReviewToken != "" {
				t.Fatalf("invalid preview = %+v (%v)", preview, err)
			}
			stored, _ := engine.Store.GetConfigFileState(t.Context(), "workspace", "github:repository:11")
			if stored.Revision != 0 {
				t.Fatal("preview persisted a blocked state")
			}
		})
	}
}

func TestPreviewDoesNotOfferAnAcceptedChoiceUntilTheConflictChanges(t *testing.T) {
	engine := conflictPreviewEngine(t)
	client := scriptedRemote(t, previewFileCalls(".smyklot.toml", previewFile)...)
	preview, err := engine.Preview(t.Context(), client, "workspace", "github:repository:11")
	if err != nil {
		t.Fatal(err)
	}
	change, err := engine.PrepareResolution(t.Context(), scriptedRemote(t, previewFileCalls(".smyklot.toml", previewFile)...),
		"workspace", "github:repository:11", preview.ReviewToken, ResolutionPanel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Store.SaveConfigFileState(t.Context(), change); err != nil {
		t.Fatal(err)
	}
	preview, err = engine.Preview(t.Context(), scriptedRemote(t, previewFileCalls(".smyklot.toml", previewFile)...), "workspace", "github:repository:11")
	if err != nil || preview.Status != StatusPending || preview.ReviewToken != "" || len(preview.Choices) != 0 {
		t.Fatalf("accepted choice requested again: %+v (%v)", preview, err)
	}
	preview, err = engine.Preview(t.Context(), scriptedRemote(t, previewFileCalls(".smyklot.toml", previewFile+"quiet_success=true\n")...), "workspace", "github:repository:11")
	if err != nil || preview.Status != StatusBlocked || preview.ReviewToken == "" || len(preview.Choices) != 2 {
		t.Fatalf("stale choice prevented a fresh review: %+v (%v)", preview, err)
	}
}

func TestPreviewChoiceWireRetainsLargeNumbersAndNulls(t *testing.T) {
	engine := conflictPreviewEngine(t)
	content := previewFile + "[panel]\nversion=1\nscope='repository'\n[panel.sync.files]\ndocument='''" +
		`{"merges":[{"path":"renovate.json","overrides":{"count":9007199254740993,"removed":null}}]}` + "'''\n"
	// Exercise the actual native file reader and merged choice encoder, rather
	// than constructing a DTO with an already encoded test value.
	preview, err := engine.Preview(t.Context(), scriptedRemote(t, previewFileCalls(".smyklot.toml", content)...), "workspace", "github:repository:11")
	if err != nil || len(preview.Choices) != 2 {
		t.Fatalf("numeric preview = %+v (%v)", preview, err)
	}
	for _, choice := range preview.Choices {
		encoded, err := json.Marshal(choice)
		if err != nil || !choice.Available || !bytes.Contains(encoded, []byte(`9007199254740993`)) ||
			bytes.Contains(encoded, []byte(`9007199254740992`)) || !bytes.Contains(encoded, []byte(`"removed":null`)) {
			t.Fatalf("choice rounded a native number = %s (%v)", encoded, err)
		}
	}
}

func TestPreviewAndWorkerDiscardAChoiceAfterItsConflictDisappears(t *testing.T) {
	engine := conflictPreviewEngine(t)
	preview, err := engine.Preview(t.Context(), scriptedRemote(t, previewFileCalls(".smyklot.toml", previewFile)...), "workspace", "github:repository:11")
	if err != nil {
		t.Fatal(err)
	}
	change, err := engine.PrepareResolution(t.Context(), scriptedRemote(t, previewFileCalls(".smyklot.toml", previewFile)...),
		"workspace", "github:repository:11", preview.ReviewToken, ResolutionPanel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Store.SaveConfigFileState(t.Context(), change); err != nil {
		t.Fatal(err)
	}
	// Both prefixes now agree, but the independently edited fields still differ.
	// Preview and the worker must agree that no user decision remains necessary.
	content := strings.Replace(previewFile, "/file ", "/panel ", 1)
	preview, err = engine.Preview(t.Context(), scriptedRemote(t, previewFileCalls(".smyklot.toml", content)...), "workspace", "github:repository:11")
	if err != nil || preview.Status != StatusPending || preview.ReviewToken != "" || len(preview.Choices) != 0 {
		t.Fatalf("disappeared conflict = %+v (%v)", preview, err)
	}
	calls := append(engineFileCalls(".smyklot.toml", content), engineFileCalls(".smyklot.toml", content)...)
	calls = append(calls, prepareEngineCalls()...)
	calls = append(calls, enginePublishCalls(false)...)
	connection, err := engine.Run(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if err != nil || connection.Status != StatusProposed || connection.Resolution != nil || connection.Problem != "" {
		t.Fatalf("worker remained blocked after conflict disappeared: %+v (%v)", connection, err)
	}
	snapshot, _ := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
	document, _ := snapshot.Document()
	if document.QuietSuccess == nil || *document.QuietSuccess || document.AllowSelfApproval == nil || !*document.AllowSelfApproval {
		t.Fatal("dropping the stale choice lost independent changes")
	}
	merged, err := config.RenderFileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	mergedCalls := append(engineFileCalls(".smyklot.toml", string(merged)), mergedProposalCalls()...)
	connection, err = engine.Run(t.Context(), scriptedRemote(t, mergedCalls...), "workspace", "github:repository:11")
	if err != nil || connection.Status != StatusReady || !connection.Base.Exists {
		t.Fatalf("automatically merged settings did not converge: %+v (%v)", connection, err)
	}
}
