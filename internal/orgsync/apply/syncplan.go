package apply

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	appconfig "github.com/smykla-skalski/smyklot/pkg/config"
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

type queuePolicyReader interface {
	GetEffectiveQueuePolicy(context.Context, workqueue.Kind, *string) (workqueue.Policy, error)
}

// PlanInstallation computes what one installation's repositories would need.
//
// A change plan exists only when work is needed. Queue-backed checks separately
// retain their comparison outcome, including results that need no changes.
func (s *Engine) PlanInstallation(
	ctx context.Context,
	client *github.Client,
	targetID string,
	trigger orgsync.Trigger,
) error {
	_, err := s.PlanInstallationWithSummary(ctx, client, targetID, trigger)

	return err
}

// PlanInstallationWithSummary computes drift and names the durable result for
// the queue ledger. Scheduled scans still avoid a domain audit row when there
// is no drift. The queue summary distinguishes observed agreement, cached
// evidence, proposals and checks that could not finish.
func (s *Engine) PlanInstallationWithSummary(
	ctx context.Context,
	client *github.Client,
	targetID string,
	trigger orgsync.Trigger,
) (string, error) {
	return s.planInstallation(ctx, client, targetID, trigger, nil)
}

// PlanInstallationForCheck retains this occurrence's comparison evidence and
// binds any resulting change plan in the same transaction.
func (s *Engine) PlanInstallationForCheck(
	ctx context.Context, client *github.Client, targetID string,
	trigger orgsync.Trigger, check orgsync.CheckReference,
) (string, error) {
	return s.planInstallation(ctx, client, targetID, trigger, &check)
}

func (s *Engine) planInstallation(
	ctx context.Context, client *github.Client, targetID string,
	trigger orgsync.Trigger, check *orgsync.CheckReference,
) (string, error) {
	configs, err := s.store.ListSyncConfigs(ctx, targetID)
	if err != nil {
		return "", fmt.Errorf("read sync configuration: %w", err)
	}

	// The stored installation, not the one the sweep is holding.
	//
	// The executor reads this row too, and it has no choice: it holds an
	// installation token and cannot ask GitHub what was granted. Two sources
	// for one fact is two answers, and the one that decides whether work runs
	// should be the one the work will be judged against.
	target, err := s.store.GetTarget(ctx, targetID)
	if err != nil {
		return "", fmt.Errorf("read sync installation: %w", err)
	}

	switchedOn := switchedOnSyncKinds(configs)
	active := activeSyncKinds(ctx, switchedOn, target)
	missing := missingCheckPermissions(switchedOn, active)
	inactive := inactiveCheckDisposition(switchedOn)

	applied, err := s.store.ListSyncRepositoryState(ctx, targetID)
	if err != nil {
		return "", fmt.Errorf("read sync repository state: %w", err)
	}

	// The state is read first and the rest of the catalog only where something
	// is going to use it. An installation with sync switched off returns below
	// having read one table, which is what it did before a refusal had to be
	// cleared - and a refusal to clear is the exception, not the tick.
	if len(active) == 0 && !anyRefused(applied) {
		return s.retainCheck(ctx, targetID, check, (syncScanResult{}).checkResult(inactive, inactiveSyncSummary(switchedOn), missing))
	}

	held, err := s.syncInventoryFor(ctx, target, applied)
	if err != nil {
		return "", err
	}

	// Scoped by what is switched on rather than by what can act, so a kind
	// waiting on a permission keeps its refusals rather than having them read
	// as nothing being wrong.
	scopes := syncScopesFor(switchedOn, held, s.formattingPolicy(), trigger)

	// Before the early returns below, and that is the whole reason this runs
	// here: a refusal is only worth keeping while the planner is still looking,
	// and the ways it stops looking include the kind being switched off, which
	// is the case that returns first.
	if err := s.clearStaleSyncProblems(ctx, scopes, held); err != nil {
		return "", err
	}

	if len(active) == 0 {
		// Nothing switched on and permitted, so there is nothing to compare
		// against.
		return s.retainCheck(ctx, targetID, check, (syncScanResult{}).checkResult(inactive, inactiveSyncSummary(switchedOn), missing))
	}

	// A plan already in flight holds the installation's one live slot. Leaving
	// it alone is what makes pressing "sync now" twice, or a reconcile landing
	// beside it, idempotent rather than a conflict somebody has to read about.
	if summary, found, err := s.livePlanSummary(ctx, targetID); err != nil {
		return "", err
	} else if found {
		return s.retainCheck(ctx, targetID, check, (syncScanResult{}).checkResult("deferred", summary, missing))
	}

	scan, err := s.planSyncActions(ctx, client, active, scopes, held)
	if err != nil {
		return "", err
	}
	scan.unpermitted = len(switchedOn) - len(active)
	if len(scan.actions) == 0 {
		return s.retainCheck(ctx, targetID, check, scan.checkResult("checked", scan.summary(), missing))
	}

	// Whoever last saved the configuration being enforced, carried onto the
	// plan. A reconcile is doing what they asked for on a timer, so naming them
	// is truthful where a synthetic account would not be.
	approvalTTL, err := s.syncApprovalTTL(ctx, targetID)
	if err != nil {
		return "", err
	}
	result := scan.checkResult("checked", "Repository changes queued for automatic sync. "+scan.summary(), missing)
	now := time.Now().UTC()
	plan, err := s.store.CreateSyncPlan(ctx, orgsync.PlanCreate{
		OriginCheck: check,
		CheckResult: &result,
		ID:          newSyncPlanID(),
		TargetID:    targetID,
		Trigger:     trigger,
		ActorID:     syncActor(active),
		Digest:      scopeDigest(configs, held, s.formattingPolicy()),
		Actions:     scan.actions,
		Now:         now,
		ExpiresAt:   now.Add(approvalTTL),
		Automatic:   true,
	})
	if err != nil {
		// Another caller won the slot between the read above and this write.
		// That is the index doing its job, not a failure worth reporting.
		if errors.Is(err, storage.ErrConflict) {
			return s.retainCheck(ctx, targetID, check, scan.checkResult("deferred", "A live sync plan is already available. "+scan.summary(), missing))
		}

		return "", fmt.Errorf("record sync plan: %w", err)
	}

	logging.From(ctx).Info("sync plan computed",
		"sync_plan", plan.ID, "trigger", trigger, "actions", len(scan.actions))

	// Only now, with a plan that has something in it. Every path above that
	// returns early returns without writing an entry, which is the rule: a
	// reconcile that found nothing is not an event.
	if err := s.store.RecordSyncAudit(ctx, orgsync.AuditEntry{
		TargetID: targetID, PlanID: plan.ID, ActorID: plan.ActorAccountID,
		Action:  orgsync.AuditPlanned,
		Summary: syncPlanSummary(plan.Counts),
		Counts:  plan.Counts,
		Now:     plan.ComputedAt,
	}); err != nil {
		return "", err
	}

	return result.Outcome.Summary, nil
}

