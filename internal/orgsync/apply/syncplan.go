package apply

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

// syncPlanTTL is how long a computed plan stays approvable.
//
// Long enough for somebody to read it, short enough that the installation's one
// live slot is not held overnight by a plan nobody came back to. A plan that
// expires is not lost: the next reconcile computes the same answer from the
// same state.
const syncPlanTTL = 2 * time.Hour

// PlanInstallation computes what one installation's repositories would need.
//
// It writes a plan only when there is something to do. A reconcile that found
// nothing is not an event, and recording one every tick would fill the audit
// with roughly a hundred and seventy-five thousand rows a year per installation
// saying that nothing happened.
func (s *Engine) PlanInstallation(
	ctx context.Context,
	client *github.Client,
	targetID string,
	trigger orgsync.Trigger,
) error {
	configs, err := s.store.ListSyncConfigs(ctx, targetID)
	if err != nil {
		return fmt.Errorf("read sync configuration: %w", err)
	}

	// The stored installation, not the one the sweep is holding.
	//
	// The executor reads this row too, and it has no choice: it holds an
	// installation token and cannot ask GitHub what was granted. Two sources
	// for one fact is two answers, and the one that decides whether work runs
	// should be the one the work will be judged against.
	target, err := s.store.GetTarget(ctx, targetID)
	if err != nil {
		return fmt.Errorf("read sync installation: %w", err)
	}

	switchedOn := switchedOnSyncKinds(configs)
	active := activeSyncKinds(ctx, switchedOn, target)

	applied, err := s.store.ListSyncRepositoryState(ctx, targetID)
	if err != nil {
		return fmt.Errorf("read sync repository state: %w", err)
	}

	// The state is read first and the rest of the catalog only where something
	// is going to use it. An installation with sync switched off returns below
	// having read one table, which is what it did before a refusal had to be
	// cleared - and a refusal to clear is the exception, not the tick.
	if len(active) == 0 && !anyRefused(applied) {
		return nil
	}

	held, err := s.syncInventoryFor(ctx, targetID, applied)
	if err != nil {
		return err
	}

	// Scoped by what is switched on rather than by what can act, so a kind
	// waiting on a permission keeps its refusals rather than having them read
	// as nothing being wrong.
	scopes := syncScopesFor(switchedOn, held)

	// Before the early returns below, and that is the whole reason this runs
	// here: a refusal is only worth keeping while the planner is still looking,
	// and the ways it stops looking include the kind being switched off, which
	// is the case that returns first.
	if err := s.clearStaleSyncProblems(ctx, scopes, held); err != nil {
		return err
	}

	if len(active) == 0 {
		// Nothing switched on and permitted, so there is nothing to compare
		// against.
		return nil
	}

	// A plan already in flight holds the installation's one live slot. Leaving
	// it alone is what makes pressing "sync now" twice, or a reconcile landing
	// beside it, idempotent rather than a conflict somebody has to read about.
	if _, _, err := s.store.GetLiveSyncPlan(ctx, targetID); err == nil {
		return nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("read live sync plan: %w", err)
	}

	actions, err := s.planSyncActions(ctx, client, active, scopes, held)
	if err != nil {
		return err
	}
	if len(actions) == 0 {
		return nil
	}

	// Whoever last saved the configuration being enforced, carried onto the
	// plan. A reconcile is doing what they asked for on a timer, so naming them
	// is truthful where a synthetic account would not be.
	plan, err := s.store.CreateSyncPlan(ctx, orgsync.PlanCreate{
		ID:        newSyncPlanID(),
		TargetID:  targetID,
		Trigger:   trigger,
		ActorID:   syncActor(active),
		Digest:    orgsync.DigestScope(configs, held.overrides),
		Actions:   actions,
		Now:       time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(syncPlanTTL),
	})
	if err != nil {
		// Another caller won the slot between the read above and this write.
		// That is the index doing its job, not a failure worth reporting.
		if errors.Is(err, storage.ErrConflict) {
			return nil
		}

		return fmt.Errorf("record sync plan: %w", err)
	}

	logging.From(ctx).Info("sync plan computed",
		"sync_plan", plan.ID, "trigger", trigger, "actions", len(actions))

	// Only now, with a plan that has something in it. Every path above that
	// returns early returns without writing an entry, which is the rule: a
	// reconcile that found nothing is not an event.
	return s.store.RecordSyncAudit(ctx, orgsync.AuditEntry{
		TargetID: targetID, PlanID: plan.ID, ActorID: plan.ActorAccountID,
		Action:  orgsync.AuditPlanned,
		Summary: syncPlanSummary(plan.Counts),
		Counts:  plan.Counts,
		Now:     plan.ComputedAt,
	})
}

