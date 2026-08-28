package apply

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

// syncLease is how long an executor holds a plan before another may claim it.
//
// Comfortably longer than a plan takes, and short enough that a process killed
// mid-apply does not leave the work parked for an afternoon. The lease is
// renewed by nothing: an executor that outlives it loses the plan, which is the
// safe direction - the actions it already recorded are recorded, and the rest
// are still pending for whoever picks it up.
const syncLease = 15 * time.Minute

var errSyncPlanScopeStale = errors.New("sync plan scope is stale")

// ApplyPlans applies whatever plan is due, one per call.
//
// One, because the database holds one live plan per installation and the work
// is almost entirely waiting on GitHub. Draining a queue here would let one
// installation's large plan delay every other installation's small one.
func (s *Engine) ApplyPlans(ctx context.Context) error {
	_, err := s.ApplyOnePlan(ctx)

	return err
}

// ApplyOnePlan applies at most one queue-selected plan and reports whether it
// claimed work. The maintenance dispatcher uses the boolean to keep draining
// the single serial lane without guessing whether a plan was eligible.
func (s *Engine) ApplyOnePlan(ctx context.Context) (bool, error) {
	now := time.Now().UTC()

	lease, err := s.leaseSyncPlan(ctx, now)
	if err != nil {
		return false, fmt.Errorf("lease sync plan: %w", err)
	}
	if !lease.Found {
		return false, nil
	}
	s.announceQueue(lease.Plan.TargetID)
	defer s.announceQueue(lease.Plan.TargetID)

	ctx = logging.With(ctx, "sync_plan", lease.Plan.ID, "target", lease.Plan.TargetID)

	outcome, err := s.applySyncPlan(ctx, lease)
	if err != nil {
		if errors.Is(err, errSyncPlanScopeStale) {
			if staleErr := s.store.InvalidateSyncPlans(
				ctx, lease.Plan.TargetID, time.Now().UTC(),
			); staleErr != nil {
				return true, fmt.Errorf("invalidate stale sync plan: %w", staleErr)
			}
			logging.From(ctx).Warn("sync plan became stale before repository mutation")

			return true, nil
		}
		retryErr := s.store.RetrySyncPlan(ctx, orgsync.PlanRetry{
			PlanID: lease.Plan.ID, Failure: err.Error(), Now: time.Now().UTC(),
		})
		if retryErr != nil {
			return true, errors.Join(err, fmt.Errorf("schedule sync plan retry: %w", retryErr))
		}

		return true, err
	}

	finishedAt := time.Now().UTC()

	if err := s.store.FinishSyncPlan(ctx, orgsync.PlanOutcome{
		PlanID: lease.Plan.ID,
		State:  outcome.State(),
		Now:    finishedAt,

		Applied: outcome.Applied,
	}); errors.Is(err, storage.ErrConflict) {
		// Somebody changed the configuration while this ran, so the plan was
		// marked stale underneath it. The work that happened is recorded against
		// each action; what is refused is the claim that the repositories now
		// match, because they match a scope that has since moved.
		logging.From(ctx).Warn("sync plan went stale while it was being applied",
			"actions", len(outcome.Actions))

		return true, nil
	} else if err != nil {
		return true, fmt.Errorf("finish sync plan: %w", err)
	}

	logging.From(ctx).Info("sync plan applied",
		"state", outcome.State(), "actions", len(outcome.Actions))

	return true, s.recordSyncOutcomeAudit(ctx, lease.Plan, outcome, finishedAt)
}

func (s *Engine) leaseSyncPlan(
	ctx context.Context,
	now time.Time,
) (orgsync.PlanLease, error) {
	if s.beginWork == nil {
		return s.store.LeaseSyncPlan(ctx, now, now.Add(syncLease))
	}
	release, allowed := s.beginWork()
	if !allowed {
		return orgsync.PlanLease{}, nil
	}
	defer release()

	return s.store.LeaseSyncPlan(ctx, now, now.Add(syncLease))
}

