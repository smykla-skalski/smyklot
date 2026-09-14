package panel

import (
	"net/http"
	"strconv"
)

func (s *Server) registerQueueRoutes(mux *http.ServeMux, base string) {
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/queue", s.getTargetQueue)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/queue/{queue}", s.getTargetQueueItem)
	mux.HandleFunc("POST "+base+"/api/v1/targets/{target}/queue/{queue}/actions/preview", s.previewTargetQueueAction)
	mux.HandleFunc("POST "+base+"/api/v1/targets/{target}/queue/{queue}/actions", s.postTargetQueueAction)
	for _, route := range []struct {
		path string
		root bool
	}{
		{base + "/api/v1/targets/{target}/deliveries/{delivery}/recovery", false},
		{base + "/api/v1/root/workspaces/{target}/deliveries/{delivery}/recovery", true},
	} {
		mux.HandleFunc("GET "+route.path, func(w http.ResponseWriter, r *http.Request) { s.getDeliveryRecovery(w, r, route.root) })
		mux.HandleFunc("POST "+route.path, func(w http.ResponseWriter, r *http.Request) { s.postDeliveryRecovery(w, r, route.root) })
	}
}

func (s *Server) requireDeliveryRecovery(w http.ResponseWriter, r *http.Request, root, write bool) (rootTargetContext, int64, bool) {
	var access rootTargetContext
	var ok bool
	if root {
		access, ok = s.requireRootTarget(w, r, write)
	} else {
		access.Account, access.Target, access.Access, ok = s.requireTarget(w, r, false)
		if ok {
			_, access.SessionHash, ok = s.requireViewerSession(w, r)
		}
	}
	if !ok {
		return access, 0, false
	}
	if write && !canRecoverDelivery(access) {
		s.writeError(w, http.StatusForbidden, "forbidden", "Admin or Owner access is required.")
		return access, 0, false
	}
	id, err := strconv.ParseInt(r.PathValue("delivery"), 10, 64)
	if err != nil || id <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid_delivery", "A valid delivery is required.")
		return access, 0, false
	}
	return access, id, true
}
