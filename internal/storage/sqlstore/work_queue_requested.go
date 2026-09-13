package sqlstore

import (
	"context"
	"fmt"
)

// QueueRunWasRequested reads durable intent for this occurrence. Immediate is
// consumed when leasing; a bounded recent-events page can lose the request.
func (s *Store) QueueRunWasRequested(ctx context.Context, itemID string) (bool, error) {
	var requested bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (
 SELECT 1 FROM queue_events WHERE queue_item_id = ? AND kind = 'action.run_now'
 )`, itemID).Scan(&requested)
	if err != nil {
		return false, fmt.Errorf("read queue run request: %w", err)
	}
	return requested, nil
}
