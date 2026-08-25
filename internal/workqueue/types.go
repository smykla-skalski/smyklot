// Package workqueue defines the durable background-work control plane.
package workqueue

import (
	"context"
	"encoding/json"
	"time"
)

const AlwaysOpenProfileID = "always-open"

type Kind string

const (
	KindWebhookDelivery Kind = "webhook_delivery"
	KindPendingCI       Kind = "pending_ci"
	KindPendingCIGate   Kind = "pending_ci_gate"
	KindCatalogRefresh  Kind = "catalog_refresh"
	KindReactionScan    Kind = "reaction_scan"
	KindConfigMigration Kind = "config_migration"
	KindSyncScan        Kind = "sync_scan"
	KindSyncApply       Kind = "sync_apply"
	KindPathRefresh     Kind = "path_refresh"
	KindDeliveryCleanup Kind = "delivery_cleanup"
	KindAuthCleanup     Kind = "auth_cleanup"
	KindScheduleChange  Kind = "schedule_change"
)

func Kinds() []Kind {
	return []Kind{
		KindWebhookDelivery,
		KindPendingCI,
		KindPendingCIGate,
		KindCatalogRefresh,
		KindReactionScan,
		KindConfigMigration,
		KindSyncScan,
		KindSyncApply,
		KindPathRefresh,
		KindDeliveryCleanup,
		KindAuthCleanup,
		KindScheduleChange,
	}
}

func (kind Kind) Valid() bool {
	for _, known := range Kinds() {
		if kind == known {
			return true
		}
	}

	return false
}

func (kind Kind) Windowed() bool { return kind != KindWebhookDelivery }

// Recurring reports whether the scheduler creates a new cadence-anchored
// occurrence after each successful run. Source-backed work such as webhook
// delivery and pending CI owns its next deadline in its domain table instead.
func (kind Kind) Recurring() bool {
	switch kind {
	case KindPendingCIGate, KindCatalogRefresh, KindReactionScan,
		KindConfigMigration, KindSyncScan, KindPathRefresh,
		KindDeliveryCleanup, KindAuthCleanup:
		return true
	default:
		return false
	}
}

func (kind Kind) InstallationConfigurable() bool {
	switch kind {
	case KindPendingCI, KindPendingCIGate, KindReactionScan,
		KindConfigMigration, KindSyncScan, KindPathRefresh:
		return true
	default:
		return false
	}
}

func (kind Kind) Lane() Lane {
	switch kind {
	case KindWebhookDelivery:
		return LaneWebhook
	case KindPendingCI:
		return LanePendingCI
	default:
		return LaneMaintenance
	}
}

type Lane string

const (
	LaneWebhook     Lane = "webhook"
	LanePendingCI   Lane = "pending_ci"
	LaneMaintenance Lane = "maintenance"
)

func (lane Lane) Workers() int {
	switch lane {
	case LaneWebhook:
		return 8
	case LanePendingCI:
		return 4
	default:
		return 1
	}
}

type State string

const (
	StateAwaitingApproval State = "awaiting_approval"
	StateScheduled        State = "scheduled"
	StateBlocked          State = "blocked"
	StateReady            State = "ready"
	StateRunning          State = "running"
	StateRetrying         State = "retrying"
	StateSucceeded        State = "succeeded"
	StateFailed           State = "failed"
	StateCancelled        State = "cancelled"
	StateSuperseded       State = "superseded"
)

func (state State) Terminal() bool {
	return state == StateSucceeded || state == StateFailed ||
		state == StateCancelled || state == StateSuperseded
}

func (state State) Valid() bool {
	switch state {
	case StateAwaitingApproval, StateScheduled, StateBlocked, StateReady,
		StateRunning, StateRetrying, StateSucceeded, StateFailed,
		StateCancelled, StateSuperseded:
		return true
	default:
		return false
	}
}

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

func (priority Priority) Valid() bool {
	return priority == PriorityLow || priority == PriorityNormal ||
		priority == PriorityHigh || priority == PriorityUrgent
}

type WindowMode string

const (
	WindowRespect WindowMode = "respect"
	WindowBypass  WindowMode = "bypass"
)

