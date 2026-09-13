package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// runSyncScan retains explicit intent after the queue consumes immediate dispatch.
// Reading the occurrence's durable event also preserves it across lease retries.
func (s *server) runSyncScan(ctx context.Context, client *github.Client, targetID string, item workqueue.Item) (string, error) {
	var result workqueue.SyncScanDetails
	if len(item.Details) > 0 {
		if err := json.Unmarshal(item.Details, &result); err != nil {
			return "", fmt.Errorf("read retained check result: %w", err)
		}
	}
	if result.ResultPlanID != "" {
		return "This check already queued a sync plan. See its retained result.", nil
	}
	requested, err := s.store.QueueRunWasRequested(ctx, item.ID)
	if err != nil {
		return "", err
	}
	trigger := orgsync.TriggerReconcile
	if requested {
		trigger = orgsync.TriggerManual
	}
	return s.sync.PlanInstallationForCheck(ctx, client, targetID, trigger, orgsync.CheckReference{QueueID: item.ID, Attempt: item.Attempt})
}