// syncPlanSummary says what a plan would do, for somebody reading a history
// page rather than a plan.
func syncPlanSummary(counts orgsync.Counts) string {
	return fmt.Sprintf("%d to add, %d to change, %d to remove",
		counts.Create, counts.Update, counts.Delete)
}

// switchedOnSyncKinds is what an installation has asked for, permitted or not.
//
// Told apart from what it can act on, because the difference decides what
// happens to a repository's recorded refusal. A kind switched off is a kind
// nothing is going to look at again, so a refusal recorded under it is stale
// and goes. A kind switched on and waiting on a permission is one somebody is
// still expecting to run: its refusals are as true as they were, and clearing
// them would answer "nothing is wrong here" on the one page built to say what
// is - while the reason nothing is happening sits on a different page.
func switchedOnSyncKinds(configs []orgsync.Config) []orgsync.Config {
	switchedOn := make([]orgsync.Config, 0, len(configs))

	for _, config := range configs {
		if config.Enabled {
			switchedOn = append(switchedOn, config)
		}
	}

	return switchedOn
}

// activeSyncKinds narrows those to the ones an installation has been permitted.
//
// A kind switched on but not granted is reported and left out, so the rest of
// the sweep proceeds: an installation that has approved labels and not settings
// should get its labels, not a plan that fails on everything because one kind
// is waiting on somebody.
func activeSyncKinds(
	ctx context.Context,
	switchedOn []orgsync.Config,
	grantor orgsync.Grantor,
) []orgsync.Config {
	active := make([]orgsync.Config, 0, len(switchedOn))

	for _, config := range switchedOn {
		if unavailable, missing := orgsync.UnpermittedConfig(grantor, config); missing {
			logging.From(ctx).Info("sync is configured but not permitted",
				"kind", unavailable.Kind, "permission", unavailable.Permission)

			continue
		}

		active = append(active, config)
	}

	return active
}

// syncActor is who a plan is attributed to: whoever last saved any of the
// configuration it enforces.
//
// The most recent, because a plan carries one actor and the newest save is the
// one that caused this plan to differ from the last. Two saved at the same
// instant is one save the panel cannot make and a tie nothing can break on the
// merits, so the earlier kind wins - MaxFunc keeps the first of equals, and the
// configurations arrive ordered by kind, so the answer is at least the same one
// every time.
//
// Never called with nothing: PlanInstallation returns before this when no
// kind is active, and MaxFunc has no answer for an empty slice.
func syncActor(active []orgsync.Config) string {
	return slices.MaxFunc(active, func(one, other orgsync.Config) int {
		return one.UpdatedAt.Compare(other.UpdatedAt)
	}).UpdatedBy
}

// syncInventory is what an installation holds, read once and asked twice: the
// planner reads it to work out what each repository needs, and the sweep reads
// it to work out which recorded refusals nothing is looking at any more.
type syncInventory struct {
	repositories []storage.Repository
	overrides    []orgsync.RepositoryOverride
	applied      []orgsync.RepositoryState
}

// syncInventoryFor reads the rest of the catalog around state already in hand.
func (s *Engine) syncInventoryFor(
	ctx context.Context,
	targetID string,
	applied []orgsync.RepositoryState,
) (syncInventory, error) {
	repositories, err := s.store.ListRepositories(ctx, targetID)
	if err != nil {
		return syncInventory{}, fmt.Errorf("read sync repositories: %w", err)
	}

	overrides, err := s.store.ListSyncRepositoryOverrides(ctx, targetID)
	if err != nil {
		return syncInventory{}, fmt.Errorf("read sync overrides: %w", err)
	}

	return syncInventory{
		repositories: repositories,
		overrides:    overrides,
		applied:      applied,
	}, nil
}

