package configsync

import (
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestPublicationAcceptsBOMAndRemovesOnlyRequestedSettings(t *testing.T) {
	for _, prefix := range []string{"", "\xef\xbb\xbf"} {
		content := prefix + "#:schema " + config.SchemaURL + "\n# keep this explanation\n" +
			"quiet_success = true # local note\ncommand_prefix = '/old '\n"
		source, err := ReadFileSource(config.FormatTOML, []byte(content), config.PanelFileRepository)
		if err != nil {
			t.Fatal(err)
		}
		document, err := DecodeDocument(source.Snapshot.Document, config.PanelFileRepository)
		if err != nil {
			t.Fatal(err)
		}
		document.CommandPrefix = nil
		document.QuietSuccess = new(false)
		semantic, err := config.EncodeJSONDocument(document)
		if err != nil {
			t.Fatal(err)
		}
		output, err := renderPublication(RemoteFile{Location: remoteLocation(), Path: ".smyklot.toml", Content: []byte(content), Source: source}, semantic)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(output), "#:schema "+config.RepositoryPanelSchemaURL+"\n") ||
			!strings.Contains(string(output), "# keep this explanation") ||
			!strings.Contains(string(output), "quiet_success = false # local note") || strings.Contains(string(output), "command_prefix") {
			t.Fatalf("publication changed unrelated content or kept a removed setting: %s", output)
		}
	}
}

func TestPublicationDoesNotReformatUnchangedSettings(t *testing.T) {
	content := "quiet_success=true\n[panel]\nversion=1\nscope='repository'\n[panel.settings]\n" +
		"quiet_period='60s'\nprotected_refs={include=['~DEFAULT_BRANCH']}\n" +
		"merge_exceptions={allow=true,actors=[{id=0,type='OrganizationAdmin',mode='always'}]}\n" +
		"[panel.sync.files]\ndocument='''{\n  \"excludes\": [\"tmp/**\"]\n}'''\n"
	source, err := ReadFileSource(config.FormatTOML, []byte(content), config.PanelFileRepository)
	if err != nil {
		t.Fatal(err)
	}
	document, err := DecodeDocument(source.Snapshot.Document, config.PanelFileRepository)
	if err != nil {
		t.Fatal(err)
	}
	document.QuietSuccess = new(false)
	semantic, err := config.EncodeJSONDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	output, err := renderPublication(RemoteFile{Location: remoteLocation(), Path: ".smyklot.toml", Content: []byte(content), Source: source}, semantic)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(content, "[panel]")
	if !strings.Contains(string(output), content[start:]) || !strings.Contains(string(output), "quiet_success=false") {
		t.Fatalf("unrelated settings were reformatted: %s", output)
	}
}
