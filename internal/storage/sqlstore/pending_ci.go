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
       requester, source_comment_id, source_revision, source_sequence, source_order,
	       label, lifecycle, schedule, next_check_trigger,
       next_check_at, lease_expires_at, last_progress_at, last_observed_state,
       last_fingerprint, last_event_key, reason, requested_at, updated_at,
       finished_at, cleanup_pending, cleanup_artifacts_done,
       cleanup_attempts, cleanup_error, revision
FROM pending_ci_requests`

// CheckArm verifies the current durable source order without changing it.
// Callers use it before publishing external artifacts; Arm remains the final
// atomic authority in case another process changes the order afterward.
func (s *Store) CheckArm(ctx context.Context, arm pendingci.ArmRequest) error {
	if err := arm.Validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin pending CI arm check: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, _, err := pendingCIArmTarget(ctx, tx, arm); err != nil {
		return err
	}

	return nil
}

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

	superseded, duplicate, err := pendingCIArmTarget(ctx, tx, arm)
	if err != nil {
		return pendingci.ArmResult{}, err
	}
	if duplicate {
		return pendingci.ArmResult{Request: superseded}, nil
	}
	if superseded.ID != 0 {
		if _, err := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    lifecycle = ?, reason = ?, next_check_at = ?, lease_expires_at = NULL,
    cleanup_pending = TRUE, cleanup_artifacts_done = FALSE,
    cleanup_attempts = 0, cleanup_error = '',
    next_check_trigger = ?, updated_at = ?, finished_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ?`,
			pendingci.LifecycleSuperseded,
			"replaced by a newer authorized command",
			arm.RequestedAt,
			pendingci.TriggerCleanup,
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
		superseded.NextCheckAt = arm.RequestedAt
		superseded.NextCheckTrigger = pendingci.TriggerCleanup
		superseded.LeaseExpiresAt = nil
		superseded.CleanupPending = true
		superseded.CleanupArtifactsDone = false
		superseded.CleanupAttempts = 0
		superseded.CleanupError = ""
		superseded.Revision++
		if err := recordPendingCIEvent(ctx, tx, pendingCIAuditEvent(
			superseded.ID,
			pendingci.EventSuperseded,
			pendingci.TriggerCommand,
			string(pendingci.LifecycleSuperseded),
			superseded.Reason,
			arm.RequestedAt,
		)); err != nil {
			return pendingci.ArmResult{}, err
		}
	}

	id, err := insertArmedPendingCI(ctx, tx, arm)
	if err != nil {
		return pendingci.ArmResult{}, err
	}
	if err := recordPendingCIEvent(ctx, tx, pendingCIAuditEvent(
		id,
		pendingci.EventArmed,
		pendingci.TriggerCommand,
		string(pendingci.LifecycleArmed),
		fmt.Sprintf("Waiting to %s pull request after CI passes", arm.MergeMethod),
		arm.RequestedAt,
	)); err != nil {
		return pendingci.ArmResult{}, err
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

func insertArmedPendingCI(
	ctx context.Context,
	tx *transaction,
	arm pendingci.ArmRequest,
) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO pending_ci_requests (
    target_id, installation_id, repository_id, repository_full_name,
    pull_request, head_sha, base_branch, merge_method, required_checks_only,
    requester, source_comment_id, source_revision, source_sequence, source_order,
		label, lifecycle, schedule, next_check_trigger,
    next_check_at, last_progress_at, requested_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		arm.SourceSequence,
		arm.SourceOrder,
		arm.Label,
		pendingci.LifecycleArmed,
		pendingci.ScheduleActive,
		pendingci.TriggerCommand,
		arm.RequestedAt,
		arm.RequestedAt,
		arm.RequestedAt,
		arm.RequestedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert pending CI request: %w", err)
	}
	if err := recordPendingCIIntent(ctx, tx, pendingCIIntent{
		repositoryID: arm.RepositoryID,
		pullRequest:  arm.PullRequest,
		commentID:    arm.SourceCommentID,
		revision:     arm.SourceRevision,
		sequence:     arm.SourceSequence,
		order:        arm.SourceOrder,
		kind:         pendingCIIntentArm,
		recordedAt:   arm.RequestedAt,
	}); err != nil {
		return 0, err
	}
	return id, nil
}

