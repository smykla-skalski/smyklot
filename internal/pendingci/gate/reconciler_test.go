package gate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

func TestPendingCIReconcilerMergesOnlyAtObservedHead(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{}
	request := reconcilerRequest(now.Add(-time.Minute))
	reconciler := newReconciler(
		store,
		reconcilerTestObserver{observation: reconcilerObservation(now, pendingci.ObservedPassing)},
		effects,
		bot.NewCoordinator(),
		defaultTiming(),
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
	if effects.completed != "" {
		t.Fatalf("merge cleanup ran before durable terminal transition: %q", effects.completed)
	}
}

func TestPendingCIReconcilerKeepsMergeRaceArmed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{mergeErr: errors.New("head changed")}
	reconciler := newReconciler(
		store,
		reconcilerTestObserver{observation: reconcilerObservation(now, pendingci.ObservedPassing)},
		effects,
		bot.NewCoordinator(),
		defaultTiming(),
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

func TestPendingCIReconcilerCancelsWhenFinalDraftHistoryInvalidatesCommand(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{mergeErr: pendingci.ErrStaleSourceRevision}
	reconciler := newReconciler(
		store,
		reconcilerTestObserver{observation: reconcilerObservation(now, pendingci.ObservedPassing)},
		effects,
		bot.NewCoordinator(),
		defaultTiming(),
	)

	if err := reconciler.Process(t.Context(), reconcilerRequest(now.Add(-time.Minute))); err != nil {
		t.Fatal(err)
	}
	if store.finished == nil || store.finished.Lifecycle != pendingci.LifecycleCancelled ||
		store.finished.Reason != pendingci.DraftCancellationReason {
		t.Fatalf("finish = %#v, want draft cancellation", store.finished)
	}
	if store.rescheduled != nil {
		t.Fatalf("invalidated draft command was rescheduled: %#v", store.rescheduled)
	}
}

func TestPendingCIReconcilerDoesNotMergeAfterLeaseInvalidation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{claimErr: errors.New("revision changed")}
	effects := &reconcilerTestEffects{}
	reconciler := newReconciler(
		store,
		reconcilerTestObserver{observation: reconcilerObservation(now, pendingci.ObservedPassing)},
		effects,
		bot.NewCoordinator(),
		defaultTiming(),
	)

	err := reconciler.Process(context.Background(), reconcilerRequest(now.Add(-time.Minute)))
	if err == nil {
		t.Fatal("invalidated lease unexpectedly merged")
	}
	if effects.mergedHead != "" {
		t.Fatalf("invalidated lease merged head %q", effects.mergedHead)
	}
	if store.finished != nil {
		t.Fatalf("invalidated lease became terminal: %#v", store.finished)
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
	reconciler := newReconciler(
		store, reconcilerTestObserver{observation: observation},
		&reconcilerTestEffects{}, bot.NewCoordinator(), defaultTiming(),
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

func TestPendingCIReconcilerAppliesQuietPeriodChangesLive(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{}
	timing := defaultTiming()
	timing.PassingQuiet = time.Minute
	reconciler := newReconciler(
		store,
		reconcilerTestObserver{observation: reconcilerObservation(now, pendingci.ObservedPassing)},
		effects,
		bot.NewCoordinator(),
		timing,
	)
	reconciler.SetPassingQuiet(10 * time.Second)

	if err := reconciler.Process(context.Background(), reconcilerRequest(now.Add(-20*time.Second))); err != nil {
		t.Fatal(err)
	}
	if effects.mergedHead != "live-head" {
		t.Fatalf("merged head %q, want live-head after live timing change", effects.mergedHead)
	}
}

func TestPendingCIReconcilerCompletesDurableCleanup(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{}
	request := reconcilerRequest(now)
	request.Lifecycle = pendingci.LifecycleCancelled
	request.CleanupPending = true
	request.UpdatedAt = now
	reconciler := newReconciler(
		store, reconcilerTestObserver{}, effects,
		bot.NewCoordinator(), defaultTiming(),
	)

	if err := reconciler.Process(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if effects.completed != pendingci.LifecycleCancelled {
		t.Fatalf("cleanup lifecycle = %q, want cancelled", effects.completed)
	}
	if store.cleanupCompleted == nil ||
		store.cleanupCompleted.ExpectedRevision != request.Revision+1 {
		t.Fatalf("cleanup completion = %#v", store.cleanupCompleted)
	}
	if store.cleanupArtifactsMarked == nil {
		t.Fatal("cleanup did not persist artifact completion")
	}
	if effects.gateWakeCalls != 1 {
		t.Fatalf("gate wake calls = %d, want 1", effects.gateWakeCalls)
	}
}

func TestPendingCIReconcilerPersistsCleanupRetry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{completeErr: errors.New("GitHub unavailable")}
	request := reconcilerRequest(now)
	request.Lifecycle = pendingci.LifecycleCancelled
	request.CleanupPending = true
	request.UpdatedAt = now
	reconciler := newReconciler(
		store, reconcilerTestObserver{}, effects,
		bot.NewCoordinator(), defaultTiming(),
	)

	if err := reconciler.Process(context.Background(), request); err == nil {
		t.Fatal("failed cleanup unexpectedly succeeded")
	}
	if store.cleanupRetried == nil {
		t.Fatal("failed cleanup did not persist a retry")
	}
	if store.cleanupRetried.NextAttemptAt != now.Add(RetryDelay) {
		t.Fatalf("cleanup retry = %s", store.cleanupRetried.NextAttemptAt)
	}
}

func TestPendingCIReconcilerNeverRepeatsCleanedArtifacts(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{completeErr: errors.New("database unavailable")}
	effects := &reconcilerTestEffects{}
	request := reconcilerRequest(now)
	request.Lifecycle = pendingci.LifecycleCancelled
	request.CleanupPending = true
	request.CleanupArtifactsDone = true
	request.UpdatedAt = now
	reconciler := newReconciler(
		store, reconcilerTestObserver{}, effects,
		bot.NewCoordinator(), defaultTiming(),
	)

	if err := reconciler.Process(context.Background(), request); err == nil {
		t.Fatal("failed durable completion unexpectedly succeeded")
	}
	store.completeErr = nil
	if err := reconciler.Process(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if effects.cleanupCalls != 0 {
		t.Fatalf("cleaned artifacts repeated %d times", effects.cleanupCalls)
	}
}

func TestPendingCIReconcilerCoordinatesLiveObservation(t *testing.T) {
	t.Parallel()
	coordinationErr := errors.New("coordination unavailable")
	reconciler := newReconciler(
		&reconcilerTestStore{}, reconcilerTestObserver{}, &reconcilerTestEffects{},
		coordinatorStub{err: coordinationErr}, defaultTiming(),
	)

	err := reconciler.Process(
		context.Background(), reconcilerRequest(time.Now().UTC()),
	)
	if !errors.Is(err, coordinationErr) {
		t.Fatalf("observation error = %v, want coordination failure", err)
	}
}

func TestPendingCIReconcilerRestoresRetiredCheckBeforeAdvancing(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	retiredID := int64(11)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{retiredSlot: pendingci.CheckSlot{
		ID: retiredID, RepositoryID: "repository", HeadSHA: "old-head",
	}}
	request := reconcilerRequest(now)
	request.RetiredCheckSlotID = &retiredID
	reconciler := newReconciler(
		store,
		reconcilerTestObserver{observation: reconcilerObservation(now, pendingci.ObservedPending)},
		effects,
		bot.NewCoordinator(),
		defaultTiming(),
	)

	if err := reconciler.Process(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if effects.retiredRestored != 1 {
		t.Fatalf("retired check restores = %d, want 1", effects.retiredRestored)
	}
	if store.retiredCleared == nil || store.retiredCleared.CheckSlotID != retiredID {
		t.Fatalf("retired check clear = %#v", store.retiredCleared)
	}
	if store.rescheduled != nil {
		t.Fatalf("request advanced before retired cleanup: %#v", store.rescheduled)
	}
}

func TestPendingCIReconcilerKeepsReturningHeadForAtomicSwap(t *testing.T) {
	t.Parallel()
	retiredID := int64(11)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{retiredSlot: pendingci.CheckSlot{
		ID: retiredID, RepositoryID: "repository", HeadSHA: "head-a",
	}}
	request := reconcilerRequest(time.Now().UTC())
	request.RetiredCheckSlotID = &retiredID
	reconciler := newReconciler(
		store, reconcilerTestObserver{}, effects,
		bot.NewCoordinator(), defaultTiming(),
	)

	handled, err := reconciler.reconcileRetiredCheck(
		t.Context(), request,
		pendingci.Decision{Kind: pendingci.DecisionReauthorize, CandidateHeadSHA: "head-a"},
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if handled || effects.retiredRestored != 0 || store.retiredCleared != nil {
		t.Fatalf(
			"returning head was cleaned instead of swapped: handled=%t restores=%d clear=%#v",
			handled, effects.retiredRestored, store.retiredCleared,
		)
	}
}

func TestPendingCIReconcilerDoesNotBaselineReplacementCurrentCheck(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	retiredID := int64(11)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{
		retiredSlot: pendingci.CheckSlot{
			ID: retiredID, RepositoryID: "repository", HeadSHA: "old-head",
		},
		retiredCurrent: true,
	}
	request := reconcilerRequest(now)
	request.Lifecycle = pendingci.LifecycleCancelled
	request.CleanupPending = true
	request.RetiredCheckSlotID = &retiredID
	reconciler := newReconciler(
		store, reconcilerTestObserver{}, effects,
		bot.NewCoordinator(), defaultTiming(),
	)

	if err := reconciler.Process(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if effects.retiredRestored != 0 {
		t.Fatalf("replacement current check was baselined %d times", effects.retiredRestored)
	}
	if store.retiredCleared == nil || store.retiredCleared.CheckSlotID != retiredID {
		t.Fatalf("retired check clear = %#v", store.retiredCleared)
	}
}

func TestPendingCIReconcilerRevalidatesMergeExclusively(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &reconcilerTestStore{}
	effects := &reconcilerTestEffects{}
	observer := &reconcilerSequenceObserver{observations: []pendingci.Observation{
		reconcilerObservation(now, pendingci.ObservedPassing),
		reconcilerObservation(now.Add(time.Second), pendingci.ObservedFailing),
	}}
	request := reconcilerRequest(now.Add(-time.Minute))
	request.RepositoryID = "repository"
	reconciler := newReconciler(
		store, observer, effects, bot.NewCoordinator(), defaultTiming(),
	)

	if err := reconciler.Process(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if observer.calls != 2 {
		t.Fatalf("observation calls = %d, want initial and exclusive revalidation", observer.calls)
	}
	if effects.mergedHead != "" {
		t.Fatalf("stale passing observation merged head %q", effects.mergedHead)
	}
	if store.rescheduled == nil || store.rescheduled.LastObservedState != string(pendingci.ObservedFailing) {
		t.Fatalf("reschedule = %#v, want revalidated failing state", store.rescheduled)
	}
}

func reconcilerRequest(progressAt time.Time) pendingci.Request {
	return pendingci.Request{
		ID: 7, Revision: 2, Lifecycle: pendingci.LifecycleArmed,
		RepositoryID: "repository",
		HeadSHA:      "live-head", BaseBranch: "main", LastProgressAt: progressAt,
		LastObservedState: string(pendingci.ObservedPassing),
		LastFingerprint:   "passing",
	}
}

func reconcilerObservation(at time.Time, state pendingci.ObservedState) pendingci.Observation {
	return pendingci.Observation{
		HeadSHA: "live-head", BaseBranch: "main",
		PullRequestOpen: true, PendingLabelFound: true,
		State: state, Fingerprint: "passing", ObservedAt: at,
	}
}

type reconcilerTestObserver struct {
	observation pendingci.Observation
}

type reconcilerSequenceObserver struct {
	observations []pendingci.Observation
	calls        int
}

func (observer *reconcilerSequenceObserver) Observe(
	context.Context,
	pendingci.Request,
) (pendingci.Observation, error) {
	if observer.calls >= len(observer.observations) {
		return pendingci.Observation{}, errors.New("unexpected observation")
	}
	observation := observer.observations[observer.calls]
	observer.calls++

	return observation, nil
}

func (observer reconcilerTestObserver) Observe(
	context.Context,
	pendingci.Request,
) (pendingci.Observation, error) {
	return observer.observation, nil
}

type reconcilerTestStore struct {
	rescheduled            *pendingci.RescheduleRequest
	finished               *pendingci.FinishRequest
	cleanupArtifactsMarked *pendingci.MarkCleanupArtifactsDoneRequest
	cleanupCompleted       *pendingci.CompleteCleanupRequest
	cleanupRetried         *pendingci.RetryCleanupRequest
	retiredCleared         *pendingci.ClearRetiredCheckSlotRequest
	claimErr               error
	completeErr            error
}

func (store *reconcilerTestStore) ClearRetiredCheckSlot(
	_ context.Context,
	request pendingci.ClearRetiredCheckSlotRequest,
) (pendingci.Request, error) {
	store.retiredCleared = &request

	return pendingci.Request{ID: request.ID, Revision: request.ExpectedRevision + 1}, nil
}

func (store *reconcilerTestStore) ClaimMerge(
	_ context.Context,
	request pendingci.ClaimMergeRequest,
) (pendingci.Request, error) {
	if store.claimErr != nil {
		return pendingci.Request{}, store.claimErr
	}

	return pendingci.Request{
		ID: request.ID, Revision: request.ExpectedRevision + 1,
		Lifecycle: pendingci.LifecycleArmed,
	}, nil
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

func (store *reconcilerTestStore) CompleteCleanup(
	_ context.Context,
	request pendingci.CompleteCleanupRequest,
) (pendingci.Request, error) {
	store.cleanupCompleted = &request

	return pendingci.Request{}, store.completeErr
}

func (store *reconcilerTestStore) MarkCleanupArtifactsDone(
	_ context.Context,
	request pendingci.MarkCleanupArtifactsDoneRequest,
) (pendingci.Request, error) {
	store.cleanupArtifactsMarked = &request

	return pendingci.Request{
		ID: request.ID, Revision: request.ExpectedRevision + 1,
		CleanupPending: true, CleanupArtifactsDone: true, UpdatedAt: request.MarkedAt,
	}, nil
}

func (store *reconcilerTestStore) RetryCleanup(
	_ context.Context,
	request pendingci.RetryCleanupRequest,
) (pendingci.Request, error) {
	store.cleanupRetried = &request

	return pendingci.Request{}, nil
}

type reconcilerTestEffects struct {
	mergedHead      string
	mergeErr        error
	completed       pendingci.Lifecycle
	completeErr     error
	cleanupCalls    int
	retiredSlot     pendingci.CheckSlot
	retiredRestored int
	retiredErr      error
	retiredCurrent  bool
	gateWakeCalls   int
}

func (effects *reconcilerTestEffects) WakePendingCIGates() {
	effects.gateWakeCalls++
}

func (effects *reconcilerTestEffects) GetPendingCICheckSlot(
	context.Context,
	int64,
) (pendingci.CheckSlot, error) {
	return effects.retiredSlot, nil
}

func (effects *reconcilerTestEffects) RestoreRetiredPendingCICheck(
	context.Context,
	pendingci.CheckSlot,
) error {
	effects.retiredRestored++

	return effects.retiredErr
}

func (effects *reconcilerTestEffects) PendingCICheckSlotIsCurrent(
	context.Context,
	pendingci.Request,
	pendingci.CheckSlot,
) (bool, error) {
	return effects.retiredCurrent, nil
}

func (effects *reconcilerTestEffects) MergeAtHead(
	_ context.Context,
	_ pendingci.Request,
	headSHA string,
) error {
	effects.mergedHead = headSHA

	return effects.mergeErr
}

func (effects *reconcilerTestEffects) CleanupArtifacts(
	_ context.Context,
	_ pendingci.Request,
	lifecycle pendingci.Lifecycle,
) error {
	effects.cleanupCalls++
	effects.completed = lifecycle

	return effects.completeErr
}
