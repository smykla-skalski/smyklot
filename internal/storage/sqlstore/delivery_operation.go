package sqlstore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// GetDeliveryOperation reads history, operation ownership and current queue state
// in one statement, so a concurrent recovery cannot mix two different runs.
func (s *Store) GetDeliveryOperation(ctx context.Context, targetID string, runID int64) (storage.DeliveryOperation, error) {
	var operation storage.DeliveryOperation
	var revision, sourceOrder, currentID sql.NullInt64
	var status, event, queueID, queueState sql.NullString
	var payloadAvailable sql.NullBool
	var eligibleAt StoredTime
	err := s.db.QueryRowContext(ctx, `SELECT original.target_id,
 operation.revision, COALESCE(operation.source_order, original.source_order, original.id), current_run.id, current_run.status,
 current_run.event, current_run.payload IS NOT NULL,
 queue.id, queue.state, queue.eligible_at
 FROM deliveries original
 LEFT JOIN delivery_operations operation ON operation.claim_key = original.claim_key
   AND operation.target_id = original.target_id
 LEFT JOIN deliveries current_run ON current_run.id = operation.current_delivery_id
   AND current_run.target_id = original.target_id
 LEFT JOIN queue_items queue ON queue.id = 'delivery:' || CAST(current_run.id AS TEXT)
   AND queue.target_id = original.target_id
 WHERE original.id = ? AND original.target_id = ?`, runID, targetID).Scan(
		&operation.TargetID, &revision, &sourceOrder, &currentID, &status,
		&event, &payloadAvailable, &queueID, &queueState, &eligibleAt)
	if err != nil {
		return storage.DeliveryOperation{}, fmt.Errorf("read delivery operation: %w", noRows(err))
	}
	operation.Revision = revision.Int64
	operation.SourceOrder = sourceOrder.Int64
	if currentID.Valid {
		operation.Current = &storage.DeliveryRun{ID: currentID.Int64, Status: storage.DeliveryStatus(status.String), Event: event.String, PayloadAvailable: payloadAvailable.Bool}
		if queueID.Valid {
			operation.Current.Queue = &storage.DeliveryRunQueue{ID: queueID.String, State: workqueue.State(queueState.String), EligibleAt: eligibleAt.Time()}
		}
	}
	return operation, nil
}
