package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// checkSyncPlanSlot runs with the target creation lock held. A completed plan
// may release the slot immediately after this read, but this identity remains
// the real reason this creation was deferred. No later lookup substitutes it.
func checkSyncPlanSlot(ctx context.Context, tx *transaction, create orgsync.PlanCreate) error {
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_plans WHERE id = ?", create.ID).Scan(&exists); err != nil {
		return fmt.Errorf("check sync plan identity: %w", err)
	}
	if exists != 0 {
		return storage.ErrConflict
	}
	var planID string
	err := tx.QueryRowContext(ctx, "SELECT id FROM sync_plans WHERE target_id = ? AND state IN "+livePlanStates, create.TargetID).Scan(&planID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read occupied sync plan slot: %w", err)
	}
	return &storage.LiveSyncPlanConflict{PlanID: planID}
}

// prepareSyncPlanCreation shares the settings writer's target lock. Acquire it
// before the originating occurrence, and acquire no plan row locks here.
func (s *Store) prepareSyncPlanCreation(ctx context.Context, tx *transaction, create orgsync.PlanCreate) error {
	if err := s.lockInstallationTarget(ctx, tx, create.TargetID); err != nil {
		if errors.Is(err, storage.ErrNotFound) && create.OriginCheck != nil {
			return orgsync.ErrStaleCheck
		}
		return err
	}
	if err := s.linkSyncCheckResult(ctx, tx, create); err != nil {
		return err
	}
	return checkSyncPlanSlot(ctx, tx, create)
}
