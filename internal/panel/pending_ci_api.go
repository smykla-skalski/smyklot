package panel

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

type pendingCIChangeRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
}

func (s *Server) getRootPendingCI(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	id, ok := s.pendingCIRequestID(w, r)
	if !ok {
		return
	}
	request, err := s.store.Get(r.Context(), id)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	events, err := s.store.ListEvents(
		r.Context(), pendingci.EventFilter{RequestID: id, Limit: 200},
	)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pendingCIDetailResponse{
		Request: pendingCIQueueDTO([]pendingci.Request{request})[0],
		Events:  pendingCIEventsDTO(events),
	})
}

func (s *Server) postRootPendingCICheck(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	id, revision, ok := s.pendingCIChange(w, r)
	if !ok {
		return
	}
	now := s.now().UTC()
	request, err := s.pendingCI.CheckNow(r.Context(), pendingci.CheckNowRequest{
		ID: id, ExpectedRevision: revision,
		EventKey:   fmt.Sprintf("panel:%s:check:%d", account.ID, now.UnixNano()),
		OccurredAt: now,
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.announcePendingCIChange()
	writeJSON(w, http.StatusOK, pendingCIQueueDTO([]pendingci.Request{request})[0])
}

func (s *Server) deleteRootPendingCI(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	id, revision, ok := s.pendingCIChange(w, r)
	if !ok {
		return
	}
	now := s.now().UTC()
	request, err := s.pendingCI.Cancel(r.Context(), pendingci.FinishRequest{
		ID: id, ExpectedRevision: revision, Lifecycle: pendingci.LifecycleCancelled,
		Trigger: pendingci.TriggerManual,
		Reason:  "cancelled by panel user @" + account.Login, FinishedAt: now,
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.announcePendingCIChange()
	writeJSON(w, http.StatusOK, pendingCIQueueDTO([]pendingci.Request{request})[0])
}

func (s *Server) pendingCIChange(
	w http.ResponseWriter,
	r *http.Request,
) (int64, int64, bool) {
	id, ok := s.pendingCIRequestID(w, r)
	if !ok {
		return 0, 0, false
	}
	var input pendingCIChangeRequest
	if !decodeJSON(w, r, &input) {
		return 0, 0, false
	}
	if input.ExpectedRevision == nil || *input.ExpectedRevision <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "expected_revision is required")
		return 0, 0, false
	}

	return id, *input.ExpectedRevision, true
}

func (s *Server) pendingCIRequestID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("request"), 10, 64)
	if err != nil || id <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "pending CI request id is invalid")
		return 0, false
	}

	return id, true
}

func (s *Server) announcePendingCIChange() {
	s.events.announce(panelEvent{Type: panelEventResync})
}