func armedRequest(id int64, arm pendingci.ArmRequest) pendingci.Request {
	return pendingci.Request{
		ID: id, TargetID: arm.TargetID, InstallationID: arm.InstallationID,
		RepositoryID: arm.RepositoryID, RepositoryFullName: arm.RepositoryFullName,
		PullRequest: arm.PullRequest, HeadSHA: arm.HeadSHA, BaseBranch: arm.BaseBranch,
		MergeMethod: arm.MergeMethod, RequiredChecksOnly: arm.RequiredChecksOnly,
		Requester: arm.Requester, SourceCommentID: arm.SourceCommentID,
		SourceRevision: arm.SourceRevision, SourceSequence: arm.SourceSequence,
		SourceOrder: arm.SourceOrder, Label: arm.Label,
		Lifecycle: pendingci.LifecycleArmed, Schedule: pendingci.ScheduleActive,
		NextCheckTrigger: pendingci.TriggerCommand,
		NextCheckAt:      arm.RequestedAt, LastProgressAt: arm.RequestedAt,
		RequestedAt: arm.RequestedAt, UpdatedAt: arm.RequestedAt, Revision: 1,
	}
}

func pendingCIArmTarget(
	ctx context.Context,
	tx *transaction,
	arm pendingci.ArmRequest,
) (pendingci.Request, bool, error) {
	current, err := getArmedPendingCI(ctx, tx, arm.RepositoryID, arm.PullRequest)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return pendingci.Request{}, false, fmt.Errorf(
			"read superseded pending CI request: %w", err,
		)
	}
	stale, equalHistory, err := comparePendingCISourceHistory(ctx, tx, arm)
	if err != nil {
		return pendingci.Request{}, false, err
	}
	if stale {
		return pendingci.Request{}, false, pendingci.ErrStaleSourceRevision
	}
	if equalHistory {
		if current.ID != 0 && samePendingCICommand(current, arm) {
			return current, true, nil
		}

		return pendingci.Request{}, false, pendingci.ErrStaleSourceRevision
	}

	return current, false, nil
}

