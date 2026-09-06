package configsync

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func conflictPreviewEngine(t *testing.T) Engine {
	t.Helper()
	engine := Engine{Store: engineStore(t)}
	_, err := engine.Store.SaveInstallationSettings(t.Context(), storage.SaveInstallationSettingsRequest{
		TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC(),
		Repositories: []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "github:repository:11", ExpectedRevision: 2, ConfigFileSyncEnabled: true,
			ConfigPatch: config.Patch{CommandPrefix: new("/panel "), QuietSuccess: new(false)},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

const previewFile = "command_prefix='/file '\nallow_self_approval=true\n"

func previewFileCalls(path, content string) []remoteCall {
	return engineFileCalls(path, content)
}

func previewReadCalls(entries string) []remoteCall {
	return engineReadCalls(entries)
}

func TestConflictPreviewOffersOnlyValidatedMergedChoicesWithoutWrites(t *testing.T) {
	engine := conflictPreviewEngine(t)
	before, _ := engine.Store.GetConfigFileState(t.Context(), "workspace", "github:repository:11")
	preview, err := engine.Preview(t.Context(), scriptedRemote(t, previewFileCalls(".smyklot.toml", previewFile)...), "workspace", "github:repository:11")
	if err != nil || preview.Status != StatusBlocked || preview.Problem != "conflicting_edits" ||
		preview.ReviewToken == "" || preview.Head != remoteHead || preview.Path != ".smyklot.toml" ||
		preview.CheckedAt.IsZero() || preview.ConflictCount != 1 || len(preview.Choices) != 2 {
		t.Fatalf("preview = %+v (%v)", preview, err)
	}
	for _, choice := range preview.Choices {
		result, err := DecodeDocument(choice.Document, config.PanelFileRepository)
		if err != nil || result.CommandPrefix == nil || result.QuietSuccess == nil ||
			*result.QuietSuccess || result.AllowSelfApproval == nil || !*result.AllowSelfApproval {
			t.Fatalf("choice lost independent edits = %s (%v)", choice.Document, err)
		}
		want := "/panel "
		if choice.Side == ResolutionFile {
			want = "/file "
		}
		if *result.CommandPrefix != want || !choice.ImportPanel || !choice.PublishFile {
			t.Fatalf("choice = %+v", choice)
		}
	}
	encoded, _ := json.Marshal(preview)
	for _, hidden := range []string{`"base":`, `"inputs":`, `"resolution":`, `"proposal":`} {
		if strings.Contains(string(encoded), hidden) {
			t.Fatalf("preview exposed private connection state: %s", encoded)
		}
	}
	after, _ := engine.Store.GetConfigFileState(t.Context(), "workspace", "github:repository:11")
	repository, _ := engine.Store.GetRepository(t.Context(), "workspace", "github:repository:11")
	if after.Revision != before.Revision || repository.Revision != 3 || repository.ConfigPatch.AllowSelfApproval != nil {
		t.Fatal("preview changed settings or connection state")
	}
}

func TestResolutionPreparationRereadsAndRejectsChangedInputs(t *testing.T) {
	for _, change := range []string{"unchanged", "panel", "sync", "file", "commit"} {
		t.Run(change, func(t *testing.T) { checkResolutionPreparation(t, change) })
	}
}

func checkResolutionPreparation(t *testing.T, change string) {
	t.Helper()
	engine := conflictPreviewEngine(t)
	preview, err := engine.Preview(t.Context(), scriptedRemote(t, previewFileCalls(".smyklot.toml", previewFile)...), "workspace", "github:repository:11")
	if err != nil {
		t.Fatal(err)
	}
	if change == "panel" || change == "sync" {
		snapshot, _ := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
		saveStatusSettingsChange(t, engine, snapshot, change == "sync")
	}
	prepared, err := engine.PrepareResolution(t.Context(), scriptedRemote(t, changedPreviewCalls(change)...),
		"workspace", "github:repository:11", preview.ReviewToken, ResolutionFile)
	if change != "unchanged" {
		if !errors.Is(err, storage.ErrConflict) {
			t.Fatalf("stale %s accepted: %+v (%v)", change, prepared, err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	connection, err := DecodeConnection(storage.ConfigFileState{Document: prepared.Document}, config.PanelFileRepository)
	if err != nil || connection.Resolution == nil || connection.Resolution.Side != ResolutionFile || connection.Status != StatusPending {
		t.Fatalf("prepared resolution = %+v (%v)", connection, err)
	}
	stored, _ := engine.Store.GetConfigFileState(t.Context(), "workspace", "github:repository:11")
	if stored.Revision != 0 {
		t.Fatal("preparing resolution persisted unaudited state")
	}
}

func changedPreviewCalls(change string) []remoteCall {
	content := previewFile
	if change == "file" {
		content += "quiet_success=true\n"
	}
	calls := previewFileCalls(".smyklot.toml", content)
	if change == "commit" {
		for index := range calls {
			calls[index].answer = strings.ReplaceAll(calls[index].answer, remoteHead, strings.Repeat("b", 40))
			calls[index].path = strings.ReplaceAll(calls[index].path, remoteHead, strings.Repeat("b", 40))
			calls[index].check = nil
		}
	}
	return calls
}

func TestPreviewDetectsChangesDuringRemoteRead(t *testing.T) {
	engine := conflictPreviewEngine(t)
	snapshot, _ := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
	// A changed panel revision rejects the review before the second remote read.
	calls := engineFileCalls(".smyklot.toml", previewFile)
	calls = calls[:len(calls)-1]
	calls[0].check = func(t *testing.T, _ *http.Request) { saveStatusSettingsChange(t, engine, snapshot, true) }
	_, err := engine.Preview(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("mixed observation accepted: %v", err)
	}
}

func TestRemovedFilePreviewCannotReplaceSettingsWithAnEmptyDocument(t *testing.T) {
	engine := conflictPreviewEngine(t)
	snapshot, _ := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
	panel, _ := snapshot.JSON()
	storeConnection(t, engine, "github:repository:11", Connection{Version: 1, Base: present(string(panel))})
	preview, err := engine.Preview(t.Context(), scriptedRemote(t, previewReadCalls(`[]`)...), "workspace", "github:repository:11")
	if err != nil || preview.Problem != "file_removed" || len(preview.Choices) != 1 ||
		preview.Choices[0].Side != ResolutionPanel || preview.Choices[0].ImportPanel || !preview.Choices[0].PublishFile {
		t.Fatalf("removed file choice = %+v (%v)", preview, err)
	}
	_, err = engine.PrepareResolution(t.Context(), scriptedRemote(t, previewReadCalls(`[]`)...), "workspace", "github:repository:11", preview.ReviewToken, ResolutionFile)
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("deleted file accepted as settings: %v", err)
	}
}

func TestPreviewOffersRecoveryWhenFileIsDeletedBeforeTheFirstBaseline(t *testing.T) {
	engine := conflictPreviewEngine(t)
	preview, err := engine.Preview(t.Context(), scriptedRemote(t, previewFileCalls(".smyklot.toml", previewFile)...), "workspace", "github:repository:11")
	if err != nil {
		t.Fatal(err)
	}
	change, err := engine.PrepareResolution(t.Context(), scriptedRemote(t, previewFileCalls(".smyklot.toml", previewFile)...),
		"workspace", "github:repository:11", preview.ReviewToken, ResolutionFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Store.SaveConfigFileState(t.Context(), change); err != nil {
		t.Fatal(err)
	}
	preview, err = engine.Preview(t.Context(), scriptedRemote(t, previewReadCalls(`[]`)...), "workspace", "github:repository:11")
	if err != nil || preview.Problem != "file_removed" || preview.ReviewToken == "" || len(preview.Choices) != 1 || preview.Choices[0].Side != ResolutionPanel {
		t.Fatalf("initial deletion has no recovery: %+v (%v)", preview, err)
	}
}
