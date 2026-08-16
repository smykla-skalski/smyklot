package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

const legacyPendingCIDrainReason = "pre-upgrade pending CI request has no recoverable authorized head; reissue the command"

// ClaimSourceRevision records a source event in the durable delivery receipt
// order assigned before concurrent execution. An exact retry reuses its order;
// a superseded retry is rejected. GitHub timestamps still reject genuinely
// older revisions.
func (s *Store) ClaimSourceRevision(
	ctx context.Context,
	request pendingci.SourceRevisionRequest,
) (pendingci.SourceRevisionResult, error) {
	if err := request.Validate(); err != nil {
		return pendingci.SourceRevisionResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pendingci.SourceRevisionResult{}, fmt.Errorf(
			"begin pending CI source claim: %w", err,
		)
	}
	defer func() { _ = tx.Rollback() }()

	retry, err := pendingCISourceRetry(ctx, tx, request)
	if err != nil || retry != nil {
		if err != nil {
			return pendingci.SourceRevisionResult{}, err
		}

		return *retry, nil
	}
	latest, err := latestPendingCISource(ctx, tx, request)
	if err != nil {
		return pendingci.SourceRevisionResult{}, err
	}
	if latest != nil {
		comparison, compareErr := pendingci.CompareSourceEvents(
			request.Revision, request.Sequence, request.SourceOrder,
			latest.revision, latest.sequence, latest.order,
		)
		if compareErr != nil {
			return pendingci.SourceRevisionResult{}, compareErr
		}
		if comparison <= 0 {
			return pendingci.SourceRevisionResult{}, nil
		}
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO pending_ci_source_revisions (
    repository_id, pull_request, source_comment_id, source_revision,
    source_sequence, event_key, source_order, observed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		request.RepositoryID, request.PullRequest, request.CommentID,
		request.Revision, request.Sequence, request.EventKey, request.SourceOrder,
		request.ObservedAt,
	)
	if err != nil {
		return pendingci.SourceRevisionResult{}, fmt.Errorf(
			"insert pending CI source revision: %w", err,
		)
	}
	if err := tx.Commit(); err != nil {
		return pendingci.SourceRevisionResult{}, fmt.Errorf(
			"commit pending CI source claim: %w", err,
		)
	}

	return pendingci.SourceRevisionResult{
		Accepted: true, SourceOrder: request.SourceOrder,
	}, nil
}

type pendingCISource struct {
	revision string
	sequence int
	order    int64
}

func pendingCISourceRetry(
	ctx context.Context,
	tx *transaction,
	request pendingci.SourceRevisionRequest,
) (*pendingci.SourceRevisionResult, error) {
	var retry pendingCISource
	err := tx.QueryRowContext(ctx, `
SELECT source_revision, source_sequence, source_order
FROM pending_ci_source_revisions
WHERE repository_id = ? AND pull_request = ? AND source_comment_id = ? AND event_key = ?`,
		request.RepositoryID, request.PullRequest, request.CommentID, request.EventKey,
	).Scan(&retry.revision, &retry.sequence, &retry.order)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read pending CI source retry: %w", err)
	}

	latest, err := latestPendingCISource(ctx, tx, request)
	if err != nil {
		return nil, fmt.Errorf("read latest pending CI source retry: %w", err)
	}
	comparison, err := pendingci.CompareSourceEvents(
		retry.revision, retry.sequence, retry.order,
		latest.revision, latest.sequence, latest.order,
	)
	if err != nil {
		return nil, err
	}

	return &pendingci.SourceRevisionResult{
		Accepted: comparison == 0, SourceOrder: retry.order,
	}, nil
}