// anyRefused reports state worth reading the rest of the catalog for.
func anyRefused(applied []orgsync.RepositoryState) bool {
	return slices.ContainsFunc(applied, func(state orgsync.RepositoryState) bool {
		return state.Problem != ""
	})
}

// syncScopesFor indexes what each active kind covers, once.
//
// Once, because two things ask: the planner, per repository, and the sweep
// clearing refusals nothing is going to rewrite. Building it twice is what let
// them answer the same question differently.
func syncScopesFor(active []orgsync.Config, held syncInventory) map[orgsync.Kind]syncScope {
	now := time.Now().UTC()
	scopes := make(map[orgsync.Kind]syncScope, len(active))

	for _, config := range active {
		scopes[config.Kind] = newSyncScope(config, held.overrides, held.applied, now)
	}

	return scopes
}

// clearStaleSyncProblems takes a recorded refusal off a repository the planner
// has stopped looking at.
//
// A refusal is written where a repository cannot be synced and rewritten every
// sweep until it can, which is what makes it worth reading. Nothing rewrites it
// once the repository leaves the planner's scope, and there are three ways out:
// the kind is switched off for the installation, it is switched off for this
// repository, or the repository is gone from the installation. The row then
// states, for ever, a reason nobody can act on - and usually the very reason
// somebody switched the kind off in the first place.
//
// Cleared rather than deleted. The row is what a repository has for a kind, and
// a repository that later comes back into scope is planned again on the next
// sweep either way.
func (s *Engine) clearStaleSyncProblems(
	ctx context.Context,
	scopes map[orgsync.Kind]syncScope,
	held syncInventory,
) error {
	var (
		now     = time.Now().UTC()
		cleared []orgsync.RepositoryState
		holding = map[string]storage.Repository{}
	)

	for _, repository := range held.repositories {
		holding[repository.ID] = repository
	}

	for _, state := range held.applied {
		if state.Problem == "" {
			continue
		}

		// An absent scope is a kind switched off for the installation. One
		// waiting on a permission has a scope, because scopes are built from
		// what is switched on - somebody is still expecting it to run, so its
		// refusals are kept rather than answered as nothing being wrong.
		scope, syncing := scopes[state.Kind]
		if !syncing {
			cleared = append(cleared, clearedState(state, now))

			continue
		}

		// An absent repository is one the installation no longer holds, and the
		// zero value it reads as is unavailable, which is what watches answers
		// no to.
		if scope.watches(holding[state.RepositoryID]) {
			continue
		}

		cleared = append(cleared, clearedState(state, now))
	}

	if len(cleared) == 0 {
		return nil
	}

	logging.From(ctx).Info(
		"taking refusals off repositories this sync no longer covers",
		"repositories", len(cleared))

	return s.store.RecordSyncRepositoryState(ctx, cleared)
}

// clearedState is the row a repository keeps once its refusal is taken off:
// what it is, with nothing known about it. Not a digest, because nothing has
// looked, and not a deletion, because the row is what a repository has for a
// kind and it is planned again either way once it comes back into scope.
func clearedState(state orgsync.RepositoryState, now time.Time) orgsync.RepositoryState {
	return orgsync.RepositoryState{
		RepositoryID: state.RepositoryID,
		Kind:         state.Kind,
		AppliedAt:    now,
	}
}