type Window struct {
	Weekday time.Weekday `json:"weekday"`
	Start   int          `json:"start_minute"`
	End     int          `json:"end_minute"`
}

type Exception struct {
	Date   string `json:"date"`
	Closed bool   `json:"closed"`
	Start  int    `json:"start_minute,omitempty"`
	End    int    `json:"end_minute,omitempty"`
}

type Profile struct {
	ID                    string      `json:"id"`
	TargetID              *string     `json:"target_id,omitempty"`
	Name                  string      `json:"name"`
	Timezone              string      `json:"timezone"`
	System                bool        `json:"system"`
	ArchivedAt            *time.Time  `json:"archived_at,omitempty"`
	Revision              int64       `json:"revision"`
	Windows               []Window    `json:"windows"`
	Exceptions            []Exception `json:"exceptions"`
	CreatedAt             time.Time   `json:"created_at"`
	UpdatedAt             time.Time   `json:"updated_at"`
	AffectedInstallations int         `json:"affected_installations,omitempty"`
	AffectedItems         int         `json:"affected_items,omitempty"`
	AffectedPolicies      int         `json:"affected_policies,omitempty"`
}

type Policy struct {
	Kind            Kind            `json:"kind"`
	TargetID        *string         `json:"target_id,omitempty"`
	Enabled         bool            `json:"enabled"`
	Cadence         time.Duration   `json:"cadence"`
	ProfileID       string          `json:"profile_id"`
	DefaultPriority Priority        `json:"default_priority"`
	RetryDelay      time.Duration   `json:"retry_delay"`
	Retention       *time.Duration  `json:"retention,omitempty"`
	ApprovalTTL     *time.Duration  `json:"approval_ttl,omitempty"`
	Configuration   json.RawMessage `json:"configuration,omitempty"`
	Revision        int64           `json:"revision"`
	UpdatedBy       *string         `json:"updated_by,omitempty"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type PolicyStatus struct {
	Kind               Kind       `json:"kind"`
	TargetID           *string    `json:"target_id,omitempty"`
	LastRunAt          *time.Time `json:"last_run_at,omitempty"`
	LastState          *State     `json:"last_state,omitempty"`
	NextEligibilityAt  *time.Time `json:"next_eligibility_at,omitempty"`
	EstimatedStartAt   *time.Time `json:"estimated_start_at,omitempty"`
	WorkAhead          int        `json:"work_ahead"`
	CurrentState       *State     `json:"current_state,omitempty"`
	CurrentQueueItemID *string    `json:"current_queue_item_id,omitempty"`
}

type ProfileChange struct {
	ID               string
	TargetID         *string
	Name             string
	Timezone         string
	Windows          []Window
	Exceptions       []Exception
	ExpectedRevision int64
	ActorID          string
	ChangedAt        time.Time
}

type PolicyChange struct {
	Kind             Kind
	TargetID         *string
	Enabled          bool
	Cadence          time.Duration
	ProfileID        string
	DefaultPriority  Priority
	RetryDelay       time.Duration
	Retention        *time.Duration
	ApprovalTTL      *time.Duration
	Configuration    json.RawMessage
	ExpectedRevision int64
	ActorID          string
	ChangedAt        time.Time
}

type RequestState string

const (
	RequestPending   RequestState = "pending"
	RequestApproved  RequestState = "approved"
	RequestRejected  RequestState = "rejected"
	RequestWithdrawn RequestState = "withdrawn"
	RequestStale     RequestState = "stale"
)

type ScheduleRequest struct {
	ID                string          `json:"id"`
	TargetID          string          `json:"target_id"`
	Kind              Kind            `json:"kind"`
	State             RequestState    `json:"state"`
	BaseRevision      int64           `json:"base_revision"`
	BaseTargetID      *string         `json:"base_target_id,omitempty"`
	ProfileID         *string         `json:"profile_id,omitempty"`
	CustomProfile     *Profile        `json:"custom_profile,omitempty"`
	Cadence           time.Duration   `json:"cadence"`
	DefaultPriority   Priority        `json:"default_priority"`
	Configuration     json.RawMessage `json:"configuration,omitempty"`
	Reason            string          `json:"reason"`
	RequestedBy       string          `json:"requested_by"`
	ReviewedBy        *string         `json:"reviewed_by,omitempty"`
	DecisionReason    string          `json:"decision_reason,omitempty"`
	PromotedProfileID *string         `json:"promoted_profile_id,omitempty"`
	Revision          int64           `json:"revision"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	ReviewedAt        *time.Time      `json:"reviewed_at,omitempty"`
}

