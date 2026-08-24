package sqlstore

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// PruneWorkQueue removes only terminal ledger rows whose effective workload
// policy has a retention period. Domain rows keep their own retention rules;
// this operation owns the queue ledger and its cascading event timeline.
func (s *Store) PruneWorkQueue(ctx context.Context, now time.Time) (int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT"+queueItemColumns+`
FROM queue_items
WHERE state IN ('succeeded', 'failed', 'cancelled', 'superseded')
  AND finished_at IS NOT NULL
ORDER BY finished_at, id`)
	if err != nil {
		return 0, fmt.Errorf("list retained queue items: %w", err)
	}
	items, err := collectRows(rows, scanQueueItem)
	if err != nil {
		return 0, fmt.Errorf("read retained queue items: %w", err)
	}
	var removed int64
	for _, item := range items {
		policy, policyErr := s.GetEffectiveQueuePolicy(ctx, item.Kind, item.TargetID)
		if policyErr != nil {
			return removed, policyErr
		}
		if !queueRetentionExpired(item, policy, now) {
			continue
		}
		result, deleteErr := s.db.ExecContext(ctx, `DELETE FROM queue_items
WHERE id = ? AND state IN ('succeeded', 'failed', 'cancelled', 'superseded')`, item.ID)
		if deleteErr != nil {
			return removed, fmt.Errorf("prune queue item %s: %w", item.ID, deleteErr)
		}
		changed, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return removed, fmt.Errorf("read pruned queue item %s: %w", item.ID, rowsErr)
		}
		removed += changed
	}

	return removed, nil
}

func queueRetentionExpired(item workqueue.Item, policy workqueue.Policy, now time.Time) bool {
	return policy.Retention != nil && item.FinishedAt != nil &&
		!item.FinishedAt.Add(*policy.Retention).After(now)
}
