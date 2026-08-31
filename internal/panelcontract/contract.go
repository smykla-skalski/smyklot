// Package panelcontract owns the wire contract for backend-rendered sync files.
package panelcontract

//go:generate go run github.com/smykla-skalski/smyklot/internal/panelcontract/cmd/generate

import (
	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// SyncFileRenderInput carries only drafts owned by the current editor. Stored layers
// and repository facts are loaded by the backend that renders the answer.
type SyncFileRenderInput struct {
	Path               string                         `json:"path"`
	DraftContent       string                         `json:"draft_content"`
	TemplateFormatting config.FormattingPatch         `json:"template_formatting"`
	Repository         *SyncFileRenderRepositoryInput `json:"repository,omitempty"`
}

// SyncFileRenderRepositoryInput selects one repository output and its two editable
// parts. Repository configuration and its default branch never cross this
// request boundary.
type SyncFileRenderRepositoryInput struct {
	ID             string                 `json:"id"`
	Merge          *filemerge.Spec        `json:"merge,omitempty"`
	PathFormatting config.FormattingPatch `json:"path_formatting"`
}

// SyncFileRenderDiagnostic explains which pipeline stage refused a render.
type SyncFileRenderDiagnostic struct {
	Stage   string `json:"stage"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// LayerState says whether one named layer is absent, persisted, being edited,
// the process baseline, or deliberately bypassed.
type LayerState string

const (
	LayerBaseline LayerState = "baseline"
	LayerStored   LayerState = "stored"
	LayerDraft    LayerState = "draft"
	LayerAbsent   LayerState = "absent"
	LayerBypassed LayerState = "bypassed"
)

// SyncFileFormattingLayer is one visible step in the precedence chain.
type SyncFileFormattingLayer struct {
	Source     config.Source           `json:"source"`
	State      LayerState              `json:"state"`
	Formatting *config.FormattingPatch `json:"formatting,omitempty"`
	ConfigPath string                  `json:"config_path,omitempty"`
}

// SyncFileFormattingResolution carries both the value inherited by the editor and
// the final result after its sparse draft layer.
type SyncFileFormattingResolution struct {
	CurrentLayer    config.Source             `json:"current_layer"`
	InheritedPolicy config.FormattingPolicy   `json:"inherited_policy"`
	EffectivePolicy config.FormattingPolicy   `json:"effective_policy"`
	Provenance      config.FormattingSources  `json:"provenance"`
	Layers          []SyncFileFormattingLayer `json:"layers"`
}

// SyncFileRenderResponse is the exact backend preview and the policy that produced it.
type SyncFileRenderResponse struct {
	Valid             bool                          `json:"valid"`
	FinalContent      string                        `json:"final_content"`
	MatchesFormatting bool                          `json:"matches_formatting"`
	Diagnostics       []SyncFileRenderDiagnostic    `json:"diagnostics"`
	Formatting        *SyncFileFormattingResolution `json:"formatting,omitempty"`
}

// ConfigSources is the complete source vocabulary accepted on this wire.
func ConfigSources() []config.Source {
	return []config.Source{
		config.SourceProcess,
		config.SourceTarget,
		config.SourceRepositoryFile,
		config.SourceRepositoryPanel,
		config.SourceTemplate,
		config.SourceRepositoryPath,
	}
}

// LayerStates is the complete layer-state vocabulary accepted on this wire.
func LayerStates() []LayerState {
	return []LayerState{LayerBaseline, LayerStored, LayerDraft, LayerAbsent, LayerBypassed}
}
