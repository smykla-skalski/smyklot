// Package pendingci defines durable merge-after-CI state independently from
// webhook transport, GitHub API access, SQLite, and panel presentation.
package pendingci

import (
	"context"
	"time"
)

type Lifecycle string

const (
	LifecycleArmed      Lifecycle = "armed"
	LifecycleMerged     Lifecycle = "merged"
	LifecycleCancelled  Lifecycle = "cancelled"
	LifecycleSuperseded Lifecycle = "superseded"
)

type Schedule string

const (
	ScheduleActive   Schedule = "active"
	ScheduleDeferred Schedule = "deferred"
)

type MergeMethod string

const (
	MergeMethodMerge  MergeMethod = "merge"
	MergeMethodSquash MergeMethod = "squash"
	MergeMethodRebase MergeMethod = "rebase"
)

type Request struct {
	ID                 int64
	TargetID           string
	InstallationID     int64
	RepositoryID       string
	RepositoryFullName string
	PullRequest        int
	HeadSHA            string
	BaseBranch         string
	MergeMethod        MergeMethod
	RequiredChecksOnly bool
	Requester          string
	SourceCommentID    int64
	SourceRevision     string
	Label              string
	Lifecycle          Lifecycle
	Schedule           Schedule
	NextCheckAt        time.Time
	LeaseExpiresAt     *time.Time
	LastProgressAt     time.Time
	LastObservedState  string
	LastFingerprint    string
	LastEventKey       string
	Reason             string
	RequestedAt        time.Time
	UpdatedAt          time.Time
	FinishedAt         *time.Time
	Revision           int64
}

type ArmRequest struct {
	TargetID           string
	InstallationID     int64
	RepositoryID       string
	RepositoryFullName string
	PullRequest        int
	HeadSHA            string
	BaseBranch         string
	MergeMethod        MergeMethod
	RequiredChecksOnly bool
	Requester          string
	SourceCommentID    int64
	SourceRevision     string
	Label              string
	RequestedAt        time.Time
}

type ArmResult struct {
	Request    Request
	Superseded *Request
}

type LeaseResult struct {
	Request     *Request
	AvailableAt *time.Time
}

type WakeRequest struct {
	RepositoryID    string
	PullRequest     int
	EventKey        string
	ExpectedHeadSHA string
	OccurredAt      time.Time
}

type RescheduleRequest struct {
	ID                int64
	ExpectedRevision  int64
	Schedule          Schedule
	HeadSHA           string
	NextCheckAt       time.Time
	LastProgressAt    time.Time
	LastObservedState string
	LastFingerprint   string
	CheckedAt         time.Time
}

type FinishRequest struct {
	ID               int64
	ExpectedRevision int64
	Lifecycle        Lifecycle
	Reason           string
	FinishedAt       time.Time
}

type CancelRequest struct {
	RepositoryID string
	PullRequest  int
	CommentID    int64
	Reason       string
	CancelledAt  time.Time
}

type QueueFilter struct {
	Schedule *Schedule
	Limit    int
}

// Store is the persistence port used by pending-CI commands, scheduling, and
// operator controls. Implementations own atomic transitions; callers own
// GitHub observations and presentation.
type Store interface {
	Arm(context.Context, ArmRequest) (ArmResult, error)
	GetArmed(context.Context, string, int) (Request, error)
	LeaseDue(context.Context, time.Time, time.Time) (LeaseResult, error)
	Wake(context.Context, WakeRequest) (bool, error)
	Reschedule(context.Context, RescheduleRequest) (Request, error)
	Finish(context.Context, FinishRequest) (Request, error)
	CancelBySource(context.Context, CancelRequest) (*Request, error)
	ListQueue(context.Context, QueueFilter) ([]Request, error)
}
