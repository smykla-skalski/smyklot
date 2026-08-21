// Package orgsync defines organization-wide synchronization of the things a
// repository is expected to have: its labels, its settings, its rulesets and
// its shared files.
//
// Named orgsync rather than sync, which would shadow the standard library in
// every file that imported both.
//
// Nothing here reaches GitHub or a database. A planner takes what is configured
// and what a repository currently has, and answers what would change; applying
// that answer is somebody else's job. That split is the whole design: a plan can
// be stored, shown to a person and approved before anything is written, and the
// tool this replaces could not do that because it computed and applied in one
// pass and reported the plan as though it were the outcome.
package orgsync

import (
	"fmt"
	"slices"
	"time"
)

// Kind is one area of a repository that is synchronized.
//
// Each kind is enabled on its own, because they do not cost the same: labels
// need a permission the App already holds, while settings and rulesets need
// Administration write and an installation that has not approved it yet is the
// normal state during a rollout rather than an error.
type Kind string

const (
	KindLabels   Kind = "labels"
	KindSettings Kind = "settings"
	KindRulesets Kind = "rulesets"
	KindFiles    Kind = "files"
)

// Kinds is every kind, in the order they are applied within one repository.
//
// Files last because a file change opens a pull request, which is the only part
// of this a person sees arrive. It should not arrive when the rest of the work
// on that repository failed.
func Kinds() []Kind { return []Kind{KindLabels, KindSettings, KindRulesets, KindFiles} }

// Valid reports a kind this package knows.
func (k Kind) Valid() bool {
	for _, known := range Kinds() {
		if k == known {
			return true
		}
	}

	return false
}

// Proposes reports a kind whose work reaches a repository as a pull request
// rather than as a change to it.
//
// Files are the only one, and the difference is what an outcome means: applying
// a file action opens or updates a proposal, so nothing has been written to the
// repository and nothing has been removed from it until somebody merges.
func (k Kind) Proposes() bool { return k == KindFiles }

// RequiredPermission is what an installation must have granted for a kind to
// run, in GitHub's own spelling.
//
// Labels need only what the bot already holds to label a pull request, which is
// why they shipped without asking any installation for anything. The rest need
// permissions an installation has to approve, and until it does, that kind is
// unavailable rather than broken.
func (k Kind) RequiredPermission() string {
	switch k {
	case KindLabels:
		return "issues"
	case KindSettings, KindRulesets:
		return "administration"
	case KindFiles:
		return "contents"
	default:
		return ""
	}
}

// Unavailable is a kind an installation has not granted the permission for.
//
// It is a state to report, not an error to fail on. An installation that has
// not approved a newly requested permission is the ordinary condition during a
// rollout, and a sweep that treated it as a failure would fill an operator's
// history with the same refusal every tick while telling them nothing about
// what to do. The permission is named so the panel can say which one to grant.
type Unavailable struct {
	Kind       Kind
	Permission string
}

// Reason says what is missing, for somebody reading it rather than matching on
// it.
func (u Unavailable) Reason() string {
	return fmt.Sprintf(
		"Smyklot has not been granted %s access, which %s sync needs",
		u.Permission, u.Kind,
	)
}

// Grantor is whatever can answer whether a permission was granted.
//
// An interface because the same question is asked in two places that hold
// different things: the planner has the installation GitHub just described, and
// the executor has the row that was stored from it. One rule, asked the same
// way, so the two cannot come to disagree about whether an installation may act.
type Grantor interface {
	Grants(permission string) bool
}

// unpermitted reports the first permission a grantor has not granted for a
// kind's work: the kind's own, and then whatever else the work needs.
//
// The kind's own first, because it is the one to grant first - being told to
// approve Workflows while Contents is still missing is advice that does not
// help.
func unpermitted(grantor Grantor, kind Kind, extra ...string) (Unavailable, bool) {
	for _, permission := range slices.Concat([]string{kind.RequiredPermission()}, extra) {
		if permission == "" || grantor.Grants(permission) {
			continue
		}

		return Unavailable{Kind: kind, Permission: permission}, true
	}

	return Unavailable{}, false
}

