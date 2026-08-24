package sqlstore

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func (s *Store) WithdrawScheduleRequest(
	ctx context.Context,
	id string,
	expectedRevision int64,
	actorID string,
	at time.Time,
) (workqueue.ScheduleRequest, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.ScheduleRequest{}, fmt.Errorf("begin schedule withdrawal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	request, err := getScheduleRequest(ctx, tx, id, s.dialect.RowLock())
	if err != nil {
		return workqueue.ScheduleRequest{}, noRows(err)
	}
	if request.State != workqueue.RequestPending || request.Revision != expectedRevision ||
		request.RequestedBy != actorID {
		return workqueue.ScheduleRequest{}, storage.ErrConflict
	}
	request.State, request.Revision = workqueue.RequestWithdrawn, request.Revision+1
	request.UpdatedAt = at
	if _, err := tx.ExecContext(ctx, `
UPDATE schedule_requests SET state = ?, revision = ?, updated_at = ? WHERE id = ?`,
		request.State, request.Revision, at, id,
	); err != nil {
		return workqueue.ScheduleRequest{}, fmt.Errorf("withdraw schedule request: %w", err)
	}
	if err := finishScheduleRequestQueueItem(
		ctx, tx, request, workqueue.StateCancelled, "Schedule request withdrawn", actorID,
	); err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return workqueue.ScheduleRequest{}, fmt.Errorf("commit schedule withdrawal: %w", err)
	}

	return request, nil
}
