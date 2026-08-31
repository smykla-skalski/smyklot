// Package panelrenderbridge adapts the authoritative Go file renderer to the
// development panel's persistent JSON-lines process.
package panelrenderbridge

import (
	"errors"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/internal/orgsync/filerender"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

const ProtocolVersion = 1

// Request is one render message. InheritedLayers identifies the layers below
// the browser's current editor, so the same run returns both policies.
type Request struct {
	Version         int                     `json:"version"`
	ID              string                  `json:"id"`
	Path            string                  `json:"path"`
	DraftContent    string                  `json:"draft_content"`
	DefaultBranch   *string                 `json:"default_branch,omitempty"`
	Merge           filemerge.Spec          `json:"merge"`
	BaseFormatting  config.FormattingPolicy `json:"base_formatting"`
	Layers          []Layer                 `json:"layers"`
	InheritedLayers int                     `json:"inherited_layers"`
}

// Layer is one named sparse policy overlay.
type Layer struct {
	Source     config.Source          `json:"source"`
	Formatting config.FormattingPatch `json:"formatting"`
}

// Diagnostic is a stable refusal that the mock returns over its HTTP wire.
type Diagnostic struct {
	Stage   string `json:"stage"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Response is one correlated render answer.
type Response struct {
	Version           int                      `json:"version"`
	ID                string                   `json:"id"`
	Valid             bool                     `json:"valid"`
	FinalContent      string                   `json:"final_content"`
	MatchesFormatting bool                     `json:"matches_formatting"`
	InheritedPolicy   config.FormattingPolicy  `json:"inherited_policy"`
	EffectivePolicy   config.FormattingPolicy  `json:"effective_policy"`
	Provenance        config.FormattingSources `json:"provenance"`
	Diagnostics       []Diagnostic             `json:"diagnostics"`
}

// Render validates and executes one bridge request.
func Render(request Request) Response {
	answer := baseResponse(request.ID)
	if request.Version != ProtocolVersion || request.ID == "" || request.Path == "" {
		return invalid(answer, "request", "invalid_request", "the render request is invalid")
	}
	if request.InheritedLayers < 0 || request.InheritedLayers > len(request.Layers) {
		return invalid(answer, "request", "invalid_request", "the inherited layer count is invalid")
	}
	if err := request.BaseFormatting.AsPatch().Validate(); err != nil {
		return invalid(answer, "policy", "invalid_policy", err.Error())
	}

	base := config.Default()
	base.Formatting = request.BaseFormatting
	layers := make([]config.Layer, 0, len(request.Layers))
	for _, layer := range request.Layers {
		if err := layer.Formatting.Validate(); err != nil {
			return invalid(answer, "policy", "invalid_policy", err.Error())
		}
		layers = append(layers, config.Layer{
			Source: layer.Source,
			Patch:  config.Patch{Formatting: &layer.Formatting},
		})
	}
	answer.InheritedPolicy = config.Resolve(base, layers[:request.InheritedLayers]...).Values.Formatting
	rendered, err := filerender.Render(filerender.Request{
		Path: request.Path, Draft: []byte(request.DraftContent),
		DefaultBranch: request.DefaultBranch, Merge: request.Merge,
		Base: base, Layers: layers,
	})
	answer.EffectivePolicy = rendered.Resolved.Values.Formatting
	answer.Provenance = rendered.Resolved.Formatting
	if err != nil {
		stage := "format"
		var applyError *filemerge.ApplyError
		if errors.As(err, &applyError) {
			stage = string(applyError.Stage)
		}
		return invalid(answer, stage, "invalid_document", err.Error())
	}
	answer.Valid = true
	answer.FinalContent = string(rendered.Final)
	answer.MatchesFormatting = rendered.MatchesFormatting
	return answer
}

// InvalidRequest returns a complete, parseable refusal for a malformed line so
// one browser mistake does not terminate the shared development renderer.
func InvalidRequest(id, message string) Response {
	return invalid(baseResponse(id), "request", "invalid_request", message)
}

func baseResponse(id string) Response {
	resolved := config.Resolve(config.Default())

	return Response{
		Version: ProtocolVersion, ID: id, Diagnostics: []Diagnostic{},
		InheritedPolicy: resolved.Values.Formatting,
		EffectivePolicy: resolved.Values.Formatting,
		Provenance:      resolved.Formatting,
	}
}

func invalid(answer Response, stage, code, message string) Response {
	answer.Diagnostics = []Diagnostic{{Stage: stage, Code: code, Message: message}}
	return answer
}