// recordSyncOutcomeAudit writes what a plan did, and separately what it removed.
//
// Deletion gets its own entry rather than a number inside the outcome, because
// it is off by default and it is the one thing that destroys something somebody
// may have made by hand. A reader scanning actions should not have to notice a
// count to learn that anything was removed.
func (s *Engine) recordSyncOutcomeAudit(
	ctx context.Context,
	plan orgsync.Plan,
	outcome orgsync.Outcome,
	at time.Time,
) error {
	if err := s.store.RecordSyncAudit(ctx, orgsync.AuditEntry{
		TargetID: plan.TargetID, PlanID: plan.ID, ActorID: plan.ActorAccountID,
		Action:  orgsync.AuditFinished,
		Summary: fmt.Sprintf("%d applied, %d failed", outcome.Succeeded, outcome.Failed),
		Counts:  plan.Counts,
		Failed:  outcome.Failed,
		Now:     at,
	}); err != nil {
		return err
	}

	if outcome.Deleted == 0 {
		return nil
	}

	return s.store.RecordSyncAudit(ctx, orgsync.AuditEntry{
		TargetID: plan.TargetID, PlanID: plan.ID, ActorID: plan.ActorAccountID,
		Action:  orgsync.AuditDeleted,
		Summary: fmt.Sprintf("%d removed", outcome.Deleted),
		Counts:  orgsync.Counts{Delete: outcome.Deleted},
		Now:     at,
	})
}

// applySyncPlan drives one plan's work through GitHub.
func (s *Engine) applySyncPlan(
	ctx context.Context,
	lease orgsync.PlanLease,
) (orgsync.Outcome, error) {
	target, err := s.store.GetTarget(ctx, lease.Plan.TargetID)
	if err != nil {
		return orgsync.Outcome{}, fmt.Errorf("read sync installation: %w", err)
	}

	// Checked again here, not only where the plan was computed.
	//
	// A plan is approved by a person and applied minutes or hours later, and a
	// permission can be revoked in between - that is what revoking one is for.
	// Without this, every action in the plan would be refused one at a time and
	// the revocation would read as a repository that failed rather than as a
	// decision somebody made.
	if unavailable, missing := unavailableForTarget(target, lease.Actions); missing {
		return orgsync.Outcome{}, fmt.Errorf("%w: %s", errSyncNotPermitted, unavailable.Reason())
	}
	currentDigest, err := s.CurrentScopeDigest(ctx, lease.Plan.TargetID)
	if err != nil {
		return orgsync.Outcome{}, err
	}
	if currentDigest != lease.Plan.Digest {
		return orgsync.Outcome{}, errSyncPlanScopeStale
	}

	client, err := s.installationClient(target.InstallationID)
	if err != nil {
		return orgsync.Outcome{}, err
	}

	// What each repository would have once this plan lands, computed the same
	// way the planner computes what to compare against. Both call the same
	// function, so a value recorded here is a value the next plan will test -
	// two spellings of the same idea would drift, and the drift would look like
	// a repository that never settles.
	digests, err := s.syncDigests(ctx, lease.Plan.TargetID)
	if err != nil {
		return orgsync.Outcome{}, err
	}

	var outcome orgsync.Outcome

	for _, work := range orgsync.Schedule(lease.Actions) {
		err := s.applyRepositoryIfEnabled(
			ctx, lease.Plan.TargetID, work, &outcome,
			func(repository storage.Repository) error {
				s.applyRepositoryWork(ctx, client, repository, work, digests, &outcome)

				return nil
			},
		)
		if err != nil {
			return outcome, fmt.Errorf("coordinate sync repository %s: %w", work.RepositoryID, err)
		}
	}

	return outcome, nil
}

func (s *Engine) applyRepositoryIfEnabled(
	ctx context.Context,
	targetID string,
	work orgsync.RepositoryWork,
	outcome *orgsync.Outcome,
	apply func(storage.Repository) error,
) error {
	run := func() error {
		target, err := s.store.GetTarget(ctx, targetID)
		if errors.Is(err, storage.ErrNotFound) {
			s.abandonRepositoryWork(ctx, outcome, work, "installation is no longer available")

			return nil
		}
		if err != nil {
			return fmt.Errorf("refresh sync installation: %w", err)
		}
		repository, err := s.store.GetRepository(ctx, targetID, work.RepositoryID)
		if errors.Is(err, storage.ErrNotFound) {
			s.abandonRepositoryWork(ctx, outcome, work, "repository is no longer available")

			return nil
		}
		if err != nil {
			return fmt.Errorf("refresh sync repository: %w", err)
		}
		if !repositoryEnabled(target, repository) {
			s.abandonRepositoryWork(ctx, outcome, work, "repository is disabled in Smyklot")

			return nil
		}

		return apply(repository)
	}
	if s.coordinator == nil {
		return run()
	}

	return s.coordinator.Exclusive(ctx, work.RepositoryID, run)
}