func (s *Engine) livePlanSummary(ctx context.Context, targetID string) (string, bool, error) {
	if _, _, err := s.store.GetLiveSyncPlan(ctx, targetID); err == nil {
		return "A live sync plan is already available", true, nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return "", false, fmt.Errorf("read live sync plan: %w", err)
	}

	return "", false, nil
}

func (s *Engine) syncApprovalTTL(ctx context.Context, targetID string) (time.Duration, error) {
	reader, ok := s.store.(queuePolicyReader)
	if !ok {
		return syncPlanTTL, nil
	}
	policy, err := reader.GetEffectiveQueuePolicy(ctx, workqueue.KindSyncScan, &targetID)
	if err != nil {
		return 0, fmt.Errorf("read sync approval policy: %w", err)
	}
	if policy.ApprovalTTL != nil {
		return *policy.ApprovalTTL, nil
	}

	return syncPlanTTL, nil
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
	target       storage.Target
	repositories []storage.Repository
	overrides    []orgsync.RepositoryOverride
	applied      []orgsync.RepositoryState
}

// syncInventoryFor reads the rest of the catalog around state already in hand.
func (s *Engine) syncInventoryFor(
	ctx context.Context,
	target storage.Target,
	applied []orgsync.RepositoryState,
) (syncInventory, error) {
	repositories, err := s.store.ListRepositories(ctx, target.ID)
	if err != nil {
		return syncInventory{}, fmt.Errorf("read sync repositories: %w", err)
	}
	repositories = slices.DeleteFunc(repositories, func(repository storage.Repository) bool {
		return !repositoryEnabled(target, repository)
	})

	overrides, err := s.store.ListSyncRepositoryOverrides(ctx, target.ID)
	if err != nil {
		return syncInventory{}, fmt.Errorf("read sync overrides: %w", err)
	}

	return syncInventory{
		target:       target,
		repositories: repositories,
		overrides:    overrides,
		applied:      applied,
	}, nil
}

