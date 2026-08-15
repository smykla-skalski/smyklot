package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

func TestPendingCIReconcilerMergesOnlyAtObservedHead(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{}
	request := reconcilerRequest(now.Add(-time.Minute))
	reconciler := newPendingCIReconciler(
		store,
		reconcilerTestObserver{observation: reconcilerObservation(now, pendingci.ObservedPassing)},
		effects,
		defaultPendingCITiming(),
	)

	if err := reconciler.Process(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if effects.mergedHead != "live-head" {
		t.Fatalf("merged head %q, want live-head", effects.mergedHead)
	}
	if store.finished == nil || store.finished.Lifecycle != pendingci.LifecycleMerged {
		t.Fatalf("finish = %#v, want merged", store.finished)
	}
	if effects.completed != pendingci.LifecycleMerged {
		t.Fatalf("completion = %q, want merged", effects.completed)
	}
}

func TestPendingCIReconcilerKeepsMergeRaceArmed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{mergeErr: errors.New("head changed")}
	reconciler := newPendingCIReconciler(
		store,
		reconcilerTestObserver{observation: reconcilerObservation(now, pendingci.ObservedPassing)},
		effects,
		defaultPendingCITiming(),
	)

	err := reconciler.Process(context.Background(), reconcilerRequest(now.Add(-time.Minute)))
	if err == nil {
		t.Fatal("merge race unexpectedly succeeded")
	}
	if store.rescheduled == nil || store.rescheduled.Schedule != pendingci.ScheduleActive {
		t.Fatalf("reschedule = %#v, want active", store.rescheduled)
	}
	if store.finished != nil {
		t.Fatalf("merge race became terminal: %#v", store.finished)
	}
}

func TestPendingCIReconcilerDefersUnchangedFailure(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{}
	request := reconcilerRequest(now.Add(-2 * time.Hour))
	request.LastObservedState = string(pendingci.ObservedFailing)
	request.LastFingerprint = "failing"
	observation := reconcilerObservation(now, pendingci.ObservedFailing)
	observation.Fingerprint = "failing"
	reconciler := newPendingCIReconciler(
		store, reconcilerTestObserver{observation: observation},
		&reconcilerTestEffects{}, defaultPendingCITiming(),
	)

	if err := reconciler.Process(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if store.rescheduled == nil || store.rescheduled.Schedule != pendingci.ScheduleDeferred {
		t.Fatalf("reschedule = %#v, want deferred", store.rescheduled)
	}
	if store.rescheduled.NextCheckAt != now.Add(6*time.Hour) {
		t.Fatalf("next check = %s, want six-hour fallback", store.rescheduled.NextCheckAt)
	}
}

func reconcilerRequest(progressAt time.Time) pendingci.Request {
	return pendingci.Request{
		ID: 7, Revision: 2, Lifecycle: pendingci.LifecycleArmed,
		HeadSHA: "live-head", LastProgressAt: progressAt,
		LastObservedState: string(pendingci.ObservedPassing),
		LastFingerprint:   "passing",
	}
}

func reconcilerObservation(at time.Time, state pendingci.ObservedState) pendingci.Observation {
	return pendingci.Observation{
		HeadSHA: "live-head", PullRequestOpen: true, PendingLabelFound: true,
		State: state, Fingerprint: "passing", ObservedAt: at,
	}
}

type reconcilerTestObserver struct {
	observation pendingci.Observation
}

func (observer reconcilerTestObserver) Observe(
	context.Context,
	pendingci.Request,
) (pendingci.Observation, error) {
	return observer.observation, nil
}

type reconcilerTestStore struct {
	rescheduled *pendingci.RescheduleRequest
	finished    *pendingci.FinishRequest
}

func (store *reconcilerTestStore) Reschedule(
	_ context.Context,
	request pendingci.RescheduleRequest,
) (pendingci.Request, error) {
	store.rescheduled = &request

	return pendingci.Request{}, nil
}

func (store *reconcilerTestStore) Finish(
	_ context.Context,
	request pendingci.FinishRequest,
) (pendingci.Request, error) {
	store.finished = &request

	return pendingci.Request{Lifecycle: request.Lifecycle}, nil
}

type reconcilerTestEffects struct {
	mergedHead string
	mergeErr   error
	completed  pendingci.Lifecycle
}

func (effects *reconcilerTestEffects) MergeAtHead(
	_ context.Context,
	_ pendingci.Request,
	headSHA string,
) error {
	effects.mergedHead = headSHA

	return effects.mergeErr
}

func (effects *reconcilerTestEffects) Complete(
	_ context.Context,
	_ pendingci.Request,
	lifecycle pendingci.Lifecycle,
) {
	effects.completed = lifecycle
}
