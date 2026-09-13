package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// CompleteDelivery marks a running delivery successful. Repeating the same
// outcome is safe when a caller lost the first database result and retries.
func (s *Store) CompleteDelivery(
	ctx context.Context,
	claimID int64,
	completedAt time.Time,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delivery completion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
UPDATE deliveries SET status = ?, finished_at = ?
WHERE id = ? AND status = ?`,
		storage.DeliverySucceeded,
		completedAt,
		claimID,
		storage.DeliveryRunning,
	)
	if err != nil {
		return fmt.Errorf("complete delivery: %w", err)
	}
	changed, err := deliveryFinalizationChanged(ctx, tx, result, claimID, storage.DeliverySucceeded)
	if err != nil || !changed {
		return err
	}
	if err := transitionLinkedQueueItem(
		ctx, tx, "delivery:"+strconv.FormatInt(claimID, 10),
		workqueue.StateSucceeded, completedAt, "Webhook delivered", queueActorSystem,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// FailDelivery marks a running delivery failed with a sanitized reason.
// Repeating the same outcome is safe when finalization is retried.
func (s *Store) FailDelivery(
	ctx context.Context,
	change storage.DeliveryFailureChange,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delivery failure: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
UPDATE deliveries SET
    status = ?,
    stage = ?,
    reason = ?,
    retryable = ?,
    finished_at = ?
WHERE id = ? AND status = ?`,
		storage.DeliveryFailed,
		change.Stage,
		change.Reason,
		change.Retryable,
		change.FailedAt,
		change.ClaimID,
		storage.DeliveryRunning,
	)
	if err != nil {
		return fmt.Errorf("fail delivery: %w", err)
	}
	changed, err := deliveryFinalizationChanged(ctx, tx, result, change.ClaimID, storage.DeliveryFailed)
	if err != nil || !changed {
		return err
	}
	if err := transitionLinkedQueueItem(
		ctx, tx, "delivery:"+strconv.FormatInt(change.ClaimID, 10),
		workqueue.StateFailed, change.FailedAt, change.Reason, queueActorSystem,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// The first terminal outcome owns its details and timestamp. A repeated
// finalization acknowledges that outcome without rewriting history or events.
func deliveryFinalizationChanged(ctx context.Context, tx *transaction, result sql.Result, id int64, outcome storage.DeliveryStatus) (bool, error) {
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read delivery finalization result: %w", err)
	}
	if changed != 0 {
		return true, nil
	}
	var status storage.DeliveryStatus
	err = tx.QueryRowContext(ctx, "SELECT status FROM deliveries WHERE id = ?", id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, storage.ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("read finalized delivery: %w", err)
	}
	if status != outcome {
		return false, storage.ErrConflict
	}
	return false, nil
}
