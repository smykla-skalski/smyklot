package sqlstore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

// CheckSourceRevision previews the same ordering decision as ClaimSourceRevision
// without recording a receipt. It is not a reservation: execution must still
// claim the source after observing current GitHub content under its coordinator.
func (s *Store) CheckSourceRevision(ctx context.Context, request pendingci.SourceRevisionRequest) (pendingci.SourceRevisionResult, error) {
	if err := request.Validate(); err != nil {
		return pendingci.SourceRevisionResult{}, err
	}
	result, _, err := checkPendingCISource(ctx, s.db, request)
	return result, err
}

type sourceHistoryReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type pendingCISource struct {
	revision string
	sequence int
	order    int64
}

func (source pendingCISource) compare(other pendingCISource) (int, error) {
	return pendingci.CompareSourceEvents(source.revision, source.sequence, source.order,
		other.revision, other.sequence, other.order)
}

// One query keeps the exact event and newest revision in the same snapshot.
// Existing receipts own their original order even if a caller passes a newer one.
func checkPendingCISource(ctx context.Context, reader sourceHistoryReader, request pendingci.SourceRevisionRequest) (pendingci.SourceRevisionResult, bool, error) {
	latest, retry, err := readPendingCISourceHistory(ctx, reader, request)
	if err != nil {
		return pendingci.SourceRevisionResult{}, false, err
	}
	candidate := pendingCISource{revision: request.Revision, sequence: request.Sequence, order: request.SourceOrder}
	if retry != nil {
		candidate = *retry
	}
	comparison := 1
	if latest != nil {
		comparison, err = candidate.compare(*latest)
		if err != nil {
			return pendingci.SourceRevisionResult{}, false, err
		}
	}
	accepted := comparison > 0 || (retry != nil && comparison == 0)
	result := pendingci.SourceRevisionResult{Accepted: accepted}
	if accepted || retry != nil {
		result.SourceOrder = candidate.order
	}
	return result, retry != nil, nil
}

func readPendingCISourceHistory(ctx context.Context, reader sourceHistoryReader, request pendingci.SourceRevisionRequest) (*pendingCISource, *pendingCISource, error) {
	rows, err := reader.QueryContext(ctx, `SELECT source_revision, source_sequence, source_order, event_key
FROM pending_ci_source_revisions
WHERE repository_id = ? AND pull_request = ? AND source_comment_id = ?`,
		request.RepositoryID, request.PullRequest, request.CommentID)
	if err != nil {
		return nil, nil, fmt.Errorf("read pending CI source history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var latest, retry *pendingCISource
	for rows.Next() {
		candidate := &pendingCISource{}
		var key string
		if err := rows.Scan(&candidate.revision, &candidate.sequence, &candidate.order, &key); err != nil {
			return nil, nil, fmt.Errorf("scan pending CI source history: %w", err)
		}
		if key == request.EventKey {
			retry = candidate
		}
		if latest == nil {
			latest = candidate
			continue
		}
		comparison, err := candidate.compare(*latest)
		if err != nil {
			return nil, nil, err
		}
		if comparison > 0 {
			latest = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate pending CI source history: %w", err)
	}
	return latest, retry, nil
}
