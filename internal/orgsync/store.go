package orgsync

import (
	"context"
	"time"
)

// Config is one installation's answer for one kind: whether it syncs, and what
// it syncs to.
//
// Document is the kind's own shape as JSON, opaque here. Labels decode it as a
// LabelConfig; settings and rulesets will decode it as theirs. Keeping it
// opaque is what lets the store, the plan and the audit stay one mechanism
// across four kinds that share nothing else.
type Config struct {
	TargetID string
	Kind     Kind
	Enabled  bool
	Document []byte

	// Digest fingerprints Enabled and Document together, so two configurations
	// that would produce the same work compare equal without a diff.
	Digest string

	Revision  int64
	UpdatedBy string
	UpdatedAt time.Time
}

// ConfigChange writes one kind's configuration.
type ConfigChange struct {
	TargetID string
	Kind     Kind
	Enabled  bool
	Document []byte
	ActorID  string
	Now      time.Time

	// Revision is what the writer believes it is changing, or zero for a first
	// write. A mismatch is a conflict rather than a silent overwrite: two
	// people editing the same label set from two tabs is the ordinary case, not
	// the exotic one.
	Revision int64
}

// RepositoryOverride is a repository's own answer for one kind. Enabled is nil
// where the repository has not given one and inherits the installation's.
type RepositoryOverride struct {
	RepositoryID string
	Kind         Kind
	Enabled      *bool
	Revision     int64
	UpdatedBy    string
	UpdatedAt    time.Time
}

// RepositoryOverrideChange writes one, or clears it back to inheriting by
// passing a nil Enabled.
type RepositoryOverrideChange struct {
	RepositoryID string
	Kind         Kind
	Enabled      *bool
	ActorID      string
	Now          time.Time
	Revision     int64
}

// RepositoryState is what a repository has already had applied for one kind,
// and the reason a steady-state reconcile costs nothing: where the stored
// digest matches the configured one, the planner does not need to ask GitHub
// what the repository looks like.
type RepositoryState struct {
	RepositoryID  string
	Kind          Kind
	AppliedDigest string
	AppliedAt     time.Time
}

// PlanCreate records a computed plan and its actions together.
//
// One call rather than a plan then its actions, because a plan with no actions
// yet is a plan another caller can see and approve. The database holds one live
// plan per installation, and a half-written one would spend that slot.
type PlanCreate struct {
	ID       string
	TargetID string
	Trigger  Trigger
	ActorID  string
	Digest   string
	Actions  []Action

	Now       time.Time
	ExpiresAt time.Time
}

// PlanApproval is somebody accepting a plan they have read.
//
// Digest is what their browser rendered. It is checked against what is stored,
// which is the only thing standing between what was reviewed and what runs -
// somebody saving a label colour while the plan is on screen must not have
// their change applied under an approval given for something else.
type PlanApproval struct {
	PlanID  string
	Digest  string
	ActorID string
	Now     time.Time
}

// PlanLease is an executor claiming a plan to apply.
type PlanLease struct {
	Plan    Plan
	Actions []Action

	// Found is false when nothing was due, which is the ordinary answer on
	// most ticks and not an error.
	Found bool
}

// ActionOutcome is what became of one action.
type ActionOutcome struct {
	ActionID int64
	State    ActionState
	Error    string
	Blocker  Kind
}

// PlanOutcome closes a plan, recording where each repository ended up.
type PlanOutcome struct {
	PlanID string
	State  PlanState
	Now    time.Time

	// Applied is the repository and kind pairs whose work all succeeded, so
	// their digest can be recorded and the next reconcile can skip them.
	Applied []RepositoryState
}

// Store is what org sync needs from the database.
//
// Its own interface, embedded into storage.Store, following internal/pendingci:
// the domain says what it needs and the engine supplies it, rather than the
// domain reaching for a handle.
type Store interface {
	GetSyncConfig(context.Context, string, Kind) (Config, error)
	ListSyncConfigs(context.Context, string) ([]Config, error)

	// SetSyncConfig writes a configuration and invalidates every live plan
	// computed from the old one, in the same transaction. Saving a label
	// colour while a plan is on screen has to invalidate that plan atomically,
	// or the plan stays approvable and applies work nobody reviewed.
	SetSyncConfig(context.Context, ConfigChange) (Config, error)

	ListSyncRepositoryOverrides(context.Context, string) ([]RepositoryOverride, error)
	SetSyncRepositoryOverride(
		context.Context, RepositoryOverrideChange,
	) (RepositoryOverride, error)

	ListSyncRepositoryState(context.Context, string) ([]RepositoryState, error)

	CreateSyncPlan(context.Context, PlanCreate) (Plan, error)
	GetSyncPlan(context.Context, string) (Plan, []Action, error)

	// GetLiveSyncPlan answers the one plan an installation may have in flight,
	// or storage.ErrNotFound. It is what makes pressing "sync now" twice
	// idempotent.
	//
	// This package names no storage errors, and deliberately: it does not
	// import the package that defines them, so the domain stays sayable
	// without a database. The engine supplies them, exactly as it does for
	// internal/pendingci.
	GetLiveSyncPlan(context.Context, string) (Plan, []Action, error)

	ApproveSyncPlan(context.Context, PlanApproval) (Plan, error)

	// LeaseSyncPlan claims an approved plan for a bounded time, exactly as
	// LeaseDelivery claims a delivery, so an executor that dies leaves work
	// somebody else can pick up rather than a plan stuck in applying.
	LeaseSyncPlan(context.Context, time.Time, time.Time) (PlanLease, error)

	RecordSyncActionOutcome(context.Context, ActionOutcome) error
	FinishSyncPlan(context.Context, PlanOutcome) error

	// ExpireSyncPlans retires plans nobody acted on. Swept rather than only
	// checked, so a plan does not sit in the one live slot for ever - but
	// approval checks expiry itself, so correctness never depends on this
	// having run.
	ExpireSyncPlans(context.Context, time.Time) error
}
