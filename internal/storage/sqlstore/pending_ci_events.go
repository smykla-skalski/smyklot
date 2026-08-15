package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// WakeByHead promotes every armed request still tracking the commit named by
// a status event. A stale event for an earlier head therefore cannot wake the
// replacement request.
func (s *Store) WakeByHead(
	ctx context.Context,
	wake pendingci.WakeHeadRequest,
) (int64, error) {
	if err := wake.Validate(); err != nil {
		return 0, err
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    schedule = ?, next_check_at = ?, lease_expires_at = NULL,
    last_event_key = ?, updated_at = ?, revision = revision + 1
WHERE repository_id = ? AND head_sha = ? AND lifecycle = ?
  AND last_event_key <> ?`,
		pendingci.ScheduleActive,
		wake.OccurredAt,
		wake.EventKey,
		wake.OccurredAt,
		wake.RepositoryID,
		wake.HeadSHA,
		pendingci.LifecycleArmed,
		wake.EventKey,
	)
	if err != nil {
		return 0, fmt.Errorf("wake pending CI requests by head: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read pending CI head wake result: %w", err)
	}

	return changed, nil
}

// FinishPR applies a terminal event only to the currently armed request. A
// later webhook cannot mutate retained history or resurrect it.
func (s *Store) FinishPR(
	ctx context.Context,
	change pendingci.FinishPRRequest,
) (*pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin pending CI pull request finish: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	request, err := getArmedPendingCI(ctx, tx, change.RepositoryID, change.PullRequest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read pending CI pull request finish target: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    lifecycle = ?, reason = ?, lease_expires_at = NULL,
    updated_at = ?, finished_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ?`,
		change.Lifecycle,
		change.Reason,
		change.FinishedAt,
		change.FinishedAt,
		request.ID,
		pendingci.LifecycleArmed,
		request.Revision,
	)
	if err != nil {
		return nil, fmt.Errorf("finish pending CI pull request: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read pending CI pull request finish result: %w", err)
	}
	if changed != 1 {
		return nil, storage.ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit pending CI pull request finish: %w", err)
	}

	request.Lifecycle = change.Lifecycle
	request.Reason = change.Reason
	request.LeaseExpiresAt = nil
	request.UpdatedAt = change.FinishedAt
	request.FinishedAt = timePointer(change.FinishedAt)
	request.Revision++

	return &request, nil
}
