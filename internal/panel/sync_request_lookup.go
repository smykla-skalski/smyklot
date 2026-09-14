package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// Read recovery works after command permission is lost. A missing acceptance is
// not proof that an in-flight command will never be accepted; this endpoint
// never creates a request or substitutes a different key.
func (s *Server) getSyncRequest(w http.ResponseWriter, r *http.Request) {
	account, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	action, key := r.PathValue("action"), r.PathValue("request")
	if r.URL.RawQuery != "" || (action != orgsync.RequestActionCheck && action != orgsync.RequestActionDispatch) || key == "" || len(key) > 200 || strings.TrimSpace(key) != key {
		s.writeError(w, http.StatusBadRequest, "invalid_request_lookup", "An exact check or dispatch request identity is required")
		return
	}
	body, err := s.readSyncOperation(r.Context(), orgsync.RequestLookup{ActorID: account.ID, TargetID: target.ID, SessionTokenHash: syncRequestSessionHash(r), Action: action, RequestKey: key})
	if errors.Is(err, storage.ErrRevoked) || errors.Is(err, storage.ErrExpired) {
		s.writeError(w, http.StatusForbidden, "access_revoked", "Your access changed; refresh the workspace to read your accepted request")
		return
	}
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, body)
}
