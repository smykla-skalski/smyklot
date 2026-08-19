package orgsync

import "sort"

// KindWork is one repository's actions for one kind.
type KindWork struct {
	Kind    Kind
	Actions []Action
}

// RepositoryWork is everything one repository has waiting, in the order it runs.
type RepositoryWork struct {
	RepositoryID string
	Kinds        []KindWork
}

// Schedule groups a plan's actions into the order they are applied.
//
// By repository, then by kind in the order Kinds() gives: labels, settings,
// rulesets, files. Files last because a file change opens a pull request, the
// only part of this a person sees arrive, and it should not arrive when the
// rest of the work on that repository failed.
//
// Repositories are sorted so that two runs of the same plan do the same things
// in the same order. An executor that resumed after a crash would otherwise
// interleave differently, and a person comparing two runs could not tell
// whether anything had changed.
func Schedule(actions []Action) []RepositoryWork {
	byRepository := map[string]map[Kind][]Action{}

	for _, action := range actions {
		if byRepository[action.RepositoryID] == nil {
			byRepository[action.RepositoryID] = map[Kind][]Action{}
		}
		byRepository[action.RepositoryID][action.Kind] = append(byRepository[action.RepositoryID][action.Kind], action)
	}

	repositories := make([]string, 0, len(byRepository))
	for repository := range byRepository {
		repositories = append(repositories, repository)
	}
	sort.Strings(repositories)

	work := make([]RepositoryWork, 0, len(repositories))
	for _, repository := range repositories {
		item := RepositoryWork{RepositoryID: repository}

		for _, kind := range Kinds() {
			if actions := byRepository[repository][kind]; len(actions) > 0 {
				item.Kinds = append(item.Kinds, KindWork{Kind: kind, Actions: actions})
			}
		}

		work = append(work, item)
	}

	return work
}

// Outcome accumulates what an executor did, so the plan can be closed from one
// value rather than from variables threaded through a loop.
type Outcome struct {
	Actions []ActionOutcome

	// Applied is the repository and kind pairs whose every action succeeded.
	// Only those: a kind that half-applied must be planned again next time, so
	// recording its digest would be recording work that was not done.
	Applied []RepositoryState

	// Deleted counts the removals that actually happened, which is audited on
	// its own. Deletion is off by default and destroys something somebody may
	// have made by hand, so a reader should not have to notice a number inside
	// a summary to learn that anything was removed.
	Deleted int

	// Succeeded and Failed count what became of the work, so closing a plan
	// does not mean walking its actions again.
	Succeeded int
	Failed    int
}

// Fail records an action that could not be applied.
func (o *Outcome) Fail(action Action, reason string) {
	o.Failed++
	o.Actions = append(o.Actions, ActionOutcome{
		ActionID: action.ID, State: ActionFailed, Error: reason,
	})
}

// Apply records an action that succeeded.
func (o *Outcome) Apply(action Action) {
	o.Succeeded++

	// Only where applying it removed something. A kind that proposes has
	// removed nothing yet - somebody has been asked - and the audit entry this
	// count writes exists to make destruction visible, so recording one for a
	// pull request nobody has merged would report a thing that did not happen.
	if action.Operation == OperationDelete && !action.Kind.Proposes() {
		o.Deleted++
	}
	o.Actions = append(o.Actions, ActionOutcome{ActionID: action.ID, State: ActionApplied})
}

// Skip records work never attempted because an earlier kind failed on the same
// repository, naming the kind that stopped it.
//
// Recorded rather than left pending, because a pending action is work a later
// lease would pick up and try - and trying the files of a repository whose
// labels just failed is exactly what this ordering exists to prevent.
func (o *Outcome) Skip(action Action, blocker Kind) {
	o.Failed++
	o.Actions = append(o.Actions, ActionOutcome{
		ActionID: action.ID, State: ActionSkipped, Blocker: blocker,
	})
}

// Carry accounts for an action a previous attempt already settled.
//
// A plan's verdict is about the plan, not about the attempt that happened to
// close it. Without this, a retry that found every action already settled would
// count no failures of its own and close a plan as applied although an earlier
// attempt had failed one - reporting success for work that never happened, and
// recording the digest that says the repository is up to date.
func (o *Outcome) Carry(action Action) {
	if action.State != ActionApplied {
		o.Failed++
	}
}

// State is the state to close the plan in. Anything failed or skipped makes the
// whole plan failed: a plan that did most of its work is not a plan that did it.
func (o *Outcome) State() PlanState {
	if o.Failed > 0 {
		return PlanFailed
	}

	return PlanApplied
}
