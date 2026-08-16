// Package pendingci defines durable merge-after-CI state independently from
// webhook transport, GitHub API access, SQLite, and panel presentation.
package pendingci

import (
	"context"
	"time"
)

const (
	// MinPassingQuiet is the shortest supported stable-passing window.
	MinPassingQuiet = time.Second
	// DefaultPassingQuiet is how long a stable passing observation must remain
	// unchanged before a pending-CI request may merge.
	DefaultPassingQuiet = 30 * time.Second
	// MaxPassingQuiet keeps runtime settings representable by every API client.
	MaxPassingQuiet = 24 * time.Hour
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

// ObservedState is the reconciler's transport-independent view of CI.
type ObservedState string

const (
	ObservedPassing       ObservedState = "passing"
	ObservedPending       ObservedState = "pending"
	ObservedFailing       ObservedState = "failing"
	ObservedNoChecks      ObservedState = "no_checks"
	ObservedIndeterminate ObservedState = "indeterminate"
)

// DecisionKind describes the single state transition a reconciliation may
// perform. GitHub side effects remain outside this package.
type DecisionKind string

const (
	DecisionReschedule DecisionKind = "reschedule"
	DecisionMerge      DecisionKind = "merge"
	DecisionFinish     DecisionKind = "finish"
)

type MergeMethod string

const (
	MergeMethodMerge  MergeMethod = "merge"
	MergeMethodSquash MergeMethod = "squash"
	MergeMethodRebase MergeMethod = "rebase"
)

type Request struct {
	ID                   int64
	TargetID             string
	InstallationID       int64
	RepositoryID         string
	RepositoryFullName   string
	PullRequest          int
	HeadSHA              string
	BaseBranch           string
	MergeMethod          MergeMethod
	RequiredChecksOnly   bool
	Requester            string
	SourceCommentID      int64
	SourceRevision       string
	SourceSequence       int
	SourceOrder          int64
	Label                string
	Lifecycle            Lifecycle
	Schedule             Schedule
	NextCheckAt          time.Time
	NextCheckTrigger     Trigger
	LeaseExpiresAt       *time.Time
	LastProgressAt       time.Time
	LastObservedState    string
	LastFingerprint      string
	LastEventKey         string
	Reason               string
	RequestedAt          time.Time
	UpdatedAt            time.Time
	FinishedAt           *time.Time
	CleanupPending       bool
	CleanupArtifactsDone bool
	CleanupAttempts      int
	CleanupError         string
	Revision             int64
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
	SourceSequence     int
	SourceOrder        int64
	Label              string
	RequestedAt        time.Time
}

type ArmResult struct {
	Request    Request
	Superseded *Request
}

// SourceRevisionRequest orders deliveries for one mutable source comment.
// Sequence orders actions sharing a timestamp; SourceOrder orders distinct
// deliveries with otherwise identical source metadata.
type SourceRevisionRequest struct {
	RepositoryID string
	PullRequest  int
	CommentID    int64
	Revision     string
	Sequence     int
	SourceOrder  int64
	EventKey     string
	ObservedAt   time.Time
}

// SourceRevisionResult carries the durable total order assigned to one
// accepted source event. An exact retry reuses the same order.
type SourceRevisionResult struct {
	Accepted    bool
	SourceOrder int64
}

// LegacyDrainRequest records a pre-durable label as terminal work. Its
// authorized head cannot be recovered, so adopting it as armed would be unsafe.
type LegacyDrainRequest struct {
	TargetID           string
	InstallationID     int64
	RepositoryID       string
	RepositoryFullName string
	PullRequest        int
	HeadSHA            string
	BaseBranch         string
	Labels             []LegacyPendingCILabel
	DrainedAt          time.Time
}

type LegacyPendingCILabel struct {
	MergeMethod        MergeMethod
	RequiredChecksOnly bool
	Label              string
}

type LegacyDrainResult struct {
	Requests []Request
}

type LeaseResult struct {
	Request     *Request
	AvailableAt *time.Time
}

// RetuneQuietPeriodRequest moves every unleased passing request to the
// deadline implied by the current stable-passing window.
type RetuneQuietPeriodRequest struct {
	PassingQuiet time.Duration
	ChangedAt    time.Time
}

type WakeRequest struct {
	RepositoryID    string
	PullRequest     int
	EventName       string
	EventKey        string
	DeliveryID      string
	ExpectedHeadSHA string
	OccurredAt      time.Time
}

type WakeHeadRequest struct {
	RepositoryID string
	HeadSHA      string
	EventName    string
	EventKey     string
	DeliveryID   string
	OccurredAt   time.Time
}

type CheckNowRequest struct {
	ID               int64
	ExpectedRevision int64
	EventKey         string
	OccurredAt       time.Time
}

type ClaimMergeRequest struct {
	ID               int64
	ExpectedRevision int64
	Observation      Observation
	ClaimedAt        time.Time
}

type RescheduleRequest struct {
	ID                 int64
	ExpectedRevision   int64
	Schedule           Schedule
	HeadSHA            string
	NextCheckAt        time.Time
	NextCheckTrigger   Trigger
	LastProgressAt     time.Time
	LastObservedState  string
	LastFingerprint    string
	ObservationSummary string
	CheckedAt          time.Time
}

type FinishRequest struct {
	ID               int64
	ExpectedRevision int64
	Lifecycle        Lifecycle
	Trigger          Trigger
	Reason           string
	FinishedAt       time.Time
}

type CancelRequest struct {
	RepositoryID   string
	PullRequest    int
	CommentID      int64
	SourceRevision string
	SourceSequence int
	SourceOrder    int64
	Trigger        Trigger
	Reason         string
	CancelledAt    time.Time
}

// CancelIntentRequest records a PR-wide cancellation command in the same
// durable source order as merge-after-CI commands.
type CancelIntentRequest struct {
	RepositoryID   string
	PullRequest    int
	CommentID      int64
	SourceRevision string
	SourceSequence int
	SourceOrder    int64
	Reason         string
	CancelledAt    time.Time
}

type CancelIntentResult struct {
	Accepted bool
	Request  *Request
}

type FinishPRRequest struct {
	RepositoryID string
	PullRequest  int
	Lifecycle    Lifecycle
	Trigger      Trigger
	Reason       string
	FinishedAt   time.Time
}

type CancelRepositoryRequest struct {
	RepositoryID string
	Reason       string
	CancelledAt  time.Time
}

// CleanupFilter scopes ownership-barrier queries. PullRequest zero means the
// whole repository; ExcludeID omits the cleanup currently being applied.
type CleanupFilter struct {
	RepositoryID         string
	PullRequest          int
	ExcludeID            int64
	ArtifactsPendingOnly bool
}

type MarkCleanupArtifactsDoneRequest struct {
	ID               int64
	ExpectedRevision int64
	MarkedAt         time.Time
}

type CompleteCleanupRequest struct {
	ID               int64
	ExpectedRevision int64
	CompletedAt      time.Time
}

type RetryCleanupRequest struct {
	ID               int64
	ExpectedRevision int64
	NextAttemptAt    time.Time
	FailedAt         time.Time
	Error            string
}

type QueueFilter struct {
	Schedule *Schedule
	Limit    int
}

// Observation is live GitHub truth projected into the pending-CI domain.
type Observation struct {
	HeadSHA           string
	BaseBranch        string
	PullRequestOpen   bool
	PullRequestMerged bool
	PendingLabelFound bool
	CancelReason      string
	State             ObservedState
	Fingerprint       string
	Summary           string
	ObservedAt        time.Time
}

// Timing controls fallback frequency and the green stability window.
type Timing struct {
	ActiveInterval   time.Duration
	DiscoveryGrace   time.Duration
	DeferAfter       time.Duration
	DeferredInterval time.Duration
	PassingQuiet     time.Duration
}

// Decision is the policy result consumed by the scheduler.
type Decision struct {
	Kind              DecisionKind
	Lifecycle         Lifecycle
	Reason            string
	Schedule          Schedule
	HeadSHA           string
	NextCheckAt       time.Time
	NextCheckTrigger  Trigger
	LastProgressAt    time.Time
	LastObservedState string
	LastFingerprint   string
}

// Store is the persistence port used by pending-CI commands, scheduling, and
// operator controls. Implementations own atomic transitions; callers own
// GitHub observations and presentation.
type Store interface {
	ClaimSourceRevision(context.Context, SourceRevisionRequest) (SourceRevisionResult, error)
	CheckArm(context.Context, ArmRequest) error
	Arm(context.Context, ArmRequest) (ArmResult, error)
	DrainLegacy(context.Context, LegacyDrainRequest) (LegacyDrainResult, error)
	Get(context.Context, int64) (Request, error)
	GetArmed(context.Context, string, int) (Request, error)
	LeaseDue(context.Context, time.Time, time.Time) (LeaseResult, error)
	RetuneQuietPeriod(context.Context, RetuneQuietPeriodRequest) (int64, error)
	Wake(context.Context, WakeRequest) (bool, error)
	WakeByHead(context.Context, WakeHeadRequest) (int64, error)
	CheckNow(context.Context, CheckNowRequest) (Request, error)
	ClaimMerge(context.Context, ClaimMergeRequest) (Request, error)
	Reschedule(context.Context, RescheduleRequest) (Request, error)
	Finish(context.Context, FinishRequest) (Request, error)
	MarkCleanupArtifactsDone(context.Context, MarkCleanupArtifactsDoneRequest) (Request, error)
	CompleteCleanup(context.Context, CompleteCleanupRequest) (Request, error)
	RetryCleanup(context.Context, RetryCleanupRequest) (Request, error)
	CancelBySource(context.Context, CancelRequest) (*Request, error)
	CancelByIntent(context.Context, CancelIntentRequest) (CancelIntentResult, error)
	FinishPR(context.Context, FinishPRRequest) (*Request, error)
	CancelRepository(context.Context, CancelRepositoryRequest) ([]Request, error)
	HasPendingCleanup(context.Context, CleanupFilter) (bool, error)
	ListQueue(context.Context, QueueFilter) ([]Request, error)
	ListHistory(context.Context, HistoryFilter) ([]Request, error)
	ListEvents(context.Context, EventFilter) ([]Event, error)
}
