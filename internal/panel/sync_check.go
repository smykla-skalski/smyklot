package panel

import (
	"errors"
	"net/http"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// A retained comparison is not a claim about successful worker execution.
// Null result means no retained comparison; null execution means no worker
// record is available. Neither absence establishes why the record is missing.
type syncCheckResponse struct {
	CheckID    string                `json:"check_id"`
	TargetID   string                `json:"target_id"`
	ObservedAt time.Time             `json:"observed_at"`
	Result     *orgsync.CheckDetails `json:"result"`
	Execution  *syncCheckExecution   `json:"execution"`
	Check      syncCheckCapability   `json:"check"`
}

type syncCheckExecution struct {
	State           workqueue.State `json:"state"`
	Summary         string          `json:"summary"`
	ProgressCurrent int             `json:"progress_current"`
	ProgressTotal   int             `json:"progress_total"`
	Attempt         int             `json:"attempt"`
	StartedAt       *time.Time      `json:"started_at"`
	FinishedAt      *time.Time      `json:"finished_at"`
}

func (s *Server) getSyncCheck(w http.ResponseWriter, r *http.Request) {
	_, target, access, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	body := syncCheckResponse{CheckID: r.PathValue("check"), TargetID: target.ID}
	item, err := s.store.GetQueueItem(r.Context(), body.CheckID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		s.writeStorageError(w, err)
		return
	}
	if err == nil && item.Kind == workqueue.KindSyncScan && item.TargetID != nil && *item.TargetID == target.ID {
		body.Execution = syncExecutionFacts(item)
	}
	result, err := s.store.GetSyncCheckResult(r.Context(), target.ID, body.CheckID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		s.writeStorageError(w, err)
		return
	}
	if err == nil {
		body.Result = &result
	}
	if body.Result == nil && body.Execution == nil {
		s.writeStorageError(w, storage.ErrNotFound)
		return
	}
	body.Check, err = s.currentSyncCheckCapability(r.Context(), target.ID, access.Role)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	body.ObservedAt = s.now().UTC()
	writeJSON(w, http.StatusOK, body)
}