// planSyncActions asks each repository in scope what it would take to match.
func (s *Engine) planSyncActions(
	ctx context.Context,
	client *github.Client,
	active []orgsync.Config,
	scopes map[orgsync.Kind]syncScope,
	held syncInventory,
) ([]orgsync.Action, error) {
	var (
		actions []orgsync.Action
		matched []orgsync.RepositoryState
	)

	// Kind by kind, because each has its own configuration, its own fingerprint
	// and its own record of what a repository already has. A repository settled
	// for its labels may be out of date for its settings.
	for _, config := range active {
		scope := scopes[config.Kind]

		ask, err := repositoryPlanner(client, config, scope.overrides)
		if err != nil {
			// A stored document this version cannot use. Every repository would
			// answer the same way, so the kind stands down rather than failing
			// once per repository - and it stands down rather than planning,
			// because a plan holding work GitHub is going to refuse asks
			// somebody to approve a promise it cannot keep.
			logging.From(ctx).Warn("sync configuration cannot be planned",
				"kind", config.Kind, "error", err)

			continue
		}

		for _, repository := range held.repositories {
			if !scope.covers(repository) {
				continue
			}

			found, learned := scope.ask(ctx, ask, repository)
			actions = append(actions, found...)
			matched = append(matched, learned...)
		}
	}

	if err := s.store.RecordSyncRepositoryState(ctx, matched); err != nil {
		return nil, err
	}

	return actions, nil
}

// repositoryQuestion asks one repository what one kind would take.
//
// problem is empty where the answer covers the whole of what this kind
// configures, which is nearly always, and for labels and settings is always. A
// ruleset a repository holds twice is one exception: nothing can say which one
// the configuration meant, so part of the kind is unresolved however much of
// the rest was worked out. A file sync has three of its own.
//
// A problem throws the actions away with it, because the executor records a
// kind settled once its every action applied - so acting on the resolved part
// would mark the unresolved part up to date too. It is words rather than a
// flag because it is the only account of why this repository is not being
// synced that anybody outside the service log ever sees.
//
// An error is different: the repository could not be read at all, which is
// nobody's mistake to fix and is retried on the next tick.
type repositoryQuestion func(
	context.Context, storage.Repository,
) (found []orgsync.Action, problem string, err error)

// repositoryPlanner reads a kind's stored document and returns what to ask each
// repository with it.
//
// The one place a kind's stored document meets its planner, and it is read once
// for the whole kind rather than once per repository: the document is the same
// for all of them, so decoding it inside the loop would decode it a hundred
// times over and report a document nobody can read a hundred times too.
//
// Validated here as well as in the panel. The panel covers what somebody typed;
// this covers a row written before a rule existed, or by a hand on the database,
// and every rule it checks is one GitHub answers with a 422. A kind this version
// does not know is refused rather than skipped, because skipping would record
// the repository as settled for work nothing did.
func repositoryPlanner(
	client *github.Client,
	config orgsync.Config,
	overrides map[string]*orgsync.RepositoryOverride,
) (repositoryQuestion, error) {
	switch config.Kind {
	case orgsync.KindLabels:
		return labelPlanner(client, config)

	case orgsync.KindSettings:
		return settingsPlanner(client, config)

	case orgsync.KindRulesets:
		return rulesetPlanner(client, config)

	case orgsync.KindFiles:
		return filePlanner(client, config, overrides)

	default:
		return nil, fmt.Errorf("%w: %s", errSyncKindUnsupported, config.Kind)
	}
}

func labelPlanner(client *github.Client, config orgsync.Config) (repositoryQuestion, error) {
	labels, err := decodeSyncDocument[orgsync.LabelConfig](config)
	if err != nil {
		return nil, err
	}

	return func(
		ctx context.Context, repository storage.Repository,
	) ([]orgsync.Action, string, error) {
		owner, name := splitFullName(repository.FullName)

		current, err := client.ListRepositoryLabels(ctx, owner, name)
		if err != nil {
			return nil, "", err
		}

		return orgsync.PlanLabels(
			repository.ID, labels, asCurrentLabels(current), labels.Exclusions(),
		), "", nil
	}, nil
}

func settingsPlanner(client *github.Client, config orgsync.Config) (repositoryQuestion, error) {
	settings, err := decodeSyncDocument[orgsync.SettingsConfig](config)
	if err != nil {
		return nil, err
	}

	return func(
		ctx context.Context, repository storage.Repository,
	) ([]orgsync.Action, string, error) {
		owner, name := splitFullName(repository.FullName)

		current, err := client.GetRepositorySettings(ctx, owner, name)
		if err != nil {
			return nil, "", err
		}

		return orgsync.PlanSettings(
			repository.ID, settings, asCurrentSettings(current),
		), "", nil
	}, nil
}

