package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// RecoverDelivery shares the ordinary claim lock and preserves prior outcomes.
// Callers must additionally establish workload-specific replay eligibility before
// exposing this operation. Execution retains its normal freshness/permission checks.
func (s *Store) RecoverDelivery(ctx context.Context, request storage.DeliveryRecovery) (storage.DeliveryRecoveryResult, error) {
	if !validDeliveryRecovery(request) {
		return storage.DeliveryRecoveryResult{}, storage.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.DeliveryRecoveryResult{}, fmt.Errorf("begin delivery recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var key string
	if err := tx.QueryRowContext(ctx, "SELECT claim_key FROM deliveries WHERE id = ? AND target_id = ? AND status = 'failed'", request.SourceRunID, request.TargetID).Scan(&key); err != nil {
		return storage.DeliveryRecoveryResult{}, fmt.Errorf("read recovery source: %w", noRows(err))
	}
	operation, err := s.lockDeliveryOperation(ctx, tx, key, request.TargetID)
	if err != nil {
		return storage.DeliveryRecoveryResult{}, err
	}
	if err := s.authorizeDeliveryRecovery(ctx, tx, request); err != nil {
		return storage.DeliveryRecoveryResult{}, err
	}
	receipt, err := deliveryRecoveryReceipt(ctx, tx, request)
	if err != nil {
		return storage.DeliveryRecoveryResult{}, err
	}
	if receipt != nil {
		return *receipt, nil
	}
	if !operation.currentID.Valid || operation.currentID.Int64 != request.ExpectedRunID || operation.revision != request.ExpectedRevision {
		return storage.DeliveryRecoveryResult{}, storage.ErrConflict
	}
	claim, err := readDeliveryRecoveryClaim(ctx, tx, request, key)
	if err != nil {
		return storage.DeliveryRecoveryResult{}, err
	}
	id, err := insertDeliveryRun(ctx, tx, claim, operation.sourceOrder, request.ActorAccountID)
	if err != nil {
		return storage.DeliveryRecoveryResult{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE delivery_operations SET current_delivery_id = ?, revision = revision + 1 WHERE claim_key = ?", id, key); err != nil {
		return storage.DeliveryRecoveryResult{}, fmt.Errorf("advance recovered delivery: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO delivery_recovery_receipts (actor_account_id, request_key, source_run_id, expected_run_id, expected_revision, run_id, requested_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, request.ActorAccountID, request.RequestKey, request.SourceRunID, request.ExpectedRunID, request.ExpectedRevision, id, request.RequestedAt); err != nil {
		return storage.DeliveryRecoveryResult{}, fmt.Errorf("record delivery recovery: %w", s.conflictConstraint(err))
	}
	if err := recordDeliveryRecoveryAudit(ctx, tx, request, id); err != nil {
		return storage.DeliveryRecoveryResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return storage.DeliveryRecoveryResult{}, fmt.Errorf("commit delivery recovery: %w", err)
	}
	return storage.DeliveryRecoveryResult{RunID: id}, nil
}

func deliveryRecoveryReceipt(ctx context.Context, tx *transaction, request storage.DeliveryRecovery) (*storage.DeliveryRecoveryResult, error) {
	var sourceID, expectedID, revision, runID int64
	err := tx.QueryRowContext(ctx, `SELECT source_run_id, expected_run_id, expected_revision, run_id FROM delivery_recovery_receipts WHERE actor_account_id = ? AND request_key = ?`, request.ActorAccountID, request.RequestKey).Scan(&sourceID, &expectedID, &revision, &runID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read delivery recovery receipt: %w", err)
	}
	if sourceID != request.SourceRunID || expectedID != request.ExpectedRunID || revision != request.ExpectedRevision {
		return nil, storage.ErrConflict
	}
	return &storage.DeliveryRecoveryResult{RunID: runID, Repeated: true}, nil
}

func readDeliveryRecoveryClaim(ctx context.Context, tx *transaction, request storage.DeliveryRecovery, key string) (storage.DeliveryClaim, error) {
	claim := storage.DeliveryClaim{ClaimKey: key, TargetID: request.TargetID, ClaimedAt: request.RequestedAt}
	var status storage.DeliveryStatus
	var repositoryID sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT delivery_id, repository_id, repository_full_name, event, payload, status FROM deliveries WHERE id = ? AND target_id = ?`, request.ExpectedRunID, request.TargetID).Scan(&claim.DeliveryID, &repositoryID, &claim.RepositoryFullName, &claim.Event, &claim.Payload, &status)
	if err != nil {
		return storage.DeliveryClaim{}, fmt.Errorf("read recoverable delivery: %w", noRows(err))
	}
	if status != storage.DeliveryFailed || len(claim.Payload) == 0 {
		return storage.DeliveryClaim{}, storage.ErrConflict
	}
	claim.RepositoryID = stringPointer(repositoryID)
	return claim, nil
}

func recordDeliveryRecoveryAudit(ctx context.Context, tx *transaction, request storage.DeliveryRecovery, id int64) error {
	var queueState workqueue.State
	if err := tx.QueryRowContext(ctx, "SELECT state FROM queue_items WHERE id = ?", "delivery:"+strconv.FormatInt(id, 10)).Scan(&queueState); err != nil {
		return err
	}
	if err := insertQueueEvent(ctx, tx, workqueue.Event{ItemID: "delivery:" + strconv.FormatInt(id, 10), ActorID: queueEventActor(request.ActorAccountID), Kind: "recovery_requested", State: queueState, Summary: "Retry requested after delivery failure", CreatedAt: request.RequestedAt}); err != nil {
		return err
	}
	sourceKind := queueSourceDelivery
	auditID, err := insertAppAudit(ctx, tx, appAuditInsert{Category: "runtime", SourceKind: &sourceKind, SourceID: &id, TargetID: &request.TargetID, ActorAccountID: request.ActorAccountID, ElevationID: request.ElevationID, Action: "delivery.recovery_requested", Summary: "Requested delivery retry", CreatedAt: request.RequestedAt})
	if err != nil {
		return err
	}
	if request.ElevationID != nil {
		elevation, err := getElevationByID(ctx, tx, *request.ElevationID, request.SessionTokenHash)
		if err != nil {
			return err
		}
		if err := insertElevatedNotifications(ctx, tx, elevation, auditID, "delivery.recovery_requested", request.RequestedAt); err != nil {
			return err
		}
	}

	return nil
}

func validDeliveryRecovery(request storage.DeliveryRecovery) bool {
	return strings.TrimSpace(request.RequestKey) != "" && len(request.RequestKey) <= 200 && request.SourceRunID > 0 && request.ExpectedRunID > 0 && request.ExpectedRevision > 0 && !request.RequestedAt.IsZero()
}
