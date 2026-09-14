package orgsync

import (
	"time"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// DispatchReason describes current eligibility, not an accepted request or outcome.
type DispatchReason string

const (
	DispatchAvailable        DispatchReason = "available"
	DispatchApprovalRequired DispatchReason = "approval_required"
	DispatchAlreadyRunning   DispatchReason = "already_running"
	DispatchPlanExpired      DispatchReason = "plan_expired"
	DispatchPlanChanged      DispatchReason = "plan_changed"
	DispatchPlanFinished     DispatchReason = "plan_finished"
	DispatchQueueUnavailable DispatchReason = "queue_unavailable"
	DispatchQueueFinished    DispatchReason = "queue_finished"
	DispatchStateUnsupported DispatchReason = "state_unsupported"
)

// PlanDispatchEligibility is shared by current capability reads and locked
// mutations. Authorization and request revision checks remain mandatory at
// their boundaries. Neither a receipt nor a historical result calls this rule.
func PlanDispatchEligibility(plan Plan, item *workqueue.Item, now time.Time) DispatchReason {
	switch plan.State {
	case PlanApplying:
		return DispatchAlreadyRunning
	case PlanStale:
		return DispatchPlanChanged
	case PlanExpired:
		return DispatchPlanExpired
	case PlanApplied, PlanFailed, PlanDiscarded:
		return DispatchPlanFinished
	case PlanComputed, PlanApproved:
		if !plan.ExpiresAt.After(now) {
			return DispatchPlanExpired
		}
		if plan.State == PlanComputed {
			return DispatchApprovalRequired
		}
	default:
		return DispatchStateUnsupported
	}
	if item == nil || item.TargetID == nil || *item.TargetID != plan.TargetID || item.SourceID != plan.ID || item.SourceKind != "sync_plan" || item.Kind != workqueue.KindSyncApply {
		return DispatchQueueUnavailable
	}
	if item.State == workqueue.StateRunning {
		return DispatchAlreadyRunning
	}
	if item.State.Terminal() {
		return DispatchQueueFinished
	}
	switch item.State {
	case workqueue.StateScheduled, workqueue.StateBlocked, workqueue.StateReady, workqueue.StateRetrying:
		return DispatchAvailable
	default:
		return DispatchStateUnsupported
	}
}
