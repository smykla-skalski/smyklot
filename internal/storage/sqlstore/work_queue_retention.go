package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func (s *Store) PruneWorkQueue(ctx context.Context, now time.Time) (int64, error) {
	scopes, err := s.queueRetentionScopes(ctx)
	if err != nil {
		return 0, err
	}
	var removed int64
	for _, scope := range scopes {
		retention := workqueue.Retention(scope.kind)
		policy, policyErr := s.GetEffectiveQueuePolicy(ctx, scope.kind, scope.targetID)
		switch {
		case policyErr == nil:
			if policy.Retention != nil {
				retention = *policy.Retention
			}
		case !errors.Is(policyErr, storage.ErrNotFound):
			return removed, policyErr
		}
		changed, deleteErr := s.pruneQueueRetentionScope(
			ctx, scope, now.Add(-retention),
		)
		if deleteErr != nil {
			return removed, deleteErr
		}
		removed += changed
	}

	return removed, nil
}

type queueRetentionScope struct {
	kind     workqueue.Kind
	targetID *string
}

func (s *Store) queueRetentionScopes(ctx context.Context) ([]queueRetentionScope, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT kind, target_id
FROM queue_items
WHERE state IN ('succeeded', 'failed', 'cancelled', 'superseded')
	  AND finished_at IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("list queue retention scopes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	scopes := make([]queueRetentionScope, 0)
	for rows.Next() {
		var kind workqueue.Kind
		var targetID sql.NullString
		if err := rows.Scan(&kind, &targetID); err != nil {
			return nil, fmt.Errorf("read queue retention scope: %w", err)
		}
		scopes = append(scopes, queueRetentionScope{
			kind: kind, targetID: stringPointer(targetID),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list queue retention scopes: %w", err)
	}

	return scopes, nil
}

func (s *Store) pruneQueueRetentionScope(
	ctx context.Context,
	scope queueRetentionScope,
	cutoff time.Time,
) (int64, error) {
	query := `DELETE FROM queue_items
WHERE kind = ? AND target_id IS NULL
  AND state IN ('succeeded', 'failed', 'cancelled', 'superseded')
  AND finished_at IS NOT NULL AND finished_at <= ?`
	arguments := []any{scope.kind, cutoff}
	if scope.targetID != nil {
		query = `DELETE FROM queue_items
WHERE kind = ? AND target_id = ?
  AND state IN ('succeeded', 'failed', 'cancelled', 'superseded')
  AND finished_at IS NOT NULL AND finished_at <= ?`
		arguments = []any{scope.kind, *scope.targetID, cutoff}
	}
	result, err := s.db.ExecContext(ctx, query, arguments...)
	if err != nil {
		return 0, fmt.Errorf("prune queue retention scope %s: %w", scope.kind, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read pruned queue retention scope %s: %w", scope.kind, err)
	}

	return changed, nil
}
