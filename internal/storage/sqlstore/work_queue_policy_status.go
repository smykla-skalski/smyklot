package sqlstore

import (
	"context"
	"fmt"
	"sort"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func (s *Store) ListQueuePolicyStatuses(
	ctx context.Context,
	targetID *string,
) ([]workqueue.PolicyStatus, error) {
	clause, arguments := queueStatusScope(targetID)
	live, err := s.queueStatusItems(ctx, clause+
		" AND state IN ('awaiting_approval', 'scheduled', 'blocked', 'ready', 'running', 'retrying')",
		arguments...,
	)
	if err != nil {
		return nil, err
	}
	if err := s.addQueuePositions(ctx, live); err != nil {
		return nil, err
	}
	last, err := s.queueStatusItems(ctx, `id IN (
  SELECT id FROM (
    SELECT id, ROW_NUMBER() OVER (
      PARTITION BY kind ORDER BY COALESCE(started_at, finished_at, updated_at) DESC, id DESC
    ) AS position
    FROM queue_items WHERE `+queueStatusInnerScope(targetID)+`
      AND state IN ('succeeded', 'failed', 'cancelled', 'superseded')
  ) latest WHERE position = 1
)`, arguments...)
	if err != nil {
		return nil, err
	}
	statuses := make(map[workqueue.Kind]workqueue.PolicyStatus)
	for _, item := range last {
		state := item.State
		at := item.UpdatedAt
		if item.FinishedAt != nil {
			at = *item.FinishedAt
		} else if item.StartedAt != nil {
			at = *item.StartedAt
		}
		statuses[item.Kind] = workqueue.PolicyStatus{
			Kind: item.Kind, TargetID: targetID, LastRunAt: &at, LastState: &state,
		}
	}
	for _, item := range live {
		status := statuses[item.Kind]
		status.Kind, status.TargetID = item.Kind, targetID
		if status.CurrentQueueItemID != nil &&
			status.EstimatedStartAt != nil && !item.EstimatedStartAt.Before(*status.EstimatedStartAt) {
			statuses[item.Kind] = status
			continue
		}
		id, state, eligible, estimated := item.ID, item.State, item.EligibleAt, item.EstimatedStartAt
		status.CurrentQueueItemID, status.CurrentState = &id, &state
		status.NextEligibilityAt, status.EstimatedStartAt = &eligible, estimated
		status.WorkAhead = item.WorkAhead
		statuses[item.Kind] = status
	}
	result := make([]workqueue.PolicyStatus, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, status)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Kind < result[right].Kind })

	return result, nil
}

func queueStatusScope(targetID *string) (string, []any) {
	if targetID == nil {
		return queryAllRows, nil
	}

	return queryTargetIDEquals, []any{*targetID}
}

func queueStatusInnerScope(targetID *string) string {
	if targetID == nil {
		return queryAllRows
	}

	return queryTargetIDEquals
}

func (s *Store) queueStatusItems(
	ctx context.Context,
	clause string,
	arguments ...any,
) ([]workqueue.Item, error) {
	rows, err := s.db.QueryContext(
		ctx, "SELECT"+queueItemColumns+" FROM queue_items WHERE "+clause, arguments...,
	)
	if err != nil {
		return nil, fmt.Errorf("list queue policy status: %w", err)
	}
	items, err := collectRows(rows, scanQueueItem)
	if err != nil {
		return nil, fmt.Errorf("read queue policy status: %w", err)
	}

	return items, nil
}
