package panel

import (
	"net/http"
	"time"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func (s *Server) getScheduleTimezone(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireViewer(w, r); !ok {
		return
	}
	at, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("at"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_instant", "Choose a date and time with an explicit UTC offset")
		return
	}
	preview, err := workqueue.PreviewTimezone(r.URL.Query().Get("timezone"), at)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_timezone", "Choose a timezone supported by the scheduler")
		return
	}
	writeJSON(w, http.StatusOK, preview)
}