// abandonRepositoryWork records every action for a repository that cannot be
// reached, so nothing is left pending for a later lease to retry for ever.
func (s *Engine) abandonRepositoryWork(
	ctx context.Context,
	outcome *orgsync.Outcome,
	work orgsync.RepositoryWork,
	reason string,
) {
	for _, kind := range work.Kinds {
		for _, action := range kind.Actions {
			outcome.Fail(action, reason)
			s.recordSyncAction(ctx, action, orgsync.ActionFailed, reason, "")
		}
	}
}

// applyRepositoryWork runs one repository's kinds in order, stopping that
// repository at the first kind that fails.
//
// Stopping this repository and no other. Actions fail alone and nothing is
// unwound: undoing a settings change because a later ruleset failed leaves a
// repository in a state nobody chose, which is worse than the partial state it
// would replace.
func (s *Engine) applyRepositoryWork(
	ctx context.Context,
	client *github.Client,
	repository storage.Repository,
	work orgsync.RepositoryWork,
	digests syncDigestIndex,
	outcome *orgsync.Outcome,
) {
	target := syncTargetFor(repository)
	ctx = logging.With(ctx, "repo", repository.FullName)

	var blocker orgsync.Kind

	for _, kind := range work.Kinds {
		if blocker != "" {
			for _, action := range kind.Actions {
				if action.State != orgsync.ActionPending {
					outcome.Carry(action)

					continue
				}
				outcome.Skip(action, blocker)
				s.recordSyncAction(ctx, action, orgsync.ActionSkipped, "", blocker)
			}

			continue
		}

		applied := s.applyKind(ctx, client, target, kind, outcome)
		if !applied {
			blocker = kind.Kind

			continue
		}

		// Only a kind whose every action succeeded records a digest. A kind
		// that half-applied has to be planned again, and recording it would
		// tell the next reconcile that work nobody did is done.
		outcome.Applied = append(outcome.Applied, orgsync.RepositoryState{
			RepositoryID:  repository.ID,
			Kind:          kind.Kind,
			AppliedDigest: digests.of(repository, kind.Kind),
			AppliedAt:     time.Now().UTC(),
		})
	}
}

// syncTarget is the repository one piece of sync work runs against.
type syncTarget struct {
	Owner string
	Name  string

	// DefaultBranch is what a file proposal is opened against, and empty where
	// GitHub named none - a repository with no commits at all.
	DefaultBranch string
}

func syncTargetFor(repository storage.Repository) syncTarget {
	owner, name := splitFullName(repository.FullName)

	return syncTarget{Owner: owner, Name: name, DefaultBranch: repository.DefaultBranch}
}

// applyKind runs one kind's actions and reports whether every one succeeded.
//
// Each outcome is written as it happens rather than accumulated and written at
// the end. An executor that dies halfway must not leave its finished work
// looking pending: the plan would be leased again and every applied action
// retried, and retrying a create GitHub has already honoured is a 422 that
// fails a repository for having succeeded.
func (s *Engine) applyKind(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	work orgsync.KindWork,
	outcome *orgsync.Outcome,
) bool {
	// A kind that proposes is one change, not a list of them. Every path a
	// repository needs goes into one commit behind one pull request, so they
	// are applied together and share whatever becomes of it.
	if work.Kind.Proposes() {
		return s.applyFileKind(ctx, client, target, work, outcome)
	}

	succeeded := true

	for _, action := range work.Actions {
		// Work an earlier attempt already settled. A lease carries every action
		// so a retry can still record the digest for a kind that finished, and
		// this is what keeps it from doing that kind's work a second time -
		// re-creating a label GitHub already made is a 422.
		if action.State != orgsync.ActionPending {
			outcome.Carry(action)
			succeeded = succeeded && action.State == orgsync.ActionApplied

			continue
		}

		err := s.applyAction(ctx, client, target, action)
		if err != nil {
			logging.From(ctx).Warn("sync action failed",
				"kind", action.Kind, "operation", action.Operation,
				"subject", action.Subject, "error", err)
			outcome.Fail(action, err.Error())
			s.recordSyncAction(ctx, action, orgsync.ActionFailed, err.Error(), "")
			succeeded = false

			continue
		}

		outcome.Apply(action)
		s.recordSyncAction(ctx, action, orgsync.ActionApplied, "", "")
	}

	return succeeded
}