func samePendingCICommand(request pendingci.Request, arm pendingci.ArmRequest) bool {
	return request.TargetID == arm.TargetID && request.InstallationID == arm.InstallationID &&
		request.RepositoryID == arm.RepositoryID && request.PullRequest == arm.PullRequest &&
		request.HeadSHA == arm.HeadSHA && request.BaseBranch == arm.BaseBranch &&
		request.MergeMethod == arm.MergeMethod &&
		request.RequiredChecksOnly == arm.RequiredChecksOnly && request.Requester == arm.Requester &&
		request.SourceCommentID == arm.SourceCommentID &&
		request.SourceRevision == arm.SourceRevision && request.SourceSequence == arm.SourceSequence &&
		request.SourceOrder == arm.SourceOrder &&
		request.Label == arm.Label
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

// Get returns one pending-CI request regardless of its lifecycle.
func (s *Store) Get(ctx context.Context, id int64) (pendingci.Request, error) {
	return s.getPendingCI(ctx, id)
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
WHERE id = ? AND revision = ?
  AND (lifecycle = ? OR cleanup_pending = TRUE)
  AND next_check_at <= ?
  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`,
		leaseExpiresAt,
		now,
		request.ID,
		request.Revision,
		pendingci.LifecycleArmed,
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
	if err := recordPendingCIEvent(ctx, tx, pendingCIAuditEvent(
		request.ID,
		pendingci.EventReconciliationStarted,
		normalizedTrigger(request.NextCheckTrigger, pendingci.TriggerFallback),
		request.LastObservedState,
		"Reading live pull request and CI state",
		now,
	)); err != nil {
		return pendingci.LeaseResult{}, err
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
WHERE (lifecycle = ? OR cleanup_pending = TRUE) AND next_check_at <= ?
  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
ORDER BY CASE
    WHEN cleanup_pending = TRUE THEN 0
    WHEN schedule = 'active' THEN 1
    ELSE 2
END, next_check_at, id
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
WHERE lifecycle = ? OR cleanup_pending = TRUE`, pendingci.LifecycleArmed).Scan(&available)
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin pending CI webhook wake: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	request, err := getArmedPendingCI(ctx, tx, wake.RepositoryID, wake.PullRequest)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read pending CI webhook wake target: %w", err)
	}
	if request.LastEventKey == wake.EventKey ||
		(wake.ExpectedHeadSHA != "" && request.HeadSHA != wake.ExpectedHeadSHA) {
		return false, nil
	}
	result, err := tx.ExecContext(ctx, `
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
	if err != nil {
		return false, fmt.Errorf("wake pending CI request: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read pending CI wake result: %w", err)
	}

	if changed != 1 {
		return false, storage.ErrConflict
	}
	event := pendingCIAuditEvent(
		request.ID,
		pendingci.EventWakeReceived,
		pendingci.TriggerWebhook,
		request.LastObservedState,
		"Received a CI state webhook and scheduled an immediate reconciliation",
		wake.OccurredAt,
	)
	event.EventName = wake.EventName
	event.EventKey = wake.EventKey
	event.DeliveryID = wake.DeliveryID
	if err := recordPendingCIEvent(ctx, tx, event); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit pending CI webhook wake: %w", err)
	}

	return true, nil
}

// CheckNow promotes exactly the operator-visible revision and makes it due.
func (s *Store) CheckNow(
	ctx context.Context,
	wake pendingci.CheckNowRequest,
) (pendingci.Request, error) {
	if err := wake.Validate(); err != nil {
		return pendingci.Request{}, err
	}
	return s.updatePendingCIWithEvents(ctx, wake.ID, "check pending CI request now", `
UPDATE pending_ci_requests SET
    schedule = ?, next_check_at = ?, lease_expires_at = NULL,
	last_event_key = ?, next_check_trigger = ?, updated_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ?`,
		func(before, _ pendingci.Request) []pendingci.Event {
			event := pendingCIAuditEvent(
				before.ID, pendingci.EventWakeReceived, pendingci.TriggerManual,
				before.LastObservedState, "Operator requested an immediate reconciliation",
				wake.OccurredAt,
			)
			event.EventName = "panel"
			event.EventKey = wake.EventKey

			return []pendingci.Event{event}
		},
		pendingci.ScheduleActive,
		wake.OccurredAt,
		wake.EventKey,
		pendingci.TriggerManual,
		wake.OccurredAt,
		wake.ID,
		pendingci.LifecycleArmed,
		wake.ExpectedRevision,
	)
}

// ClaimMerge proves that the passing observation still owns the current
// lease. A webhook that arrived after observation clears that lease and bumps
// the revision, so a stale worker cannot start the merge side effect.
func (s *Store) ClaimMerge(
	ctx context.Context,
	claim pendingci.ClaimMergeRequest,
) (pendingci.Request, error) {
	if err := claim.Validate(); err != nil {
		return pendingci.Request{}, err
	}
	return s.updatePendingCIWithEvents(ctx, claim.ID, "claim pending CI merge", `
UPDATE pending_ci_requests SET
    updated_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ?
  AND lease_expires_at IS NOT NULL AND lease_expires_at > ?`,
		func(before, _ pendingci.Request) []pendingci.Event {
			observation := claim.Observation
			state := string(observation.State)
			if state == "" {
				state = before.LastObservedState
			}
			summary := observation.Summary
			if summary == "" {
				summary = "CI remained passing after the quiet period"
			}
			observed := pendingCIAuditEvent(
				before.ID, pendingci.EventChecksObserved,
				normalizedTrigger(before.NextCheckTrigger, pendingci.TriggerQuietPeriod),
				state, summary, claim.ClaimedAt,
			)
			started := pendingCIAuditEvent(
				before.ID, pendingci.EventMergeStarted,
				normalizedTrigger(before.NextCheckTrigger, pendingci.TriggerQuietPeriod),
				state, "CI is stable; submitting the exact-head merge", claim.ClaimedAt,
			)

			return []pendingci.Event{observed, started}
		},
		claim.ClaimedAt,
		claim.ID,
		pendingci.LifecycleArmed,
		claim.ExpectedRevision,
		claim.ClaimedAt,
	)
}

func (s *Store) Reschedule(
	ctx context.Context,
	change pendingci.RescheduleRequest,
) (pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return pendingci.Request{}, err
	}

	trigger := change.NextCheckTrigger
	if trigger == "" {
		trigger = pendingci.TriggerFallback
		if change.LastObservedState == string(pendingci.ObservedPassing) {
			trigger = pendingci.TriggerQuietPeriod
		}
	}
	return s.updatePendingCIWithEvents(ctx, change.ID, "reschedule pending CI request", `
UPDATE pending_ci_requests SET
    schedule = ?, head_sha = ?, next_check_at = ?, lease_expires_at = NULL,
    last_progress_at = ?, last_observed_state = ?, last_fingerprint = ?,
	next_check_trigger = ?, updated_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ?`,
		func(before, _ pendingci.Request) []pendingci.Event {
			summary := change.ObservationSummary
			if summary == "" {
				summary = "Observed CI state " + change.LastObservedState
			}
			return []pendingci.Event{pendingCIAuditEvent(
				before.ID, pendingci.EventChecksObserved,
				normalizedTrigger(before.NextCheckTrigger, pendingci.TriggerFallback),
				change.LastObservedState, summary, change.CheckedAt,
			)}
		},
		change.Schedule,
		change.HeadSHA,
		change.NextCheckAt,
		change.LastProgressAt,
		change.LastObservedState,
		change.LastFingerprint,
		trigger,
		change.CheckedAt,
		change.ID,
		pendingci.LifecycleArmed,
		change.ExpectedRevision,
	)
}

func (s *Store) Finish(
	ctx context.Context,
	change pendingci.FinishRequest,
) (pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return pendingci.Request{}, err
	}

	return s.updatePendingCIWithEvents(ctx, change.ID, "finish pending CI request", `
UPDATE pending_ci_requests SET
    lifecycle = ?, reason = ?, next_check_at = ?, lease_expires_at = NULL,
    cleanup_pending = TRUE, cleanup_artifacts_done = FALSE,
    cleanup_attempts = 0, cleanup_error = '',
	next_check_trigger = ?, updated_at = ?, finished_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ?`,
		func(before, _ pendingci.Request) []pendingci.Event {
			return []pendingci.Event{pendingCIAuditEvent(
				before.ID, pendingci.EventFinished,
				normalizedTrigger(change.Trigger, before.NextCheckTrigger),
				string(change.Lifecycle), change.Reason, change.FinishedAt,
			)}
		},
		change.Lifecycle,
		change.Reason,
		change.FinishedAt,
		pendingci.TriggerCleanup,
		change.FinishedAt,
		change.FinishedAt,
		change.ID,
		pendingci.LifecycleArmed,
		change.ExpectedRevision,
	)
}

func (s *Store) CompleteCleanup(
	ctx context.Context,
	change pendingci.CompleteCleanupRequest,
) (pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return pendingci.Request{}, err
	}

	return s.updatePendingCIWithEvents(ctx, change.ID, "complete pending CI cleanup", `
UPDATE pending_ci_requests SET
    cleanup_pending = FALSE, cleanup_error = '', lease_expires_at = NULL,
    updated_at = ?, revision = revision + 1
WHERE id = ? AND cleanup_pending = TRUE AND cleanup_artifacts_done = TRUE AND revision = ?`,
		func(before, _ pendingci.Request) []pendingci.Event {
			return []pendingci.Event{pendingCIAuditEvent(
				before.ID, pendingci.EventCleanupCompleted, pendingci.TriggerCleanup,
				string(before.Lifecycle), "Removed pending-CI artifacts", change.CompletedAt,
			)}
		},
		change.CompletedAt,
		change.ID,
		change.ExpectedRevision,
	)
}

func (s *Store) MarkCleanupArtifactsDone(
	ctx context.Context,
	change pendingci.MarkCleanupArtifactsDoneRequest,
) (pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return pendingci.Request{}, err
	}
	return s.updatePendingCIWithEvents(ctx, change.ID, "mark pending CI cleanup artifacts", `
UPDATE pending_ci_requests SET
    cleanup_artifacts_done = TRUE, next_check_at = ?, lease_expires_at = NULL,
    cleanup_error = '', updated_at = ?, revision = revision + 1
WHERE id = ? AND cleanup_pending = TRUE AND cleanup_artifacts_done = FALSE AND revision = ?`,
		func(_, _ pendingci.Request) []pendingci.Event { return nil },
		change.MarkedAt,
		change.MarkedAt,
		change.ID,
		change.ExpectedRevision,
	)
}

// HasPendingCleanup reports whether terminal artifacts still belong to the
// service within a repository or pull-request scope.
func (s *Store) HasPendingCleanup(
	ctx context.Context,
	filter pendingci.CleanupFilter,
) (bool, error) {
	if err := filter.Validate(); err != nil {
		return false, err
	}
	query := "SELECT EXISTS(SELECT 1 FROM pending_ci_requests" +
		" WHERE repository_id = ? AND cleanup_pending = TRUE"
	arguments := []any{filter.RepositoryID}
	if filter.PullRequest > 0 {
		query += " AND pull_request = ?"
		arguments = append(arguments, filter.PullRequest)
	}
	if filter.ExcludeID > 0 {
		query += " AND id != ?"
		arguments = append(arguments, filter.ExcludeID)
	}
	if filter.ArtifactsPendingOnly {
		query += " AND cleanup_artifacts_done = FALSE"
	}
	query += ")"

	var pending bool
	if err := s.db.QueryRowContext(ctx, query, arguments...).Scan(&pending); err != nil {
		return false, fmt.Errorf("read pending CI cleanup ownership: %w", err)
	}

	return pending, nil
}

func (s *Store) RetryCleanup(
	ctx context.Context,
	change pendingci.RetryCleanupRequest,
) (pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return pendingci.Request{}, err
	}

	return s.updatePendingCIWithEvents(ctx, change.ID, "retry pending CI cleanup", `
UPDATE pending_ci_requests SET
	next_check_at = ?, next_check_trigger = ?, lease_expires_at = NULL,
    cleanup_attempts = cleanup_attempts + 1, cleanup_error = ?,
    updated_at = ?, revision = revision + 1
WHERE id = ? AND cleanup_pending = TRUE AND revision = ?`,
		func(before, _ pendingci.Request) []pendingci.Event {
			return []pendingci.Event{pendingCIAuditEvent(
				before.ID, pendingci.EventCleanupRetry, pendingci.TriggerCleanup,
				string(before.Lifecycle), change.Error, change.FailedAt,
			)}
		},
		change.NextAttemptAt,
		pendingci.TriggerCleanup,
		change.Error,
		change.FailedAt,
		change.ID,
		change.ExpectedRevision,
	)
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
	comparison, err := pendingci.CompareSourceEvents(
		change.SourceRevision, change.SourceSequence, change.SourceOrder,
		request.SourceRevision, request.SourceSequence, request.SourceOrder,
	)
	if err != nil {
		return nil, err
	}
	if comparison < 0 {
		return nil, nil
	}

	result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    lifecycle = ?, reason = ?, next_check_at = ?, lease_expires_at = NULL,
    cleanup_pending = TRUE, cleanup_artifacts_done = FALSE,
    cleanup_attempts = 0, cleanup_error = '',
    next_check_trigger = ?, updated_at = ?, finished_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ?`,
		pendingci.LifecycleCancelled,
		change.Reason,
		change.CancelledAt,
		pendingci.TriggerCleanup,
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
	if err := recordPendingCIEvent(ctx, tx, pendingCIAuditEvent(
		request.ID,
		pendingci.EventFinished,
		change.Trigger,
		string(pendingci.LifecycleCancelled),
		change.Reason,
		change.CancelledAt,
	)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit pending CI cancellation: %w", err)
	}
	request.Lifecycle = pendingci.LifecycleCancelled
	request.Reason = change.Reason
	request.LeaseExpiresAt = nil
	request.UpdatedAt = change.CancelledAt
	request.FinishedAt = timePointer(change.CancelledAt)
	request.NextCheckAt = change.CancelledAt
	request.NextCheckTrigger = pendingci.TriggerCleanup
	request.CleanupPending = true
	request.CleanupArtifactsDone = false
	request.CleanupAttempts = 0
	request.CleanupError = ""
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
