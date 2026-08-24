package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func (s *Store) ApplyQueueAction(
	ctx context.Context,
	id string,
	action workqueue.ItemAction,
) (workqueue.Item, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Item{}, fmt.Errorf("begin queue action: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	item, err := getQueueItem(ctx, tx, id, s.dialect.RowLock())
	if err != nil {
		return workqueue.Item{}, noRows(err)
	}
	if item.Revision != action.ExpectedRevision {
		return workqueue.Item{}, storage.ErrConflict
	}
	updated, summary, err := s.applyQueueAction(ctx, tx, item, action)
	if err != nil {
		return workqueue.Item{}, err
	}
	if err := syncQueueSourceSchedule(ctx, tx, item, updated, action.Type); err != nil {
		return workqueue.Item{}, err
	}
	if err := updateQueueItemForAction(ctx, tx, item, updated); err != nil {
		return workqueue.Item{}, err
	}
	actor := action.ActorID
	if err := insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: item.ID, ActorID: &actor, Kind: "action." + string(action.Type),
		State: updated.State, Summary: summary, CreatedAt: action.ChangedAt,
	}); err != nil {
		return workqueue.Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return workqueue.Item{}, fmt.Errorf("commit queue action: %w", err)
	}

	return updated, nil
}

// syncQueueSourceSchedule keeps source-backed dispatch predicates aligned with
// the ledger. Leasing checks both records transactionally; changing only the
// queue row would either select the same item in a tight loop while its source
// remained deferred, or leave a Run now request silently waiting on the old
// domain deadline.
func syncQueueSourceSchedule(
	ctx context.Context,
	tx *transaction,
	before, after workqueue.Item,
	action workqueue.ActionType,
) error {
	if action != workqueue.ActionRunNow && action != workqueue.ActionNextWindow &&
		action != workqueue.ActionScheduleAt {
		return nil
	}
	var (
		result sql.Result
		err    error
	)
	switch before.SourceKind {
	case queueSourcePendingCI:
		result, err = tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET next_check_at = ?, lease_expires_at = NULL,
    updated_at = ?, revision = revision + 1
WHERE id = ?`, after.EligibleAt, after.UpdatedAt, before.SourceID)
	case queueSourceDelivery:
		result, err = tx.ExecContext(ctx, `
UPDATE deliveries SET next_attempt_at = ?, lease_expires_at = NULL
WHERE id = ?`, after.EligibleAt, before.SourceID)
	default:
		return nil
	}
	if err != nil {
		return fmt.Errorf("synchronize queue source schedule: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read queue source schedule result: %w", err)
	}
	if changed != 1 {
		return storage.ErrConflict
	}

	return nil
}

func (s *Store) applyQueueAction(
	ctx context.Context,
	tx *transaction,
	item workqueue.Item,
	action workqueue.ItemAction,
) (workqueue.Item, string, error) {
	if item.State.Terminal() {
		return workqueue.Item{}, "", storage.ErrConflict
	}
	updated := item
	updated.Revision++
	updated.UpdatedAt = action.ChangedAt
	switch action.Type {
	case workqueue.ActionRunNow:
		return runQueueNow(updated, action)
	case workqueue.ActionNextWindow:
		return s.moveQueueToWindow(ctx, tx, updated, action.ChangedAt, "Moved to the next window")
	case workqueue.ActionScheduleAt:
		return s.scheduleQueueAt(ctx, tx, updated, action)
	case workqueue.ActionSetPriority:
		if !action.Priority.Valid() {
			return workqueue.Item{}, "", errors.New("queue priority is invalid")
		}
		updated.Priority = action.Priority
		updated.PriorityOverride = true
		return updated, "Priority changed to " + string(action.Priority), nil
	case workqueue.ActionCancel:
		if item.SourceKind != "" && item.SourceKind != queueSourceRecurring {
			return workqueue.Item{}, "", storage.ErrConflict
		}
		if item.State == workqueue.StateRunning {
			return workqueue.Item{}, "", storage.ErrConflict
		}
		updated.State, updated.FinishedAt = workqueue.StateCancelled, &action.ChangedAt
		updated.Reason = action.Reason
		return updated, "Queue item cancelled", nil
	default:
		return workqueue.Item{}, "", errors.New("queue action is invalid")
	}
}

func runQueueNow(
	item workqueue.Item,
	action workqueue.ItemAction,
) (workqueue.Item, string, error) {
	if strings.TrimSpace(action.Reason) == "" {
		return workqueue.Item{}, "", errors.New("run-now reason is required")
	}
	item.State = workqueue.StateReady
	item.WindowMode = workqueue.WindowBypass
	item.Immediate = true
	item.NotBefore, item.EligibleAt = action.ChangedAt, action.ChangedAt
	item.LeaseExpiresAt, item.FinishedAt = nil, nil
	item.BlockedReason, item.Reason = "", action.Reason

	return item, "Run now requested: " + action.Reason, nil
}

func (s *Store) moveQueueToWindow(
	ctx context.Context,
	tx *transaction,
	item workqueue.Item,
	at time.Time,
	summary string,
) (workqueue.Item, string, error) {
	eligible, err := s.queueEligibility(ctx, tx, item, at)
	if err != nil {
		return workqueue.Item{}, "", err
	}
	item.WindowMode, item.Immediate = workqueue.WindowRespect, false
	item.NotBefore, item.EligibleAt = at, eligible
	item.State = stateForEligibility(eligible, at)
	item.LeaseExpiresAt, item.FinishedAt = nil, nil
	item.BlockedReason = ""

	return item, summary, nil
}

func (s *Store) scheduleQueueAt(
	ctx context.Context,
	tx *transaction,
	item workqueue.Item,
	action workqueue.ItemAction,
) (workqueue.Item, string, error) {
	if action.At.IsZero() {
		return workqueue.Item{}, "", errors.New("scheduled time is required")
	}
	if action.OutsideWindow {
		if strings.TrimSpace(action.Reason) == "" {
			return workqueue.Item{}, "", errors.New("outside-window reason is required")
		}
		item.WindowMode, item.Immediate = workqueue.WindowBypass, false
		item.NotBefore, item.EligibleAt = action.At, action.At
		item.State, item.Reason = stateForEligibility(action.At, action.ChangedAt), action.Reason

		return item, "Scheduled outside the window: " + action.Reason, nil
	}

	return s.moveQueueToWindow(ctx, tx, item, action.At, "Scheduled for the assigned window")
}

func (s *Store) queueEligibility(
	ctx context.Context,
	tx *transaction,
	item workqueue.Item,
	at time.Time,
) (time.Time, error) {
	if item.ProfileID == nil {
		return time.Time{}, errors.New("queue item has no schedule profile")
	}
	profile, err := getScheduleProfile(ctx, tx, *item.ProfileID)
	if err != nil {
		return time.Time{}, noRows(err)
	}

	return workqueue.NextEligible(profile, at)
}

func stateForEligibility(eligible, now time.Time) workqueue.State {
	if eligible.After(now) {
		return workqueue.StateScheduled
	}

	return workqueue.StateReady
}

func updateQueueItemForAction(
	ctx context.Context,
	tx *transaction,
	before, after workqueue.Item,
) error {
	result, err := tx.ExecContext(ctx, `
UPDATE queue_items SET
    state = ?, priority = ?, priority_overridden = ?, window_mode = ?, immediate_dispatch = ?,
    not_before = ?, eligible_at = ?, blocked_reason = ?, lease_expires_at = ?,
    reason = ?, revision = ?, updated_at = ?, finished_at = ?
WHERE id = ? AND revision = ?`,
		after.State, after.Priority, after.PriorityOverride, after.WindowMode, after.Immediate,
		after.NotBefore, after.EligibleAt, after.BlockedReason, after.LeaseExpiresAt,
		after.Reason, after.Revision, after.UpdatedAt, after.FinishedAt,
		after.ID, before.Revision,
	)
	if err != nil {
		return fmt.Errorf("update queue item action: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read queue action result: %w", err)
	}
	if changed != 1 {
		return storage.ErrConflict
	}

	return nil
}
