package orgsync

import "time"

// PlanDispatch names the exact reviewed action. Now is execution context, not
// part of its identity. SessionTokenHash supplies current authority at acceptance.
type PlanDispatch struct {
	TargetID         string
	PlanID           string
	ActorID          string
	SessionTokenHash string
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
