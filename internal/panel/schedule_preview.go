package panel

import (
	"net/http"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func (s *Server) postSchedulePreview(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	if _, ok := s.requireViewer(w, r); !ok {
		return
	}
	var input struct {
		Date    string               `json:"date"`
		Profile scheduleProfileInput `json:"profile"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	preview, err := workqueue.PreviewDate(workqueue.Profile{
		ID: "preview", Name: input.Profile.Name, Timezone: input.Profile.Timezone,
		Windows: input.Profile.Windows, Exceptions: input.Profile.Exceptions,
	}, input.Date)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_schedule", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, preview)
}
