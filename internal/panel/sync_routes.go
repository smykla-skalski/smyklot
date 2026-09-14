package panel

import "net/http"

func (s *Server) registerSyncHistoryRoutes(mux *http.ServeMux, base string) {
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/sync/plans", s.getSyncHistory)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/sync/requests", s.getSyncRequests)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/sync/checks/{check}", s.getSyncCheck)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/sync/checks/{check}/observations", s.getSyncCheckObservations)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/sync/plans/{plan}", s.getSyncPlan)
}
