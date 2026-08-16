package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
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

// applySyncPlans applies whatever plan is due, one per call.
//
// One, because the database holds one live plan per installation and the work
// is almost entirely waiting on GitHub. Draining a queue here would let one
// installation's large plan delay every other installation's small one.
func (s *server) applySyncPlans(ctx context.Context) error {
	now := time.Now().UTC()

	lease, err := s.store.LeaseSyncPlan(ctx, now, now.Add(syncLease))
	if err != nil {
		return fmt.Errorf("lease sync plan: %w", err)
	}
	if !lease.Found {
		return nil
	}

	ctx = logging.With(ctx, "sync_plan", lease.Plan.ID, "target", lease.Plan.TargetID)

	outcome, err := s.applySyncPlan(ctx, lease)
	if err != nil {
		// The plan keeps its lease and is offered again when that runs out.
		// Closing it here would record a verdict on work that was never tried.
		return err
	}

	if err := s.store.FinishSyncPlan(ctx, orgsync.PlanOutcome{
		PlanID: lease.Plan.ID,
		State:  outcome.State(),
		Now:    time.Now().UTC(),

		Applied: outcome.Applied,
	}); err != nil {
		return fmt.Errorf("finish sync plan: %w", err)
	}

	logging.From(ctx).Info("sync plan applied",
		"state", outcome.State(), "actions", len(outcome.Actions))

	return nil
}

// applySyncPlan drives one plan's work through GitHub.
func (s *server) applySyncPlan(
	ctx context.Context,
	lease orgsync.PlanLease,
) (orgsync.Outcome, error) {
	target, err := s.store.GetTarget(ctx, lease.Plan.TargetID)
	if err != nil {
		return orgsync.Outcome{}, fmt.Errorf("read sync installation: %w", err)
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
		repository, err := s.store.GetRepository(ctx, lease.Plan.TargetID, work.RepositoryID)
		if err != nil {
			// A repository the catalog no longer has is not a failure of the
			// plan, but its actions cannot run and must not stay pending.
			s.abandonRepositoryWork(&outcome, work, "repository is no longer available")

			continue
		}

		s.applyRepositoryWork(ctx, client, repository, work, digests, &outcome)
	}

	return outcome, nil
}

// abandonRepositoryWork records every action for a repository that cannot be
// reached, so nothing is left pending for a later lease to retry for ever.
func (s *server) abandonRepositoryWork(
	outcome *orgsync.Outcome,
	work orgsync.RepositoryWork,
	reason string,
) {
	for _, kind := range work.Kinds {
		for _, action := range kind.Actions {
			outcome.Fail(action, reason)
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
func (s *server) applyRepositoryWork(
	ctx context.Context,
	client *github.Client,
	repository storage.Repository,
	work orgsync.RepositoryWork,
	digests syncDigestIndex,
	outcome *orgsync.Outcome,
) {
	owner, name := splitFullName(repository.FullName)
	ctx = logging.With(ctx, "repo", repository.FullName)

	var blocker orgsync.Kind

	for _, kind := range work.Kinds {
		if blocker != "" {
			for _, action := range kind.Actions {
				outcome.Skip(action, blocker)
			}

			continue
		}

		applied := s.applyKind(ctx, client, owner, name, kind, outcome)
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
			AppliedDigest: digests.of(repository.ID, kind.Kind),
			AppliedAt:     time.Now().UTC(),
		})
	}
}

// applyKind runs one kind's actions and reports whether every one succeeded.
func (s *server) applyKind(
	ctx context.Context,
	client *github.Client,
	owner, name string,
	work orgsync.KindWork,
	outcome *orgsync.Outcome,
) bool {
	succeeded := true

	for _, action := range work.Actions {
		err := s.applyAction(ctx, client, owner, name, action)
		if err != nil {
			logging.From(ctx).Warn("sync action failed",
				"kind", action.Kind, "operation", action.Operation,
				"subject", action.Subject, "error", err)
			outcome.Fail(action, err.Error())
			succeeded = false

			continue
		}

		outcome.Apply(action)
	}

	return succeeded
}

// applyAction performs one action against GitHub.
func (s *server) applyAction(
	ctx context.Context,
	client *github.Client,
	owner, name string,
	action orgsync.Action,
) error {
	if action.Kind != orgsync.KindLabels {
		// Settings, rulesets and files arrive in later work. Refusing loudly
		// beats silently reporting an action applied that nothing performed.
		return fmt.Errorf("%w: %s", errSyncKindUnsupported, action.Kind)
	}

	return s.applyLabelAction(ctx, client, owner, name, action)
}

var errSyncKindUnsupported = errors.New("this Smyklot cannot apply that kind of sync yet")

// syncDigestIndex answers what a repository and kind should record once its
// work lands.
type syncDigestIndex struct {
	configs   map[orgsync.Kind]string
	overrides map[string]map[orgsync.Kind]*bool
}

func (i syncDigestIndex) of(repositoryID string, kind orgsync.Kind) string {
	return orgsync.DigestRepositoryKind(i.configs[kind], i.overrides[repositoryID][kind])
}

// syncDigests reads what an installation has configured, once per plan rather
// than once per repository.
func (s *server) syncDigests(ctx context.Context, targetID string) (syncDigestIndex, error) {
	configs, err := s.store.ListSyncConfigs(ctx, targetID)
	if err != nil {
		return syncDigestIndex{}, fmt.Errorf("read sync configuration: %w", err)
	}

	overrides, err := s.store.ListSyncRepositoryOverrides(ctx, targetID)
	if err != nil {
		return syncDigestIndex{}, fmt.Errorf("read sync overrides: %w", err)
	}

	index := syncDigestIndex{
		configs:   make(map[orgsync.Kind]string, len(configs)),
		overrides: map[string]map[orgsync.Kind]*bool{},
	}

	for _, config := range configs {
		index.configs[config.Kind] = config.Digest
	}
	for _, override := range overrides {
		if index.overrides[override.RepositoryID] == nil {
			index.overrides[override.RepositoryID] = map[orgsync.Kind]*bool{}
		}
		index.overrides[override.RepositoryID][override.Kind] = override.Enabled
	}

	return index, nil
}

// installationClient mints a client for one installation.
func (s *server) installationClient(installationID string) (*github.Client, error) {
	id, err := strconv.ParseInt(installationID, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("%w: installation id %q", ErrGitHubClient, installationID)
	}

	token, err := s.tokens.InstallationToken(id)
	if err != nil {
		return nil, NewGitHubError(ErrGitHubAppAuth, err)
	}

	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return nil, NewGitHubError(ErrGitHubClient, err)
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
