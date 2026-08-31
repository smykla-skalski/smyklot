package panelrenderbridge_test

import (
	"testing"

	"github.com/smykla-skalski/smyklot/internal/panelrenderbridge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestRenderReturnsExactGoBytesAndBothPolicies(t *testing.T) {
	t.Parallel()
	newline := "insert"
	answer := panelrenderbridge.Render(panelrenderbridge.Request{
		Version: panelrenderbridge.ProtocolVersion, ID: "one", Path: "settings.json",
		DraftContent: `{"enabled":true}`, BaseFormatting: config.DefaultFormattingPolicy(),
		Layers: []panelrenderbridge.Layer{{
			Source: config.SourceTemplate,
			Formatting: config.FormattingPatch{Common: &config.FormattingCommonPatch{
				FinalNewline: &newline,
			}},
		}},
		InheritedLayers: 0,
	})
	if !answer.Valid {
		t.Fatalf("render failed: %+v", answer.Diagnostics)
	}
	if answer.FinalContent != "{\"enabled\":true}\n" {
		t.Fatalf("final content = %q", answer.FinalContent)
	}
	if answer.InheritedPolicy.Common.FinalNewline != "preserve" {
		t.Fatal("inherited policy included the current layer")
	}
	if answer.EffectivePolicy.Common.FinalNewline != "insert" {
		t.Fatal("effective policy omitted the current layer")
	}
}

func TestInvalidPolicyStillReturnsACompleteBridgeResponse(t *testing.T) {
	t.Parallel()
	policy := config.DefaultFormattingPolicy()
	policy.Common.LineWidth = 1
	answer := panelrenderbridge.Render(panelrenderbridge.Request{
		Version: panelrenderbridge.ProtocolVersion, ID: "invalid", Path: "README.md",
		DraftContent: "# Read me", BaseFormatting: policy,
	})
	if answer.Valid || len(answer.Diagnostics) != 1 || answer.Diagnostics[0].Code != "invalid_policy" {
		t.Fatalf("expected invalid policy response, got %+v", answer)
	}
	if answer.InheritedPolicy.Preset != "preserve" || answer.Provenance.Preset == "" {
		t.Fatal("invalid response omitted its parseable policy resolution")
	}
}
