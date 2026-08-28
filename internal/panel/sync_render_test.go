package panel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestSyncFileRenderReturnsBackendExactBytes(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	policy := config.DefaultFormattingPolicy()
	policy.JSON.Arrays = "compact"
	response := postSyncFileRender(t, harness, syncFileRenderRequest{
		Path: "renovate.json", DraftContent: "{\n  \"labels\": [\n    \"one\",\n    \"two\"\n  ]\n}\n",
		BasePolicy: policy,
	})

	if response.Valid != true {
		t.Fatalf("render was invalid: %#v", response.Diagnostics)
	}
	if response.Content != "{\n  \"labels\": [\"one\", \"two\"]\n}\n" {
		t.Fatalf("rendered content = %q", response.Content)
	}
	if !response.Changed {
		t.Fatal("render did not report its byte change")
	}
}

func TestSyncFileRenderAppliesOrderedTypedOverlays(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	compact, preserve := "compact", "preserve"
	policy := config.DefaultFormattingPolicy()
	policy.JSON.Arrays = "expanded"
	response := postSyncFileRender(t, harness, syncFileRenderRequest{
		Path: "renovate.json", DraftContent: `{"labels":["one","two"]}`,
		BasePolicy: policy,
		Overlays: []config.FormattingPatch{
			{JSON: &config.FormattingJSONPatch{Arrays: &compact}},
			{JSON: &config.FormattingJSONPatch{Arrays: &preserve}},
		},
	})

	if !response.Valid || response.Content != `{"labels":["one","two"]}` {
		t.Fatalf("ordered overlay render = %#v", response)
	}
	if response.Changed {
		t.Fatal("explicit preserve changed the draft")
	}
}

func TestSyncFileRenderReturnsStructuredPolicyDiagnostics(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	policy := config.DefaultFormattingPolicy()
	policy.Common.LineWidth = 1
	response := postSyncFileRender(t, harness, syncFileRenderRequest{
		Path: "renovate.json", DraftContent: `{}`, BasePolicy: policy,
	})

	if response.Valid || len(response.Diagnostics) != 1 {
		t.Fatalf("invalid policy response = %#v", response)
	}
	if response.Diagnostics[0].Code != "invalid_policy" {
		t.Fatalf("diagnostic code = %q", response.Diagnostics[0].Code)
	}
}

func TestSyncFileRenderLeavesRepositoryPlaceholderForSharedTemplate(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	policy := config.DefaultFormattingPolicy()
	response := postSyncFileRender(t, harness, syncFileRenderRequest{
		Path: "README.md", DraftContent: "Use {{DEFAULT_BRANCH}}.\n", BasePolicy: policy,
	})

	if !response.Valid || response.Content != "Use {{DEFAULT_BRANCH}}.\n" || response.Changed {
		t.Fatalf("shared template render = %#v", response)
	}
	branch := "trunk"
	response = postSyncFileRender(t, harness, syncFileRenderRequest{
		Path: "README.md", DraftContent: "Use {{DEFAULT_BRANCH}}.\n", BasePolicy: policy,
		DefaultBranch: &branch,
	})
	if !response.Valid || response.Content != "Use trunk.\n" || !response.Changed {
		t.Fatalf("repository template render = %#v", response)
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
