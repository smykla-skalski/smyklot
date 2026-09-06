package configsync

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestPanelSettingsSurviveFileRoundTrip(t *testing.T) {
	for _, scope := range []config.PanelFileScope{config.PanelFileWorkspace, config.PanelFileRepository} {
		t.Run(string(scope), func(t *testing.T) {
			snapshot := completeSnapshot(scope)
			request := roundTripImport(t, snapshot)
			if request.TargetID != snapshot.Target.ID || request.ActorAccountID != "" ||
				request.ConfigFileImport == nil || request.ConfigFileImport.HeadSHA != strings.Repeat("a", 40) {
				t.Fatal("round trip changed import identity or source")
			}
			if scope == config.PanelFileWorkspace {
				checkWorkspaceImport(t, snapshot, request)
			} else {
				checkRepositoryImport(t, snapshot, request)
			}
		})
	}
}

func roundTripImport(t *testing.T, snapshot PanelSnapshot) storage.SaveInstallationSettingsRequest {
	t.Helper()
	document, err := snapshot.Document()
	if err != nil {
		t.Fatal(err)
	}
	if document.Runner != nil {
		t.Fatal("panel export included the file-owned runner")
	}
	before, err := snapshot.JSON()
	if err != nil {
		t.Fatal(err)
	}
	content, err := config.RenderFileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.ParseFileDocument(config.FormatTOML, content)
	if err != nil {
		t.Fatal(err)
	}
	after, err := config.EncodeJSONDocument(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if same, err := Equivalent(before, after); err != nil || !same {
		t.Fatalf("file round trip changed settings: %s => %s (%v)", before, after, err)
	}
	// A person edits one command setting in the file. The import must preserve
	// all untouched Sync bytes, including authored JSON whitespace.
	parsed.CommandPrefix = new("/edited ")
	after, err = config.EncodeJSONDocument(parsed)
	if err != nil {
		t.Fatal(err)
	}
	request, err := snapshot.PrepareImport(after, storage.ConfigFileImport{
		State: storage.ConfigFileStateChange{
			TargetID: snapshot.Target.ID, RepositoryID: snapshot.RepositoryID(),
			OwnerRevision: snapshot.OwnerRevision(), SyncRevisions: snapshot.SyncRevisions(),
			ChangedAt: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC), Document: []byte(`{}`),
		},
		Path: ".smyklot.toml", HeadSHA: strings.Repeat("a", 40),
	}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func completeSnapshot(scope config.PanelFileScope) PanelSnapshot {
	patch := config.Default().AsPatch()
	patch.CommandPrefix = new("/team ")
	patch.AllowedCommands = new([]string{})
	patch.CommandAliases = new(map[string]string{"ship": "merge"})
	refs := storage.PendingCIBranchPatterns{
		Include: []string{"~DEFAULT_BRANCH", "refs/heads/release/**"},
		Exclude: []string{"refs/heads/release/archive/**"},
	}
	policy := &storage.PendingCIBypassPolicy{Allow: true, Actors: []orgsync.RulesetBypassActor{
		{ActorID: 1197525, ActorType: "Integration", Mode: "always"},
	}}
	snapshot := PanelSnapshot{Target: storage.Target{
		ID: "workspace", Kind: storage.TargetOrganization, Revision: 9, ConfigFileSyncEnabled: true,
		RepositoryDefaultEnabled: false, ConfigPatch: patch, PendingCIModeDefault: storage.PendingCIModeChecks,
		PendingCIBranchPatternsDefault: refs, PendingCIBypassPolicyDefault: policy,
		PendingCIQuietPeriodOverride: new(time.Duration(0)), PathIndexIntervalOverride: new(time.Hour),
	}}
	if scope == config.PanelFileWorkspace {
		snapshot.SyncConfigs = []orgsync.Config{
			{
				TargetID: "workspace", Kind: orgsync.KindSettings, Enabled: false, Revision: 4,
				Document: []byte("{\n  \"delete_branch_on_merge\": false\n}"),
			},
			{
				TargetID: "workspace", Kind: orgsync.KindFiles, Enabled: true, Revision: 7,
				Document: []byte(`{"files":[{"path":"renovate.json","content":"{\"value\":null,\"exact\":9007199254740993}\n"}]}`),
			},
		}
		return snapshot
	}
	snapshot.Repository = &storage.Repository{
		ID: "repo", TargetID: "workspace", Revision: 12, ConfigFileSyncEnabled: true,
		EnabledOverride: new(false), ConfigPatch: patch, PendingCIModeOverride: new(storage.PendingCIModeChecks),
		PendingCIBranchPatternsOverride: &refs, PendingCIBypassPolicyOverride: policy,
		PendingCIQuietPeriodOverride: new(time.Duration(0)), PathIndexIntervalOverride: new(time.Hour),
	}
	snapshot.SyncOverrides = []orgsync.RepositoryOverride{
		{
			RepositoryID: "repo", Kind: orgsync.KindSettings, Enabled: new(false), Revision: 4, Document: []byte(`{}`),
		},
		{
			RepositoryID: "repo", Kind: orgsync.KindFiles, Revision: 7,
			Document: []byte("{\n  \"merges\": [{\"path\":\"renovate.json\",\"overrides\":{\"value\":null,\"exact\":9007199254740993}}]\n}"),
		},
	}
	return snapshot
}

func TestFileSyncRejectsCaseAliasesBeforeReordering(t *testing.T) {
	for _, document := range []string{
		`{"has_wiki":true,"Has_wiki":false}`, `{"Has_wiki":false,"has_wiki":true}`,
		`{"has_wiki":true,"has_wiki":false}`,
	} {
		snapshot := completeSnapshot(config.PanelFileWorkspace)
		snapshot.SyncConfigs[0].Document = []byte(document)
		if _, err := snapshot.JSON(); err == nil {
			t.Fatalf("export accepted an ambiguous Sync document: %s", document)
		}
		// Files take a separate path into the semantic merge, so the decoded
		// envelope must enforce the same exact-field contract before import.
		file := config.FileDocument{Panel: &config.PanelFileSection{
			Version: 1, Scope: config.PanelFileWorkspace,
			Settings: config.PanelFileSettings{
				RepositoryDefaultEnabled: new(true), MergeMode: new("checks"),
				ProtectedRefs: &config.PanelFileRefs{Include: []string{"~DEFAULT_BRANCH"}},
			},
			Sync: map[string]config.PanelFileSync{"settings": {Enabled: new(true), Document: []byte(document)}},
		}}
		encoded, err := config.EncodeJSONDocument(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeDocument(encoded, config.PanelFileWorkspace); err == nil {
			t.Fatalf("import accepted an ambiguous Sync document: %s", document)
		}
	}
}

func TestFileEnvelopeRejectsCaseAliases(t *testing.T) {
	for _, document := range []string{
		`{"Panel":{"version":1,"scope":"repository","settings":{}}}`,
		`{"panel":{"version":1,"scope":"repository","settings":{"Enabled":false}}}`,
		`{"Command_prefix":"/other ","panel":{"version":1,"scope":"repository","settings":{}}}`,
	} {
		if _, err := DecodeDocument([]byte(document), config.PanelFileRepository); err == nil {
			t.Fatalf("accepted field alias: %s", document)
		}
	}
}

func checkWorkspaceImport(t *testing.T, snapshot PanelSnapshot, request storage.SaveInstallationSettingsRequest) {
	t.Helper()
	got, want := request.Target, snapshot.Target
	if got == nil || got.ConfigFileSyncEnabled != want.ConfigFileSyncEnabled ||
		got.RepositoryDefaultEnabled != want.RepositoryDefaultEnabled || got.PendingCIModeDefault != want.PendingCIModeDefault ||
		!reflect.DeepEqual(got.PendingCIBranchPatternsDefault, want.PendingCIBranchPatternsDefault) ||
		!reflect.DeepEqual(got.PendingCIBypassPolicyDefault, want.PendingCIBypassPolicyDefault) ||
		!reflect.DeepEqual(got.PendingCIQuietPeriodOverride, want.PendingCIQuietPeriodOverride) ||
		!reflect.DeepEqual(got.PathIndexIntervalOverride, want.PathIndexIntervalOverride) || got.ExpectedRevision != want.Revision {
		t.Fatalf("workspace import lost settings: %+v", got)
	}
	want.ConfigPatch.Runner = nil
	want.ConfigPatch.CommandPrefix = new("/edited ")
	if !reflect.DeepEqual(got.ConfigPatch, want.ConfigPatch) || len(request.Repositories) != 0 {
		t.Fatal("workspace import changed root configuration or repository scope")
	}
	if len(request.SyncConfigs) != len(orgsync.Kinds()) {
		t.Fatal("import must represent present and removed Sync kinds")
	}
	for _, item := range request.SyncConfigs {
		if item.Kind == orgsync.KindLabels || item.Kind == orgsync.KindRulesets {
			if !item.Remove || item.ExpectedRevision != 0 {
				t.Fatalf("absent kind was not removed: %+v", item)
			}
			continue
		}
		for _, original := range snapshot.SyncConfigs {
			if item.Kind == original.Kind && (item.Remove || item.Enabled != original.Enabled ||
				item.ExpectedRevision != original.Revision || !bytes.Equal(item.Document, original.Document)) {
				t.Fatalf("sync settings changed: %+v", item)
			}
		}
	}
}

func checkRepositoryImport(t *testing.T, snapshot PanelSnapshot, request storage.SaveInstallationSettingsRequest) {
	t.Helper()
	if len(request.Repositories) != 1 || request.Target != nil {
		t.Fatal("repository import escaped its scope")
	}
	got, want := request.Repositories[0], *snapshot.Repository
	if got.RepositoryID != want.ID || got.ConfigFileSyncEnabled != want.ConfigFileSyncEnabled ||
		got.IgnoreRepositoryFile != want.IgnoreRepositoryFile || got.ExpectedRevision != want.Revision ||
		!reflect.DeepEqual(got.EnabledOverride, want.EnabledOverride) ||
		!reflect.DeepEqual(got.PendingCIModeOverride, want.PendingCIModeOverride) ||
		!reflect.DeepEqual(got.PendingCIBranchPatternsOverride, want.PendingCIBranchPatternsOverride) ||
		!reflect.DeepEqual(got.PendingCIBypassPolicyOverride, want.PendingCIBypassPolicyOverride) ||
		!reflect.DeepEqual(got.PendingCIQuietPeriodOverride, want.PendingCIQuietPeriodOverride) ||
		!reflect.DeepEqual(got.PathIndexIntervalOverride, want.PathIndexIntervalOverride) {
		t.Fatalf("repository import lost settings: %+v", got)
	}
	want.ConfigPatch.Runner = nil
	want.ConfigPatch.CommandPrefix = new("/edited ")
	if !reflect.DeepEqual(got.ConfigPatch, want.ConfigPatch) || len(request.SyncOverrides) != len(orgsync.Kinds()) {
		t.Fatal("repository import changed root configuration or omitted Sync kinds")
	}
	for _, item := range request.SyncOverrides {
		if item.RepositoryID != want.ID {
			t.Fatal("sync adjustment escaped its repository")
		}
		for _, original := range snapshot.SyncOverrides {
			if item.Kind == original.Kind && (item.Remove || !reflect.DeepEqual(item.Enabled, original.Enabled) ||
				item.ExpectedRevision != original.Revision || !bytes.Equal(item.Document, original.Document)) {
				t.Fatalf("sync adjustment changed: %+v", item)
			}
		}
	}
}
