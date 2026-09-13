package main

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// runSyncScan retains explicit intent after the queue consumes immediate dispatch.
// Reading the occurrence's durable event also preserves it across lease retries.
func (s *server) runSyncScan(ctx context.Context, client *github.Client, targetID string, item workqueue.Item) (string, error) {
	requested, err := s.store.QueueRunWasRequested(ctx, item.ID)
	if err != nil {
		return "", err
	}
	trigger := orgsync.TriggerReconcile
	if requested {
		trigger = orgsync.TriggerManual
	}
	return s.sync.PlanInstallationWithSummary(ctx, client, targetID, trigger)
}
