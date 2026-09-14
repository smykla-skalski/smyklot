package sqlstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func validSyncDispatch(request orgsync.PlanDispatch) bool {
	return request.TargetID != "" && request.PlanID != "" && request.ActorID != "" &&
		request.RequestKey != "" && validRecurringRequestKey(request.RequestKey) &&
		request.ExpectedRevision > 0 && strings.TrimSpace(request.Reason) != "" && !request.Now.IsZero()
}

// FindSyncPlanDispatch reads immutable acceptance under current session and
// workspace authority, including when a plan no longer exists.
func (s *Store) FindSyncPlanDispatch(ctx context.Context, request orgsync.PlanDispatch) (orgsync.PlanDispatchReceipt, error) {
	if !validSyncDispatch(request) {
		return orgsync.PlanDispatchReceipt{}, storage.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return orgsync.PlanDispatchReceipt{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.authorizeSyncDispatch(ctx, tx, request); err != nil {
		return orgsync.PlanDispatchReceipt{}, err
	}
	return syncDispatchReceipt(ctx, tx, request)
}

// DispatchSyncPlan checks the selected source and revision in the transaction
// that records acceptance. No repeated request changes queue state or events.
func (s *Store) DispatchSyncPlan(ctx context.Context, request orgsync.PlanDispatch) (orgsync.PlanDispatchReceipt, error) {
	if !validSyncDispatch(request) {
		return orgsync.PlanDispatchReceipt{}, storage.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return orgsync.PlanDispatchReceipt{}, fmt.Errorf("begin sync dispatch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize with maintenance leasing before acquiring plan and queue locks.
	// Plan-before-queue matches configuration invalidation and plan decisions.
	if _, err := s.lockQueueDispatchState(ctx, tx, workqueue.LaneMaintenance); err != nil {
		return orgsync.PlanDispatchReceipt{}, err
	}
	if err := s.authorizeSyncDispatch(ctx, tx, request); err != nil {
		return orgsync.PlanDispatchReceipt{}, err
	}
	receipt, err := syncDispatchReceipt(ctx, tx, request)
	if err == nil {
		return receipt, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return orgsync.PlanDispatchReceipt{}, err
	}
	plan, err := scanSyncPlan(tx.QueryRowContext(ctx, "SELECT"+syncPlanColumns+" FROM sync_plans WHERE id = ? AND target_id = ?"+s.dialect.RowLock(), request.PlanID, request.TargetID))
	if err != nil {
		return orgsync.PlanDispatchReceipt{}, noRows(err)
	}
	// Before reading the queue, only an otherwise eligible plan can report
	// a missing queue. Revalidate with the locked item below.
	if reason := orgsync.PlanDispatchEligibility(plan, nil, request.Now); reason != orgsync.DispatchQueueUnavailable {
		return orgsync.PlanDispatchReceipt{}, storage.ErrConflict
	}
	item, err := getQueueItem(ctx, tx, "sync-plan:"+plan.ID, s.dialect.RowLock())
	if err != nil {
		return orgsync.PlanDispatchReceipt{}, noRows(err)
	}
	if item.Revision != request.ExpectedRevision || orgsync.PlanDispatchEligibility(plan, &item, request.Now) != orgsync.DispatchAvailable {
		return orgsync.PlanDispatchReceipt{}, storage.ErrConflict
	}
	updated, summary, err := s.applyQueueAction(ctx, tx, item, workqueue.ItemAction{Type: workqueue.ActionRunNow, ExpectedRevision: request.ExpectedRevision, ActorID: request.ActorID, Reason: request.Reason, ChangedAt: request.Now})
	if err != nil {
		return orgsync.PlanDispatchReceipt{}, err
	}
	if err := updateQueueItemForAction(ctx, tx, item, updated); err != nil {
		return orgsync.PlanDispatchReceipt{}, err
	}
	actor := request.ActorID
	if err := insertQueueEvent(ctx, tx, workqueue.Event{ItemID: item.ID, ActorID: &actor, Kind: "action.run_now", State: updated.State, Summary: summary, CreatedAt: request.Now}); err != nil {
		return orgsync.PlanDispatchReceipt{}, err
	}
	receipt = orgsync.PlanDispatchReceipt{PlanID: plan.ID, QueueID: item.ID, AcceptedAt: request.Now}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sync_dispatch_receipts (actor_account_id, request_key, target_id, plan_id, expected_revision, reason, queue_id, accepted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, request.ActorID, request.RequestKey, request.TargetID, request.PlanID, request.ExpectedRevision, request.Reason, receipt.QueueID, receipt.AcceptedAt); err != nil {
		return orgsync.PlanDispatchReceipt{}, fmt.Errorf("record sync dispatch: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return orgsync.PlanDispatchReceipt{}, fmt.Errorf("commit sync dispatch: %w", err)
	}
	return receipt, nil
}

func syncDispatchReceipt(ctx context.Context, reader runner, request orgsync.PlanDispatch) (orgsync.PlanDispatchReceipt, error) {
	var target, plan, reason, queue string
	var revision int64
	var accepted StoredTime
	err := reader.QueryRowContext(ctx, `SELECT target_id, plan_id, expected_revision, reason, queue_id, accepted_at FROM sync_dispatch_receipts WHERE actor_account_id = ? AND request_key = ?`, request.ActorID, request.RequestKey).Scan(&target, &plan, &revision, &reason, &queue, &accepted)
	if err != nil {
		return orgsync.PlanDispatchReceipt{}, fmt.Errorf("read sync dispatch receipt: %w", noRows(err))
	}
	if target != request.TargetID || plan != request.PlanID || revision != request.ExpectedRevision || reason != request.Reason {
		return orgsync.PlanDispatchReceipt{}, storage.ErrConflict
	}
	return orgsync.PlanDispatchReceipt{PlanID: plan, QueueID: queue, AcceptedAt: accepted.Time()}, nil
}

func (s *Store) authorizeSyncDispatch(ctx context.Context, tx *transaction, request orgsync.PlanDispatch) error {
	return s.authorizeWorkspaceCommand(ctx, tx, workspaceCommandAuthority{ActorAccountID: request.ActorID, SessionTokenHash: request.SessionTokenHash, TargetID: request.TargetID, RequestedAt: request.Now})
}
