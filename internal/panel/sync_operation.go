package panel

import (
	"context"
	"errors"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// Acceptance is immutable. The other facts are read during the observation
// interval, not an atomic snapshot. Null means unavailable, never successful.
// The exact accepted subject remains valid when its worker record is pruned.
type syncOperationResponse struct {
	TargetID             string                  `json:"target_id"`
	Acceptance           syncRequestHistoryItem  `json:"acceptance"`
	ObservationStartedAt time.Time               `json:"observation_started_at"`
	ObservedAt           time.Time               `json:"observed_at"`
	Comparison           *orgsync.CheckDetails   `json:"comparison"`
	Plan                 *syncPlanSummaryDTO     `json:"plan"`
	Execution            *syncCheckExecution     `json:"execution"`
	Check                syncCheckCapability     `json:"check"`
	Dispatch             *syncDispatchCapability `json:"dispatch"`
}

func (s *Server) readSyncOperation(ctx context.Context, query orgsync.RequestLookup) (syncOperationResponse, error) {
	body := syncOperationResponse{TargetID: query.TargetID, ObservationStartedAt: s.now().UTC()}
	accepted, err := s.store.GetSyncRequest(ctx, query, s.now)
	if err != nil {
		return body, err
	}
	body.Acceptance, err = syncAcceptanceDTO(accepted)
	if err != nil {
		return body, err
	}
	item, err := s.syncOperationExecution(ctx, query.TargetID, accepted)
	if err != nil {
		return body, err
	}
	if item != nil {
		body.Execution = syncExecutionFacts(*item)
	}
	var originalPlan *orgsync.Plan
	switch accepted.Action {
	case orgsync.RequestActionCheck:
		comparison, err := s.store.GetSyncCheckResult(ctx, query.TargetID, accepted.QueueID)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return body, err
		}
		if err == nil {
			body.Comparison = &comparison
		}
	case orgsync.RequestActionDispatch:
		plan, err := s.store.GetSyncPlanSummary(ctx, query.TargetID, accepted.PlanID)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return body, err
		}
		if err == nil {
			summary := syncPlanSummary(plan)
			body.Plan = &summary
			originalPlan = &plan
		}
	}
	access, err := s.store.ResolveTargetAccess(ctx, query.ActorID, query.TargetID, s.now().UTC())
	if err != nil {
		return body, err
	}
	if originalPlan != nil {
		capability := currentSyncDispatchCapability(*originalPlan, item, access.Role, s.now().UTC())
		body.Dispatch = &capability
	}
	body.Check, err = s.currentSyncCheckCapability(ctx, query.TargetID, access.Role)
	if err != nil {
		return body, err
	}
	// Slow dependent reads cannot expose results after session/workspace access is
	// revoked. Capabilities remain advisory and commands recheck current authority.
	if _, err := s.store.GetSyncRequest(ctx, query, s.now); err != nil {
		return body, err
	}
	body.ObservedAt = s.now().UTC()
	return body, nil
}

func (s *Server) syncOperationExecution(ctx context.Context, targetID string, accepted orgsync.RequestAcceptance) (*workqueue.Item, error) {
	item, err := s.store.GetQueueItem(ctx, accepted.QueueID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if item.TargetID == nil || *item.TargetID != targetID {
		return nil, nil
	}
	if accepted.Action == orgsync.RequestActionCheck && item.Kind != workqueue.KindSyncScan {
		return nil, nil
	}
	if accepted.Action == orgsync.RequestActionDispatch && (item.Kind != workqueue.KindSyncApply || item.SourceKind != "sync_plan" || item.SourceID != accepted.PlanID) {
		return nil, nil
	}
	return &item, nil
}

func syncExecutionFacts(item workqueue.Item) *syncCheckExecution {
	return &syncCheckExecution{State: item.State, Summary: item.Summary, ProgressCurrent: item.ProgressCurrent, ProgressTotal: item.ProgressTotal, Attempt: item.Attempt, StartedAt: item.StartedAt, FinishedAt: item.FinishedAt}
}