type ScheduleRequestCreate struct {
	ID              string
	TargetID        string
	Kind            Kind
	BaseRevision    int64
	ProfileID       *string
	CustomProfile   *Profile
	Cadence         time.Duration
	DefaultPriority Priority
	Configuration   json.RawMessage
	Reason          string
	RequestedBy     string
	CreatedAt       time.Time
}

type ScheduleDecision struct {
	Approve          bool
	PromoteProfile   bool
	ProfileID        *string
	DecisionReason   string
	ExpectedRevision int64
	ReviewerID       string
	ReviewedAt       time.Time
}

type Item struct {
	ID               string          `json:"id"`
	Kind             Kind            `json:"kind"`
	Lane             Lane            `json:"lane"`
	TargetID         *string         `json:"target_id,omitempty"`
	RepositoryID     *string         `json:"repository_id,omitempty"`
	SourceKind       string          `json:"source_kind,omitempty"`
	SourceID         string          `json:"source_id,omitempty"`
	Title            string          `json:"title"`
	Summary          string          `json:"summary,omitempty"`
	State            State           `json:"state"`
	Priority         Priority        `json:"priority"`
	PriorityOverride bool            `json:"priority_overridden"`
	WindowMode       WindowMode      `json:"window_mode"`
	Immediate        bool            `json:"immediate"`
	ProfileID        *string         `json:"profile_id,omitempty"`
	ProfileName      string          `json:"profile_name,omitempty"`
	ProfileTimezone  string          `json:"profile_timezone,omitempty"`
	NotBefore        time.Time       `json:"not_before"`
	CadenceAnchorAt  *time.Time      `json:"cadence_anchor_at,omitempty"`
	EligibleAt       time.Time       `json:"eligible_at"`
	EstimatedStartAt *time.Time      `json:"estimated_start_at,omitempty"`
	WorkAhead        int             `json:"work_ahead"`
	BlockedReason    string          `json:"blocked_reason,omitempty"`
	ProgressCurrent  int             `json:"progress_current"`
	ProgressTotal    int             `json:"progress_total"`
	Attempt          int             `json:"attempt"`
	LeaseExpiresAt   *time.Time      `json:"lease_expires_at,omitempty"`
	RequestedBy      *string         `json:"requested_by,omitempty"`
	Reason           string          `json:"reason,omitempty"`
	Details          json.RawMessage `json:"details,omitempty"`
	Revision         int64           `json:"revision"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	StartedAt        *time.Time      `json:"started_at,omitempty"`
	FinishedAt       *time.Time      `json:"finished_at,omitempty"`
	Actions          []ActionType    `json:"actions,omitempty"`
}

type Event struct {
	ID        int64           `json:"id"`
	ItemID    string          `json:"item_id"`
	ActorID   *string         `json:"actor_id,omitempty"`
	Actor     string          `json:"actor"`
	Kind      string          `json:"kind"`
	State     State           `json:"state"`
	Summary   string          `json:"summary"`
	Details   json.RawMessage `json:"details,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type Filter struct {
	TargetID      *string
	RepositoryID  *string
	ProfileID     *string
	States        []State
	Kinds         []Kind
	Priorities    []Priority
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	DispatchOrder bool
	Summary       bool
	Limit         int
	Offset        int
}

type Facets struct {
	Targets      []string   `json:"targets"`
	Repositories []string   `json:"repositories"`
	Profiles     []string   `json:"profiles"`
	States       []State    `json:"states"`
	Kinds        []Kind     `json:"workloads"`
	Priorities   []Priority `json:"priorities"`
}

