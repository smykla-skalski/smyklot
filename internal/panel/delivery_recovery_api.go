package panel

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

type deliveryRecoveryResponse struct {
	Available    bool   `json:"available"`
	Reason       string `json:"reason"`
	Effect       string `json:"effect,omitempty"`
	Revision     int64  `json:"revision"`
	CurrentRunID int64  `json:"current_run_id,omitempty"`
}

type deliveryRecoveryInput struct {
	ExpectedRunID    int64  `json:"expected_run_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	RequestKey       string `json:"request_key"`
}

func (s *Server) deliveryRecoveryPreview(r *http.Request, access rootTargetContext, runID int64, expected *deliveryRecoveryInput) (deliveryRecoveryResponse, error) {
	operation, err := s.store.GetDeliveryOperation(r.Context(), access.Target.ID, runID)
	if errors.Is(err, storage.ErrNotFound) {
		return deliveryRecoveryResponse{Reason: "history_unavailable"}, nil
	}
	if err != nil {
		return deliveryRecoveryResponse{}, err
	}
	result := deliveryRecoveryResponse{Revision: operation.Revision, Reason: "history_unavailable"}
	if operation.Current == nil {
		return result, nil
	}
	current := operation.Current
	result.CurrentRunID = current.ID
	if expected != nil && (expected.ExpectedRunID != current.ID || expected.ExpectedRevision != operation.Revision) {
		result.Reason = "state_changed"
		return result, nil
	}
	switch current.Status {
	case storage.DeliveryRunning:
		result.Reason = "already_running"
		return result, nil
	case storage.DeliverySucceeded:
		result.Reason = "already_succeeded"
		return result, nil
	}
	if !current.PayloadAvailable {
		result.Reason = "payload_unavailable"
		return result, nil
	}
	if !canRecoverDelivery(access) {
		result.Reason = "access_required"
		return result, nil
	}
	if s.recovery == nil {
		result.Reason = "service_unavailable"
		return result, nil
	}
	input, err := s.store.GetDeliveryRecoveryInput(r.Context(), access.Target.ID, current.ID, operation.Revision)
	if errors.Is(err, storage.ErrNotFound) {
		result.Reason = "state_changed"
		return result, nil
	}
	if err != nil {
		return result, err
	}
	check, err := s.recovery.CheckDeliveryRecovery(r.Context(), input)
	if err != nil {
		return result, err
	}
	result.Available = check.Reason == RecoveryAvailable
	result.Reason = string(check.Reason)
	result.Effect = check.Effect
	return result, nil
}

func canRecoverDelivery(access rootTargetContext) bool {
	return access.Elevation != nil || (access.Access.Capabilities.Write && (access.Access.Role == storage.InstallationRoleOwner || access.Access.Role == storage.InstallationRoleAdmin))
}

func (s *Server) getDeliveryRecovery(w http.ResponseWriter, r *http.Request, root bool) {
	access, runID, ok := s.requireDeliveryRecovery(w, r, root, false)
	if !ok {
		return
	}
	preview, err := s.deliveryRecoveryPreview(r, access, runID, nil)
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, "verification_unavailable", "Recovery could not be checked. Try again.")
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) postDeliveryRecovery(w http.ResponseWriter, r *http.Request, root bool) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	access, runID, ok := s.requireDeliveryRecovery(w, r, root, true)
	if !ok {
		return
	}
	var input deliveryRecoveryInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.ExpectedRunID <= 0 || input.ExpectedRevision <= 0 || strings.TrimSpace(input.RequestKey) == "" || len(input.RequestKey) > 200 {
		s.writeError(w, http.StatusBadRequest, "invalid_recovery", "Current run, revision and request key are required.")
		return
	}
	request := storage.DeliveryRecovery{TargetID: access.Target.ID, SourceRunID: runID, ExpectedRunID: input.ExpectedRunID, ExpectedRevision: input.ExpectedRevision, RequestKey: input.RequestKey, ActorAccountID: access.Account.ID, SessionTokenHash: access.SessionHash, ElevationID: elevationID(access.Elevation), RequestedAt: s.now().UTC()}
	receipt, err := s.store.GetDeliveryRecoveryReceipt(r.Context(), request, s.now)
	if err != nil {
		s.writeDeliveryRecoveryError(w, err)
		return
	}
	if receipt != nil {
		s.writeDeliveryRecoveryAccepted(w, request.TargetID, *receipt)
		return
	}
	preview, err := s.deliveryRecoveryPreview(r, access, runID, &input)
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, "verification_unavailable", "Recovery could not be checked. Try again.")
		return
	}
	if !preview.Available || preview.CurrentRunID != request.ExpectedRunID || preview.Revision != request.ExpectedRevision {
		// A concurrent copy of this request may have won while eligibility was read.
		receipt, err = s.store.GetDeliveryRecoveryReceipt(r.Context(), request, s.now)
		if err != nil {
			s.writeDeliveryRecoveryError(w, err)
			return
		}
		if receipt != nil {
			s.writeDeliveryRecoveryAccepted(w, request.TargetID, *receipt)
			return
		}
		writeJSON(w, http.StatusConflict, map[string]any{"error": map[string]string{"code": "recovery_unavailable", "message": "Review the latest recovery state."}, "current": preview})
		return
	}
	result, err := s.store.RecoverDelivery(r.Context(), request, s.now)
	if err != nil {
		s.writeDeliveryRecoveryError(w, err)
		return
	}
	s.writeDeliveryRecoveryAccepted(w, request.TargetID, result)
}

func (s *Server) writeDeliveryRecoveryAccepted(w http.ResponseWriter, targetID string, result storage.DeliveryRecoveryResult) {
	status := http.StatusAccepted
	if result.Repeated {
		status = http.StatusOK
	} else {
		s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: targetID})
	}
	// Repeated requests also recover a wake lost after the original commit.
	if s.queue != nil {
		s.queue.WakeQueue(workqueue.LaneWebhook)
	}
	writeJSON(w, status, map[string]any{"run_id": result.RunID, "queue_id": "delivery:" + strconv.FormatInt(result.RunID, 10), "repeated": result.Repeated})
}

func (s *Server) writeDeliveryRecoveryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrRevoked), errors.Is(err, storage.ErrExpired):
		s.writeError(w, http.StatusForbidden, "access_revoked", "Your recovery access changed. Refresh the workspace.")
	case errors.Is(err, storage.ErrConflict):
		s.writeError(w, http.StatusConflict, "state_changed", "Recovery state changed. Review the latest execution.")
	case errors.Is(err, storage.ErrNotFound):
		s.writeError(w, http.StatusNotFound, "history_unavailable", "The recovery record is no longer available.")
	default:
		s.writeInternal(w, err)
	}
}