func rulesetPlanner(client *github.Client, config orgsync.Config) (repositoryQuestion, error) {
	rulesets, err := decodeSyncDocument[orgsync.RulesetConfig](config)
	if err != nil {
		return nil, err
	}

	return func(
		ctx context.Context, repository storage.Repository,
	) ([]orgsync.Action, string, error) {
		owner, name := splitFullName(repository.FullName)

		current, err := readRulesets(ctx, client, owner, name, rulesets)
		if err != nil {
			return nil, "", err
		}

		actions, ambiguous := orgsync.PlanRulesets(
			repository.ID, rulesets, current, rulesets.Exclusions())
		if len(ambiguous) > 0 {
			// A ruleset nothing can address produces no action, so a plan
			// cannot carry it and a person reading one would see a repository
			// that looks finished.
			return nil, "more than one ruleset here carries a configured name (" +
				strings.Join(ambiguous, ", ") +
				"), so nothing can say which one the configuration means", nil
		}

		return actions, "", nil
	}, nil
}

func filePlanner(
	client *github.Client,
	config orgsync.Config,
	overrides map[string]*orgsync.RepositoryOverride,
) (repositoryQuestion, error) {
	files, err := decodeSyncDocument[orgsync.FileConfig](config)
	if err != nil {
		return nil, err
	}

	return func(
		ctx context.Context, repository storage.Repository,
	) ([]orgsync.Action, string, error) {
		return planRepositoryFiles(ctx, client, repository, files, overrides[repository.ID])
	}, nil
}

// planRepositoryFiles answers what one repository's files would take, and where
// it cannot answer, why.
//
// Three ways not to: a repository with nowhere to propose against, adjustments
// that cannot be used, and files that cannot be composed. The last two are
// somebody's to fix, and recording a digest against either would say the
// repository matches for six hours when nothing has looked at it. All three are
// returned in words, because the alternative is a repository that is quietly
// receiving none of the organization's files and nothing anybody can read that
// says so.
func planRepositoryFiles(
	ctx context.Context,
	client *github.Client,
	repository storage.Repository,
	config orgsync.FileConfig,
	override *orgsync.RepositoryOverride,
) ([]orgsync.Action, string, error) {
	target := syncTargetFor(repository)

	if target.DefaultBranch == "" {
		// A repository with no commits has nowhere to propose against, and
		// GitHub names no branch for one. Said here rather than discovered
		// against the API, which would spend a request per repository per tick
		// learning it again.
		return nil, "this repository has no default branch, " +
			"so there is nowhere to propose a change", nil
	}

	adjustments, err := decodeFileOverride(override, config)
	if err != nil {
		return nil, "the adjustments saved for this repository cannot be used: " +
			err.Error(), nil
	}

	current, err := readTreePaths(
		ctx, client, target, target.DefaultBranch, config.Managed())
	if err != nil {
		return nil, "", err
	}

	if current.Missing {
		// There is no tree at that branch. GitHub names a default branch
		// whatever the case - the name is configuration, and it is there long
		// before the branch is - so the name says nothing about whether there
		// is anything to propose against. The tree read does, and it is a read
		// the planner makes already.
		//
		// Said rather than planned. Every managed path is absent from a
		// repository with no tree, so the planner would emit a create for each,
		// a person would approve them, and the apply would refuse for want of a
		// branch to build on - which spends the installation's one live plan
		// slot and marks every plan riding with it failed.
		//
		// The reason lists the causes rather than picking one. GitHub answers
		// 404 for a repository with no commits, for a branch that was renamed
		// since the catalog last looked, and for one this installation can no
		// longer read, and the read cannot tell them apart.
		return nil, "there is nothing at " + target.DefaultBranch +
			" to propose against: this repository has no commits, the branch was " +
			"renamed, or Smyklot can no longer read it", nil
	}

	plan, err := orgsync.PlanFiles(
		repository.ID, config, adjustments, target.DefaultBranch, current.Files)
	if err != nil {
		// A merge that cannot be applied. Fail-closed: no actions, and no
		// digest, so the repository is asked again once somebody fixes it.
		return nil, "these files cannot be composed: " + err.Error(), nil
	}

	if len(plan.Actions) == 0 {
		return nil, "", nil
	}

	asked, err := proposalOutstanding(ctx, client, target, plan.Proposal)
	if err != nil {
		return nil, "", err
	}

	if asked {
		// Already asked, so there is nothing to plan and the repository is
		// settled rather than asked again on every horizon. This is the whole
		// of what a file sync can do: propose. The branch is named after what
		// the files should end up saying, so a configuration that changes is a
		// different branch and the question is put once more.
		logging.From(ctx).Info(
			"this repository already has this change in front of it, so it is left alone",
			"repo", repository.FullName, "branch", plan.Proposal)

		return nil, "", nil
	}

	return plan.Actions, "", nil
}

