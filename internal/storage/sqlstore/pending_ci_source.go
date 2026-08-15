package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

const legacyPendingCIDrainReason = "pre-upgrade pending CI request has no recoverable authorized head; reissue the command"

// ClaimSourceRevision records the newest delivery seen for one mutable issue
// comment. A retry of the same event remains eligible; an older event does not.
func (s *Store) ClaimSourceRevision(
	ctx context.Context,
	request pendingci.SourceRevisionRequest,
) (bool, error) {
	if err := request.Validate(); err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin pending CI source claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var revision, eventKey string
	var sequence int
	err = tx.QueryRowContext(ctx, `
SELECT source_revision, source_sequence, event_key
FROM pending_ci_source_revisions
WHERE repository_id = ? AND pull_request = ? AND source_comment_id = ?`,
		request.RepositoryID, request.PullRequest, request.CommentID,
	).Scan(&revision, &sequence, &eventKey)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `
INSERT INTO pending_ci_source_revisions (
    repository_id, pull_request, source_comment_id, source_revision,
    source_sequence, event_key, observed_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			request.RepositoryID, request.PullRequest, request.CommentID,
			request.Revision, request.Sequence, request.EventKey, request.ObservedAt,
		)
		if err != nil {
			return false, fmt.Errorf("insert pending CI source revision: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit pending CI source claim: %w", err)
		}

		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read pending CI source revision: %w", err)
	}

	comparison, err := pendingci.CompareSourceRevisions(
		request.Revision, request.Sequence, revision, sequence,
	)
	if err != nil {
		return false, err
	}
	if comparison < 0 || (comparison == 0 && eventKey != request.EventKey) {
		return false, nil
	}
	if comparison == 0 {
		return true, nil
	}

	_, err = tx.ExecContext(ctx, `
UPDATE pending_ci_source_revisions SET
    source_revision = ?, source_sequence = ?, event_key = ?, observed_at = ?
WHERE repository_id = ? AND pull_request = ? AND source_comment_id = ?`,
		request.Revision, request.Sequence, request.EventKey, request.ObservedAt,
		request.RepositoryID, request.PullRequest, request.CommentID,
	)
	if err != nil {
		return false, fmt.Errorf("update pending CI source revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit pending CI source claim: %w", err)
	}

	return true, nil
}

// DrainLegacy creates one terminal cleanup record for a label that predates
// durable exact-head requests. Any request history suppresses the import, so a
// stale label can never resurrect work that already finished.
func (s *Store) DrainLegacy(
	ctx context.Context,
	request pendingci.LegacyDrainRequest,
) (pendingci.LegacyDrainResult, error) {
	if err := request.Validate(); err != nil {
		return pendingci.LegacyDrainResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pendingci.LegacyDrainResult{}, fmt.Errorf("begin legacy pending CI drain: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existing int64
	err = tx.QueryRowContext(ctx, `
SELECT id FROM pending_ci_requests
WHERE repository_id = ? AND pull_request = ?
ORDER BY id DESC LIMIT 1`, request.RepositoryID, request.PullRequest).Scan(&existing)
	if err == nil {
		return pendingci.LegacyDrainResult{}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return pendingci.LegacyDrainResult{}, fmt.Errorf("read legacy pending CI history: %w", err)
	}

	var id int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO pending_ci_requests (
    target_id, installation_id, repository_id, repository_full_name,
    pull_request, head_sha, base_branch, merge_method, required_checks_only,
    requester, source_comment_id, source_revision, source_sequence, label,
    lifecycle, schedule, next_check_at, last_progress_at, reason,
    requested_at, updated_at, finished_at, cleanup_pending
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, TRUE)
RETURNING id`,
		request.TargetID, request.InstallationID, request.RepositoryID,
		request.RepositoryFullName, request.PullRequest, request.HeadSHA,
		request.BaseBranch, request.MergeMethod, request.RequiredChecksOnly,
		"smyklot-migration", "legacy-label-drain:v1", request.Label,
		pendingci.LifecycleCancelled, pendingci.ScheduleActive,
		request.DrainedAt, request.DrainedAt,
		legacyPendingCIDrainReason, request.DrainedAt,
		request.DrainedAt, request.DrainedAt,
	).Scan(&id)
	if err != nil {
		return pendingci.LegacyDrainResult{}, fmt.Errorf("insert legacy pending CI drain: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return pendingci.LegacyDrainResult{}, fmt.Errorf("commit legacy pending CI drain: %w", err)
	}

	drained := pendingci.Request{
		ID: id, TargetID: request.TargetID, InstallationID: request.InstallationID,
		RepositoryID: request.RepositoryID, RepositoryFullName: request.RepositoryFullName,
		PullRequest: request.PullRequest, HeadSHA: request.HeadSHA, BaseBranch: request.BaseBranch,
		MergeMethod: request.MergeMethod, RequiredChecksOnly: request.RequiredChecksOnly,
		Requester: "smyklot-migration", SourceRevision: "legacy-label-drain:v1",
		Label: request.Label, Lifecycle: pendingci.LifecycleCancelled,
		Schedule: pendingci.ScheduleActive, NextCheckAt: request.DrainedAt,
		LastProgressAt: request.DrainedAt, Reason: legacyPendingCIDrainReason,
		RequestedAt: request.DrainedAt, UpdatedAt: request.DrainedAt,
		FinishedAt: &request.DrainedAt, CleanupPending: true, Revision: 1,
	}

	return pendingci.LegacyDrainResult{Request: &drained}, nil
}

func comparePendingCISourceHistory(
	ctx context.Context,
	tx *transaction,
	arm pendingci.ArmRequest,
) (bool, bool, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT source_revision, source_sequence
FROM pending_ci_requests
WHERE repository_id = ? AND pull_request = ? AND source_comment_id > 0`,
		arm.RepositoryID, arm.PullRequest,
	)
	if err != nil {
		return false, false, fmt.Errorf("read pending CI source history: %w", err)
	}
	equal := false

	for rows.Next() {
		var revision string
		var sequence int
		if err := rows.Scan(&revision, &sequence); err != nil {
			return false, false, fmt.Errorf("scan pending CI source history: %w", err)
		}
		comparison, err := pendingci.CompareSourceRevisions(
			arm.SourceRevision, arm.SourceSequence, revision, sequence,
		)
		if err != nil {
			return false, false, err
		}
		if comparison < 0 {
			_ = rows.Close()

			return true, false, nil
		}
		if comparison == 0 {
			equal = true
		}
	}
	if err := rows.Err(); err != nil {
		return false, false, fmt.Errorf("iterate pending CI source history: %w", err)
	}
	if err := rows.Close(); err != nil {
		return false, false, fmt.Errorf("close pending CI source history: %w", err)
	}

	var revision string
	var sequence int
	err = tx.QueryRowContext(ctx, `
SELECT source_revision, source_sequence
FROM pending_ci_source_revisions
WHERE repository_id = ? AND pull_request = ? AND source_comment_id = ?`,
		arm.RepositoryID, arm.PullRequest, arm.SourceCommentID,
	).Scan(&revision, &sequence)
	if errors.Is(err, sql.ErrNoRows) {
		return false, equal, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("read current pending CI source revision: %w", err)
	}
	comparison, err := pendingci.CompareSourceRevisions(
		arm.SourceRevision, arm.SourceSequence, revision, sequence,
	)

	return comparison < 0, equal, err
}
