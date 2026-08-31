package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

const pendingCISelect = `
SELECT id, target_id, installation_id, repository_id, repository_full_name,
       pull_request, pull_request_title, head_sha, base_branch, merge_method,
       required_checks_only,
       requester, source_comment_id, source_revision, source_sequence, source_order,
	       artifact_kind, label, check_slot_id, retired_check_slot_id,
	       authorization_state, gate_state,
	       candidate_head_sha, candidate_base_branch, authorized_by, authorized_at,
	       merge_phase, lifecycle, schedule, next_check_trigger,
       next_check_at, lease_expires_at, last_progress_at, last_observed_state,
       last_fingerprint, last_event_key, reason, requested_at, updated_at,
       finished_at, cleanup_pending, cleanup_artifacts_done,
       cleanup_attempts, cleanup_error, revision
FROM pending_ci_requests`

// CheckArm verifies the current durable source order without changing it.
// Callers use it before publishing external artifacts; Arm remains the final
// atomic authority in case another process changes the order afterward.
func (s *Store) CheckArm(ctx context.Context, arm pendingci.ArmRequest) error {
	arm = normalizedArmRequest(arm)
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
	arm = normalizedArmRequest(arm)
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
		if err := syncPendingCIQueue(ctx, tx, superseded); err != nil {
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
	request := armedRequest(id, arm)
	if err := insertArmedPendingCIQueue(ctx, tx, id, arm); err != nil {
		return pendingci.ArmResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return pendingci.ArmResult{}, fmt.Errorf("commit pending CI arm: %w", err)
	}

	resultValue := pendingci.ArmResult{Request: request}
	if superseded.ID != 0 {
		resultValue.Superseded = &superseded
	}

	return resultValue, nil
}

func insertArmedPendingCIQueue(
	ctx context.Context,
	tx *transaction,
	id int64,
	arm pendingci.ArmRequest,
) error {
	sourceID := strconv.FormatInt(id, 10)

	return insertLinkedQueueItem(ctx, tx, linkedQueueItem{
		ID: "pending-ci:" + sourceID, Kind: workqueue.KindPendingCI,
		Lane: workqueue.LanePendingCI, TargetID: arm.TargetID,
		RepositoryID: &arm.RepositoryID, SourceKind: queueSourcePendingCI,
		SourceID: sourceID,
		/* What the work IS. The repository and the pull request ride the row as its
		   own fields, so a title that spelled them out - "Pending CI owner/repo #184" -
		   said the subject twice and the act not at all. */
		Title:   queuePendingCITitle(arm.MergeMethod),
		Summary: "Waiting for required checks", State: workqueue.StateScheduled,
		NotBefore: arm.RequestedAt,
		ActorID:   queueActorSystem,
		/* The pull request's own title rides the row, because the panel lists
		   work in flight by what it is ABOUT and the title is not derivable
		   from anything else the queue holds. Omitted when it is empty, which
		   is what a row armed before this column reads as. */
		Details: pendingCIQueueDetails(arm),
	})
}

// pendingCIQueueDetails carries what only the arm knows onto the queue row.
//
// An absent title and an empty one are the same fact - nobody recorded one -
// and a key holding "" would make a reader render an empty heading rather
// than fall back to the act.
func pendingCIQueueDetails(arm pendingci.ArmRequest) map[string]any {
	details := map[string]any{"pull_request": arm.PullRequest, "head_sha": arm.HeadSHA}
	if arm.PullRequestTitle != "" {
		details["pull_request_title"] = arm.PullRequestTitle
	}

	return details
}

// queuePendingCITitle names the act in the words the command used, so a row reads
// as the thing that will happen rather than as the lane it waits in.
func queuePendingCITitle(method pendingci.MergeMethod) string {
	switch string(method) {
	case "squash":
		return "Squash when CI passes"
	case "rebase":
		return "Rebase when CI passes"
	default:
		return "Merge when CI passes"
	}
}

func normalizedArmRequest(arm pendingci.ArmRequest) pendingci.ArmRequest {
	if arm.ArtifactKind == "" {
		arm.ArtifactKind = pendingci.ArtifactLabel
	}

	return arm
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
    pull_request, pull_request_title, head_sha, base_branch, merge_method,
    required_checks_only,
    requester, source_comment_id, source_revision, source_sequence, source_order,
		artifact_kind, label, check_slot_id, authorized_by, authorized_at,
		lifecycle, schedule, next_check_trigger,
    next_check_at, last_progress_at, requested_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id`,
		arm.TargetID,
		arm.InstallationID,
		arm.RepositoryID,
		arm.RepositoryFullName,
		arm.PullRequest,
		arm.PullRequestTitle,
		arm.HeadSHA,
		arm.BaseBranch,
		arm.MergeMethod,
		arm.RequiredChecksOnly,
		arm.Requester,
		arm.SourceCommentID,
		arm.SourceRevision,
		arm.SourceSequence,
		arm.SourceOrder,
		arm.ArtifactKind,
		nullableString(arm.Label),
		arm.CheckSlotID,
		arm.Requester,
		arm.RequestedAt,
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
		PullRequest: arm.PullRequest, PullRequestTitle: arm.PullRequestTitle,
		HeadSHA: arm.HeadSHA, BaseBranch: arm.BaseBranch,
		MergeMethod: arm.MergeMethod, RequiredChecksOnly: arm.RequiredChecksOnly,
		Requester: arm.Requester, SourceCommentID: arm.SourceCommentID,
		SourceRevision: arm.SourceRevision, SourceSequence: arm.SourceSequence,
		SourceOrder: arm.SourceOrder, ArtifactKind: arm.ArtifactKind,
		Label: arm.Label, CheckSlotID: arm.CheckSlotID,
		AuthorizationState: pendingci.AuthorizationAuthorized,
		GateState:          pendingci.GateReady, AuthorizedBy: arm.Requester,
		AuthorizedAt: arm.RequestedAt, MergePhase: pendingci.MergeWaiting,
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
		request.ArtifactKind == arm.ArtifactKind && request.Label == arm.Label &&
		equalInt64Pointers(request.CheckSlotID, arm.CheckSlotID)
}

func equalInt64Pointers(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
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

	request, choice, err := s.selectDuePendingCI(ctx, tx, now)
	if errors.Is(err, sql.ErrNoRows) {
		availableAt, availableErr := nextQueueAvailability(
			ctx, tx, workqueue.LanePendingCI, now,
		)
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
	if err := leaseLinkedQueueItem(
		ctx, tx, "pending-ci:"+strconv.FormatInt(request.ID, 10), now, leaseExpiresAt,
		"Checking pull request and CI state",
	); err != nil {
		return pendingci.LeaseResult{}, err
	}
	if err := advanceQueueDispatch(ctx, tx, choice, now); err != nil {
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

func (s *Store) selectDuePendingCI(
	ctx context.Context,
	tx *transaction,
	now time.Time,
) (pendingci.Request, queueDispatchChoice, error) {
	choice, available, err := s.nextQueueDispatch(ctx, tx, workqueue.LanePendingCI, now)
	if err != nil {
		return pendingci.Request{}, queueDispatchChoice{}, err
	}
	if !available {
		return pendingci.Request{}, queueDispatchChoice{}, sql.ErrNoRows
	}
	if choice.item.SourceKind != queueSourcePendingCI {
		return pendingci.Request{}, queueDispatchChoice{}, fmt.Errorf(
			"pending-CI queue item %q has unsupported source %q",
			choice.item.ID, choice.item.SourceKind,
		)
	}
	request, err := scanPendingCI(tx.QueryRowContext(ctx, pendingCISelect+`
WHERE id = ? AND (lifecycle = ? OR cleanup_pending = TRUE) AND next_check_at <= ?
  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
	`,
		choice.item.SourceID,
		pendingci.LifecycleArmed,
		now,
		now,
	))

	return request, choice, err
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
	request.Schedule = pendingci.ScheduleActive
	request.NextCheckAt = wake.OccurredAt
	request.NextCheckTrigger = pendingci.TriggerWebhook
	request.LeaseExpiresAt = nil
	request.UpdatedAt = wake.OccurredAt
	if err := syncPendingCIQueue(ctx, tx, request); err != nil {
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
    schedule = ?,
    next_check_at = ?, lease_expires_at = NULL,
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
	merge_phase = 'claimed', updated_at = ?, revision = revision + 1
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

func (s *Store) MarkMergeCheckSucceeded(
	ctx context.Context,
	change pendingci.MarkMergeCheckSucceededRequest,
) (pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return pendingci.Request{}, err
	}

	return s.updatePendingCIWithEvents(ctx, change.ID, "mark pending CI check successful", `
UPDATE pending_ci_requests SET
	merge_phase = 'check_succeeded', updated_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ? AND merge_phase = 'claimed'`,
		nil,
		change.MarkedAt,
		change.ID,
		pendingci.LifecycleArmed,
		change.ExpectedRevision,
	)
}

func (s *Store) RequireReauthorization(
	ctx context.Context,
	change pendingci.RequireReauthorizationRequest,
) (pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return pendingci.Request{}, err
	}

	return s.updatePendingCIWithEvents(ctx, change.ID, "require pending CI reauthorization", `
UPDATE pending_ci_requests SET
    authorization_state = 'reauthorization_required',
	candidate_head_sha = ?, candidate_base_branch = ?,
	retired_check_slot_id = CASE
		WHEN check_slot_id = ? THEN retired_check_slot_id
		WHEN retired_check_slot_id = ? THEN check_slot_id
		WHEN retired_check_slot_id IS NULL THEN check_slot_id
		ELSE retired_check_slot_id
	END,
	check_slot_id = ?,
    schedule = 'active', next_check_at = ?, lease_expires_at = NULL,
    last_observed_state = '', last_fingerprint = '', merge_phase = 'waiting',
    next_check_trigger = 'fallback', updated_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND artifact_kind = 'check' AND revision = ?
  AND (
	check_slot_id = ? OR retired_check_slot_id = ? OR retired_check_slot_id IS NULL
  )`,
		func(before, _ pendingci.Request) []pendingci.Event {
			return []pendingci.Event{pendingCIAuditEvent(
				before.ID,
				pendingci.EventChecksObserved,
				pendingci.TriggerFallback,
				string(pendingci.AuthorizationReauthorizationNeeded),
				"Pull request revision changed; waiting for reauthorization",
				change.ObservedAt,
			)}
		},
		change.CandidateHeadSHA,
		change.CandidateBase,
		change.CandidateCheckID,
		change.CandidateCheckID,
		change.CandidateCheckID,
		change.ObservedAt,
		change.ObservedAt,
		change.ID,
		pendingci.LifecycleArmed,
		change.ExpectedRevision,
		change.CandidateCheckID,
		change.CandidateCheckID,
	)
}

func (s *Store) ClearRetiredCheckSlot(
	ctx context.Context,
	change pendingci.ClearRetiredCheckSlotRequest,
) (pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return pendingci.Request{}, err
	}

	return s.updatePendingCIWithEvents(ctx, change.ID, "clear retired pending CI check", `
UPDATE pending_ci_requests SET
	retired_check_slot_id = NULL, next_check_at = ?, lease_expires_at = NULL,
	next_check_trigger = 'fallback', updated_at = ?, revision = revision + 1
WHERE id = ? AND revision = ? AND retired_check_slot_id = ?
  AND (lifecycle = 'armed' OR cleanup_pending = TRUE)`,
		func(_, _ pendingci.Request) []pendingci.Event { return nil },
		change.ClearedAt,
		change.ClearedAt,
		change.ID,
		change.ExpectedRevision,
		change.CheckSlotID,
	)
}

func (s *Store) Reauthorize(
	ctx context.Context,
	change pendingci.ReauthorizeRequest,
) (*pendingci.Request, error) {
	if err := change.Validate(); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin pending CI reauthorization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	request, err := getArmedPendingCI(ctx, tx, change.RepositoryID, change.PullRequest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read pending CI reauthorization target: %w", err)
	}
	if request.ArtifactKind == pendingci.ArtifactCheck &&
		request.AuthorizationState == pendingci.AuthorizationAuthorized &&
		request.HeadSHA == change.HeadSHA && request.BaseBranch == change.BaseBranch &&
		request.CheckSlotID != nil && *request.CheckSlotID == change.CheckSlotID &&
		request.LastEventKey == change.EventKey {
		return &request, nil
	}
	if request.ArtifactKind != pendingci.ArtifactCheck ||
		request.AuthorizationState != pendingci.AuthorizationReauthorizationNeeded ||
		request.CandidateHeadSHA != change.HeadSHA ||
		request.CandidateBaseBranch != change.BaseBranch ||
		request.CheckSlotID == nil || *request.CheckSlotID != change.CheckSlotID {
		return nil, nil
	}
	result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    head_sha = candidate_head_sha, base_branch = candidate_base_branch,
    candidate_head_sha = NULL, candidate_base_branch = NULL,
    authorization_state = 'authorized', authorized_by = ?, authorized_at = ?,
    schedule = 'active', next_check_at = ?, lease_expires_at = NULL,
    last_progress_at = ?, last_observed_state = '', last_fingerprint = '',
    last_event_key = ?, merge_phase = 'waiting', next_check_trigger = 'webhook',
    updated_at = ?, revision = revision + 1
WHERE id = ? AND revision = ? AND lifecycle = 'armed'
  AND artifact_kind = 'check' AND authorization_state = 'reauthorization_required'
  AND candidate_head_sha = ? AND candidate_base_branch = ? AND check_slot_id = ?`,
		change.Actor,
		change.AuthorizedAt,
		change.AuthorizedAt,
		change.AuthorizedAt,
		change.EventKey,
		change.AuthorizedAt,
		request.ID,
		request.Revision,
		change.HeadSHA,
		change.BaseBranch,
		change.CheckSlotID,
	)
	if err != nil {
		return nil, fmt.Errorf("reauthorize pending CI request: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read pending CI reauthorization result: %w", err)
	}
	if changed != 1 {
		return nil, nil
	}
	event := pendingCIAuditEvent(
		request.ID,
		pendingci.EventWakeReceived,
		pendingci.TriggerWebhook,
		string(pendingci.AuthorizationAuthorized),
		fmt.Sprintf("%s reauthorized merge after CI for the current revision", change.Actor),
		change.AuthorizedAt,
	)
	event.EventName = "check_run.requested_action"
	event.EventKey = change.EventKey
	event.DeliveryID = change.DeliveryID
	if err := recordPendingCIEvent(ctx, tx, event); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit pending CI reauthorization: %w", err)
	}

	applyReauthorization(&request, change)

	return &request, nil
}

