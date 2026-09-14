package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

type deliveryOperation struct {
	sourceOrder sql.NullInt64
	currentID   sql.NullInt64
	revision    int64
}

// ClaimDelivery serializes original acceptance and redelivery on the operation.
// Permanent failures remain retained; an explicit recovery is a separate action.
func (s *Store) ClaimDelivery(ctx context.Context, claim storage.DeliveryClaim) (storage.DeliveryClaimResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.DeliveryClaimResult{}, fmt.Errorf("begin delivery claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	operation, err := s.lockDeliveryOperation(ctx, tx, claim.ClaimKey, claim.TargetID)
	if err != nil {
		return storage.DeliveryClaimResult{}, err
	}
	if operation.currentID.Valid {
		var status storage.DeliveryStatus
		var retryable sql.NullBool
		if err := tx.QueryRowContext(ctx, "SELECT status, retryable FROM deliveries WHERE id = ?", operation.currentID.Int64).Scan(&status, &retryable); err != nil {
			return storage.DeliveryClaimResult{}, fmt.Errorf("read current delivery: %w", err)
		}
		if status != storage.DeliveryFailed || !retryable.Bool {
			disposition := storage.DeliveryClaimRetained
			if status == storage.DeliveryRunning {
				disposition = storage.DeliveryClaimInProgress
			}
			if err := tx.Commit(); err != nil {
				return storage.DeliveryClaimResult{}, fmt.Errorf("commit duplicate claim: %w", err)
			}
			return storage.DeliveryClaimResult{Disposition: disposition}, nil
		}
	}
	id, err := insertDeliveryRun(ctx, tx, claim, operation.sourceOrder, queueActorSystem)
	if err != nil {
		return storage.DeliveryClaimResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE delivery_operations SET
  current_delivery_id = ?, source_order = COALESCE(source_order, ?), revision = revision + 1
  WHERE claim_key = ?`, id, id, claim.ClaimKey); err != nil {
		return storage.DeliveryClaimResult{}, fmt.Errorf("advance delivery operation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return storage.DeliveryClaimResult{}, fmt.Errorf("commit delivery claim: %w", err)
	}
	return storage.DeliveryClaimResult{ID: id, Disposition: storage.DeliveryClaimAccepted}, nil
}

func (s *Store) lockDeliveryOperation(ctx context.Context, tx *transaction, key, targetID string) (deliveryOperation, error) {
	if _, err := tx.ExecContext(ctx, `INSERT INTO delivery_operations
 (claim_key, target_id, source_order, current_delivery_id)
 VALUES (?, ?,
 (SELECT MIN(COALESCE(source_order, id)) FROM deliveries WHERE claim_key = ? AND target_id = ?),
 (SELECT MAX(id) FROM deliveries WHERE claim_key = ? AND target_id = ?))
 ON CONFLICT DO NOTHING`, key, targetID, key, targetID, key, targetID); err != nil {
		return deliveryOperation{}, fmt.Errorf("ensure delivery operation: %w", err)
	}
	var operation deliveryOperation
	var storedTarget string
	if err := tx.QueryRowContext(ctx, `SELECT target_id, source_order, current_delivery_id, revision
 FROM delivery_operations WHERE claim_key = ?`+s.dialect.RowLock(), key).Scan(
		&storedTarget, &operation.sourceOrder, &operation.currentID, &operation.revision,
	); err != nil {
		return deliveryOperation{}, fmt.Errorf("lock delivery operation: %w", err)
	}
	if storedTarget != targetID {
		return deliveryOperation{}, storage.ErrConflict
	}
	return operation, nil
}

func insertDeliveryRun(ctx context.Context, tx *transaction, claim storage.DeliveryClaim, sourceOrder sql.NullInt64, actorID string) (int64, error) {
	var id int64
	var order any
	if sourceOrder.Valid {
		order = sourceOrder.Int64
	}
	err := tx.QueryRowContext(ctx, `INSERT INTO deliveries (
  claim_key, delivery_id, target_id, repository_id, repository_full_name,
  event, status, payload, claimed_at, next_attempt_at, source_order
 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		claim.ClaimKey, claim.DeliveryID, claim.TargetID, claim.RepositoryID, claim.RepositoryFullName,
		claim.Event, storage.DeliveryRunning, claim.Payload, claim.ClaimedAt, claim.ClaimedAt, order,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert delivery run: %w", err)
	}
	err = insertLinkedQueueItem(ctx, tx, linkedQueueItem{
		ID: "delivery:" + strconv.FormatInt(id, 10), Kind: workqueue.KindWebhookDelivery,
		Lane: workqueue.LaneWebhook, TargetID: claim.TargetID, RepositoryID: claim.RepositoryID,
		SourceKind: queueSourceDelivery, SourceID: strconv.FormatInt(id, 10),
		Title: "Webhook: " + claim.Event, Summary: claim.RepositoryFullName,
		State: workqueue.StateScheduled, NotBefore: claim.ClaimedAt, ActorID: actorID,
		Details: map[string]any{"delivery_id": claim.DeliveryID, "event": claim.Event},
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}
