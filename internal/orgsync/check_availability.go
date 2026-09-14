package orgsync

import "time"

// CheckAvailability describes observed blockers, not request acceptance.
// A submitting actor must still be authorized and the request revalidated.
type CheckAvailability struct {
	Reason         string
	BlockingPlanID string
	RunningCheckID string
}

// FreshCheckPlanBlocker is shared by reads and the locked acceptance path.
// Applying work outlives its approval window and must not be replaced.
func FreshCheckPlanBlocker(plan Plan, now time.Time) string {
	switch plan.State {
	case PlanApplying:
		return "changes_running"
	case PlanComputed, PlanApproved:
		if plan.ExpiresAt.After(now) {
			return "changes_pending"
		}
	}
	return ""
}
