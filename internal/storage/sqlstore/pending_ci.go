package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const pendingCISelect = `
SELECT id, target_id, installation_id, repository_id, repository_full_name,
       pull_request, head_sha, base_branch, merge_method, required_checks_only,
       requester, source_comment_id, source_revision, label, lifecycle, schedule,
       next_check_at, lease_expires_at, last_progress_at, last_observed_state,
       last_fingerprint, last_event_key, reason, requested_at, updated_at,
       finished_at, revision
FROM pending_ci_requests`

// Arm atomically supersedes the current request for a PR and records the last
// authorized command as the only armed request.
func (s *Store) Arm(ctx context.Context, arm pendingci.ArmRequest) (pendingci.ArmResult, error) {
	if err := arm.Validate(); err != nil {
		return pendingci.ArmResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pendingci.ArmResult{}, fmt.Errorf("begin pending CI arm: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	superseded, err := getArmedPendingCI(ctx, tx, arm.RepositoryID, arm.PullRequest)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return pendingci.ArmResult{}, fmt.Errorf("read superseded pending CI request: %w", err)
	}
	if err == nil {
		if _, err := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    lifecycle = ?, reason = ?, lease_expires_at = NULL,
    updated_at = ?, finished_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ?`,
			pendingci.LifecycleSuperseded,
			"replaced by a newer authorized command",
			arm.RequestedAt,
			arm.RequestedAt,
			superseded.ID,
			pendingci.LifecycleArmed,
		); err != nil {
			return pendingci.ArmResult{}, fmt.Errorf("supersede pending CI request: %w", err)
		}
		superseded.Lifecycle = pendingci.LifecycleSuperseded
		superseded.Reason = "replaced by a newer authorized command"
		superseded.UpdatedAt = arm.RequestedAt
		superseded.FinishedAt = timePointer(arm.RequestedAt)
		superseded.LeaseExpiresAt = nil
		superseded.Revision++
	}

	var id int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO pending_ci_requests (
    target_id, installation_id, repository_id, repository_full_name,
    pull_request, head_sha, base_branch, merge_method, required_checks_only,
    requester, source_comment_id, source_revision, label, lifecycle, schedule,
    next_check_at, last_progress_at, requested_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id`,
		arm.TargetID,
		arm.InstallationID,
		arm.RepositoryID,
		arm.RepositoryFullName,
		arm.PullRequest,
		arm.HeadSHA,
		arm.BaseBranch,
		arm.MergeMethod,
		arm.RequiredChecksOnly,
		arm.Requester,
		arm.SourceCommentID,
		arm.SourceRevision,
		arm.Label,
		pendingci.LifecycleArmed,
		pendingci.ScheduleActive,
		arm.RequestedAt,
		arm.RequestedAt,
		arm.RequestedAt,
		arm.RequestedAt,
	).Scan(&id)
	if err != nil {
		return pendingci.ArmResult{}, fmt.Errorf("insert pending CI request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return pendingci.ArmResult{}, fmt.Errorf("commit pending CI arm: %w", err)
	}

	request := armedRequest(id, arm)
	resultValue := pendingci.ArmResult{Request: request}
	if superseded.ID != 0 {
		resultValue.Superseded = &superseded
	}

	return resultValue, nil
}

func armedRequest(id int64, arm pendingci.ArmRequest) pendingci.Request {
	return pendingci.Request{
		ID: id, TargetID: arm.TargetID, InstallationID: arm.InstallationID,
		RepositoryID: arm.RepositoryID, RepositoryFullName: arm.RepositoryFullName,
		PullRequest: arm.PullRequest, HeadSHA: arm.HeadSHA, BaseBranch: arm.BaseBranch,
		MergeMethod: arm.MergeMethod, RequiredChecksOnly: arm.RequiredChecksOnly,
		Requester: arm.Requester, SourceCommentID: arm.SourceCommentID,
		SourceRevision: arm.SourceRevision, Label: arm.Label,
		Lifecycle: pendingci.LifecycleArmed, Schedule: pendingci.ScheduleActive,
		NextCheckAt: arm.RequestedAt, LastProgressAt: arm.RequestedAt,
		RequestedAt: arm.RequestedAt, UpdatedAt: arm.RequestedAt, Revision: 1,
	}
}

func (s *Store) GetArmed(
	ctx context.Context,
	repositoryID string,
	pullRequest int,
) (pendingci.Request, error) {
	request, err := scanPendingCI(s.db.QueryRowContext(ctx, pendingCISelect+`
WHERE repository_id = ? AND pull_request = ? AND lifecycle = ?`,
		repositoryID,
		pullRequest,
		pendingci.LifecycleArmed,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return pendingci.Request{}, storage.ErrNotFound
	}
	if err != nil {
		return pendingci.Request{}, fmt.Errorf("get armed pending CI request: %w", err)
	}

	return request, nil
}

// LeaseDue reserves one due request and increments its optimistic revision.
func (s *Store) LeaseDue(
	ctx context.Context,
	now time.Time,
	leaseExpiresAt time.Time,
) (pendingci.LeaseResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pendingci.LeaseResult{}, fmt.Errorf("begin pending CI lease: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	request, err := selectDuePendingCI(ctx, tx, now)
	if errors.Is(err, sql.ErrNoRows) {
		availableAt, availableErr := nextPendingCIAvailability(ctx, tx)
		if availableErr != nil {
			return pendingci.LeaseResult{}, availableErr
		}
		if err := tx.Commit(); err != nil {
			return pendingci.LeaseResult{}, fmt.Errorf("commit empty pending CI lease: %w", err)
		}

		return pendingci.LeaseResult{AvailableAt: availableAt}, nil
	}
	if err != nil {
		return pendingci.LeaseResult{}, err
	}

	result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    lease_expires_at = ?, updated_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ?
  AND next_check_at <= ?
  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`,
		leaseExpiresAt,
		now,
		request.ID,
		pendingci.LifecycleArmed,
		request.Revision,
		now,
		now,
	)
	if err != nil {
		return pendingci.LeaseResult{}, fmt.Errorf("lease pending CI request: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return pendingci.LeaseResult{}, fmt.Errorf("read pending CI lease result: %w", err)
	}
	if changed != 1 {
		return pendingci.LeaseResult{}, storage.ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return pendingci.LeaseResult{}, fmt.Errorf("commit pending CI lease: %w", err)
	}
	request.LeaseExpiresAt = timePointer(leaseExpiresAt)
	request.UpdatedAt = now
	request.Revision++

	return pendingci.LeaseResult{Request: &request}, nil
}

func selectDuePendingCI(
	ctx context.Context,
	tx *transaction,
	now time.Time,
) (pendingci.Request, error) {
	return scanPendingCI(tx.QueryRowContext(ctx, pendingCISelect+`
WHERE lifecycle = ? AND next_check_at <= ?
  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
ORDER BY CASE schedule WHEN 'active' THEN 0 ELSE 1 END, next_check_at, id
LIMIT 1`,
		pendingci.LifecycleArmed,
		now,
		now,
	))
}

func nextPendingCIAvailability(ctx context.Context, tx *transaction) (*time.Time, error) {
	var available StoredTime
	err := tx.QueryRowContext(ctx, `
SELECT MIN(
    CASE
        WHEN lease_expires_at IS NOT NULL AND lease_expires_at > next_check_at
            THEN lease_expires_at
        ELSE next_check_at
    END
)
FROM pending_ci_requests
WHERE lifecycle = ?`, pendingci.LifecycleArmed).Scan(&available)
	if err != nil {
		return nil, fmt.Errorf("read next pending CI availability: %w", err)
	}
	if !available.Valid() {
		return nil, nil
	}
	parsed := available.Time()

	return &parsed, nil
}

// Wake promotes an armed request to Active exactly once per meaningful event.
func (s *Store) Wake(ctx context.Context, wake pendingci.WakeRequest) (bool, error) {
	if err := wake.Validate(); err != nil {
		return false, err
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    schedule = ?, next_check_at = ?, lease_expires_at = NULL,
    last_event_key = ?, updated_at = ?, revision = revision + 1
WHERE repository_id = ? AND pull_request = ? AND lifecycle = ?
  AND last_event_key <> ?
  AND (? = '' OR head_sha = ?)`,
		pendingci.ScheduleActive,
		wake.OccurredAt,
		wake.EventKey,
		wake.OccurredAt,
		wake.RepositoryID,
		wake.PullRequest,
		pendingci.LifecycleArmed,
		wake.EventKey,
		wake.ExpectedHeadSHA,
		wake.ExpectedHeadSHA,
	)
	if err != nil {
		return false, fmt.Errorf("wake pending CI request: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read pending CI wake result: %w", err)
	}

	return changed == 1, nil
}

func (s *Store) Reschedule(
	ctx context.Context,
	change pendingci.RescheduleRequest,
) (pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return pendingci.Request{}, err
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    schedule = ?, head_sha = ?, next_check_at = ?, lease_expires_at = NULL,
    last_progress_at = ?, last_observed_state = ?, last_fingerprint = ?,
    updated_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ?`,
		change.Schedule,
		change.HeadSHA,
		change.NextCheckAt,
		change.LastProgressAt,
		change.LastObservedState,
		change.LastFingerprint,
		change.CheckedAt,
		change.ID,
		pendingci.LifecycleArmed,
		change.ExpectedRevision,
	)
	if err != nil {
		return pendingci.Request{}, fmt.Errorf("reschedule pending CI request: %w", err)
	}
	if err := s.checkPendingCIUpdate(ctx, result, change.ID); err != nil {
		return pendingci.Request{}, err
	}

	return s.getPendingCI(ctx, change.ID)
}

func (s *Store) Finish(
	ctx context.Context,
	change pendingci.FinishRequest,
) (pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return pendingci.Request{}, err
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    lifecycle = ?, reason = ?, lease_expires_at = NULL,
    updated_at = ?, finished_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ?`,
		change.Lifecycle,
		change.Reason,
		change.FinishedAt,
		change.FinishedAt,
		change.ID,
		pendingci.LifecycleArmed,
		change.ExpectedRevision,
	)
	if err != nil {
		return pendingci.Request{}, fmt.Errorf("finish pending CI request: %w", err)
	}
	if err := s.checkPendingCIUpdate(ctx, result, change.ID); err != nil {
		return pendingci.Request{}, err
	}

	return s.getPendingCI(ctx, change.ID)
}

func (s *Store) CancelBySource(
	ctx context.Context,
	change pendingci.CancelRequest,
) (*pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin pending CI cancellation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	request, err := scanPendingCI(tx.QueryRowContext(ctx, pendingCISelect+`
WHERE repository_id = ? AND pull_request = ? AND source_comment_id = ? AND lifecycle = ?`,
		change.RepositoryID,
		change.PullRequest,
		change.CommentID,
		pendingci.LifecycleArmed,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read pending CI cancellation target: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    lifecycle = ?, reason = ?, lease_expires_at = NULL,
    updated_at = ?, finished_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ?`,
		pendingci.LifecycleCancelled,
		change.Reason,
		change.CancelledAt,
		change.CancelledAt,
		request.ID,
		pendingci.LifecycleArmed,
	)
	if err != nil {
		return nil, fmt.Errorf("cancel pending CI request: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read pending CI cancellation result: %w", err)
	}
	if changed != 1 {
		return nil, storage.ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit pending CI cancellation: %w", err)
	}
	request.Lifecycle = pendingci.LifecycleCancelled
	request.Reason = change.Reason
	request.LeaseExpiresAt = nil
	request.UpdatedAt = change.CancelledAt
	request.FinishedAt = timePointer(change.CancelledAt)
	request.Revision++

	return &request, nil
}

func (s *Store) ListQueue(
	ctx context.Context,
	filter pendingci.QueueFilter,
) ([]pendingci.Request, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	query := pendingCISelect + " WHERE lifecycle = ?"
	arguments := []any{pendingci.LifecycleArmed}
	if filter.Schedule != nil {
		query += " AND schedule = ?"
		arguments = append(arguments, *filter.Schedule)
	}
	query += " ORDER BY next_check_at, id LIMIT ?"
	arguments = append(arguments, limit)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list pending CI queue: %w", err)
	}

	items, err := collectRows(rows, scanPendingCI)
	if err != nil {
		return nil, fmt.Errorf("read pending CI queue: %w", err)
	}

	return items, nil
}

func getArmedPendingCI(
	ctx context.Context,
	tx *transaction,
	repositoryID string,
	pullRequest int,
) (pendingci.Request, error) {
	return scanPendingCI(tx.QueryRowContext(ctx, pendingCISelect+`
WHERE repository_id = ? AND pull_request = ? AND lifecycle = ?`,
		repositoryID,
		pullRequest,
		pendingci.LifecycleArmed,
	))
}

func (s *Store) getPendingCI(ctx context.Context, id int64) (pendingci.Request, error) {
	request, err := scanPendingCI(s.db.QueryRowContext(ctx, pendingCISelect+" WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return pendingci.Request{}, storage.ErrNotFound
	}
	if err != nil {
		return pendingci.Request{}, fmt.Errorf("get pending CI request: %w", err)
	}

	return request, nil
}

func (s *Store) checkPendingCIUpdate(ctx context.Context, result sql.Result, id int64) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read pending CI update result: %w", err)
	}
	if changed == 1 {
		return nil
	}

	var exists int
	if err := s.db.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM pending_ci_requests WHERE id = ?",
		id,
	).Scan(&exists); err != nil {
		return fmt.Errorf("classify pending CI update: %w", err)
	}
	if exists == 0 {
		return storage.ErrNotFound
	}

	return storage.ErrConflict
}
