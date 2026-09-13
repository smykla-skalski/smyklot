package panel

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

const syncDispatchAction = "dispatch"

type syncRunNowInput struct {
	RequestKey       string `json:"request_key,omitempty"`
	Action           string `json:"action"`
	PlanID           string `json:"plan_id,omitempty"`
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type syncRunNowResponse struct {
	Status   string          `json:"status"`
	CheckID  string          `json:"check_id,omitempty"`
	PlanID   string          `json:"plan_id,omitempty"`
	QueueID  string          `json:"queue_id,omitempty"`
	Repeated bool            `json:"repeated,omitempty"`
	Plan     *syncPlanDTO    `json:"plan,omitempty"`
	Queue    *workqueue.Item `json:"queue_item,omitempty"`
}

// valid keeps checking repositories distinct from dispatching approved changes.
// A revision belongs to one plan, so it cannot identify a dispatch by itself.
func (input syncRunNowInput) valid() bool {
	if input.RequestKey == "" || len(input.RequestKey) > 200 || strings.TrimSpace(input.RequestKey) != input.RequestKey {
		return false
	}
	switch input.Action {
	case "check":
		return input.PlanID == "" && input.ExpectedRevision == 0
	case syncDispatchAction:
		return strings.TrimSpace(input.PlanID) != "" && input.PlanID == strings.TrimSpace(input.PlanID) && input.ExpectedRevision > 0
	default:
		return false
	}
}

func (s *Server) postSyncRunNow(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, access, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	if access.Role != storage.InstallationRoleAdmin && access.Role != storage.InstallationRoleOwner {
		s.writeError(w, http.StatusForbidden, "forbidden", "Admin or Owner access is required")
		return
	}
	var input syncRunNowInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" || !input.valid() {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "a check or exact plan dispatch and a reason are required")
		return
	}
	if input.Action == syncDispatchAction {
		request := dispatchRequest(account, target, input, s.now().UTC())
		receipt, err := s.store.FindSyncPlanDispatch(r.Context(), request)
		if err == nil {
			writeJSON(w, http.StatusOK, syncRunNowResponse{Status: "dispatch_accepted", PlanID: receipt.PlanID, QueueID: receipt.QueueID, Repeated: true})
			return
		}
		if !errors.Is(err, storage.ErrNotFound) {
			s.writeStorageError(w, err)
			return
		}
		plan, actions, err := s.store.GetSyncPlan(r.Context(), target.ID, input.PlanID)
		if err != nil {
			s.writeStorageError(w, err)
			return
		}
		s.handleSyncDispatch(w, r, account, target, access.Role, input, plan, actions)
		return
	}
	s.handleSyncCheck(w, r, account, target, access.Role, input)
}

func (s *Server) handleSyncDispatch(
	w http.ResponseWriter,
	r *http.Request,
	account storage.Account,
	target storage.Target,
	role storage.InstallationRole,
	input syncRunNowInput,
	plan orgsync.Plan,
	actions []orgsync.Action,
) {
	dto, err := s.syncPlanDTO(r.Context(), plan, actions, role)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	switch dto.Dispatch.Reason {
	case string(orgsync.DispatchApprovalRequired):
		writeJSON(w, http.StatusOK, syncRunNowResponse{Status: "approval_required", Plan: &dto})
	case string(orgsync.DispatchAlreadyRunning):
		writeJSON(w, http.StatusOK, syncRunNowResponse{Status: "already_running", Plan: &dto})
	case string(orgsync.DispatchAvailable):
		if dto.Queue == nil || input.ExpectedRevision != dto.Queue.Revision {
			writeJSON(w, http.StatusConflict, map[string]any{
				jsonFieldCode: errorCodeStaleRevision, jsonFieldMessage: "sync queue item changed; review the latest state",
				jsonFieldCurrent: dto,
			})
			return
		}
		receipt, actionErr := s.store.DispatchSyncPlan(r.Context(), dispatchRequest(account, target, input, s.now().UTC()))
		if actionErr != nil {
			s.writeStorageError(w, actionErr)
			return
		}
		s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: target.ID})
		s.wakeScheduledWork(workqueue.LaneMaintenance)
		writeJSON(w, http.StatusAccepted, syncRunNowResponse{Status: "dispatch_accepted", PlanID: receipt.PlanID, QueueID: receipt.QueueID})
	default:
		writeJSON(w, http.StatusConflict, map[string]any{
			"code": "unsupported_plan_state", "message": "these changes cannot run now", syncDispatchAction: dto.Dispatch,
		})
	}
}

func dispatchRequest(account storage.Account, target storage.Target, input syncRunNowInput, now time.Time) orgsync.PlanDispatch {
	return orgsync.PlanDispatch{TargetID: target.ID, PlanID: input.PlanID, ActorID: account.ID, RequestKey: input.RequestKey, ExpectedRevision: input.ExpectedRevision, Reason: input.Reason, Now: now}
}