type Page struct {
	Items       []Item        `json:"items"`
	NextOffset  int           `json:"next_offset"`
	Total       int           `json:"total"`
	Facets      Facets        `json:"facets"`
	StateCounts map[State]int `json:"state_counts,omitempty"`
}

type BacklogMetric struct {
	Lane                   Lane
	ProfileID              string
	Depth                  int
	OldestAge              time.Duration
	EligibleToStartLatency time.Duration
}

type MetricsSnapshot struct {
	Backlogs      []BacklogMetric
	Failures      int
	MissedWindows int
	RunningLeases int
}

type ActionType string

const (
	ActionRunNow      ActionType = "run_now"
	ActionNextWindow  ActionType = "next_window"
	ActionScheduleAt  ActionType = "schedule_at"
	ActionSetPriority ActionType = "set_priority"
	ActionCancel      ActionType = "cancel"
)

type ItemAction struct {
	Type             ActionType
	ExpectedRevision int64
	ActorID          string
	Reason           string
	At               time.Time
	OutsideWindow    bool
	Priority         Priority
	ChangedAt        time.Time
}

type RecurringClaim struct {
	Kind          Kind
	TargetID      *string
	RepositoryID  *string
	Title         string
	Now           time.Time
	LeaseDuration time.Duration
}

type RecurringLease struct {
	Now           time.Time
	LeaseDuration time.Duration
}

type RecurringCompletion struct {
	Failure        string
	SuccessSummary string
	Retryable      bool
	Blocked        bool
}

type RecurringRequest struct {
	Kind         Kind
	TargetID     *string
	RepositoryID *string
	Title        string
	ActorID      string
	Reason       string
	Now          time.Time
}

type Store interface {
	ListWorkQueue(context.Context, Filter) (Page, error)
	GetQueueItem(context.Context, string) (Item, error)
	ListQueueEvents(context.Context, string, int) ([]Event, error)
	CreateQueueItem(context.Context, Item) (Item, error)
	ApplyQueueAction(context.Context, string, ItemAction) (Item, error)
	ClaimNextRecurringWork(context.Context, RecurringLease) (Item, bool, error)
	ClaimRecurringWork(context.Context, RecurringClaim) (Item, bool, error)
	EnsureRecurringWork(context.Context, RecurringClaim) (Item, error)
	SupersedeMissingRecurringWork(context.Context, []RecurringClaim, time.Time) ([]Item, error)
	RequestRecurringWork(context.Context, RecurringRequest) (Item, error)
	FinishRecurringWork(context.Context, string, RecurringCompletion, time.Time) (Item, error)
	PruneWorkQueue(context.Context, time.Time) (int64, error)
	NextQueueAvailability(context.Context, Lane, time.Time) (*time.Time, error)
	WorkQueueMetrics(context.Context, time.Time) (MetricsSnapshot, error)
	ListScheduleProfiles(context.Context, bool) ([]Profile, error)
	GetScheduleProfile(context.Context, string) (Profile, error)
	SaveScheduleProfile(context.Context, ProfileChange) (Profile, error)
	ArchiveScheduleProfile(context.Context, string, int64, string, time.Time) (Profile, error)
	ListQueuePolicies(context.Context, *string) ([]Policy, error)
	ListAllQueuePolicies(context.Context) ([]Policy, error)
	InitializeQueuePolicies(context.Context, DeploymentDefaults, time.Time) error
	ListQueuePolicyStatuses(context.Context, *string) ([]PolicyStatus, error)
	GetEffectiveQueuePolicy(context.Context, Kind, *string) (Policy, error)
	SaveQueuePolicy(context.Context, PolicyChange) (Policy, error)
	DeleteQueuePolicyOverride(context.Context, Kind, string, int64, string, time.Time) (Policy, error)
	ListScheduleRequests(context.Context, *string) ([]ScheduleRequest, error)
	GetScheduleRequest(context.Context, string) (ScheduleRequest, error)
	CreateScheduleRequest(context.Context, ScheduleRequestCreate) (ScheduleRequest, error)
	DecideScheduleRequest(context.Context, string, ScheduleDecision) (ScheduleRequest, error)
	WithdrawScheduleRequest(context.Context, string, int64, string, time.Time) (ScheduleRequest, error)
}
