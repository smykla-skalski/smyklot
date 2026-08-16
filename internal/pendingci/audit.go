package pendingci

import "time"

// Trigger identifies why a durable reconciliation became due.
type Trigger string

const (
	TriggerCommand     Trigger = "command"
	TriggerWebhook     Trigger = "webhook"
	TriggerFallback    Trigger = "fallback"
	TriggerQuietPeriod Trigger = "quiet_period"
	TriggerManual      Trigger = "manual"
	TriggerCleanup     Trigger = "cleanup"
)

// EventKind identifies one durable step in a pending-CI request's history.
type EventKind string

const (
	EventArmed                 EventKind = "armed"
	EventSuperseded            EventKind = "superseded"
	EventWakeReceived          EventKind = "wake_received"
	EventReconciliationStarted EventKind = "reconciliation_started"
	EventChecksObserved        EventKind = "checks_observed"
	EventMergeStarted          EventKind = "merge_started"
	EventFinished              EventKind = "finished"
	EventCleanupRetry          EventKind = "cleanup_retry"
	EventCleanupCompleted      EventKind = "cleanup_completed"
)

// Event is one immutable fact in a pending-CI request's operational timeline.
type Event struct {
	ID         int64
	RequestID  int64
	Kind       EventKind
	Trigger    Trigger
	EventName  string
	EventKey   string
	DeliveryID string
	State      string
	Summary    string
	CreatedAt  time.Time
}

// HistoryFilter selects recently finished requests for operator diagnosis.
type HistoryFilter struct {
	Limit int
}

// EventFilter selects the newest events belonging to one request.
type EventFilter struct {
	RequestID int64
	Limit     int
}

func (trigger Trigger) valid() bool {
	switch trigger {
	case TriggerCommand, TriggerWebhook, TriggerFallback,
		TriggerQuietPeriod, TriggerManual, TriggerCleanup:
		return true
	default:
		return false
	}
}
