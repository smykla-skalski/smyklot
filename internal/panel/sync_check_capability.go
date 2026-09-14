package panel

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const syncCheckAction = "check"

type syncCheckCapability struct {
	Action         string `json:"action"`
	TargetID       string `json:"target_id"`
	Available      bool   `json:"available"`
	Reason         string `json:"reason"`
	Effect         string `json:"effect"`
	BlockingPlanID string `json:"blocking_plan_id,omitempty"`
	RunningCheckID string `json:"running_check_id,omitempty"`
}

func (s *Server) currentSyncCheckCapability(ctx context.Context, targetID string, role storage.InstallationRole) (syncCheckCapability, error) {
	result := syncCheckCapability{Action: syncCheckAction, TargetID: targetID, Effect: "request_repository_check"}
	if role != storage.InstallationRoleOwner && role != storage.InstallationRoleAdmin {
		result.Reason = "admin_or_owner_required"
		return result, nil
	}
	availability, err := s.store.GetSyncCheckAvailability(ctx, targetID, s.now().UTC())
	if err != nil {
		return result, err
	}
	result.Reason = availability.Reason
	result.Available = availability.Reason == "available"
	result.BlockingPlanID = availability.BlockingPlanID
	result.RunningCheckID = availability.RunningCheckID
	return result, nil
}
