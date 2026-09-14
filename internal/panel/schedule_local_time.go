package panel

import (
	"net/http"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func (s *Server) getScheduleLocalTime(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireViewer(w, r); !ok {
		return
	}
	resolution, err := workqueue.ResolveLocalTime(r.URL.Query().Get("timezone"), r.URL.Query().Get("local_time"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_local_time", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resolution)
}