func repositoryEnabled(target storage.Target, repository storage.Repository) bool {
	return target.Available && repository.Available &&
		storage.RepositoryEnabled(target, repository)
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
func syncScopesFor(
	active []orgsync.Config,
	held syncInventory,
	formatting appconfig.FormattingPolicy,
	trigger orgsync.Trigger,
) map[orgsync.Kind]syncScope {
	now := time.Now().UTC()
	scopes := make(map[orgsync.Kind]syncScope, len(active))

	for _, config := range active {
		scope := newSyncScope(
			config, held.overrides, held.applied, now, formatting, held.target.ConfigPatch,
		)
		scope.fresh = trigger == orgsync.TriggerManual
		scopes[config.Kind] = scope
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
const RecheckInterval = orgsync.RecheckInterval

// syncScope answers which repositories a plan covers.
type syncScope struct {
	fresh       bool
	config      orgsync.Config
	overrides   map[string]*orgsync.RepositoryOverride
	applied     map[string]orgsync.RepositoryState
	now         time.Time
	formatting  appconfig.FormattingPolicy
	targetPatch appconfig.Patch
}

func newSyncScope(
	config orgsync.Config,
	overrides []orgsync.RepositoryOverride,
	applied []orgsync.RepositoryState,
	now time.Time,
	formatting appconfig.FormattingPolicy,
	targetPatch appconfig.Patch,
) syncScope {
	scope := syncScope{
		config:      config,
		overrides:   map[string]*orgsync.RepositoryOverride{},
		applied:     map[string]orgsync.RepositoryState{},
		now:         now,
		formatting:  formatting,
		targetPatch: targetPatch,
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

	// Explicit checks must observe GitHub again, including a proposal somebody
	// reopened or merged since the last scheduled observation. Scope still applies.
	if s.fresh {
		return true
	}

	// A refusal is recorded with no digest, which is what keeps it out of this:
	// digestFor is a sha256 and never empty, so a refused repository never
	// matches and is read again every sweep until it is fixed.
	state, known := s.applied[repository.ID]
	if !known || state.AppliedDigest != s.digestFor(repository) {
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
func (s syncScope) digestFor(repository storage.Repository) string {
	return orgsync.DigestRepositoryConfiguration(
		s.config.Kind, s.config.Digest, s.overrides[repository.ID],
		storage.RepositoryFormattingPolicy(s.formatting, s.targetPatch, repository),
	)
}

// ask puts the question to one repository and reads the answer as two things:
// what to plan, and what is now known about the repository.
//
// Every check replaces earlier evidence. Failure and newly observed drift
// invalidate the cache; agreement, a proposal or its rejection records the
// outcome beside the digest. None of those outcomes is inferred from silence.
func (s syncScope) ask(
	ctx context.Context,
	question repositoryQuestion,
	repository storage.Repository,
) (found []orgsync.Action, learned []orgsync.RepositoryState) {
	// state is this repository's row for this kind, filled in by whichever
	// answer writes one.
	state := orgsync.RepositoryState{
		RepositoryID:   repository.ID,
		Kind:           s.config.Kind,
		AppliedAt:      s.now,
		ObservedDigest: s.digestFor(repository),
	}

	answer, err := question(ctx, repository)
	found, problem := answer.actions, answer.problem
	if err != nil {
		// One repository refusing must not stop the rest. It will be planned
		// again on the next tick, and reporting a plan that silently omitted it
		// would be worse than a shorter one.
		logging.From(ctx).Warn("could not read a repository while planning",
			"repo", repository.FullName, "kind", s.config.Kind, "error", err)

		state.Observation = orgsync.ObservationFailed
		state.Problem = "Could not check this repository. Smyklot will retry automatically."
		return nil, []orgsync.RepositoryState{state}
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

		state.Observation = orgsync.ObservationBlocked
		state.Problem = problem

		return nil, []orgsync.RepositoryState{state}
	}

	if len(found) == 0 && answer.observation != "" {
		// Nothing to do, which is a fact worth keeping. It appears in no plan,
		// so an apply would never record it, and without a record this
		// repository is read from GitHub again on every tick for ever - the
		// cost the digest exists to remove.
		state.AppliedDigest = state.ObservedDigest
		state.Observation = answer.observation
		state.ProposalURL = answer.proposalURL

		return nil, []orgsync.RepositoryState{state}
	}

	// A newly observed difference invalidates earlier agreement even if this
	// plan later expires. Preserve no cache proof for work still to do.
	for index := range found {
		found[index].InputDigest = state.ObservedDigest
	}
	if len(found) > 0 {
		state.Observation = orgsync.ObservationDifferent
	}
	return found, []orgsync.RepositoryState{state}
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
