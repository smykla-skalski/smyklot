package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// GetSyncCheckAvailability never mutates expiry or request state. Acceptance rechecks under
// locks, so a concurrent change can make this observation stale immediately.
func (s *Store) GetSyncCheckAvailability(ctx context.Context, targetID string, now time.Time) (orgsync.CheckAvailability, error) {
	if targetID == "" || now.IsZero() {
		return orgsync.CheckAvailability{}, storage.ErrConflict
	}
	plan, err := scanSyncPlan(s.db.QueryRowContext(ctx, "SELECT"+syncPlanColumns+" FROM sync_plans WHERE target_id = ? AND state IN "+livePlanStates, targetID))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return orgsync.CheckAvailability{}, fmt.Errorf("read check plan availability: %w", err)
	}
	if err == nil {
		if reason := orgsync.FreshCheckPlanBlocker(plan, now); reason != "" {
			return orgsync.CheckAvailability{Reason: reason, BlockingPlanID: plan.ID}, nil
		}
	}
	source := recurringSourceID(workqueue.RecurringClaim{Kind: workqueue.KindSyncScan, TargetID: &targetID})
	item, err := latestRecurringItem(ctx, s.db, source, "")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return orgsync.CheckAvailability{}, fmt.Errorf("read check queue availability: %w", err)
	}
	if err == nil && item.State == workqueue.StateRunning {
		return orgsync.CheckAvailability{Reason: "check_running", RunningCheckID: item.ID}, nil
	}
	return orgsync.CheckAvailability{Reason: "available"}, nil
}
