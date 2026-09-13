package orgsync

import "time"

// PlanDispatch names the exact reviewed action. Now is execution context, not
// part of its identity. The caller must authorize ActorID for TargetID.
type PlanDispatch struct {
	TargetID         string
	PlanID           string
	ActorID          string
	RequestKey       string
	ExpectedRevision int64
	Reason           string
	Now              time.Time
}

// PlanDispatchReceipt records acceptance, never current execution state.
type PlanDispatchReceipt struct {
	PlanID     string
	QueueID    string
	AcceptedAt time.Time
}
