package panel

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

// TestSyncDocumentRefusesFilesGitHubOrGitWould keeps the answer beside the
// field somebody typed rather than in a sweep log an hour later.
func TestSyncDocumentRefusesFilesGitHubOrGitWould(t *testing.T) {
	for _, invalid := range []struct {
		name     string
		document string
	}{
		{"a file with no path", `{"files":[{"path":"","content":"x"}]}`},
		{"a file with nothing in it", `{"files":[{"path":"README.md","content":""}]}`},
		{"a path that climbs out", `{"files":[{"path":"../x","content":"x"}]}`},
		{"a path anchored at the root", `{"files":[{"path":"/etc/x","content":"x"}]}`},
		{"two files a checkout could not tell apart", `{"files":[
			{"path":"Readme.md","content":"a"},{"path":"README.md","content":"b"}]}`},
		{"a placeholder nothing fills in", `{"files":[
			{"path":"README.md","content":"See {{REPO}}."}]}`},
		{"a path written and retired at once", `{
			"files":[{"path":"a.md","content":"x"}],"retired":["a.md"]}`},
		{"a key this version does not know", `{
			"files":[{"path":"a.md","content":"x"}],"delete_everything":true}`},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			_, err := syncDocumentFor(orgsync.KindFiles, syncConfigRequest{
				Document: []byte(invalid.document),
			})
			if err == nil {
				t.Fatalf("%s was accepted", invalid.name)
			}
		})
	}
}

// TestSyncDocumentStoresTheFilesType keeps what is stored the type the planner
// decodes. Two shapes is how chunk 3's exclusions came to be saved and never
// read.
func TestSyncDocumentStoresTheFilesType(t *testing.T) {
	document, err := syncDocumentFor(orgsync.KindFiles, syncConfigRequest{
		Document: []byte(`{"files":[{"path":"CONTRIBUTING.md","content":"# Contributing\n"}],
			"retired":[".github/workflows/sync-trigger.yml"],"excludes":["LICENSE"]}`),
	})
	if err != nil {
		t.Fatalf("a file document git accepts was refused: %v", err)
	}

	var stored orgsync.FileConfig
	if err := json.Unmarshal(document, &stored); err != nil {
		t.Fatalf("what was stored is not a file configuration: %v", err)
	}

	if len(stored.Files) != 1 || stored.Files[0].Path != "CONTRIBUTING.md" {
		t.Errorf("files = %v, wanted the one that was sent", stored.Files)
	}
	if stored.Files[0].Content != "# Contributing\n" {
		t.Errorf("content = %q, wanted what was sent", stored.Files[0].Content)
	}
	if len(stored.Retired) != 1 {
		t.Errorf("retired = %v, wanted the one that was sent", stored.Retired)
	}
	if len(stored.Excludes) != 1 {
		t.Errorf("excludes = %v, wanted the one that was sent", stored.Excludes)
	}
}

// TestSyncOverrideReadsARepositoryThatHasNeverAnswered is the same guard the
// installation's own configuration carries: a repository that has said nothing
// and one that said no are different, and the browser gets one shape either
// way.
func TestSyncOverrideReadsARepositoryThatHasNeverAnswered(t *testing.T) {
	dto := syncOverrideToDTO(orgsync.KindFiles, nil)

	if dto.Enabled != nil {
		t.Errorf("enabled = %v, wanted nothing: this repository has never answered", *dto.Enabled)
	}
	if string(dto.Document) != "{}" {
		t.Errorf("document = %s, wanted an empty object rather than null", dto.Document)
	}
	if dto.Revision != 0 {
		t.Errorf("revision = %d, wanted none", dto.Revision)
	}
}

// TestSyncOverrideReportsADocumentItCannotRead is what keeps a row this version
// cannot decode from rendering as a repository that adjusts nothing - which is
// what somebody would then save over.
func TestSyncOverrideReportsADocumentItCannotRead(t *testing.T) {
	dto := syncOverrideToDTO(orgsync.KindFiles, &orgsync.RepositoryOverride{
		Kind:      orgsync.KindFiles,
		Document:  []byte(`{"merges": [ this is not json`),
		Revision:  4,
		UpdatedAt: time.Now().UTC(),
	})

	if !dto.Unreadable {
		t.Error("a document that does not decode was reported as readable")
	}
	if string(dto.Document) != "{}" {
		t.Errorf("document = %s, wanted an empty object", dto.Document)
	}
	if dto.Revision != 4 {
		t.Errorf("revision = %d, wanted 4: the row is still there", dto.Revision)
	}
}

func TestSyncOverrideKeepsWhatARepositoryAdjusts(t *testing.T) {
	document := []byte(`{"merges":[{"path":"renovate.json"}]}`)

	dto := syncOverrideToDTO(orgsync.KindFiles, &orgsync.RepositoryOverride{
		Kind: orgsync.KindFiles, Document: document, Revision: 2,
	})

	if dto.Unreadable {
		t.Error("a document that decodes was reported as unreadable")
	}
	if string(dto.Document) != string(document) {
		t.Errorf("document = %s, wanted %s", dto.Document, document)
	}
}
