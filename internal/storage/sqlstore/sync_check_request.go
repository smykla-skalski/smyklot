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

// Run under the maintenance dispatch lock, after receipt recovery and before
// accepting new work. The target lock serializes this decision with creation
// of a new plan. Expiry and request acceptance either both commit or neither.
func (s *Store) prepareSyncCheckRequest(ctx context.Context, tx *transaction, request workqueue.RecurringRequest, now func() time.Time) error {
	if request.Kind != workqueue.KindSyncScan {
		return nil
	}
	if request.TargetID == nil {
		return storage.ErrConflict
	}
	if err := s.lockInstallationTarget(ctx, tx, *request.TargetID); err != nil {
		return err
	}
	plan, err := scanSyncPlan(tx.QueryRowContext(ctx, "SELECT"+syncPlanColumns+" FROM sync_plans WHERE target_id = ? AND state IN "+livePlanStates+s.dialect.RowLock(), *request.TargetID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read check blocker: %w", err)
	}
	request.Now = now().UTC()
	if orgsync.FreshCheckPlanBlocker(plan, request.Now) != "" {
		return &storage.LiveSyncPlanConflict{PlanID: plan.ID}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE sync_plans SET state = 'expired', finished_at = ? WHERE id = ?", request.Now, plan.ID); err != nil {
		return fmt.Errorf("retire expired check blocker: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET state = 'superseded', finished_at = ?, updated_at = ?, revision = revision + 1
 WHERE source_kind = 'sync_plan' AND source_id = ? AND state IN ('awaiting_approval', 'blocked', 'scheduled', 'ready', 'retrying')`, request.Now, request.Now, plan.ID); err != nil {
		return fmt.Errorf("retire expired check work: %w", err)
	}
	return nil
}

// Scheduled checks use ClaimRecurringWork; this boundary handles explicit intent.
func (s *Store) authorizeSyncCheckRequest(ctx context.Context, tx *transaction, request workqueue.RecurringRequest, now func() time.Time) error {
	if request.Kind != workqueue.KindSyncScan {
		return nil
	}
	if request.TargetID == nil || *request.TargetID == "" || request.RequestKey == "" || request.Now.IsZero() {
		return storage.ErrConflict
	}
	return s.authorizeWorkspaceCommand(ctx, tx, checkAuthority(request, now))
}

func (s *Store) validateSyncCheckRequest(ctx context.Context, tx *transaction, request workqueue.RecurringRequest, now func() time.Time) error {
	if request.Kind != workqueue.KindSyncScan {
		return nil
	}
	return s.validateWorkspaceCommand(ctx, tx, checkAuthority(request, now))
}

func checkAuthority(request workqueue.RecurringRequest, now func() time.Time) workspaceCommandAuthority {
	return workspaceCommandAuthority{ActorAccountID: request.ActorID, SessionTokenHash: request.SessionTokenHash, TargetID: *request.TargetID, Clock: now}
}