// recordSyncAction writes what became of one action.
//
// A failure to write is logged rather than returned. The work against GitHub
// has already happened, and abandoning the rest of the plan because the note
// about it could not be filed would turn one lost record into a repository left
// half-synchronised.
func (s *Engine) recordSyncAction(
	ctx context.Context,
	action orgsync.Action,
	state orgsync.ActionState,
	reason string,
	blocker orgsync.Kind,
) {
	if err := s.store.RecordSyncActionOutcome(ctx, orgsync.ActionOutcome{
		ActionID: action.ID, State: state, Error: reason, Blocker: blocker,
	}); err != nil {
		logging.From(ctx).Error("could not record what became of a sync action",
			"subject", action.Subject, "error", err)
	}
}

// applyFileKind puts a repository's whole file change behind one pull request,
// and gives every action the same answer.
//
// The same answer, because it is one piece of work: the commit lands or it does
// not, and an action recorded as applied beside one recorded as failed would
// describe a repository that half-received a commit.
//
// Every action is replayed on a retry, including the ones an earlier attempt
// already recorded, because the change is what is being applied rather than the
// paths one at a time. It is safe to replay: the branch is named after what the
// files should say, so a second run builds the same tree, finds nothing to
// commit and adopts the pull request that is already open.
func (s *Engine) applyFileKind(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	work orgsync.KindWork,
	outcome *orgsync.Outcome,
) bool {
	pending := slices.ContainsFunc(work.Actions, func(action orgsync.Action) bool {
		return action.State == orgsync.ActionPending
	})

	if !pending {
		succeeded := true

		for _, action := range work.Actions {
			outcome.Carry(action)
			succeeded = succeeded && action.State == orgsync.ActionApplied
		}

		return succeeded
	}

	err := applyFileActions(ctx, client, target, work.Actions)
	if err != nil {
		logging.From(ctx).Warn("sync files failed", "error", err)
	}

	for _, action := range work.Actions {
		// Success covers every action, including ones an earlier attempt
		// already recorded: this attempt replayed the whole change, so its
		// answer is theirs. An attempt that died between recording one and
		// recording the next would otherwise leave the first saying "failed"
		// about a change that is now in the repository's proposal.
		if err == nil {
			outcome.Apply(action)
			s.recordSyncAction(ctx, action, orgsync.ActionApplied, "", "")

			continue
		}

		// Failure does not reach back the same way. The change did not land
		// this time, which says nothing about a commit an earlier attempt got
		// as far as making.
		if action.State != orgsync.ActionPending {
			outcome.Carry(action)

			continue
		}

		outcome.Fail(action, err.Error())
		s.recordSyncAction(ctx, action, orgsync.ActionFailed, err.Error(), "")
	}

	return err == nil
}

// applyAction performs one action against GitHub.
func (s *Engine) applyAction(
	ctx context.Context,
	client *github.Client,
	target syncTarget,
	action orgsync.Action,
) error {
	switch action.Kind {
	case orgsync.KindLabels:
		return s.applyLabelAction(ctx, client, target.Owner, target.Name, action)

	case orgsync.KindSettings:
		return applySettingsAction(ctx, client, target.Owner, target.Name, action)

	case orgsync.KindRulesets:
		return applyRulesetAction(ctx, client, target.Owner, target.Name, action)

	default:
		// Files never reach here: applyKind sends the whole kind through one
		// pull request rather than one action at a time. Refusing loudly beats
		// silently reporting an action applied that nothing performed.
		return fmt.Errorf("%w: %s", errSyncKindUnsupported, action.Kind)
	}
}

var (
	errSyncKindUnsupported = errors.New("this Smyklot cannot apply that kind of sync yet")

	// errSyncNotPermitted is a plan whose permission was revoked between being
	// approved and being applied. The plan keeps its lease and is offered again,
	// which is right: granting the permission back is all it needs.
	errSyncNotPermitted = errors.New("this installation has not permitted that sync")
)

