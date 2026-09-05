package panel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestSyncFileRenderReturnsBackendExactBytesAndProvenance(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	saveRenderFile(t, harness, orgsync.File{
		Path: "renovate.json", Content: `{"labels":["one","two"]}`,
	})
	compact := "compact"
	response := postSyncFileRender(t, harness, syncFileRenderRequest{
		Path: "renovate.json", DraftContent: "{\n  \"labels\": [\n    \"one\",\n    \"two\"\n  ]\n}\n",
		TemplateFormatting: config.FormattingPatch{
			JSON: &config.FormattingJSONPatch{Arrays: &compact},
		},
	})

	if !response.Valid {
		t.Fatalf("render was invalid: %#v", response.Diagnostics)
	}
	if response.FinalContent != "{\n  \"labels\": [\"one\", \"two\"]\n}\n" {
		t.Fatalf("rendered content = %q", response.FinalContent)
	}
	if response.MatchesFormatting {
		t.Fatal("render reported a formatting match for changed bytes")
	}
	if response.Formatting == nil ||
		response.Formatting.CurrentLayer != config.SourceTemplate ||
		response.Formatting.Provenance.JSON.Arrays != config.SourceTemplate ||
		len(response.Formatting.Layers) != 3 {
		t.Fatalf("formatting resolution = %#v", response.Formatting)
	}
}

func TestSyncFileRenderAppliesNamedRepositoryPathAfterTemplate(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	saveRenderFile(t, harness, orgsync.File{
		Path: "renovate.json", Content: `{"labels":["one","two"]}`,
	})
	compact, preserve := "compact", "preserve"
	response := postSyncFileRender(t, harness, syncFileRenderRequest{
		Path: "renovate.json", DraftContent: `{"labels":["one","two"]}`,
		TemplateFormatting: config.FormattingPatch{
			JSON: &config.FormattingJSONPatch{Arrays: &compact},
		},
		Repository: &syncFileRenderRepositoryRequest{
			ID: "repository-20",
			PathFormatting: config.FormattingPatch{
				JSON: &config.FormattingJSONPatch{Arrays: &preserve},
			},
		},
	})

	if !response.Valid || response.FinalContent != "{\"labels\":[\"one\",\"two\"]}\n" {
		t.Fatalf("repository render = %#v", response)
	}
	if !response.MatchesFormatting {
		t.Fatal("explicit preserve did not report matching bytes")
	}
	if response.Formatting == nil ||
		response.Formatting.CurrentLayer != config.SourceRepositoryPath ||
		response.Formatting.Provenance.JSON.Arrays != config.SourceRepositoryPath ||
		response.Formatting.InheritedPolicy.JSON.Arrays != "compact" ||
		len(response.Formatting.Layers) != 6 {
		t.Fatalf("repository formatting resolution = %#v", response.Formatting)
	}
}

func TestSyncFileRenderSeparatesMergeFromFormattingMismatch(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	saveRenderFile(t, harness, orgsync.File{
		Path: "renovate.json", Content: `{"timezone":"UTC"}`,
	})
	response := postSyncFileRender(t, harness, syncFileRenderRequest{
		Path: "renovate.json", DraftContent: `{"timezone":"UTC"}`,
		Repository: &syncFileRenderRepositoryRequest{
			ID:    "repository-20",
			Merge: &filemerge.Spec{Overrides: json.RawMessage(`{"timezone":"Europe/Warsaw"}`)},
		},
	})

	if !response.Valid || !response.MatchesFormatting ||
		response.FinalContent != "{\"timezone\":\"Europe/Warsaw\"}\n" {
		t.Fatalf("merge-only render = %#v", response)
	}
}

func TestSyncFileRenderReturnsStructuredPolicyDiagnostics(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	width := 1
	response := postSyncFileRender(t, harness, syncFileRenderRequest{
		Path: "renovate.json", DraftContent: `{}`,
		TemplateFormatting: config.FormattingPatch{
			Common: &config.FormattingCommonPatch{LineWidth: &width},
		},
	})

	if response.Valid || len(response.Diagnostics) != 1 {
		t.Fatalf("invalid policy response = %#v", response)
	}
	if response.Diagnostics[0].Stage != "policy" ||
		response.Diagnostics[0].Code != "invalid_policy" {
		t.Fatalf("diagnostic = %#v", response.Diagnostics[0])
	}
}

func TestSyncFileRenderAcceptsUnsavedTemplate(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	saveRenderFile(t, harness, orgsync.File{Path: "README.md", Content: "Hello\n"})
	response := postSyncFileRender(t, harness, syncFileRenderRequest{
		Path: "other.json", DraftContent: `{}`,
	})
	if !response.Valid || response.FinalContent != "{}\n" || response.Formatting == nil {
		t.Fatalf("unsaved template response = %#v", response)
	}
}

func TestSyncFileRenderRejectsInvalidDestination(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	response := postSyncFileRender(t, harness, syncFileRenderRequest{
		Path: "../other.json", DraftContent: `{}`,
	})
	if response.Valid || len(response.Diagnostics) != 1 || response.Diagnostics[0].Code != "invalid_document" {
		t.Fatalf("invalid path response = %#v", response)
	}
}

func saveRenderFile(t *testing.T, harness *panelHarness, file orgsync.File) {
	t.Helper()
	type syncConfigInput struct {
		Kind             string             `json:"kind"`
		Enabled          bool               `json:"enabled"`
		ExpectedRevision int64              `json:"expected_revision"`
		Document         orgsync.FileConfig `json:"document"`
	}
	type batchInput struct {
		SyncConfigs []syncConfigInput `json:"sync_configs"`
	}
	document, err := json.Marshal(batchInput{SyncConfigs: []syncConfigInput{{
		Kind: string(orgsync.KindFiles), Enabled: true,
		Document: orgsync.FileConfig{Files: []orgsync.File{file}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	response := harness.request(
		t, http.MethodPut, workspaceSettingsBatchPath,
		bytes.NewReader(document), harness.signIn(t),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("saving render fixture = %d %s", response.Code, response.Body.String())
	}
}

func postSyncFileRender(
	t *testing.T,
	harness *panelHarness,
	request syncFileRenderRequest,
) syncFileRenderResponse {
	t.Helper()
	document, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response := harness.request(
		t,
		http.MethodPost,
		"/panel/api/v1/targets/"+panelSyncTarget+"/sync/files/render",
		bytes.NewReader(document),
		harness.signIn(t),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("render response = %d %s", response.Code, response.Body.String())
	}
	var answer syncFileRenderResponse
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}

	return answer
}
