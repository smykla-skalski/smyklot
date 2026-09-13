package storage

// LiveSyncPlanConflict identifies the plan observed occupying a target's live
// slot while creation was serialized. It is not an identifier collision.
type LiveSyncPlanConflict struct {
	PlanID string
}

func (*LiveSyncPlanConflict) Error() string { return ErrConflict.Error() }

func (*LiveSyncPlanConflict) Unwrap() error { return ErrConflict }
