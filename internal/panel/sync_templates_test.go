package panel

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

func TestLegacyTemplateReadPreservesValuesAndUnknownFields(t *testing.T) {
	document := []byte(`{"future":9007199254740993,"files":[{"path":"config.yaml","content":"value: |\n  text","future":{"a":9007199254740993}},{"path":"broken.json","content":"{ broken }"}]}`)
	dto := syncConfigToDTO(orgsync.Config{Kind: orgsync.KindFiles, Document: document}, "")
	if dto.Unreadable {
		t.Fatal("legacy document became unreadable")
	}
	if strings.Count(string(dto.Document), "9007199254740993") != 2 {
		t.Fatalf("lost unknown values: %s", dto.Document)
	}
	var files orgsync.FileConfig
	if err := json.Unmarshal(dto.Document, &files); err != nil {
		t.Fatal(err)
	}
	if files.Files[0].Content != "value: |-\n  text\n" {
		t.Fatalf("changed YAML value: %q", files.Files[0].Content)
	}
	if files.Files[1].Content != "{ broken }" {
		t.Fatal("invalid source must stay editable")
	}
	if string(readableTemplateDocument(dto.Document)) != string(dto.Document) {
		t.Fatal("read normalization is not idempotent")
	}
	states := workspaceSyncConfigSettingsStates("target", []orgsync.Config{{Kind: orgsync.KindFiles, Document: document}})
	if string(states[0].Document) != string(dto.Document) {
		t.Fatal("settings response disagrees with the editor's canonical baseline")
	}
}

func TestStoredTemplatesIncludeTheRequiredTerminatorWithinLimits(t *testing.T) {
	for _, size := range []int{orgsync.MaxFileContentBytes - 1, orgsync.MaxFileContentBytes} {
		body := strings.Repeat("x", size)
		document, _ := json.Marshal(orgsync.FileConfig{Files: []orgsync.File{{Path: "README.md", Content: body}}})
		stored, err := syncDocumentFor(orgsync.KindFiles, syncConfigRequest{Document: document})
		if size == orgsync.MaxFileContentBytes {
			if err == nil {
				t.Fatal("normalization exceeded the storage limit")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var files orgsync.FileConfig
		if err := json.Unmarshal(stored, &files); err != nil {
			t.Fatal(err)
		}
		if files.Files[0].Content != body+"\n" {
			t.Fatal("stored template has no terminator")
		}
	}
}
