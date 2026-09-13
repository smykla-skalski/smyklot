package panel

import (
	"errors"
	"net/http"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// The caller authorizes the current actor and target before any receipt lookup.
// Acceptance identifies the check; it does not claim the check is still queued.
func (s *Server) answerAcceptedSyncCheck(w http.ResponseWriter, r *http.Request, request workqueue.RecurringRequest) bool {
	item, err := s.store.FindRecurringWorkRequest(r.Context(), request)
	if errors.Is(err, storage.ErrNotFound) {
		return false
	}
	if err != nil {
		s.writeStorageError(w, err)
		return true
	}
	writeJSON(w, http.StatusOK, syncRunNowResponse{Status: "check_accepted", CheckID: item.ID, Repeated: true})
	return true
}

func (s *Server) handleSyncCheck(w http.ResponseWriter, r *http.Request, account storage.Account, target storage.Target, role storage.InstallationRole, input syncRunNowInput) {
	request := workqueue.RecurringRequest{
		RequestKey: input.RequestKey,
		Kind:       workqueue.KindSyncScan, TargetID: &target.ID,
		Title: "Check which repositories are in step", ActorID: account.ID,
		Reason: input.Reason, Now: s.now().UTC(),
	}
	if input.RequestKey != "" && s.answerAcceptedSyncCheck(w, r, request) {
		return
	}
	plan, actions, err := s.store.GetLiveSyncPlan(r.Context(), target.ID)
	if err == nil {
		dto, err := s.syncPlanDTO(r.Context(), plan, actions, role)
		if err != nil {
			s.writeStorageError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, syncRunNowResponse{Status: "changes_pending", Plan: &dto})
		return
	}
	if !errors.Is(err, storage.ErrNotFound) {
		s.writeStorageError(w, err)
		return
	}
	item, err := s.store.RequestRecurringWork(r.Context(), request)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	prepareQueueItem(&item, true, false)
	s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: target.ID})
	s.wakeScheduledWork(workqueue.LaneMaintenance)
	if input.RequestKey != "" {
		writeJSON(w, http.StatusAccepted, syncRunNowResponse{Status: "check_accepted", CheckID: item.ID})
		return
	}
	writeJSON(w, http.StatusAccepted, syncRunNowResponse{Status: "scan_queued", Queue: &item})
}