func applyReauthorization(request *pendingci.Request, change pendingci.ReauthorizeRequest) {
	request.HeadSHA = change.HeadSHA
	request.BaseBranch = change.BaseBranch
	request.CandidateHeadSHA = ""
	request.CandidateBaseBranch = ""
	request.AuthorizationState = pendingci.AuthorizationAuthorized
	request.AuthorizedBy = change.Actor
	request.AuthorizedAt = change.AuthorizedAt
	request.Schedule = pendingci.ScheduleActive
	request.NextCheckAt = change.AuthorizedAt
	request.LeaseExpiresAt = nil
	request.LastProgressAt = change.AuthorizedAt
	request.LastObservedState = ""
	request.LastFingerprint = ""
	request.LastEventKey = change.EventKey
	request.MergePhase = pendingci.MergeWaiting
	request.NextCheckTrigger = pendingci.TriggerWebhook
	request.UpdatedAt = change.AuthorizedAt
	request.Revision++
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
	schedule = ?, head_sha = CASE WHEN artifact_kind = 'check' THEN head_sha ELSE ? END,
	next_check_at = ?, lease_expires_at = NULL,
	merge_phase = 'waiting',
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
WHERE id = ? AND cleanup_pending = TRUE AND cleanup_artifacts_done = TRUE
  AND retired_check_slot_id IS NULL AND revision = ?`,
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
	if err := syncPendingCIQueue(ctx, tx, request); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit pending CI cancellation: %w", err)
	}

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
	if filter.RepositoryID != "" {
		query += " AND repository_id = ?"
		arguments = append(arguments, filter.RepositoryID)
	}
	if filter.ArtifactKind != "" {
		query += " AND artifact_kind = ?"
		arguments = append(arguments, filter.ArtifactKind)
	}
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
