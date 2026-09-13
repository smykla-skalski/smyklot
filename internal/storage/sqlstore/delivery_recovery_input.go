package sqlstore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// GetDeliveryRecoveryInput only reads the expected current failed run. Joining
// operation and payload prevents a preview from validating a different run's
// source after a concurrent recovery. No source is leased or claimed here.
func (s *Store) GetDeliveryRecoveryInput(ctx context.Context, targetID string, runID, revision int64) (storage.DeliveryRecoveryInput, error) {
	var input storage.DeliveryRecoveryInput
	var repositoryID sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT current_run.id, operation.revision,
 operation.source_order, current_run.claim_key, current_run.target_id,
 current_run.repository_id, current_run.repository_full_name, current_run.event, current_run.payload
 FROM delivery_operations operation
 JOIN deliveries current_run ON current_run.id = operation.current_delivery_id
   AND current_run.target_id = operation.target_id AND current_run.claim_key = operation.claim_key
 WHERE operation.target_id = ? AND current_run.id = ? AND operation.revision = ?
   AND current_run.status = 'failed'`, targetID, runID, revision).Scan(
		&input.RunID, &input.Revision, &input.SourceOrder, &input.ClaimKey, &input.TargetID,
		&repositoryID, &input.RepositoryFullName, &input.Event, &input.Payload)
	if err != nil {
		return storage.DeliveryRecoveryInput{}, fmt.Errorf("read delivery recovery input: %w", noRows(err))
	}
	input.RepositoryID = stringPointer(repositoryID)
	return input, nil
}
