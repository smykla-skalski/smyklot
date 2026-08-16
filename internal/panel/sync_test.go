package panel

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

// TestSyncConfigReportsADocumentItCannotRead is the guard on the difference
// between "nothing is configured" and "nothing could be read".
//
// They render the same - an empty list - and the panel is where somebody then
// presses save. A save built from an invented empty form sends that emptiness
// back and wipes a label set nobody was ever shown, so the two have to be told
// apart on the wire rather than in a comment.
func TestSyncConfigReportsADocumentItCannotRead(t *testing.T) {
	stored := orgsync.Config{
		Kind:      orgsync.KindLabels,
		Enabled:   true,
		Document:  []byte(`{"labels": [ this is not json`),
		Digest:    "digest-1",
		Revision:  3,
		UpdatedAt: time.Now().UTC(),
	}

	dto := syncConfigToDTO(stored)

	if !dto.Unreadable {
		t.Error("a document that does not decode was reported as readable")
	}
	if len(dto.Labels) != 0 {
		t.Errorf("labels = %d, wanted none: nothing could be read out of the document",
			len(dto.Labels))
	}

	// The rest still describes the row, because the row is still there and the
	// revision is what a later save would have to match.
	if dto.Revision != stored.Revision {
		t.Errorf("revision = %d, wanted %d", dto.Revision, stored.Revision)
	}
	if !dto.Enabled {
		t.Error("enabled was dropped, though it is a column rather than part of the document")
	}
}

// TestSyncConfigReadsADocumentItCan is the other half: a readable document must
// not be reported as unreadable, or the panel would refuse to edit a
// configuration that is perfectly fine.
func TestSyncConfigReadsADocumentItCan(t *testing.T) {
	dto := syncConfigToDTO(orgsync.Config{
		Kind:     orgsync.KindLabels,
		Document: []byte(`{"labels":[{"name":"bug","color":"d73a4a"}],"excludes":["ci/*"]}`),
	})

	if dto.Unreadable {
		t.Fatal("a document that decodes was reported as unreadable")
	}
	if len(dto.Labels) != 1 || dto.Labels[0].Name != "bug" {
		t.Errorf("labels = %v, wanted the one that was stored", dto.Labels)
	}
	if len(dto.Excludes) != 1 || dto.Excludes[0] != "ci/*" {
		t.Errorf("excludes = %v, wanted the one that was stored", dto.Excludes)
	}
}

// TestSyncDocumentRefusesSettingsGitHubWould is the panel half of the settings
// rules: a value GitHub answers with a 422 is refused beside the field somebody
// typed rather than in a plan that fails later against somebody's repositories.
func TestSyncDocumentRefusesSettingsGitHubWould(t *testing.T) {
	for _, invalid := range []struct {
		name     string
		document string
	}{
		{"a wording nobody defined", `{"merge_commit_title":"NONSENSE"}`},
		{"no way of merging at all", `{"allow_merge_commit":false,` +
			`"allow_squash_merge":false,"allow_rebase_merge":false}`},
		{"a wording its own strategy forbids", `{"allow_squash_merge":false,` +
			`"squash_merge_commit_title":"PR_TITLE"}`},
		{"a key this version does not know", `{"allow_forking":true}`},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			_, err := syncDocumentFor(orgsync.KindSettings, syncConfigRequest{
				Document: []byte(invalid.document),
			})
			if err == nil {
				t.Fatalf("%s was accepted", invalid.name)
			}
		})
	}
}

// TestSyncDocumentStoresTheSettingsType keeps what is stored the type the
// planner decodes.
//
// A second shape here is what made every configured exclusion a silent no-op in
// the kind before this one: the panel wrote one document and the planner decoded
// another, which had no field for them.
func TestSyncDocumentStoresTheSettingsType(t *testing.T) {
	document, err := syncDocumentFor(orgsync.KindSettings, syncConfigRequest{
		Document: []byte(`{"has_wiki":false,"allow_squash_merge":true}`),
	})
	if err != nil {
		t.Fatalf("a settings document GitHub accepts was refused: %v", err)
	}

	var stored orgsync.SettingsConfig
	if err := json.Unmarshal(document, &stored); err != nil {
		t.Fatalf("what was stored is not a settings configuration: %v", err)
	}
	if stored.HasWiki == nil || *stored.HasWiki {
		t.Error("has_wiki did not survive the round trip")
	}
	if stored.AllowSquashMerge == nil || !*stored.AllowSquashMerge {
		t.Error("allow_squash_merge did not survive the round trip")
	}
}

// TestSyncConfigNeverAnswersNullLists guards the shape rather than the values.
// A JSON null where the browser expects a list is a crash in the view, and an
// installation that has configured nothing is the ordinary case.
func TestSyncConfigNeverAnswersNullLists(t *testing.T) {
	dto := syncConfigToDTO(orgsync.Config{Kind: orgsync.KindLabels, Document: []byte(`{}`)})

	if dto.Labels == nil {
		t.Error("labels came back null rather than empty")
	}
	if dto.Excludes == nil {
		t.Error("excludes came back null rather than empty")
	}
}
