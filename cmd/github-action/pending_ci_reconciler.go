package main

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

type pendingCITransitionStore interface {
	Reschedule(context.Context, pendingci.RescheduleRequest) (pendingci.Request, error)
	Finish(context.Context, pendingci.FinishRequest) (pendingci.Request, error)
}

type pendingCIObserver interface {
	Observe(context.Context, pendingci.Request) (pendingci.Observation, error)
}

type pendingCIEffects interface {
	MergeAtHead(context.Context, pendingci.Request, string) error
	Complete(context.Context, pendingci.Request, pendingci.Lifecycle)
}

// pendingCIReconciler combines live truth with the pure policy, then applies
// one optimistic durable transition. GitHub access stays behind narrow ports.
type pendingCIReconciler struct {
	store    pendingCITransitionStore
	observer pendingCIObserver
	effects  pendingCIEffects
	timing   pendingci.Timing
}

func newPendingCIReconciler(
	store pendingCITransitionStore,
	observer pendingCIObserver,
	effects pendingCIEffects,
	timing pendingci.Timing,
) *pendingCIReconciler {
	return &pendingCIReconciler{
		store: store, observer: observer, effects: effects, timing: timing,
	}
}

func defaultPendingCITiming() pendingci.Timing {
	return pendingci.Timing{
		ActiveInterval: 5 * time.Minute, DiscoveryGrace: 10 * time.Minute,
		DeferAfter: time.Hour, DeferredInterval: 6 * time.Hour,
		PassingQuiet: 30 * time.Second,
	}
}

func (reconciler *pendingCIReconciler) Process(
	ctx context.Context,
	request pendingci.Request,
) error {
	observation, err := reconciler.observer.Observe(ctx, request)
	if err != nil {
		return fmt.Errorf("observe live GitHub state: %w", err)
	}
	decision, err := pendingci.Decide(request, observation, reconciler.timing)
	if err != nil {
		return fmt.Errorf("decide pending CI transition: %w", err)
	}

	switch decision.Kind {
	case pendingci.DecisionReschedule:
		return reconciler.reschedule(ctx, request, decision, observation.ObservedAt)
	case pendingci.DecisionFinish:
		return reconciler.finish(ctx, request, decision.Lifecycle, decision.Reason, observation.ObservedAt)
	case pendingci.DecisionMerge:
		return reconciler.merge(ctx, request, observation)
	default:
		return fmt.Errorf("unsupported pending CI decision %q", decision.Kind)
	}
}

func (reconciler *pendingCIReconciler) reschedule(
	ctx context.Context,
	request pendingci.Request,
	decision pendingci.Decision,
	checkedAt time.Time,
) error {
	_, err := reconciler.store.Reschedule(ctx, pendingci.RescheduleRequest{
		ID: request.ID, ExpectedRevision: request.Revision,
		Schedule: decision.Schedule, HeadSHA: decision.HeadSHA,
		NextCheckAt: decision.NextCheckAt, LastProgressAt: decision.LastProgressAt,
		LastObservedState: decision.LastObservedState,
		LastFingerprint:   decision.LastFingerprint, CheckedAt: checkedAt,
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
	finished, err := reconciler.store.Finish(ctx, pendingci.FinishRequest{
		ID: request.ID, ExpectedRevision: request.Revision,
		Lifecycle: lifecycle, Reason: reason, FinishedAt: finishedAt,
	})
	if err != nil {
		return err
	}
	reconciler.effects.Complete(ctx, finished, lifecycle)

	return nil
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
			LastProgressAt:    observation.ObservedAt,
			LastObservedState: string(observation.State),
			LastFingerprint:   observation.Fingerprint,
		}
		if scheduleErr := reconciler.reschedule(ctx, request, decision, observation.ObservedAt); scheduleErr != nil {
			return fmt.Errorf("merge failed: %v; reschedule failed: %w", err, scheduleErr)
		}

		return fmt.Errorf("merge failed and remains armed: %w", err)
	}

	return reconciler.finish(
		ctx, request, pendingci.LifecycleMerged,
		"CI passed and pull request merged", observation.ObservedAt,
	)
}
