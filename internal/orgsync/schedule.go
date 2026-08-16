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
		byRepository[action.RepositoryID][action.Kind] =
			append(byRepository[action.RepositoryID][action.Kind], action)
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

	failed bool
}

// Fail records an action that could not be applied.
func (o *Outcome) Fail(action Action, reason string) {
	o.failed = true
	o.Actions = append(o.Actions, ActionOutcome{
		ActionID: action.ID, State: ActionFailed, Error: reason,
	})
}

// Apply records an action that succeeded.
func (o *Outcome) Apply(action Action) {
	o.Actions = append(o.Actions, ActionOutcome{ActionID: action.ID, State: ActionApplied})
}

// Skip records work never attempted because an earlier kind failed on the same
// repository, naming the kind that stopped it.
//
// Recorded rather than left pending, because a pending action is work a later
// lease would pick up and try - and trying the files of a repository whose
// labels just failed is exactly what this ordering exists to prevent.
func (o *Outcome) Skip(action Action, blocker Kind) {
	o.failed = true
	o.Actions = append(o.Actions, ActionOutcome{
		ActionID: action.ID, State: ActionSkipped, Blocker: blocker,
	})
}

// Failed reports whether anything failed or was skipped, which is what decides
// the plan's own state.
func (o *Outcome) Failed() bool { return o.failed }

// State is the state to close the plan in.
func (o *Outcome) State() PlanState {
	if o.failed {
		return PlanFailed
	}

	return PlanApplied
}
