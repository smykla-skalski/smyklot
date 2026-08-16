package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

	labels, found := syncConfigOf(configs, orgsync.KindLabels)
	if !found || !labels.Enabled {
		// Nothing is switched on, so there is nothing to compare against.
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

	actions, err := s.planSyncActions(ctx, client, targetID, labels, overrides)
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
		ActorID:   labels.UpdatedBy,
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

// planSyncActions asks each repository in scope what it would take to match.
func (s *server) planSyncActions(
	ctx context.Context,
	client *github.Client,
	targetID string,
	labels orgsync.Config,
	overrides []orgsync.RepositoryOverride,
) ([]orgsync.Action, error) {
	var config orgsync.LabelConfig
	if err := json.Unmarshal(labels.Document, &config); err != nil {
		// A stored configuration that will not decode is not a reason to plan
		// nothing quietly: it is a reason to say so and change nothing.
		return nil, fmt.Errorf("decode label configuration: %w", err)
	}

	repositories, err := s.store.ListRepositories(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("read sync repositories: %w", err)
	}

	applied, err := s.store.ListSyncRepositoryState(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("read sync repository state: %w", err)
	}

	scope := newSyncScope(labels, overrides, applied)

	var (
		actions []orgsync.Action
		matched []orgsync.RepositoryState
		now     = time.Now().UTC()
	)

	for _, repository := range repositories {
		if !scope.covers(repository) {
			continue
		}

		owner, name := splitFullName(repository.FullName)

		current, err := client.ListRepositoryLabels(ctx, owner, name)
		if err != nil {
			// One repository refusing must not stop the rest. It will be
			// planned again on the next tick, and reporting a plan that
			// silently omitted it would be worse than a shorter one.
			logging.From(ctx).Warn("could not read labels while planning",
				"repo", repository.FullName, "error", err)

			continue
		}

		found := orgsync.PlanLabels(
			repository.ID, config, asCurrentLabels(current), config.Exclusions(),
		)
		if len(found) == 0 {
			// Nothing to do, which is a fact worth keeping. It appears in no
			// plan, so an apply would never record it, and without a record
			// this repository is read from GitHub again on every tick for ever
			// - the cost the recorded digest exists to remove.
			matched = append(matched, orgsync.RepositoryState{
				RepositoryID:  repository.ID,
				Kind:          orgsync.KindLabels,
				AppliedDigest: scope.digestFor(repository.ID),
				AppliedAt:     now,
			})

			continue
		}

		actions = append(actions, found...)
	}

	if err := s.store.RecordSyncRepositoryState(ctx, matched); err != nil {
		return nil, err
	}

	return actions, nil
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

// syncScope answers which repositories a plan covers.
type syncScope struct {
	config    orgsync.Config
	overrides map[string]*bool
	applied   map[string]string
}

func newSyncScope(
	config orgsync.Config,
	overrides []orgsync.RepositoryOverride,
	applied []orgsync.RepositoryState,
) syncScope {
	scope := syncScope{
		config:    config,
		overrides: map[string]*bool{},
		applied:   map[string]string{},
	}

	for _, override := range overrides {
		if override.Kind == orgsync.KindLabels {
			scope.overrides[override.RepositoryID] = override.Enabled
		}
	}
	for _, state := range applied {
		if state.Kind == orgsync.KindLabels {
			scope.applied[state.RepositoryID] = state.AppliedDigest
		}
	}

	return scope
}

// covers reports a repository worth asking GitHub about.
//
// Two reasons to skip. A repository that turned this off, and a repository
// whose recorded digest already matches what the configuration asks for - the
// second is what keeps a steady-state reconcile at zero API calls rather than
// one per repository, which is the difference between a sweep that costs
// nothing and one that spends an installation's whole hourly budget.
func (s syncScope) covers(repository storage.Repository) bool {
	if !repository.Available {
		return false
	}

	enabled, overridden := s.overrides[repository.ID]
	if overridden && enabled != nil && !*enabled {
		return false
	}

	return s.applied[repository.ID] != s.digestFor(repository.ID)
}

// digestFor is what a repository would record once it matches, and what covers
// compares against. One expression, so the value written and the value tested
// cannot drift into disagreeing about whether a repository is settled.
func (s syncScope) digestFor(repositoryID string) string {
	return orgsync.DigestRepositoryKind(s.config.Digest, s.overrides[repositoryID])
}

func syncConfigOf(configs []orgsync.Config, kind orgsync.Kind) (orgsync.Config, bool) {
	for _, config := range configs {
		if config.Kind == kind {
			return config, true
		}
	}

	return orgsync.Config{}, false
}

func asCurrentLabels(labels []github.RepositoryLabel) []orgsync.CurrentLabel {
	current := make([]orgsync.CurrentLabel, 0, len(labels))
	for _, label := range labels {
		current = append(current, orgsync.CurrentLabel(label))
	}

	return current
}
