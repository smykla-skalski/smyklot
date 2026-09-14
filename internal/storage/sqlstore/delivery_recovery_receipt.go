package sqlstore

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// GetDeliveryRecoveryReceipt checks current access before returning an accepted
// request. A nil receipt never creates work. Callers can therefore resolve a
// lost response before checking whether a new execution would still be eligible.
func (s *Store) GetDeliveryRecoveryReceipt(ctx context.Context, request storage.DeliveryRecovery, now func() time.Time) (*storage.DeliveryRecoveryResult, error) {
	if now == nil || !validDeliveryRecovery(request) {
		return nil, storage.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin recovery receipt lookup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.authorizeWorkspaceCommand(ctx, tx, deliveryRecoveryAuthority(request, now)); err != nil {
		return nil, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM deliveries WHERE id = ? AND target_id = ? AND status = 'failed'", request.SourceRunID, request.TargetID).Scan(&id); err != nil {
		return nil, fmt.Errorf("read recovery receipt source: %w", noRows(err))
	}
	return deliveryRecoveryReceipt(ctx, tx, request)
}