// UnpermittedPath reports a permission an installation has not granted for work
// on one path, which can need more than its kind does. See Kind.PathPermission.
func UnpermittedPath(grantor Grantor, kind Kind, path string) (Unavailable, bool) {
	return unpermitted(grantor, kind, kind.PathPermission(path))
}

// UnpermittedConfig reports a permission an installation has not granted for a
// whole configuration: the kind's own, and whatever the paths it names need on
// top of it.
//
// Asked before anything is planned, because the alternative is a plan somebody
// approves and GitHub then refuses - which is the rule repositoryPlanner states
// for itself: a plan holding work GitHub is going to refuse asks somebody to
// approve a promise it cannot keep.
func UnpermittedConfig(grantor Grantor, config Config) (Unavailable, bool) {
	return unpermitted(grantor, config.Kind, configPermissions(config)...)
}

// configPermissions is what a configuration's own contents need.
//
// A document this version cannot read contributes nothing, and nothing slips
// through on that: a kind whose document does not decode plans no work at all,
// which the planner reports one step later and in better words than a
// permission check could find.
func configPermissions(config Config) []string {
	if config.Kind != KindFiles {
		return nil
	}

	named, err := decodeFilePaths(config.Document)
	if err != nil {
		return nil
	}

	return named.Permissions()
}

// Operation is what an action does to its subject.
type Operation string

const (
	// OperationCreate adds something the repository does not have.
	OperationCreate Operation = "create"

	// OperationUpdate changes something it has.
	OperationUpdate Operation = "update"

	// OperationDelete removes something configuration no longer names.
	//
	// Never produced unless removal is switched on for that kind. It is the one
	// operation that destroys something a person may have created by hand, so
	// it is off by default and it appears in the plan before it ever runs.
	OperationDelete Operation = "delete"
)

// Trigger records what caused a plan to be computed, because the answer decides
// who is accountable for it and how it should be reported.
type Trigger string

const (
	// TriggerManual is somebody pressing the button in the panel.
	TriggerManual Trigger = "manual"

	// TriggerSave is a configuration change, planned immediately so the person
	// who made it sees what it would do.
	TriggerSave Trigger = "save"

	// TriggerReconcile is the periodic sweep, which is what catches a change
	// made on GitHub rather than here.
	TriggerReconcile Trigger = "reconcile"

	// TriggerWebhook is GitHub reporting that a repository changed.
	TriggerWebhook Trigger = "webhook"
)

// Valid reports a trigger this package knows.
func (t Trigger) Valid() bool {
	switch t {
	case TriggerManual, TriggerSave, TriggerReconcile, TriggerWebhook:
		return true
	default:
		return false
	}
}

// Action is one change to one subject in one repository.
//
// Subject is the thing's name within its kind - a label name, a file path, a
// ruleset name. It is what makes an action addressable in a list a person is
// reading, and what lets a second plan recognise the same work.
type Action struct {
	ID           int64
	PlanID       string
	RepositoryID string
	Kind         Kind
	Operation    Operation
	Subject      string

	// Before and After are what the subject looks like on either side of the
	// change, rendered for display. Before is empty for a creation and After is
	// empty for a deletion.
	Before string
	After  string

	// Payload is what to apply, as the kind that owns it spells it.
	//
	// Carried here rather than re-read from the configuration when the work
	// runs. The plan is the contract between what somebody reviewed and what
	// happens, and reading the configuration again at apply time would apply
	// what it says then - which is exactly what the plan exists to stop.
	//
	// A deletion carries one only where the subject is not enough to address
	// the thing. A label is deleted by name; a ruleset is deleted by an id
	// GitHub minted, and looking that id up again at apply time would find
	// whatever holds the name by then.
	Payload []byte

	State ActionState

	// Error is why the action failed, empty otherwise. Actions fail alone: no
	// attempt is made to undo the ones that already succeeded, because
	// unwinding a settings change because a later ruleset failed leaves a
	// repository in a state nobody chose.
	Error string

	// Blocker names the earlier kind whose failure stopped this action from
	// being attempted at all, empty otherwise. A repository that fails on
	// labels does not go on to have its files rewritten.
	Blocker Kind
}

