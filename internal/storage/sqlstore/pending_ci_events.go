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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin pending CI head wake: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, pendingCISelect+`
WHERE repository_id = ? AND head_sha = ? AND lifecycle = ?
  AND last_event_key <> ?`,
		wake.RepositoryID,
		wake.HeadSHA,
		pendingci.LifecycleArmed,
		wake.EventKey,
	)
	if err != nil {
		return 0, fmt.Errorf("read pending CI head wake targets: %w", err)
	}
	requests, err := collectRows(rows, scanPendingCI)
	if err != nil {
		return 0, fmt.Errorf("collect pending CI head wake targets: %w", err)
	}

	for _, request := range requests {
		result, updateErr := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    schedule = ?, next_check_at = ?, lease_expires_at = NULL,
	last_event_key = ?, next_check_trigger = ?, updated_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ?`,
			pendingci.ScheduleActive,
			wake.OccurredAt,
			wake.EventKey,
			pendingci.TriggerWebhook,
			wake.OccurredAt,
			request.ID,
			pendingci.LifecycleArmed,
			request.Revision,
		)
		if updateErr != nil {
			return 0, fmt.Errorf("wake pending CI request by head: %w", updateErr)
		}
		changed, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return 0, fmt.Errorf("read pending CI head wake result: %w", rowsErr)
		}
		if changed != 1 {
			return 0, storage.ErrConflict
		}
		event := pendingCIAuditEvent(
			request.ID,
			pendingci.EventWakeReceived,
			pendingci.TriggerWebhook,
			request.LastObservedState,
			"Received a CI status webhook and scheduled an immediate reconciliation",
			wake.OccurredAt,
		)
		event.EventName = wake.EventName
		event.EventKey = wake.EventKey
		event.DeliveryID = wake.DeliveryID
		if err := recordPendingCIEvent(ctx, tx, event); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit pending CI head wake: %w", err)
	}

	return int64(len(requests)), nil
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
    lifecycle = ?, reason = ?, next_check_at = ?, lease_expires_at = NULL,
    cleanup_pending = TRUE, cleanup_artifacts_done = FALSE,
    cleanup_attempts = 0, cleanup_error = '',
    next_check_trigger = ?, updated_at = ?, finished_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ?`,
		change.Lifecycle,
		change.Reason,
		change.FinishedAt,
		pendingci.TriggerCleanup,
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
	if err := recordPendingCIEvent(ctx, tx, pendingCIAuditEvent(
		request.ID,
		pendingci.EventFinished,
		change.Trigger,
		string(change.Lifecycle),
		change.Reason,
		change.FinishedAt,
	)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit pending CI pull request finish: %w", err)
	}

	request.Lifecycle = change.Lifecycle
	request.Reason = change.Reason
	request.LeaseExpiresAt = nil
	request.UpdatedAt = change.FinishedAt
	request.FinishedAt = timePointer(change.FinishedAt)
	request.NextCheckAt = change.FinishedAt
	request.NextCheckTrigger = pendingci.TriggerCleanup
	request.CleanupPending = true
	request.CleanupArtifactsDone = false
	request.CleanupAttempts = 0
	request.CleanupError = ""
	request.Revision++

	return &request, nil
}

// CancelRepository terminalizes every armed request before the service hands
// a repository back to the GitHub Action runner.
func (s *Store) CancelRepository(
	ctx context.Context,
	change pendingci.CancelRepositoryRequest,
) ([]pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin pending CI repository cancellation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    lifecycle = ?, reason = ?, next_check_at = ?, lease_expires_at = NULL,
    cleanup_pending = TRUE, cleanup_artifacts_done = FALSE,
    cleanup_attempts = 0, cleanup_error = '',
    next_check_trigger = ?, updated_at = ?, finished_at = ?, revision = revision + 1
WHERE repository_id = ? AND lifecycle = ?`,
		pendingci.LifecycleCancelled, change.Reason,
		change.CancelledAt, pendingci.TriggerCleanup, change.CancelledAt,
		change.CancelledAt, change.RepositoryID, pendingci.LifecycleArmed,
	)
	if err != nil {
		return nil, fmt.Errorf("cancel pending CI repository: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read pending CI repository cancellation result: %w", err)
	}
	if changed == 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, pendingCISelect+`
WHERE repository_id = ? AND lifecycle = ? AND reason = ? AND finished_at = ?`,
		change.RepositoryID, pendingci.LifecycleCancelled,
		change.Reason, change.CancelledAt,
	)
	if err != nil {
		return nil, fmt.Errorf("read cancelled pending CI repository: %w", err)
	}
	requests, err := collectRows(rows, scanPendingCI)
	if err != nil {
		return nil, fmt.Errorf("collect cancelled pending CI repository: %w", err)
	}
	for _, request := range requests {
		if err := recordPendingCIEvent(ctx, tx, pendingCIAuditEvent(
			request.ID,
			pendingci.EventFinished,
			pendingci.TriggerFallback,
			string(pendingci.LifecycleCancelled),
			change.Reason,
			change.CancelledAt,
		)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit pending CI repository cancellation: %w", err)
	}

	return requests, nil
}
