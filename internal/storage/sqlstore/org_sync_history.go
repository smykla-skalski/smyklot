package sqlstore

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

// ListSyncPlans keeps the installation predicate on both count and page reads.
// Creation time and ID are immutable even while a plan's execution state changes.
func (s *Store) ListSyncPlans(ctx context.Context, targetID string, request orgsync.PlanPageRequest) (orgsync.PlanPage, error) {
	limit := pageLimit(request.Limit)
	page := orgsync.PlanPage{Items: []orgsync.Plan{}}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_plans WHERE target_id = ?", targetID).Scan(&page.Total); err != nil {
		return orgsync.PlanPage{}, fmt.Errorf("count sync plans: %w", err)
	}
	clause := " WHERE sync_plans.target_id = ?"
	arguments := []any{targetID}
	if request.Before != nil {
		clause += " AND (computed_at < ? OR (computed_at = ? AND id < ?))"
		arguments = append(arguments, request.Before.ComputedAt, request.Before.ComputedAt, request.Before.ID)
	}
	arguments = append(arguments, limit+1)
	// #nosec G202 -- clauses are fixed constants and every input is a bound parameter.
	rows, err := s.db.QueryContext(ctx, "SELECT"+syncPlanColumns+" FROM sync_plans"+clause+" ORDER BY computed_at DESC, id DESC LIMIT ?", arguments...)
	if err != nil {
		return orgsync.PlanPage{}, fmt.Errorf("list sync plans: %w", err)
	}
	items, err := collectRows(rows, scanSyncPlan)
	if err != nil {
		return orgsync.PlanPage{}, fmt.Errorf("read sync plans: %w", err)
	}
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		page.Next = &orgsync.PlanCursor{ComputedAt: last.ComputedAt, ID: last.ID}
	}
	page.Items = items
	return page, nil
}
