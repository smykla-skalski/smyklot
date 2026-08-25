package apply

import (
	"context"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type syncExecutionStore struct {
	Store
	leaseCalls  int
	target      storage.Target
	repository  storage.Repository
	actionNotes []orgsync.ActionOutcome
}

func (store *syncExecutionStore) LeaseSyncPlan(
	context.Context,
	time.Time,
	time.Time,
) (orgsync.PlanLease, error) {
	store.leaseCalls++

	return orgsync.PlanLease{}, nil
}

func (store *syncExecutionStore) GetTarget(
	context.Context,
	string,
) (storage.Target, error) {
	return store.target, nil
}

func (store *syncExecutionStore) GetRepository(
	context.Context,
	string,
	string,
) (storage.Repository, error) {
	return store.repository, nil
}

func (store *syncExecutionStore) RecordSyncActionOutcome(
	_ context.Context,
	outcome orgsync.ActionOutcome,
) error {
	store.actionNotes = append(store.actionNotes, outcome)

	return nil
}

type switchingCoordinator struct{ before func() }

func (coordinator switchingCoordinator) Exclusive(
	_ context.Context,
	_ string,
	operation func() error,
) error {
	coordinator.before()

	return operation()
}

// TestUnavailableForTargetIgnoresWorkAlreadyDone keeps a plan finishable after
// a permission is withdrawn part-way through it.
//
// A lease can expire with some of a plan applied, and the moment a workflow
// proposal lands is exactly when somebody might take Workflows access back.
// Holding the re-leased plan on an action that already applied leaves the rest
// of it - other repositories, other kinds - unrun for ever: the plan stays
// `applying`, which fills the installation's one live slot, and nothing expires
// a plan in that state.
func TestUnavailableForTargetIgnoresWorkAlreadyDone(t *testing.T) {
	target := storage.Target{Permissions: map[string]string{
		"issues": "write", "contents": "write",
	}}

	done := []orgsync.Action{
		{
			Kind: orgsync.KindFiles, Subject: ".github/workflows/ci.yaml",
			State: orgsync.ActionApplied,
		},
		{Kind: orgsync.KindLabels, Subject: "bug", State: orgsync.ActionPending},
	}

	if unavailable, missing := unavailableForTarget(target, done); missing {
		t.Errorf("held the plan on %s, which had already applied", unavailable.Permission)
	}

	// And the same action still pending is still refused, because that is work
	// this attempt would go on to do.
	pending := []orgsync.Action{{
		Kind: orgsync.KindFiles, Subject: ".github/workflows/ci.yaml",
		State: orgsync.ActionPending,
	}}

	unavailable, missing := unavailableForTarget(target, pending)
	if !missing {
		t.Fatal("a workflow nobody permitted was allowed through")
	}
	if unavailable.Permission != "workflows" {
		t.Errorf("permission = %q, wanted workflows", unavailable.Permission)
	}
}

func TestSyncPlanLeaseRespectsBackgroundPauseFence(t *testing.T) {
	store := &syncExecutionStore{}
	engine := New(store, nil, "")
	engine.SetBeginWork(func() (func(), bool) { return nil, false })

	claimed, err := engine.ApplyOnePlan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if claimed || store.leaseCalls != 0 {
		t.Fatalf("claimed = %t, lease calls = %d while paused", claimed, store.leaseCalls)
	}
}

func TestSyncExecutionRechecksEnablementInsideCoordinator(t *testing.T) {
	enabled := false
	store := &syncExecutionStore{
		target: storage.Target{
			ID: "github:installation:10", Available: true,
			RepositoryDefaultEnabled: true,
		},
		repository: storage.Repository{
			ID: "repository-20", TargetID: "github:installation:10", Available: true,
		},
	}
	engine := New(store, nil, "")
	engine.SetCoordinator(switchingCoordinator{before: func() {
		store.repository.EnabledOverride = &enabled
	}})
	work := orgsync.RepositoryWork{
		RepositoryID: "repository-20",
		Kinds: []orgsync.KindWork{{
			Kind: orgsync.KindLabels,
			Actions: []orgsync.Action{{
				ID: 1, RepositoryID: "repository-20", Kind: orgsync.KindLabels,
				State: orgsync.ActionPending,
			}},
		}},
	}
	var outcome orgsync.Outcome
	applied := false
	err := engine.applyRepositoryIfEnabled(
		t.Context(), store.target.ID, work, &outcome,
		func(storage.Repository) error {
			applied = true

			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if applied || outcome.Failed != 1 || len(store.actionNotes) != 1 {
		t.Fatalf(
			"applied = %t, outcome = %#v, action notes = %#v",
			applied, outcome, store.actionNotes,
		)
	}
}
