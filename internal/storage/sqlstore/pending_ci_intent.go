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

const (
	pendingCIIntentArm    = "arm"
	pendingCIIntentCancel = "cancel"
)

type pendingCIIntent struct {
	repositoryID string
	pullRequest  int
	commentID    int64
	revision     string
	sequence     int
	order        int64
	kind         string
	recordedAt   time.Time
}

// CancelByIntent records cleanup before terminalizing the current request.
// The tombstone prevents delayed merge commands from recreating cancelled work.
func (s *Store) CancelByIntent(
	ctx context.Context,
	change pendingci.CancelIntentRequest,
) (pendingci.CancelIntentResult, error) {
	if err := change.Validate(); err != nil {
		return pendingci.CancelIntentResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pendingci.CancelIntentResult{}, fmt.Errorf(
			"begin pending CI intent cancellation: %w", err,
		)
	}
	defer func() { _ = tx.Rollback() }()

	stale, duplicate, err := comparePendingCICancelHistory(ctx, tx, change)
	if err != nil {
		return pendingci.CancelIntentResult{}, err
	}
	if stale {
		return pendingci.CancelIntentResult{}, nil
	}
	if duplicate {
		return pendingci.CancelIntentResult{Accepted: true}, nil
	}
	if err := recordPendingCIIntent(ctx, tx, pendingCIIntent{
		repositoryID: change.RepositoryID, pullRequest: change.PullRequest,
		commentID: change.CommentID, revision: change.SourceRevision,
		sequence: change.SourceSequence, order: change.SourceOrder,
		kind: pendingCIIntentCancel, recordedAt: change.CancelledAt,
	}); err != nil {
		return pendingci.CancelIntentResult{}, err
	}
	request, err := getArmedPendingCI(ctx, tx, change.RepositoryID, change.PullRequest)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return pendingci.CancelIntentResult{}, fmt.Errorf(
				"commit pending CI cancellation intent: %w", err,
			)
		}

		return pendingci.CancelIntentResult{Accepted: true}, nil
	}
	if err != nil {
		return pendingci.CancelIntentResult{}, fmt.Errorf(
			"read pending CI intent cancellation target: %w", err,
		)
	}
	if err := cancelPendingCIRequest(ctx, tx, &request, change); err != nil {
		return pendingci.CancelIntentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return pendingci.CancelIntentResult{}, fmt.Errorf(
			"commit pending CI intent cancellation: %w", err,
		)
	}

	return pendingci.CancelIntentResult{Accepted: true, Request: &request}, nil
}

