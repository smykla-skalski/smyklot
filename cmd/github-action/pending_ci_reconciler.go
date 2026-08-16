package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

type pendingCITransitionStore interface {
	ClaimMerge(context.Context, pendingci.ClaimMergeRequest) (pendingci.Request, error)
	Reschedule(context.Context, pendingci.RescheduleRequest) (pendingci.Request, error)
	Finish(context.Context, pendingci.FinishRequest) (pendingci.Request, error)
	MarkCleanupArtifactsDone(
		context.Context,
		pendingci.MarkCleanupArtifactsDoneRequest,
	) (pendingci.Request, error)
	CompleteCleanup(context.Context, pendingci.CompleteCleanupRequest) (pendingci.Request, error)
	RetryCleanup(context.Context, pendingci.RetryCleanupRequest) (pendingci.Request, error)
}

type pendingCIObserver interface {
	Observe(context.Context, pendingci.Request) (pendingci.Observation, error)
}

type pendingCIEffects interface {
	MergeAtHead(context.Context, pendingci.Request, string) error
	CleanupArtifacts(context.Context, pendingci.Request, pendingci.Lifecycle) error
}

// pendingCIReconciler combines live truth with the pure policy, then applies
// one optimistic durable transition. GitHub access stays behind narrow ports.
type pendingCIReconciler struct {
	store     pendingCITransitionStore
	observer  pendingCIObserver
	effects   pendingCIEffects
	exclusive pendingCIExclusive
	timingMu  sync.RWMutex
	timing    pendingci.Timing
}

func newPendingCIReconciler(
	store pendingCITransitionStore,
	observer pendingCIObserver,
	effects pendingCIEffects,
	exclusive pendingCIExclusive,
	timing pendingci.Timing,
) *pendingCIReconciler {
	return &pendingCIReconciler{
		store: store, observer: observer, effects: effects,
		exclusive: exclusive, timing: timing,
	}
}

func defaultPendingCITiming() pendingci.Timing {
	return pendingci.Timing{
		ActiveInterval: 5 * time.Minute, DiscoveryGrace: 10 * time.Minute,
		DeferAfter: time.Hour, DeferredInterval: 6 * time.Hour,
		PassingQuiet: pendingci.DefaultPassingQuiet,
	}
}

func (reconciler *pendingCIReconciler) SetPassingQuiet(value time.Duration) bool {
	reconciler.timingMu.Lock()
	defer reconciler.timingMu.Unlock()
	if reconciler.timing.PassingQuiet == value {
		return false
	}
	reconciler.timing.PassingQuiet = value

	return true
}

func (reconciler *pendingCIReconciler) currentTiming() pendingci.Timing {
	reconciler.timingMu.RLock()
	defer reconciler.timingMu.RUnlock()

	return reconciler.timing
}

func (reconciler *pendingCIReconciler) Process(
	ctx context.Context,
	request pendingci.Request,
) error {
	if request.CleanupPending {
		return reconciler.cleanup(ctx, request)
	}

	return reconciler.exclusive.Exclusive(ctx, request.RepositoryID, func() error {
		return reconciler.processArmedExclusive(ctx, request)
	})
}

func (reconciler *pendingCIReconciler) processArmedExclusive(
	ctx context.Context,
	request pendingci.Request,
) error {
	observation, err := reconciler.observer.Observe(ctx, request)
	if err != nil {
		return fmt.Errorf("observe live GitHub state: %w", err)
	}
	decision, err := pendingci.Decide(request, observation, reconciler.currentTiming())
	if err != nil {
		return fmt.Errorf("decide pending CI transition: %w", err)
	}

	switch decision.Kind {
	case pendingci.DecisionReschedule:
		return reconciler.reschedule(ctx, request, decision, observation)
	case pendingci.DecisionFinish:
		return reconciler.finish(ctx, request, decision.Lifecycle, decision.Reason, observation.ObservedAt)
	case pendingci.DecisionMerge:
		return reconciler.mergeExclusive(ctx, request)
	default:
		return fmt.Errorf("unsupported pending CI decision %q", decision.Kind)
	}
}

func (reconciler *pendingCIReconciler) mergeExclusive(
	ctx context.Context,
	request pendingci.Request,
) error {
	observation, err := reconciler.observer.Observe(ctx, request)
	if err != nil {
		return fmt.Errorf("revalidate live GitHub state: %w", err)
	}
	decision, err := pendingci.Decide(request, observation, reconciler.currentTiming())
	if err != nil {
		return fmt.Errorf("revalidate pending CI transition: %w", err)
	}
	if decision.Kind == pendingci.DecisionReschedule {
		return reconciler.reschedule(ctx, request, decision, observation)
	}
	if decision.Kind == pendingci.DecisionFinish {
		return reconciler.finish(
			ctx, request, decision.Lifecycle, decision.Reason, observation.ObservedAt,
		)
	}
	if decision.Kind != pendingci.DecisionMerge {
		return fmt.Errorf("unsupported revalidated pending CI decision %q", decision.Kind)
	}

	claimed, err := reconciler.store.ClaimMerge(ctx, pendingci.ClaimMergeRequest{
		ID: request.ID, ExpectedRevision: request.Revision,
		Observation: observation, ClaimedAt: observation.ObservedAt,
	})
	if err != nil {
		return fmt.Errorf("claim pending CI merge: %w", err)
	}

	return reconciler.merge(ctx, claimed, observation)
}

