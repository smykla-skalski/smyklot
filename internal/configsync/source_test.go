package configsync

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestSourceSeparatesMissingEmptyAndLegacyRepositoryFiles(t *testing.T) {
	missing, err := ReadFileSource(config.FormatTOML, nil, config.PanelFileRepository)
	if err != nil || missing.Snapshot.Exists {
		t.Fatalf("missing source = %+v (%v)", missing, err)
	}
	empty, err := ReadFileSource(config.FormatTOML, []byte{}, config.PanelFileRepository)
	if err != nil || !empty.Snapshot.Exists {
		t.Fatalf("empty source = %+v (%v)", empty, err)
	}
	parsed, err := DecodeDocument(empty.Snapshot.Document, config.PanelFileRepository)
	if err != nil || parsed.Panel.Settings.Enabled != nil || len(parsed.SetKeys()) != 0 {
		t.Fatalf("empty repository file lost inheritance: %+v (%v)", parsed, err)
	}
	for format, content := range map[config.Format]string{
		config.FormatTOML: "runner = 'action'\nquiet_success = false\n",
		config.FormatYAML: "runner: action\nquiet_success: false\n",
	} {
		source, err := ReadFileSource(format, []byte(content), config.PanelFileRepository)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := DecodeDocument(source.Snapshot.Document, config.PanelFileRepository)
		if err != nil || parsed.Runner != nil || source.Runner == nil || *source.Runner != config.RunnerAction ||
			parsed.QuietSuccess == nil || *parsed.QuietSuccess {
			t.Fatalf("legacy %s lost file ownership or explicit false: %+v (%v)", format, source, err)
		}
	}
}

func TestSourceRejectsWrongScopeBeforeComparison(t *testing.T) {
	repository := "[panel]\nversion=1\nscope='repository'\n"
	workspace := "[panel]\nversion=1\nscope='workspace'\n[panel.settings]\n" +
		"repository_default_enabled=true\nmerge_mode='checks'\nprotected_refs={include=['~DEFAULT_BRANCH']}\n"
	for _, example := range []struct {
		content string
		scope   config.PanelFileScope
	}{
		{repository, config.PanelFileWorkspace},
		{workspace, config.PanelFileRepository},
		{"", config.PanelFileWorkspace},
		{"runner='action'\n" + workspace, config.PanelFileWorkspace},
		{repository, "unknown"},
		{repository + "[panel.sync.settings]\ndocument='{\"Has_wiki\":false}'\n", config.PanelFileRepository},
	} {
		if source, err := ReadFileSource(config.FormatTOML, []byte(example.content), example.scope); err == nil || source.Snapshot.Exists {
			t.Fatalf("accepted unusable file for %s: %s", example.scope, example.content)
		}
	}
}

func TestEquivalentDurationSpellingDoesNotCreateSyncWork(t *testing.T) {
	snapshot := completeSnapshot(config.PanelFileRepository)
	snapshot.Repository.PendingCIQuietPeriodOverride = new(time.Minute)
	document, err := snapshot.Document()
	if err != nil {
		t.Fatal(err)
	}
	panel, err := snapshot.JSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, spelling := range []string{"60s", "1m", "60000ms"} {
		document.Panel.Settings.QuietPeriod = new(spelling)
		document.Panel.Settings.FileIndexInterval = new("60m")
		content, err := config.RenderFileDocument(document)
		if err != nil {
			t.Fatal(err)
		}
		source, err := ReadFileSource(config.FormatTOML, content, config.PanelFileRepository)
		if err != nil {
			t.Fatal(err)
		}
		decision, err := Reconcile(ReconcileInput{
			Base: Snapshot{Exists: true, Document: panel}, Panel: panel, File: source.Snapshot,
		})
		if err != nil || decision.Problem != "" || decision.ImportPanel || decision.PublishFile || !decision.AdvanceBase {
			t.Fatalf("equivalent duration %q created work: %+v (%v)", spelling, decision, err)
		}
		if *document.Panel.Settings.QuietPeriod != spelling {
			t.Fatal("comparison mutated the authored spelling")
		}
	}
}

func TestSourceKeepsExactSharedFileValues(t *testing.T) {
	content := []byte("[panel]\nversion=1\nscope='repository'\n[panel.sync.files]\ndocument='''" +
		`{"merges":[{"path":"renovate.json","overrides":{"exact":9007199254740993,"removed":null}}]}` + "'''\n")
	source, err := ReadFileSource(config.FormatTOML, content, config.PanelFileRepository)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(source.Snapshot.Document, []byte("9007199254740993")) ||
		!bytes.Contains(source.Snapshot.Document, []byte(`"removed":null`)) {
		t.Fatalf("file preparation lost JSON meaning: %s", source.Snapshot.Document)
	}
	invalid := strings.Replace(string(content), "9007199254740993", `0,"exact":1`, 1)
	if _, err := ReadFileSource(config.FormatTOML, []byte(invalid), config.PanelFileRepository); err == nil {
		t.Fatal("file preparation accepted duplicate JSON fields")
	}
}

