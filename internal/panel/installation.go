package panel

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) getInstallation(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireViewer(w, r); !ok {
		return
	}
	var destination *string
	if s.installation != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		resolved, err := s.installation.AppInstallationURL(ctx)
		if err != nil {
			s.writeError(w, http.StatusServiceUnavailable, "installation_lookup_failed", "Could not load the installation link. Try again.")
			return
		}
		if resolved != "" {
			destination = &resolved
		}
	}
	writeJSON(w, http.StatusOK, map[string]*string{"installation_url": destination})
}
