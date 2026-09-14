package orgsync

import "time"

const (
	RequestActionCheck    = "check"
	RequestActionDispatch = "dispatch"
)

// RequestAcceptance is historical intent and identity, never live progress.
// RequestKey is an opaque identity, not proof that this was a lost browser click.
type RequestAcceptance struct {
	Action           string
	RequestKey       string
	QueueID          string
	PlanID           string
	ExpectedRevision int64
	Reason           string
	AcceptedAt       time.Time
}

// RequestHistoryPosition binds continuation to the caller and workspace.
type RequestHistoryPosition struct {
	ActorID    string
	TargetID   string
	AcceptedAt time.Time
	Action     string
	RequestKey string
}

type RequestHistoryQuery struct {
	ActorID          string
	SessionTokenHash string
	TargetID         string
	After            *RequestHistoryPosition
	Limit            int
}

type RequestHistoryPage struct {
	Items []RequestAcceptance
	Next  *RequestHistoryPosition
}