func cancelPendingCIRequest(
	ctx context.Context,
	tx *transaction,
	request *pendingci.Request,
	change pendingci.CancelIntentRequest,
) error {
	result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    lifecycle = ?, reason = ?, next_check_at = ?, lease_expires_at = NULL,
    cleanup_pending = TRUE, cleanup_artifacts_done = FALSE,
    cleanup_attempts = 0, cleanup_error = '',
    updated_at = ?, finished_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND revision = ?`,
		pendingci.LifecycleCancelled, change.Reason,
		change.CancelledAt, change.CancelledAt,
		change.CancelledAt, request.ID,
		pendingci.LifecycleArmed, request.Revision,
	)
	if err != nil {
		return fmt.Errorf("cancel pending CI request by intent: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read pending CI intent cancellation result: %w", err)
	}
	if changed != 1 {
		return storage.ErrConflict
	}
	request.Lifecycle = pendingci.LifecycleCancelled
	request.Reason = change.Reason
	request.LeaseExpiresAt = nil
	request.UpdatedAt = change.CancelledAt
	request.FinishedAt = timePointer(change.CancelledAt)
	request.NextCheckAt = change.CancelledAt
	request.CleanupPending = true
	request.CleanupArtifactsDone = false
	request.CleanupAttempts = 0
	request.CleanupError = ""
	request.Revision++

	return nil
}

func comparePendingCICancelHistory(
	ctx context.Context,
	tx *transaction,
	change pendingci.CancelIntentRequest,
) (bool, bool, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT source_comment_id, source_revision, source_order
FROM pending_ci_requests
WHERE repository_id = ? AND pull_request = ? AND source_comment_id > 0`,
		change.RepositoryID, change.PullRequest,
	)
	if err != nil {
		return false, false, fmt.Errorf("read pending CI cancellation history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var commentID, order int64
		var revision string
		if err := rows.Scan(&commentID, &revision, &order); err != nil {
			return false, false, fmt.Errorf("scan pending CI cancellation history: %w", err)
		}
		comparison, err := pendingci.CompareSourceIntent(
			change.SourceRevision, change.CommentID, change.SourceOrder,
			revision, commentID, order,
		)
		if errors.Is(err, pendingci.ErrAmbiguousSourceRevision) {
			continue
		}
		if err != nil {
			return false, false, err
		}
		if comparison < 0 {
			return true, false, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, false, fmt.Errorf("iterate pending CI cancellation history: %w", err)
	}
	if err := rows.Close(); err != nil {
		return false, false, fmt.Errorf("close pending CI cancellation history: %w", err)
	}
	intent, err := latestPendingCIIntent(ctx, tx, change.RepositoryID, change.PullRequest)
	if err != nil || intent == nil {
		return false, false, err
	}
	comparison, err := pendingci.CompareSourceIntent(
		change.SourceRevision, change.CommentID, change.SourceOrder,
		intent.revision, intent.commentID, intent.order,
	)
	if errors.Is(err, pendingci.ErrAmbiguousSourceRevision) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}

	return comparison < 0, comparison == 0 && intent.kind == pendingCIIntentCancel, nil
}

func latestPendingCIIntent(
	ctx context.Context,
	tx *transaction,
	repositoryID string,
	pullRequest int,
) (*pendingCIIntent, error) {
	intent := &pendingCIIntent{}
	err := tx.QueryRowContext(ctx, `
SELECT source_comment_id, source_revision, source_sequence, source_order,
       intent_kind
FROM pending_ci_intents
WHERE repository_id = ? AND pull_request = ?`, repositoryID, pullRequest).Scan(
		&intent.commentID, &intent.revision, &intent.sequence, &intent.order,
		&intent.kind,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read latest pending CI intent: %w", err)
	}
	intent.repositoryID = repositoryID
	intent.pullRequest = pullRequest

	return intent, nil
}

func comparePendingCIArmIntent(
	ctx context.Context,
	tx *transaction,
	arm pendingci.ArmRequest,
) (bool, bool, error) {
	intent, err := latestPendingCIIntent(ctx, tx, arm.RepositoryID, arm.PullRequest)
	if err != nil || intent == nil {
		return false, false, err
	}
	comparison, err := pendingci.CompareSourceIntent(
		arm.SourceRevision, arm.SourceCommentID, arm.SourceOrder,
		intent.revision, intent.commentID, intent.order,
	)
	if err != nil {
		return false, false, err
	}

	return comparison < 0, comparison == 0, nil
}

func recordPendingCIIntent(ctx context.Context, tx *transaction, intent pendingCIIntent) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO pending_ci_intents (
    repository_id, pull_request, source_comment_id, source_revision,
    source_sequence, source_order, intent_kind, recorded_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(repository_id, pull_request) DO UPDATE SET
    source_comment_id = excluded.source_comment_id,
    source_revision = excluded.source_revision,
    source_sequence = excluded.source_sequence,
    source_order = excluded.source_order,
    intent_kind = excluded.intent_kind,
    recorded_at = excluded.recorded_at`,
		intent.repositoryID, intent.pullRequest, intent.commentID, intent.revision,
		intent.sequence, intent.order, intent.kind, intent.recordedAt,
	)
	if err != nil {
		return fmt.Errorf("record latest pending CI intent: %w", err)
	}

	return nil
}
