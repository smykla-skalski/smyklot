package panel

import (
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// This is a current observation. Dispatch revalidates the revision and state.
// Labels and navigation belong to the client; the effect never promises success.
type syncDispatchCapability struct {
	Action           string `json:"action"`
	Available        bool   `json:"available"`
	Reason           string `json:"reason"`
	Effect           string `json:"effect"`
	PlanID           string `json:"plan_id"`
	QueueID          string `json:"queue_id,omitempty"`
	ExpectedRevision int64  `json:"expected_revision,omitempty"`
}

func currentSyncDispatchCapability(plan orgsync.Plan, item *workqueue.Item, role storage.InstallationRole, now time.Time) syncDispatchCapability {
	capability := syncDispatchCapability{Action: syncDispatchAction, PlanID: plan.ID, Effect: "schedule_reviewed_changes_without_waiting_for_window"}
	if role != storage.InstallationRoleAdmin && role != storage.InstallationRoleOwner {
		capability.Reason = "admin_or_owner_required"
		return capability
	}
	reason := orgsync.PlanDispatchEligibility(plan, item, now)
	capability.Reason = string(reason)
	capability.Available = reason == orgsync.DispatchAvailable
	if capability.Available {
		capability.QueueID = item.ID
		capability.ExpectedRevision = item.Revision
	}
	return capability
}
