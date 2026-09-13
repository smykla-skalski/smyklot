package orgsync

import "time"

// PlanCursor identifies an exclusive boundary in immutable creation order.
// The ID breaks timestamp ties, so concurrent results cannot disappear between pages.
type PlanCursor struct {
	ComputedAt time.Time
	ID         string
}

// PlanPageRequest reads summaries without loading repository action payloads.
type PlanPageRequest struct {
	Limit  int
	Before *PlanCursor
}

type PlanPage struct {
	Items []Plan
	Next  *PlanCursor
	Total int
}
