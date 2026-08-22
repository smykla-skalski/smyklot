package webhook

import (
	"context"
	"time"
)

type Inbox interface {
	Claim(ctx context.Context, claim Claim) (ClaimResult, error)
	Lease(ctx context.Context, now, leaseExpiresAt time.Time) (Lease, error)
	Complete(ctx context.Context, claimID int64, at time.Time) error
	Fail(ctx context.Context, failure Failure) error
	Retry(ctx context.Context, retry Retry) error
}

type Disposition string

const (
	Accepted   Disposition = "accepted"
	InProgress Disposition = "in_progress"
	Retained   Disposition = "retained"
)

type Claim struct {
	Key        string
	DeliveryID string
	Event      string
	Source     Source
	Payload    []byte
	At         time.Time
}

type ClaimResult struct {
	ID          int64
	Disposition Disposition
}

type Work struct {
	ClaimID    int64
	Key        string
	DeliveryID string
	Event      string
	Payload    []byte
	Attempt    int
}

type Lease struct {
	Work        *Work
	AvailableAt *time.Time
}

type Failure struct {
	ClaimID   int64
	Stage     string
	Reason    string
	Retryable bool
	At        time.Time
}

type Retry struct {
	ClaimID int64
	Stage   string
	Reason  string
	At      time.Time
}

const (
	StageDecode  = "decode"
	StageExecute = "execute"
)