// unavailableForTarget reports the first action in a plan that the installation
// no longer permits.
//
// The first, because a plan stops at one: there is no useful partial answer
// between "apply this" and "somebody has to grant something".
//
// Asked of every action rather than once per kind, because what an action needs
// is not the kind's alone: a file under .github/workflows needs a permission of
// its own, and GitHub enforces that where the ref moves - which is after the
// commit has been built and after somebody approved the plan.
//
// Of the pending ones only. An action that already applied is not work this
// attempt is going to do, and holding the plan on its account is how a lease
// that expired after the workflow landed came back, refused, and left the
// installation's one live slot filled by a plan nothing could finish or expire.
func unavailableForTarget(
	target storage.Target,
	actions []orgsync.Action,
) (orgsync.Unavailable, bool) {
	for _, action := range actions {
		if action.State != orgsync.ActionPending {
			continue
		}

		if unavailable, missing := orgsync.UnpermittedPath(
			target, action.Kind, action.Subject,
		); missing {
			return unavailable, true
		}
	}

	return orgsync.Unavailable{}, false
}

// syncDigestIndex answers what a repository and kind should record once its
// work lands.
type syncDigestIndex struct {
	configs     map[orgsync.Kind]string
	overrides   map[string]map[orgsync.Kind]*orgsync.RepositoryOverride
	formatting  config.FormattingPolicy
	targetPatch config.Patch
}

func (i syncDigestIndex) of(repository storage.Repository, kind orgsync.Kind) string {
	var inputs []orgsync.DigestInput
	if kind == orgsync.KindFiles {
		policy := repositoryFormattingPolicy(i.formatting, i.targetPatch, repository)
		inputs = append(inputs, orgsync.DigestInput{
			Name: digestInputFormatting, Digest: orgsync.DigestFormattingPolicy(policy),
		})
	}

	return orgsync.DigestRepositoryKindWithInputs(
		i.configs[kind], i.overrides[repository.ID][kind], inputs,
	)
}

// syncDigests reads what an installation has configured, once per plan rather
// than once per repository.
func (s *Engine) syncDigests(ctx context.Context, targetID string) (syncDigestIndex, error) {
	configs, err := s.store.ListSyncConfigs(ctx, targetID)
	if err != nil {
		return syncDigestIndex{}, fmt.Errorf("read sync configuration: %w", err)
	}

	overrides, err := s.store.ListSyncRepositoryOverrides(ctx, targetID)
	if err != nil {
		return syncDigestIndex{}, fmt.Errorf("read sync overrides: %w", err)
	}
	target, err := s.store.GetTarget(ctx, targetID)
	if err != nil {
		return syncDigestIndex{}, fmt.Errorf("read sync installation: %w", err)
	}

	index := syncDigestIndex{
		configs:     make(map[orgsync.Kind]string, len(configs)),
		overrides:   map[string]map[orgsync.Kind]*orgsync.RepositoryOverride{},
		formatting:  s.formattingPolicy(),
		targetPatch: target.ConfigPatch,
	}

	for _, config := range configs {
		index.configs[config.Kind] = config.Digest
	}
	for _, override := range overrides {
		if index.overrides[override.RepositoryID] == nil {
			index.overrides[override.RepositoryID] = map[orgsync.Kind]*orgsync.RepositoryOverride{}
		}
		index.overrides[override.RepositoryID][override.Kind] = &override
	}

	return index, nil
}

// installationClient mints a client for one installation.
func (s *Engine) installationClient(installationID string) (*github.Client, error) {
	id, err := strconv.ParseInt(installationID, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("%w: installation id %q", bot.ErrGitHubClient, installationID)
	}

	token, err := s.tokens.InstallationToken(id)
	if err != nil {
		return nil, bot.NewGitHubError(bot.ErrGitHubAppAuth, err)
	}

	client, err := github.NewClient(token, s.apiBaseURL)
	if err != nil {
		return nil, bot.NewGitHubError(bot.ErrGitHubClient, err)
	}

	return client, nil
}

func splitFullName(fullName string) (owner, name string) {
	for index := range len(fullName) {
		if fullName[index] == '/' {
			return fullName[:index], fullName[index+1:]
		}
	}

	return "", fullName
}