func latestPendingCISource(
	ctx context.Context,
	tx *transaction,
	request pendingci.SourceRevisionRequest,
) (*pendingCISource, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT source_revision, source_sequence, source_order
FROM pending_ci_source_revisions
WHERE repository_id = ? AND pull_request = ? AND source_comment_id = ?`,
		request.RepositoryID, request.PullRequest, request.CommentID,
	)
	if err != nil {
		return nil, fmt.Errorf("read latest pending CI source revision: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var latest *pendingCISource
	for rows.Next() {
		candidate := &pendingCISource{}
		if err := rows.Scan(&candidate.revision, &candidate.sequence, &candidate.order); err != nil {
			return nil, fmt.Errorf("scan pending CI source revision: %w", err)
		}
		if latest == nil {
			latest = candidate

			continue
		}
		comparison, err := pendingci.CompareSourceEvents(
			candidate.revision, candidate.sequence, candidate.order,
			latest.revision, latest.sequence, latest.order,
		)
		if err != nil {
			return nil, err
		}
		if comparison > 0 {
			latest = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending CI source revisions: %w", err)
	}

	return latest, nil
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

	hasHistory, err := hasPendingCIHistory(ctx, tx, request)
	if err != nil {
		return pendingci.LegacyDrainResult{}, err
	}
	if hasHistory {
		return pendingci.LegacyDrainResult{}, nil
	}

	requests := make([]pendingci.Request, 0, len(request.Labels))
	for _, label := range request.Labels {
		drained, insertErr := insertLegacyPendingCI(ctx, tx, request, label)
		if insertErr != nil {
			return pendingci.LegacyDrainResult{}, insertErr
		}
		requests = append(requests, drained)
	}
	if err := tx.Commit(); err != nil {
		return pendingci.LegacyDrainResult{}, fmt.Errorf("commit legacy pending CI drain: %w", err)
	}

	return pendingci.LegacyDrainResult{Requests: requests}, nil
}

func hasPendingCIHistory(
	ctx context.Context,
	tx *transaction,
	request pendingci.LegacyDrainRequest,
) (bool, error) {
	var existing int64
	err := tx.QueryRowContext(ctx, `
SELECT id FROM pending_ci_requests
WHERE repository_id = ? AND pull_request = ?
ORDER BY id DESC LIMIT 1`, request.RepositoryID, request.PullRequest).Scan(&existing)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read legacy pending CI history: %w", err)
	}

	return false, nil
}

func insertLegacyPendingCI(
	ctx context.Context,
	tx *transaction,
	request pendingci.LegacyDrainRequest,
	label pendingci.LegacyPendingCILabel,
) (pendingci.Request, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO pending_ci_requests (
    target_id, installation_id, repository_id, repository_full_name,
    pull_request, head_sha, base_branch, merge_method, required_checks_only,
    requester, source_comment_id, source_revision, source_sequence, label,
	    lifecycle, schedule, next_check_trigger, next_check_at, last_progress_at, reason,
    requested_at, updated_at, finished_at, cleanup_pending
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, TRUE)
RETURNING id`,
		request.TargetID, request.InstallationID, request.RepositoryID,
		request.RepositoryFullName, request.PullRequest, request.HeadSHA,
		request.BaseBranch, label.MergeMethod, label.RequiredChecksOnly,
		"smyklot-migration", "legacy-label-drain:v1", label.Label,
		pendingci.LifecycleCancelled, pendingci.ScheduleActive,
		pendingci.TriggerCleanup,
		request.DrainedAt, request.DrainedAt,
		legacyPendingCIDrainReason, request.DrainedAt,
		request.DrainedAt, request.DrainedAt,
	).Scan(&id)
	if err != nil {
		return pendingci.Request{}, fmt.Errorf("insert legacy pending CI drain: %w", err)
	}
	if err := recordPendingCIEvent(ctx, tx, pendingCIAuditEvent(
		id,
		pendingci.EventFinished,
		pendingci.TriggerCleanup,
		string(pendingci.LifecycleCancelled),
		legacyPendingCIDrainReason,
		request.DrainedAt,
	)); err != nil {
		return pendingci.Request{}, err
	}

	drained := pendingci.Request{
		ID: id, TargetID: request.TargetID, InstallationID: request.InstallationID,
		RepositoryID: request.RepositoryID, RepositoryFullName: request.RepositoryFullName,
		PullRequest: request.PullRequest, HeadSHA: request.HeadSHA, BaseBranch: request.BaseBranch,
		MergeMethod: label.MergeMethod, RequiredChecksOnly: label.RequiredChecksOnly,
		Requester: "smyklot-migration", SourceRevision: "legacy-label-drain:v1",
		Label: label.Label, Lifecycle: pendingci.LifecycleCancelled,
		Schedule: pendingci.ScheduleActive, NextCheckTrigger: pendingci.TriggerCleanup,
		NextCheckAt:    request.DrainedAt,
		LastProgressAt: request.DrainedAt, Reason: legacyPendingCIDrainReason,
		RequestedAt: request.DrainedAt, UpdatedAt: request.DrainedAt,
		FinishedAt: &request.DrainedAt, CleanupPending: true, Revision: 1,
	}

	return drained, nil
}

func comparePendingCISourceHistory(
	ctx context.Context,
	tx *transaction,
	arm pendingci.ArmRequest,
) (bool, bool, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT source_comment_id, source_revision, source_order
FROM pending_ci_requests
WHERE repository_id = ? AND pull_request = ? AND source_comment_id > 0`,
		arm.RepositoryID, arm.PullRequest,
	)
	if err != nil {
		return false, false, fmt.Errorf("read pending CI source history: %w", err)
	}
	equal := false

	for rows.Next() {
		var commentID int64
		var revision string
		var order int64
		if err := rows.Scan(&commentID, &revision, &order); err != nil {
			return false, false, fmt.Errorf("scan pending CI source history: %w", err)
		}
		comparison, err := pendingci.CompareSourceIntent(
			arm.SourceRevision, arm.SourceCommentID, arm.SourceOrder,
			revision, commentID, order,
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
	staleIntent, equalIntent, err := comparePendingCIArmIntent(ctx, tx, arm)
	if err != nil || staleIntent {
		return staleIntent, false, err
	}
	equal = equal || equalIntent

	var revision string
	var sequence int
	var order int64
	err = tx.QueryRowContext(ctx, `
SELECT source_revision, source_sequence, source_order
FROM pending_ci_source_revisions
WHERE repository_id = ? AND pull_request = ? AND source_comment_id = ?
ORDER BY source_order DESC LIMIT 1`,
		arm.RepositoryID, arm.PullRequest, arm.SourceCommentID,
	).Scan(&revision, &sequence, &order)
	if errors.Is(err, sql.ErrNoRows) {
		return false, equal, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("read current pending CI source revision: %w", err)
	}
	comparison, err := pendingci.CompareSourceEvents(
		arm.SourceRevision, arm.SourceSequence, arm.SourceOrder,
		revision, sequence, order,
	)

	return comparison < 0, equal, err
}
