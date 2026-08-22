package webhook

import (
	"context"
	"time"
)

// Inbox is the durable boundary the pipeline needs, and nothing more.
//
// It is a port rather than an implementation because the library must not name
// a database: a consumer's schema is theirs to choose, and this repository
// denies database/sql outside internal/storage. MemoryInbox is here for a
// consumer who has no database yet; anything that must survive a restart
// implements these five methods over its own table.
//
// Claim must be atomic against a concurrent Claim of the same key, and Lease
// against a concurrent Lease. Everything else the pipeline does is safe under
// an ordinary mutex.
//
// Nothing here recovers rows a previous process left running. That is a startup
// concern for whoever opens the store, and it is only ever an optimisation: an
// expired lease is leasable again, so a crashed process's work comes back on
// its own once the lease runs out.
type Inbox interface {
	// Claim accepts a delivery once. A key that is already claimed comes back
	// as InProgress or Retained with no id, and the caller must not run it.
	Claim(ctx context.Context, claim Claim) (ClaimResult, error)

	// Lease reserves the oldest ready delivery for one executor until
	// leaseExpiresAt. When nothing is ready it reports when to ask again, so
	// the dispatcher can sleep instead of polling.
	Lease(ctx context.Context, now, leaseExpiresAt time.Time) (Lease, error)

	// Complete records a delivery that ran to the end.
	Complete(ctx context.Context, claimID int64, at time.Time) error

	// Fail records a terminal outcome. A failure marked retryable is forgotten
	// by Claim, so GitHub's own redelivery can try again; one marked
	// non-retryable is retained, so redelivering it changes nothing.
	Fail(ctx context.Context, failure Failure) error

	// Retry returns leased work for another attempt. It must not pass the
	// delivery through a terminal state on the way, or a redelivery arriving in
	// that window would be accepted as new work.
	Retry(ctx context.Context, retry Retry) error
}

// Disposition explains what Claim did.
type Disposition string

const (
	// Accepted means this caller owns the delivery and must run it.
	Accepted Disposition = "accepted"

	// InProgress means another attempt holds it and has not finished.
	InProgress Disposition = "in_progress"

	// Retained means it has already been settled and will not be run again.
	Retained Disposition = "retained"
)

// Claim is the immutable identity of an accepted delivery.
type Claim struct {
	Key        string
	DeliveryID string
	Event      string
	Source     Source
	Payload    []byte
	At         time.Time
}

// ClaimResult carries the inbox's handle, populated only when Accepted.
type ClaimResult struct {
	ID          int64
	Disposition Disposition
}

// Work is one leased payload.
//
// It carries no source because the pipeline re-derives that from Payload: an
// inbox that had to hand it back would have to store it in whatever shape the
// consumer namespaces identifiers in, and then undo that on the way out.
type Work struct {
	ClaimID    int64
	Key        string
	DeliveryID string
	Event      string
	Payload    []byte
	Attempt    int
}

// Lease is either ready work or the instant to ask again. Both nil means the
// inbox is empty and the dispatcher should wait to be woken.
type Lease struct {
	Work        *Work
	AvailableAt *time.Time
}

// Failure finishes a delivery with a reason an operator can read.
//
// The reason is stored, so a caller whose errors can carry a secret redacts
// before it gets here - the library does not know what one looks like.
type Failure struct {
	ClaimID   int64
	Stage     string
	Reason    string
	Retryable bool
	At        time.Time
}

// Retry schedules another attempt.
type Retry struct {
	ClaimID int64
	Stage   string
	Reason  string
	At      time.Time
}

// Stages a delivery can fail at, recorded on the row so an operator can tell a
// payload that never parsed from work that ran and did not finish.
const (
	StageDecode  = "decode"
	StageExecute = "execute"
)
