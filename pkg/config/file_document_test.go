package config_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestFileDocumentRoundTrip(t *testing.T) {
	const source = `command_prefix = '/team '
quiet_success = false
runner = 'service'

[panel]
version = 1
scope = 'repository'

[panel.settings]
enabled = false
merge_mode = 'checks'
protected_refs = { include = ['~DEFAULT_BRANCH'], exclude = ['refs/heads/archive/**'] }
quiet_period = '2m'
file_index_interval = '1h'

[panel.settings.merge_exceptions]
allow = true
actors = [{ id = 1197525, type = 'Integration', mode = 'always' }]

[panel.sync.files]
enabled = false
document = '''{
  "ignore": ["local/**"],
  "files": [{"path": "renovate.json", "overrides": {"automerge": false}}]
}'''
`
	parsed, err := config.ParseFileDocument(config.FormatTOML, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.CommandPrefix == nil || *parsed.CommandPrefix != "/team " || parsed.Panel == nil ||
		parsed.Panel.Settings.Enabled == nil || *parsed.Panel.Settings.Enabled {
		t.Fatalf("lost root config or explicit false: %+v", parsed)
	}
	content, err := config.RenderFileDocument(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(content, []byte("\n")) || bytes.HasSuffix(content, []byte("\n\n")) {
		t.Fatalf("expected exactly one terminal newline: %q", content)
	}
	again, err := config.ParseFileDocument(config.FormatTOML, content)
	if err != nil || !reflect.DeepEqual(parsed, again) {
		t.Fatalf("round trip changed settings: %s (%v)", content, err)
	}
}

func TestFileDocumentKeepsLegacySparseSettings(t *testing.T) {
	for _, source := range []string{"", "# only a comment\n", "quiet_success = false\n", "allowed_commands = []\n"} {
		legacy, err := config.ParsePatch(config.FormatTOML, []byte(source))
		if err != nil {
			t.Fatal(err)
		}
		file, err := config.ParseFileDocument(config.FormatTOML, []byte(source))
		if err != nil || file.Panel != nil || !reflect.DeepEqual(file.Patch, legacy) {
			t.Fatalf("legacy file changed: %q (%v)", source, err)
		}
		content, err := config.RenderFileDocument(file)
		if err != nil || !bytes.HasSuffix(content, []byte("\n")) {
			t.Fatalf("missing terminal newline: %q (%v)", content, err)
		}
	}
}

func TestFileDocumentPointsEditorsAtItsScope(t *testing.T) {
	for _, scope := range []config.PanelFileScope{config.PanelFileRepository, config.PanelFileWorkspace} {
		section := &config.PanelFileSection{Version: 1, Scope: scope}
		url := config.RepositoryPanelSchemaURL
		if scope == config.PanelFileWorkspace {
			url = config.WorkspacePanelSchemaURL
			section.Settings = config.PanelFileSettings{
				RepositoryDefaultEnabled: new(false), MergeMode: new("checks"),
				ProtectedRefs: &config.PanelFileRefs{Include: []string{"~DEFAULT_BRANCH"}},
			}
		}
		content, err := config.RenderFileDocument(config.FileDocument{Panel: section})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(content, []byte("#:schema "+url+"\n\n")) {
			t.Fatalf("%s file points editors at the wrong schema: %s", scope, content)
		}
		parsed, err := config.ParseFileDocument(config.FormatTOML, content)
		if err != nil || parsed.Panel == nil || parsed.Panel.Scope != scope {
			t.Fatalf("schema directive broke %s parsing: %v", scope, err)
		}
	}
}

func TestFileDocumentRejectsWrongScopeAndUnknownSettings(t *testing.T) {
	const repository = "[panel]\nversion=1\nscope='repository'\n"
	const workspace = "[panel]\nversion=1\nscope='workspace'\n"
	cases := map[string]string{
		"unknown root":                     "unknown_setting=true\n",
		"unknown metadata":                 repository + "automatic=true\n",
		"unknown setting":                  repository + "[panel.settings]\nquiet_peirod='1m'\n",
		"unsupported version":              strings.Replace(repository, "version=1", "version=2", 1),
		"missing version":                  "[panel]\nscope='repository'\n",
		"unknown scope":                    "[panel]\nversion=1\nscope='root'\n",
		"repository changes workspace":     repository + "[panel.settings]\nrepository_default_enabled=true\n",
		"workspace uses repo enabled":      workspace + "[panel.settings]\nenabled=true\n",
		"workspace misses required policy": workspace,
		"workspace sets repository runner": "runner='action'\n" + workspace +
			"[panel.settings]\nrepository_default_enabled=true\nmerge_mode='checks'\nprotected_refs={include=['~DEFAULT_BRANCH']}\n",
		"invalid merge mode":       repository + "[panel.settings]\nmerge_mode='off'\n",
		"negative duration":        repository + "[panel.settings]\nquiet_period='-1s'\n",
		"fractional stored second": repository + "[panel.settings]\nquiet_period='1500ms'\n",
		"overflow duration":        repository + "[panel.settings]\nquiet_period='999999999999999h'\n",
		"unknown sync":             repository + "[panel.sync.access]\ndocument='{}'\n",
		"sync missing values":      repository + "[panel.sync.files]\nenabled=false\n",
		"sync typo":                repository + "[panel.sync.files]\nenabled=false\ndocumnt='{}'\n",
		"duplicate key":            repository + "[panel.settings]\nenabled=true\nenabled=false\n",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := config.ParseFileDocument(config.FormatTOML, []byte(source)); err == nil {
				t.Fatalf("accepted invalid file: %s", source)
			}
		})
	}
}

func TestFileDocumentPreservesOmissionAndEmptyValues(t *testing.T) {
	const source = "[panel]\nversion=1\nscope='repository'\n[panel.settings]\nprotected_refs={include=['~DEFAULT_BRANCH'],exclude=[]}\n[panel.sync.files]\ndocument='{}'\n"
	document, err := config.ParseFileDocument(config.FormatTOML, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if document.Panel.Settings.Enabled != nil || document.Panel.Settings.ProtectedRefs == nil ||
		document.Panel.Sync["files"].Enabled != nil || document.Panel.Sync["files"].Document == nil {
		t.Fatalf("omission and empty state conflated: %+v", document.Panel)
	}
	rendered, err := config.RenderFileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := config.ParseFileDocument(config.FormatTOML, rendered)
	if err != nil || !reflect.DeepEqual(document, again) {
		t.Fatalf("sparse state lost on render: %s (%v)", rendered, err)
	}
}

func TestFileDocumentPreservesEveryJSONValue(t *testing.T) {
	values := []string{
		`{"remove":null,"nested":[null,{},[],false,0,""]}`,
		`{"large":9007199254740993,"bigger":18446744073709551616,"precise":1.0000000000000001}`,
		`{"scientific":1e999,"tiny":1e-999}`,
		`{"template":"# quotes \"\"\" and ''' and \\r\\n\n"}`,
	}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			document := config.FileDocument{Panel: &config.PanelFileSection{
				Version: 1, Scope: config.PanelFileRepository,
				Sync: map[string]config.PanelFileSync{"files": {Document: json.RawMessage(value)}},
			}}
			rendered, err := config.RenderFileDocument(document)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := config.ParseFileDocument(config.FormatTOML, rendered)
			if err != nil {
				t.Fatalf("%v: %s", err, rendered)
			}
			if string(parsed.Panel.Sync["files"].Document) != value {
				t.Fatalf("changed JSON bytes: %s", rendered)
			}
		})
	}
}
