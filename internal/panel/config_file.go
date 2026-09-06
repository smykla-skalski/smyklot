package panel

import (
	"net/http"

	"github.com/smykla-skalski/smyklot/internal/configsync"
)

func (s *Server) registerConfigFileRoutes(mux *http.ServeMux, base string) {
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/config-file", s.getConfigFileStatus)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/repositories/{repository}/config-file", s.getConfigFileStatus)
	mux.HandleFunc("GET "+base+"/api/v1/root/workspaces/{target}/config-file", s.getRootConfigFileStatus)
	mux.HandleFunc("GET "+base+"/api/v1/root/workspaces/{target}/repositories/{repository}/config-file", s.getRootConfigFileStatus)
}

func (s *Server) getConfigFileStatus(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	s.writeConfigFileStatus(w, r, target.ID)
}

func (s *Server) getRootConfigFileStatus(w http.ResponseWriter, r *http.Request) {
	context, ok := s.requireRootTarget(w, r, false)
	if !ok {
		return
	}
	s.writeConfigFileStatus(w, r, context.Target.ID)
}

func (s *Server) writeConfigFileStatus(w http.ResponseWriter, r *http.Request, targetID string) {
	answer, err := (configsync.Engine{Store: s.store}).ReadStatus(r.Context(), targetID, r.PathValue("repository"))
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, answer)
}