// proposalOutstanding reports a change this repository has already been asked
// about and not resolved.
//
// Whatever state, because the answer decides whether to propose again. An open
// one is being considered and a closed one was refused, and both mean the
// asking is done - a plan computed for either would be the same plan, approved
// again, adopting the same pull request, once every horizon for as long as it
// sat there. A merged one is not outstanding: the change landed, and files that
// still differ after it are a new question.
func proposalOutstanding(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	proposal string,
) (bool, error) {
	pull, err := client.FindPullRequestByHead(
		ctx, target.Owner, target.Name, proposal, target.DefaultBranch)
	if err != nil || pull == nil {
		return false, err
	}

	return !pull.Merged, nil
}

// syncDocument is a kind's configuration: something to decode, and something
// that knows what GitHub would refuse.
type syncDocument interface{ Validate() error }

// decodeSyncDocument reads one and checks it.
func decodeSyncDocument[T syncDocument](config orgsync.Config) (T, error) {
	var document T
	if err := json.Unmarshal(config.Document, &document); err != nil {
		return document, fmt.Errorf("decode %s configuration: %w", config.Kind, err)
	}

	if err := document.Validate(); err != nil {
		return document, fmt.Errorf("%s configuration: %w", config.Kind, err)
	}

	return document, nil
}

// newSyncPlanID mints a plan identifier.
//
// Random rather than derived from the installation and a timestamp: a plan is
// addressable in a URL somebody can share, and an identifier that could be
// guessed from an installation name would let one be probed for.
func newSyncPlanID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// crypto/rand does not fail on any platform Smyklot runs on, and a
		// plan without an identifier is not something to carry on with.
		panic("read random bytes for a sync plan id: " + err.Error())
	}

	return "sync-" + hex.EncodeToString(raw[:])
}

// RecheckInterval is how long a repository's recorded state counts as
// evidence that it still matches.
//
// The record says what a repository looked like when it was last read, which is
// a fact about the past. Nothing on GitHub stops somebody renaming a label or
// turning a feature off by hand, and without a horizon a repository that
// settled once is never looked at again - so the drift this exists to correct
// would be the one thing it cannot see.
//
// Six hours because the two costs are not close. A full pass is one request per
// repository per kind, and two hundred repositories on two kinds is four
// hundred requests every six hours against a budget of five thousand an hour;
// what it buys is the difference between noticing a hand-made change by the
// same evening and never.
const RecheckInterval = 6 * time.Hour

// syncScope answers which repositories a plan covers.
type syncScope struct {
	config    orgsync.Config
	overrides map[string]*orgsync.RepositoryOverride
	applied   map[string]orgsync.RepositoryState
	now       time.Time
}

func newSyncScope(
	config orgsync.Config,
	overrides []orgsync.RepositoryOverride,
	applied []orgsync.RepositoryState,
	now time.Time,
) syncScope {
	scope := syncScope{
		config:    config,
		overrides: map[string]*orgsync.RepositoryOverride{},
		applied:   map[string]orgsync.RepositoryState{},
		now:       now,
	}

	// This kind's rows and no other's. A repository decides each kind on its
	// own - somebody may want their labels left alone and their settings kept
	// in step - and it settles each on its own too, against that kind's digest.
	// Reading another kind's rows here would answer both questions with the
	// wrong one's answer.
	for _, override := range overrides {
		if override.Kind == config.Kind {
			scope.overrides[override.RepositoryID] = &override
		}
	}
	for _, state := range applied {
		if state.Kind == config.Kind {
			scope.applied[state.RepositoryID] = state
		}
	}

	return scope
}