// ActionState is how far one action got.
type ActionState string

const (
	ActionPending ActionState = "pending"
	ActionApplied ActionState = "applied"
	ActionFailed  ActionState = "failed"

	// ActionSkipped is an action that was never attempted, because an earlier
	// kind failed on the same repository.
	ActionSkipped ActionState = "skipped"
)

// PlanState is where a plan is in its life.
type PlanState string

const (
	// PlanComputed is a plan waiting for somebody to approve it.
	PlanComputed PlanState = "computed"

	// PlanApproved is one somebody approved, waiting for an executor.
	PlanApproved PlanState = "approved"

	// PlanApplying is one an executor holds a lease on.
	PlanApplying PlanState = "applying"

	// PlanApplied is one that finished with every action applied.
	PlanApplied PlanState = "applied"

	// PlanFailed is one that finished with at least one action failed.
	PlanFailed PlanState = "failed"

	// PlanStale is one whose configuration changed underneath it. Content, not
	// time: the digest it was computed from no longer matches.
	PlanStale PlanState = "stale"

	// PlanExpired is one nobody acted on in time. Time, not content.
	//
	// Separate from stale on purpose. A person reading "expired" knows to press
	// the button again; a person reading "stale" knows somebody changed
	// something. Collapsing them would lose which of those happened.
	PlanExpired PlanState = "expired"

	// PlanDiscarded is one somebody read and declined. An act, not a timer:
	// history should say who turned it down, and the next sweep computes a
	// fresh plan from whatever the configuration says by then.
	PlanDiscarded PlanState = "discarded"
)

// Live reports a plan that could still be applied, which is what makes it worth
// invalidating when configuration changes.
func (s PlanState) Live() bool {
	return s == PlanComputed || s == PlanApproved || s == PlanApplying
}

// Plan is one computed answer to "what would change", and the unit a person
// approves.
type Plan struct {
	ID       string
	TargetID string
	Trigger  Trigger

	// ActorAccountID is who is accountable. For a reconcile it is whoever last
	// saved the configuration being enforced, carried onto the plan rather than
	// replaced by a synthetic account: the sweep is doing what they asked for,
	// on a timer.
	ActorAccountID string

	// Digest fingerprints the configuration this was computed from. Comparing
	// digests is how invalidation stays an equality check rather than a diff,
	// and the apply request carries the digest the browser rendered - which is
	// the only thing standing between what somebody reviewed and what runs.
	Digest string

	State PlanState

	// Counts summarise the actions without reading them, for a list that shows
	// many plans at once.
	Counts Counts

	ComputedAt time.Time
	ApprovedAt *time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time

	// ExpiresAt is when this stops being offerable. Checked inside the approve
	// transaction as well as swept, so correctness never depends on the sweeper
	// having run.
	ExpiresAt time.Time

	// LeaseExpiresAt is held by the executor applying it, so a crashed executor
	// leaves work that can be claimed again rather than a plan stuck in
	// applying for ever.
	LeaseExpiresAt *time.Time

	// Attempt bounds retries of an apply that keeps dying.
	Attempt int
}

// Counts are the actions in a plan, by what they would do.
type Counts struct {
	Create int
	Update int
	Delete int
}

// Total is every action in the plan.
func (c Counts) Total() int { return c.Create + c.Update + c.Delete }

// Empty reports a plan with nothing to do.
//
// Worth its own name because it decides whether anything is recorded at all: a
// reconcile that found nothing is not an event, and writing one every tick
// would add on the order of a hundred thousand audit rows a year per
// installation to say that nothing happened.
func (c Counts) Empty() bool { return c.Total() == 0 }
