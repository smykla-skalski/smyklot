package main

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

// planInstallationSync computes what one installation's repositories would need.
//
// It writes a plan only when there is something to do. A reconcile that found
// nothing is not an event, and recording one every tick would fill the audit
// with roughly a hundred and seventy-five thousand rows a year per installation
// saying that nothing happened.
func (s *server) planInstallationSync(
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

	active := activeSyncKinds(ctx, configs, target)
	if len(active) == 0 {
		// Nothing switched on and permitted, so there is nothing to compare
		// against.
		return nil
	}

	overrides, err := s.store.ListSyncRepositoryOverrides(ctx, targetID)
	if err != nil {
		return fmt.Errorf("read sync overrides: %w", err)
	}

	// A plan already in flight holds the installation's one live slot. Leaving
	// it alone is what makes pressing "sync now" twice, or a reconcile landing
	// beside it, idempotent rather than a conflict somebody has to read about.
	if _, _, err := s.store.GetLiveSyncPlan(ctx, targetID); err == nil {
		return nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("read live sync plan: %w", err)
	}

	actions, err := s.planSyncActions(ctx, client, targetID, active, overrides)
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
		Digest:    orgsync.DigestScope(configs, overrides),
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

// activeSyncKinds is what an installation has switched on and been permitted.
//
// Both, in one place. A kind switched on but not granted is reported and left
// out, so the rest of the sweep proceeds: an installation that has approved
// labels and not settings should get its labels, not a plan that fails on
// everything because one kind is waiting on somebody.
func activeSyncKinds(
	ctx context.Context,
	configs []orgsync.Config,
	grantor orgsync.Grantor,
) []orgsync.Config {
	active := make([]orgsync.Config, 0, len(configs))

	for _, config := range configs {
		if !config.Enabled {
			continue
		}

		if unavailable, missing := orgsync.Unpermitted(grantor, config.Kind); missing {
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
// Never called with nothing: planInstallationSync returns before this when no
// kind is active, and MaxFunc has no answer for an empty slice.
func syncActor(active []orgsync.Config) string {
	return slices.MaxFunc(active, func(one, other orgsync.Config) int {
		return one.UpdatedAt.Compare(other.UpdatedAt)
	}).UpdatedBy
}

// planSyncActions asks each repository in scope what it would take to match.
func (s *server) planSyncActions(
	ctx context.Context,
	client *github.Client,
	targetID string,
	active []orgsync.Config,
	overrides []orgsync.RepositoryOverride,
) ([]orgsync.Action, error) {
	repositories, err := s.store.ListRepositories(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("read sync repositories: %w", err)
	}

	applied, err := s.store.ListSyncRepositoryState(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("read sync repository state: %w", err)
	}

	var (
		actions []orgsync.Action
		matched []orgsync.RepositoryState
		now     = time.Now().UTC()
	)

	// Kind by kind, because each has its own configuration, its own fingerprint
	// and its own record of what a repository already has. A repository settled
	// for its labels may be out of date for its settings.
	for _, config := range active {
		ask, err := repositoryPlanner(client, config)
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

		scope := newSyncScope(config, overrides, applied, now)

		for _, repository := range repositories {
			if !scope.covers(repository) {
				continue
			}

			found, err := ask(ctx, repository)
			if err != nil {
				// One repository refusing must not stop the rest. It will be
				// planned again on the next tick, and reporting a plan that
				// silently omitted it would be worse than a shorter one.
				logging.From(ctx).Warn("could not read a repository while planning",
					"repo", repository.FullName, "kind", config.Kind, "error", err)

				continue
			}

			if len(found) == 0 {
				// Nothing to do, which is a fact worth keeping. It appears in
				// no plan, so an apply would never record it, and without a
				// record this repository is read from GitHub again on every
				// tick for ever - the cost the digest exists to remove.
				matched = append(matched, orgsync.RepositoryState{
					RepositoryID:  repository.ID,
					Kind:          config.Kind,
					AppliedDigest: scope.digestFor(repository.ID),
					AppliedAt:     now,
				})

				continue
			}

			actions = append(actions, found...)
		}
	}

	if err := s.store.RecordSyncRepositoryState(ctx, matched); err != nil {
		return nil, err
	}

	return actions, nil
}

// repositoryQuestion asks one repository what one kind would take.
type repositoryQuestion func(
	context.Context, storage.Repository,
) ([]orgsync.Action, error)

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
) (repositoryQuestion, error) {
	switch config.Kind {
	case orgsync.KindLabels:
		labels, err := decodeSyncDocument[orgsync.LabelConfig](config)
		if err != nil {
			return nil, err
		}

		return func(
			ctx context.Context, repository storage.Repository,
		) ([]orgsync.Action, error) {
			owner, name := splitFullName(repository.FullName)

			current, err := client.ListRepositoryLabels(ctx, owner, name)
			if err != nil {
				return nil, err
			}

			return orgsync.PlanLabels(
				repository.ID, labels, asCurrentLabels(current), labels.Exclusions(),
			), nil
		}, nil

	case orgsync.KindSettings:
		settings, err := decodeSyncDocument[orgsync.SettingsConfig](config)
		if err != nil {
			return nil, err
		}

		return func(
			ctx context.Context, repository storage.Repository,
		) ([]orgsync.Action, error) {
			owner, name := splitFullName(repository.FullName)

			current, err := client.GetRepositorySettings(ctx, owner, name)
			if err != nil {
				return nil, err
			}

			return orgsync.PlanSettings(
				repository.ID, settings, asCurrentSettings(current),
			), nil
		}, nil

	default:
		return nil, fmt.Errorf("%w: %s", errSyncKindUnsupported, config.Kind)
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

// syncRecheckInterval is how long a repository's recorded state counts as
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
const syncRecheckInterval = 6 * time.Hour

// syncScope answers which repositories a plan covers.
type syncScope struct {
	config    orgsync.Config
	overrides map[string]*bool
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
		overrides: map[string]*bool{},
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
			scope.overrides[override.RepositoryID] = override.Enabled
		}
	}
	for _, state := range applied {
		if state.Kind == config.Kind {
			scope.applied[state.RepositoryID] = state
		}
	}

	return scope
}

// covers reports a repository worth asking GitHub about.
//
// Two reasons to skip. A repository that turned this off, and a repository
// whose recorded digest already matches what the configuration asks for and was
// read recently enough for that to still mean something - the second is what
// keeps a steady-state reconcile at zero API calls rather than one per
// repository, which is the difference between a sweep that costs nothing and
// one that spends an installation's whole hourly budget.
func (s syncScope) covers(repository storage.Repository) bool {
	if !repository.Available {
		return false
	}

	enabled, overridden := s.overrides[repository.ID]
	if overridden && enabled != nil && !*enabled {
		return false
	}

	state, settled := s.applied[repository.ID]
	if !settled || state.AppliedDigest != s.digestFor(repository.ID) {
		return true
	}

	// Settled, and how long ago decides whether that is still evidence. The
	// record answers what this repository looked like when it was read, and
	// nothing on GitHub stops somebody changing it by hand afterwards.
	return s.now.Sub(state.AppliedAt) >= syncRecheckInterval
}

// digestFor is what a repository would record once it matches, and what covers
// compares against. One expression, so the value written and the value tested
// cannot drift into disagreeing about whether a repository is settled.
func (s syncScope) digestFor(repositoryID string) string {
	return orgsync.DigestRepositoryKind(s.config.Digest, s.overrides[repositoryID])
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