// watches reports a repository this kind is being synced on at all.
//
// Everything about scope except how recently the repository was read, which is
// the half the sweep asks on its own: a refusal is only worth keeping while
// something is still going to rewrite it, and what stops that is a repository
// leaving scope rather than a repository being up to date. Written once so the
// two askers cannot come to different answers - a reason to fall out of scope
// that only one of them knew about would leave a refusal nothing rewrites and
// the panel stating it for ever.
func (s syncScope) watches(repository storage.Repository) bool {
	return repository.Available && !s.overrides[repository.ID].Disabled()
}

// covers reports a repository worth asking GitHub about.
//
// Two reasons to skip. A repository this kind is not synced on at all, and a
// repository whose recorded digest already matches what the configuration asks
// for and was read recently enough for that to still mean something - the
// second is what keeps a steady-state reconcile at zero API calls rather than
// one per repository, which is the difference between a sweep that costs
// nothing and one that spends an installation's whole hourly budget.
func (s syncScope) covers(repository storage.Repository) bool {
	if !s.watches(repository) {
		return false
	}

	// A refusal is recorded with no digest, which is what keeps it out of this:
	// digestFor is a sha256 and never empty, so a refused repository never
	// matches and is read again every sweep until it is fixed.
	state, known := s.applied[repository.ID]
	if !known || state.AppliedDigest != s.digestFor(repository.ID) {
		return true
	}

	// Settled, and how long ago decides whether that is still evidence. The
	// record answers what this repository looked like when it was read, and
	// nothing on GitHub stops somebody changing it by hand afterwards.
	return s.now.Sub(state.AppliedAt) >= RecheckInterval
}

// digestFor is what a repository would record once it matches, and what covers
// compares against. One expression, so the value written and the value tested
// cannot drift into disagreeing about whether a repository is settled.
func (s syncScope) digestFor(repositoryID string) string {
	return orgsync.DigestRepositoryKind(s.config.Digest, s.overrides[repositoryID])
}

// refused reports a repository the last look could not manage this kind on.
//
// Asked where a repository plans work, and only there: a refusal that still
// stands is rewritten with whatever the reason is now, and one that settles is
// overwritten by the digest. Work planned is the one outcome that writes
// nothing of its own, so it is the one that has to ask.
func (s syncScope) refused(repositoryID string) bool {
	return s.applied[repositoryID].Problem != ""
}

