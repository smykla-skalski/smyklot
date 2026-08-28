package panel

import (
	"bytes"
	"net/http"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

type syncFileRenderRequest struct {
	Path          string                   `json:"path"`
	DraftContent  string                   `json:"draft_content"`
	Merge         *filemerge.Spec          `json:"merge,omitempty"`
	DefaultBranch *string                  `json:"default_branch,omitempty"`
	BasePolicy    config.FormattingPolicy  `json:"base_policy"`
	Overlays      []config.FormattingPatch `json:"overlays,omitempty"`
}

type syncFileRenderDiagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type syncFileRenderResponse struct {
	Valid       bool                       `json:"valid"`
	Content     string                     `json:"content"`
	Changed     bool                       `json:"changed"`
	Diagnostics []syncFileRenderDiagnostic `json:"diagnostics"`
}

func (s *Server) postSyncFileRender(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	if _, _, _, ok := s.requireTarget(w, r, false); !ok {
		return
	}
	var input syncFileRenderRequest
	if !decodeJSONWithin(w, r, &input, maxDocumentBody) {
		return
	}
	if input.Path == "" {
		writeJSON(w, http.StatusOK, invalidSyncFileRender("invalid_path", "a file path is required"))

		return
	}
	if err := input.BasePolicy.AsPatch().Validate(); err != nil {
		writeJSON(w, http.StatusOK, invalidSyncFileRender("invalid_policy", err.Error()))

		return
	}
	policy := input.BasePolicy
	for _, overlay := range input.Overlays {
		if err := overlay.Validate(); err != nil {
			writeJSON(w, http.StatusOK, invalidSyncFileRender("invalid_policy", err.Error()))

			return
		}
		policy = config.ApplyFormattingPatch(policy, overlay)
	}
	spec := filemerge.Spec{}
	if input.Merge != nil {
		spec = *input.Merge
	}
	draft := []byte(input.DraftContent)
	content := input.DraftContent
	if input.DefaultBranch != nil {
		content = orgsync.Render(content, *input.DefaultBranch)
	}
	rendered, err := filemerge.Apply(input.Path, []byte(content), spec, policy)
	if err != nil {
		writeJSON(w, http.StatusOK, invalidSyncFileRender("invalid_document", err.Error()))

		return
	}
	writeJSON(w, http.StatusOK, syncFileRenderResponse{
		Valid: true, Content: string(rendered), Changed: !bytes.Equal(draft, rendered),
		Diagnostics: []syncFileRenderDiagnostic{},
	})
}

func invalidSyncFileRender(code, message string) syncFileRenderResponse {
	return syncFileRenderResponse{
		Valid:       false,
		Diagnostics: []syncFileRenderDiagnostic{{Code: code, Message: message}},
	}
}