func TestSourceOptionalPolicyFieldsConvergeWithPanel(t *testing.T) {
	for _, scope := range []config.PanelFileScope{config.PanelFileRepository, config.PanelFileWorkspace} {
		for _, actors := range []string{
			"",
			"actors=[{id=0,type='OrganizationAdmin',mode='always'}]\n",
			"actors=[{id=0,type='DeployKey',mode='exempt'}]\n",
		} {
			t.Run(string(scope)+actors, func(t *testing.T) {
				checkSourcePolicyConverges(t, scope, actors)
			})
		}
	}
}

func checkSourcePolicyConverges(t *testing.T, scope config.PanelFileScope, actors string) {
	t.Helper()
	settings := "[panel.settings]\n"
	if scope == config.PanelFileWorkspace {
		settings += "repository_default_enabled=true\nmerge_mode='checks'\n"
	}
	content := "[panel]\nversion=1\nscope='" + string(scope) + "'\n" + settings +
		"[panel.settings.protected_refs]\ninclude=['~DEFAULT_BRANCH']\n" +
		"[panel.settings.merge_exceptions]\nallow=false\n" + actors
	source, err := ReadFileSource(config.FormatTOML, []byte(content), scope)
	if err != nil {
		t.Fatal(err)
	}
	document, err := DecodeDocument(source.Snapshot.Document, scope)
	if err != nil {
		t.Fatal(err)
	}
	refs := storageRefs(document.Panel.Settings.ProtectedRefs)
	policy, err := storageExceptions(document.Panel.Settings.MergeExceptions, storage.TargetOrganization)
	if err != nil {
		t.Fatal(err)
	}
	panel := PanelSnapshot{Target: storage.Target{ID: "workspace", Kind: storage.TargetOrganization}}
	if scope == config.PanelFileRepository {
		panel.Repository = &storage.Repository{
			ID: "github:repository:11", TargetID: "workspace",
			PendingCIBranchPatternsOverride: refs, PendingCIBypassPolicyOverride: policy,
		}
	} else {
		panel.Target.RepositoryDefaultEnabled = true
		panel.Target.PendingCIModeDefault = storage.PendingCIModeChecks
		panel.Target.PendingCIBranchPatternsDefault = *refs
		panel.Target.PendingCIBypassPolicyDefault = policy
	}
	encoded, err := panel.JSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, baseline := range []Snapshot{{}, source.Snapshot} {
		decision, err := Reconcile(ReconcileInput{Base: baseline, Panel: encoded, File: source.Snapshot})
		if err != nil || decision.Problem != "" || decision.ImportPanel || decision.PublishFile || !decision.AdvanceBase {
			t.Fatalf("optional policy fields caused work for %s (%q): %+v (%v)", scope, actors, decision, err)
		}
	}
}

func TestSourceNormalizationPreservesSparseAndAuthoredSettings(t *testing.T) {
	settings := config.PanelFileSettings{
		ProtectedRefs: &config.PanelFileRefs{Include: []string{"~DEFAULT_BRANCH"}},
		MergeExceptions: &config.PanelFileExceptions{Actors: []config.PanelFileActor{
			{ID: new(int64(0)), Type: "OrganizationAdmin", Mode: "always"},
			{ID: new(int64(1197525)), Type: "Integration", Mode: "always"},
		}},
	}
	normalized := normalizeFileSettings(settings)
	if settings.ProtectedRefs.Exclude != nil || settings.MergeExceptions.Actors[0].ID == nil ||
		*normalized.MergeExceptions.Actors[1].ID != 1197525 {
		t.Fatal("normalization mutated authored policy or a real actor ID")
	}
	normalized = normalizeFileSettings(config.PanelFileSettings{})
	if normalized.ProtectedRefs != nil || normalized.MergeExceptions != nil {
		t.Fatal("normalization replaced inheritance with a policy")
	}
	// Invalid required IDs stay invalid instead of acquiring special-actor semantics.
	invalid := normalizeFileSettings(config.PanelFileSettings{MergeExceptions: &config.PanelFileExceptions{
		Actors: []config.PanelFileActor{{ID: new(int64(0)), Type: "Integration", Mode: "always"}},
	}})
	if _, err := storageExceptions(invalid.MergeExceptions, storage.TargetOrganization); err == nil {
		t.Fatal("normalization accepted a missing integration ID")
	}
}