// ask puts the question to one repository and reads the answer as two things:
// what to plan, and what is now known about the repository.
//
// Four answers, and only one of them plans anything. A repository that cannot
// be read at all is left for the next tick; one this kind cannot be managed on
// records why; one that matches records the digest that lets the next sweep
// skip it; and one with work to do records nothing unless it is taking a
// refusal off.
func (s syncScope) ask(
	ctx context.Context,
	question repositoryQuestion,
	repository storage.Repository,
) (found []orgsync.Action, learned []orgsync.RepositoryState) {
	// state is this repository's row for this kind, filled in by whichever
	// answer writes one.
	state := orgsync.RepositoryState{
		RepositoryID: repository.ID,
		Kind:         s.config.Kind,
		AppliedAt:    s.now,
	}

	found, problem, err := question(ctx, repository)
	if err != nil {
		// One repository refusing must not stop the rest. It will be planned
		// again on the next tick, and reporting a plan that silently omitted it
		// would be worse than a shorter one.
		logging.From(ctx).Warn("could not read a repository while planning",
			"repo", repository.FullName, "kind", s.config.Kind, "error", err)

		return nil, nil
	}

	if problem != "" {
		// Read, and the answer was that this kind cannot be managed on this
		// repository rather than that it matches.
		//
		// Its actions go with it, and that is the whole of the reason: a kind
		// whose every action applied is a kind the executor records as settled,
		// against the digest for the whole configuration. Send one action and
		// the repository is marked up to date for everything the kind covers -
		// including the part nothing could address, which then goes
		// unlooked-at for six hours. Whichever end that silence is created at,
		// it is the same silence.
		//
		// So nothing is planned, and what is recorded is the reason rather than
		// a digest: the repository is read again every sweep until whoever owns
		// it resolves what is wrong, and meanwhile the panel can say why
		// nothing is happening. The kinds beside this one are untouched: this
		// is one kind on one repository.
		//
		// Logged here rather than where each reason is decided, because here is
		// where the repository and the kind are both in hand. Written out at
		// each of the sites that produce one, four of the five left the kind off
		// and two of them chose a different level for the same class of event -
		// and a sixth reason added later would have been silent unless whoever
		// wrote it remembered.
		logging.From(ctx).Warn("this kind is not being synced on this repository",
			"repo", repository.FullName, "kind", s.config.Kind, "reason", problem)

		state.Problem = problem

		return nil, []orgsync.RepositoryState{state}
	}

	if len(found) == 0 {
		// Nothing to do, which is a fact worth keeping. It appears in no plan,
		// so an apply would never record it, and without a record this
		// repository is read from GitHub again on every tick for ever - the
		// cost the digest exists to remove.
		state.AppliedDigest = s.digestFor(repository.ID)

		return nil, []orgsync.RepositoryState{state}
	}

	if s.refused(repository.ID) {
		// Planned, so whatever stopped this repository last time no longer
		// does. The digest is not written - the work has not been applied, and
		// the executor records that when it lands - but a refusal left standing
		// would have the panel saying the files are not being synced here while
		// a plan to sync them waited for approval.
		return found, []orgsync.RepositoryState{state}
	}

	return found, nil
}

// asCurrentSettings reads what GitHub said as what the planner compares.
//
// Written out rather than converted, because the two types no longer say the
// same thing: a security feature is absent from GitHub's answer where the
// repository cannot have it, and the planner needs that as a state of its own
// rather than as a missing pointer it might read as off.
func asCurrentSettings(settings github.RepositorySettings) orgsync.CurrentSettings {
	return orgsync.CurrentSettings{
		AllowMergeCommit:    settings.AllowMergeCommit,
		AllowSquashMerge:    settings.AllowSquashMerge,
		AllowRebaseMerge:    settings.AllowRebaseMerge,
		AllowAutoMerge:      settings.AllowAutoMerge,
		DeleteBranchOnMerge: settings.DeleteBranchOnMerge,
		AllowUpdateBranch:   settings.AllowUpdateBranch,

		SquashMergeCommitTitle:   settings.SquashMergeCommitTitle,
		SquashMergeCommitMessage: settings.SquashMergeCommitMessage,
		MergeCommitTitle:         settings.MergeCommitTitle,
		MergeCommitMessage:       settings.MergeCommitMessage,

		HasIssues:      settings.HasIssues,
		HasProjects:    settings.HasProjects,
		HasWiki:        settings.HasWiki,
		HasDiscussions: settings.HasDiscussions,

		AdvancedSecurity: featureState(settings.Security.AdvancedSecurity),
		SecretScanning:   featureState(settings.Security.SecretScanning),
		SecretScanningPushProtection: featureState(
			settings.Security.SecretScanningPushProtection),
		DependabotSecurityUpdates: featureState(
			settings.Security.DependabotSecurityUpdates),
	}
}

// featureState reads a security feature GitHub may not have mentioned.
func featureState(feature *github.SecurityFeature) orgsync.FeatureState {
	switch {
	case feature == nil:
		return orgsync.FeatureUnavailable
	case feature.On():
		return orgsync.FeatureOn
	default:
		return orgsync.FeatureOff
	}
}

func asCurrentLabels(labels []github.RepositoryLabel) []orgsync.CurrentLabel {
	current := make([]orgsync.CurrentLabel, 0, len(labels))
	for _, label := range labels {
		current = append(current, orgsync.CurrentLabel(label))
	}

	return current
}
