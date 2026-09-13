package storage

import "time"

// DeliveryRecovery requests a new run without changing the original failure.
// ExpectedRunID and ExpectedRevision bind the current operation observed by the user.
type DeliveryRecovery struct {
	TargetID         string
	SourceRunID      int64
	ExpectedRunID    int64
	ExpectedRevision int64
	RequestKey       string
	ActorAccountID   string
	SessionTokenHash string
	ElevationID      *string
	RequestedAt      time.Time
}

// DeliveryRecoveryResult is also returned for a repeated accepted request.
// A returned run may have finished or been pruned since its original acceptance.
type DeliveryRecoveryResult struct {
	RunID    int64
	Repeated bool
}