func (reconciler *pendingCIReconciler) reschedule(
	ctx context.Context,
	request pendingci.Request,
	decision pendingci.Decision,
	observation pendingci.Observation,
) error {
	_, err := reconciler.store.Reschedule(ctx, pendingci.RescheduleRequest{
		ID: request.ID, ExpectedRevision: request.Revision,
		Schedule: decision.Schedule, HeadSHA: decision.HeadSHA,
		NextCheckAt: decision.NextCheckAt, NextCheckTrigger: decision.NextCheckTrigger,
		LastProgressAt:    decision.LastProgressAt,
		LastObservedState: decision.LastObservedState,
		LastFingerprint:   decision.LastFingerprint, ObservationSummary: observation.Summary,
		CheckedAt: observation.ObservedAt,
	})

	return err
}

func (reconciler *pendingCIReconciler) finish(
	ctx context.Context,
	request pendingci.Request,
	lifecycle pendingci.Lifecycle,
	reason string,
	finishedAt time.Time,
) error {
	_, err := reconciler.store.Finish(ctx, pendingci.FinishRequest{
		ID: request.ID, ExpectedRevision: request.Revision,
		Lifecycle: lifecycle, Trigger: request.NextCheckTrigger,
		Reason: reason, FinishedAt: finishedAt,
	})
	if err != nil {
		return err
	}
	return nil
}

func (reconciler *pendingCIReconciler) cleanup(
	ctx context.Context,
	request pendingci.Request,
) error {
	return reconciler.exclusive.Exclusive(ctx, request.RepositoryID, func() error {
		return reconciler.cleanupExclusive(ctx, request)
	})
}

func (reconciler *pendingCIReconciler) cleanupExclusive(
	ctx context.Context,
	request pendingci.Request,
) error {
	current := request
	if !current.CleanupArtifactsDone {
		if err := reconciler.effects.CleanupArtifacts(ctx, current, current.Lifecycle); err != nil {
			return reconciler.retryCleanup(ctx, current, err)
		}
		marked, err := reconciler.store.MarkCleanupArtifactsDone(
			ctx,
			pendingci.MarkCleanupArtifactsDoneRequest{
				ID: current.ID, ExpectedRevision: current.Revision,
				MarkedAt: current.UpdatedAt,
			},
		)
		if err != nil {
			return fmt.Errorf("mark pending CI artifacts cleaned: %w", err)
		}
		current = marked
	}
	_, err := reconciler.store.CompleteCleanup(ctx, pendingci.CompleteCleanupRequest{
		ID: current.ID, ExpectedRevision: current.Revision,
		CompletedAt: current.UpdatedAt,
	})

	return err
}

func (reconciler *pendingCIReconciler) retryCleanup(
	ctx context.Context,
	request pendingci.Request,
	cause error,
) error {
	_, retryErr := reconciler.store.RetryCleanup(ctx, pendingci.RetryCleanupRequest{
		ID: request.ID, ExpectedRevision: request.Revision,
		NextAttemptAt: request.UpdatedAt.Add(pendingCIRetryDelay),
		FailedAt:      request.UpdatedAt, Error: cause.Error(),
	})
	if retryErr != nil {
		return fmt.Errorf("cleanup failed: %v; schedule retry: %w", cause, retryErr)
	}

	return fmt.Errorf("cleanup failed and will retry: %w", cause)
}

func (reconciler *pendingCIReconciler) merge(
	ctx context.Context,
	request pendingci.Request,
	observation pendingci.Observation,
) error {
	if err := reconciler.effects.MergeAtHead(ctx, request, observation.HeadSHA); err != nil {
		decision := pendingci.Decision{
			Schedule: pendingci.ScheduleActive, HeadSHA: observation.HeadSHA,
			NextCheckAt:       observation.ObservedAt.Add(reconciler.timing.ActiveInterval),
			NextCheckTrigger:  pendingci.TriggerFallback,
			LastProgressAt:    observation.ObservedAt,
			LastObservedState: string(observation.State),
			LastFingerprint:   observation.Fingerprint,
		}
		if scheduleErr := reconciler.reschedule(ctx, request, decision, observation); scheduleErr != nil {
			return fmt.Errorf("merge failed: %v; reschedule failed: %w", err, scheduleErr)
		}

		return fmt.Errorf("merge failed and remains armed: %w", err)
	}

	return reconciler.finish(
		ctx, request, pendingci.LifecycleMerged,
		"CI passed and pull request merged", observation.ObservedAt,
	)
}
