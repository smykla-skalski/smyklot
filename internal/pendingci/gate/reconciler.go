package gate

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

type transitionStore interface {
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

type observer interface {
	Observe(context.Context, pendingci.Request) (pendingci.Observation, error)
}

type effects interface {
	MergeAtHead(context.Context, pendingci.Request, string) error
	CleanupArtifacts(context.Context, pendingci.Request, pendingci.Lifecycle) error
}

type checkMergeEffects interface {
	SatisfyCheck(context.Context, pendingci.Request) error
	RestoreBlockingCheck(context.Context, pendingci.Request) error
}

type mergePhaseStore interface {
	MarkMergeCheckSucceeded(
		context.Context,
		pendingci.MarkMergeCheckSucceededRequest,
	) (pendingci.Request, error)
}

type reauthorizationStore interface {
	RequireReauthorization(
		context.Context,
		pendingci.RequireReauthorizationRequest,
	) (pendingci.Request, error)
}

type reauthorizationEffects interface {
	RequireReauthorizationCheck(
		context.Context,
		pendingci.Request,
		string,
	) (pendingci.CheckSlot, error)
}

type retiredCheckStore interface {
	ClearRetiredCheckSlot(
		context.Context,
		pendingci.ClearRetiredCheckSlotRequest,
	) (pendingci.Request, error)
}

type retiredCheckEffects interface {
	GetPendingCICheckSlot(context.Context, int64) (pendingci.CheckSlot, error)
	PendingCICheckSlotIsCurrent(
		context.Context,
		pendingci.Request,
		pendingci.CheckSlot,
	) (bool, error)
	RestoreRetiredPendingCICheck(context.Context, pendingci.CheckSlot) error
}

type gateWakeEffects interface {
	WakePendingCIGates()
}

type queuePolicyReader interface {
	GetEffectiveQueuePolicy(context.Context, workqueue.Kind, *string) (workqueue.Policy, error)
}

// Reconciler combines live truth with the pure policy, then applies
// one optimistic durable transition. GitHub access stays behind narrow ports.
type Reconciler struct {
	store     transitionStore
	observer  observer
	effects   effects
	exclusive bot.Exclusive
	timingMu  sync.RWMutex
	timing    pendingci.Timing
}

func newReconciler(
	store transitionStore,
	observer observer,
	effects effects,
	exclusive bot.Exclusive,
	timing pendingci.Timing,
) *Reconciler {
	return &Reconciler{
		store: store, observer: observer, effects: effects,
		exclusive: exclusive, timing: timing,
	}
}

func defaultTiming() pendingci.Timing {
	return pendingci.Timing{
		ActiveInterval: 5 * time.Minute, DiscoveryGrace: 10 * time.Minute,
		DeferAfter: time.Hour, DeferredInterval: 6 * time.Hour,
		PassingQuiet: pendingci.DefaultPassingQuiet,
	}
}

func (reconciler *Reconciler) SetPassingQuiet(value time.Duration) bool {
	reconciler.timingMu.Lock()
	defer reconciler.timingMu.Unlock()
	if reconciler.timing.PassingQuiet == value {
		return false
	}
	reconciler.timing.PassingQuiet = value

	return true
}

func (reconciler *Reconciler) currentTiming() pendingci.Timing {
	reconciler.timingMu.RLock()
	defer reconciler.timingMu.RUnlock()

	return reconciler.timing
}

func (reconciler *Reconciler) timingFor(
	ctx context.Context,
	request pendingci.Request,
	observation pendingci.Observation,
) (pendingci.Timing, error) {
	timing := reconciler.currentTiming()
	if reader, ok := reconciler.store.(queuePolicyReader); ok {
		policy, err := reader.GetEffectiveQueuePolicy(
			ctx, workqueue.KindPendingCI, &request.TargetID,
		)
		if err != nil {
			return pendingci.Timing{}, fmt.Errorf("read pending-CI queue policy: %w", err)
		}
		configured, err := workqueue.ParsePendingCITiming(
			policy.Configuration,
			workqueue.PendingCITiming{
				ActiveInterval: timing.ActiveInterval, DiscoveryGrace: timing.DiscoveryGrace,
				DeferAfter: timing.DeferAfter, DeferredInterval: timing.DeferredInterval,
				PassingQuiet: timing.PassingQuiet,
			},
		)
		if err != nil {
			return pendingci.Timing{}, err
		}
		timing = pendingci.Timing{
			ActiveInterval: configured.ActiveInterval, DiscoveryGrace: configured.DiscoveryGrace,
			DeferAfter: configured.DeferAfter, DeferredInterval: configured.DeferredInterval,
			PassingQuiet: configured.PassingQuiet,
		}
	}
	if observation.PassingQuiet != nil {
		timing.PassingQuiet = *observation.PassingQuiet
	}

	return timing, nil
}

func (reconciler *Reconciler) Process(
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

func (reconciler *Reconciler) processArmedExclusive(
	ctx context.Context,
	request pendingci.Request,
) error {
	observation, err := reconciler.observer.Observe(ctx, request)
	if err != nil {
		return fmt.Errorf("observe live GitHub state: %w", err)
	}
	timing, err := reconciler.timingFor(ctx, request, observation)
	if err != nil {
		return fmt.Errorf("resolve pending CI timing: %w", err)
	}
	decision, err := pendingci.Decide(request, observation, timing)
	if err != nil {
		return fmt.Errorf("decide pending CI transition: %w", err)
	}
	handled, err := reconciler.reconcileRetiredCheck(ctx, request, decision, observation.ObservedAt)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	switch decision.Kind {
	case pendingci.DecisionReschedule:
		return reconciler.reschedule(ctx, request, decision, observation)
	case pendingci.DecisionReauthorize:
		return reconciler.requireReauthorization(ctx, request, decision, observation)
	case pendingci.DecisionFinish:
		return reconciler.finish(ctx, request, decision.Lifecycle, decision.Reason, observation.ObservedAt)
	case pendingci.DecisionMerge:
		return reconciler.mergeExclusive(ctx, request)
	default:
		return fmt.Errorf("unsupported pending CI decision %q", decision.Kind)
	}
}

func (reconciler *Reconciler) reconcileRetiredCheck(
	ctx context.Context,
	request pendingci.Request,
	decision pendingci.Decision,
	clearedAt time.Time,
) (bool, error) {
	if request.RetiredCheckSlotID == nil {
		return false, nil
	}
	effects, ok := reconciler.effects.(retiredCheckEffects)
	if !ok {
		return false, errors.New("pending CI retired-check effects are unavailable")
	}
	store, ok := reconciler.store.(retiredCheckStore)
	if !ok {
		return false, errors.New("pending CI retired-check store is unavailable")
	}
	slot, err := effects.GetPendingCICheckSlot(ctx, *request.RetiredCheckSlotID)
	if err != nil {
		return false, fmt.Errorf("read retired pending CI check: %w", err)
	}
	if slot.RepositoryID != request.RepositoryID || slot.PullRequest != request.PullRequest {
		return false, errors.New("retired pending CI check does not belong to its request")
	}
	if decision.Kind == pendingci.DecisionReauthorize &&
		decision.CandidateHeadSHA == slot.HeadSHA {
		return false, nil
	}
	current, err := effects.PendingCICheckSlotIsCurrent(ctx, request, slot)
	if err != nil {
		return false, fmt.Errorf("read retired pending CI check ownership: %w", err)
	}
	if !current {
		if err := effects.RestoreRetiredPendingCICheck(ctx, slot); err != nil {
			return false, fmt.Errorf("restore retired pending CI check baseline: %w", err)
		}
	}
	_, err = store.ClearRetiredCheckSlot(ctx, pendingci.ClearRetiredCheckSlotRequest{
		ID: request.ID, ExpectedRevision: request.Revision,
		CheckSlotID: slot.ID, ClearedAt: clearedAt,
	})
	if err != nil {
		return false, fmt.Errorf("clear retired pending CI check: %w", err)
	}

	return true, nil
}

func (reconciler *Reconciler) requireReauthorization(
	ctx context.Context,
	request pendingci.Request,
	decision pendingci.Decision,
	observation pendingci.Observation,
) error {
	effects, ok := reconciler.effects.(reauthorizationEffects)
	if !ok {
		return errors.New("pending CI reauthorization effects are unavailable")
	}
	store, ok := reconciler.store.(reauthorizationStore)
	if !ok {
		return errors.New("pending CI reauthorization store is unavailable")
	}
	slot, err := effects.RequireReauthorizationCheck(
		ctx,
		request,
		decision.CandidateHeadSHA,
	)
	if err != nil {
		return fmt.Errorf("publish pending CI reauthorization check: %w", err)
	}
	_, err = store.RequireReauthorization(ctx, pendingci.RequireReauthorizationRequest{
		ID: request.ID, ExpectedRevision: request.Revision,
		CandidateHeadSHA: decision.CandidateHeadSHA,
		CandidateBase:    decision.CandidateBase,
		CandidateCheckID: slot.ID,
		ObservedAt:       observation.ObservedAt,
	})
	if err != nil {
		return fmt.Errorf("record pending CI reauthorization: %w", err)
	}

	return nil
}

func (reconciler *Reconciler) mergeExclusive(
	ctx context.Context,
	request pendingci.Request,
) error {
	observation, err := reconciler.observer.Observe(ctx, request)
	if err != nil {
		return fmt.Errorf("revalidate live GitHub state: %w", err)
	}
	timing, err := reconciler.timingFor(ctx, request, observation)
	if err != nil {
		return fmt.Errorf("resolve pending CI timing: %w", err)
	}
	decision, err := pendingci.Decide(request, observation, timing)
	if err != nil {
		return fmt.Errorf("revalidate pending CI transition: %w", err)
	}
	if decision.Kind == pendingci.DecisionReschedule {
		return reconciler.reschedule(ctx, request, decision, observation)
	}
	if decision.Kind == pendingci.DecisionReauthorize {
		return reconciler.requireReauthorization(ctx, request, decision, observation)
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
	if claimed.ArtifactKind == pendingci.ArtifactCheck {
		checkEffects, ok := reconciler.effects.(checkMergeEffects)
		if !ok {
			return errors.New("pending CI check merge effects are unavailable")
		}
		if err := checkEffects.SatisfyCheck(ctx, claimed); err != nil {
			return fmt.Errorf("satisfy pending CI required check: %w", err)
		}
		phaseStore, ok := reconciler.store.(mergePhaseStore)
		if !ok {
			return errors.New("pending CI merge phase store is unavailable")
		}
		claimed, err = phaseStore.MarkMergeCheckSucceeded(
			ctx,
			pendingci.MarkMergeCheckSucceededRequest{
				ID: claimed.ID, ExpectedRevision: claimed.Revision,
				MarkedAt: observation.ObservedAt,
			},
		)
		if err != nil {
			restoreErr := checkEffects.RestoreBlockingCheck(ctx, claimed)

			return errors.Join(
				fmt.Errorf("record successful pending CI required check: %w", err),
				restoreErr,
			)
		}
	}

	return reconciler.merge(ctx, claimed, observation)
}

func (reconciler *Reconciler) reschedule(
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

func (reconciler *Reconciler) finish(
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

func (reconciler *Reconciler) cleanup(
	ctx context.Context,
	request pendingci.Request,
) error {
	return reconciler.exclusive.Exclusive(ctx, request.RepositoryID, func() error {
		return reconciler.cleanupExclusive(ctx, request)
	})
}

func (reconciler *Reconciler) cleanupExclusive(
	ctx context.Context,
	request pendingci.Request,
) error {
	current := request
	if current.RetiredCheckSlotID != nil {
		handled, err := reconciler.reconcileRetiredCheck(
			ctx,
			current,
			pendingci.Decision{},
			current.UpdatedAt,
		)
		if err != nil {
			return reconciler.retryCleanup(ctx, current, err)
		}
		if handled {
			return nil
		}
	}
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
	if err != nil {
		return err
	}
	if effects, ok := reconciler.effects.(gateWakeEffects); ok {
		effects.WakePendingCIGates()
	}

	return nil
}

func (reconciler *Reconciler) retryCleanup(
	ctx context.Context,
	request pendingci.Request,
	cause error,
) error {
	_, retryErr := reconciler.store.RetryCleanup(ctx, pendingci.RetryCleanupRequest{
		ID: request.ID, ExpectedRevision: request.Revision,
		NextAttemptAt: request.UpdatedAt.Add(RetryDelay),
		FailedAt:      request.UpdatedAt, Error: cause.Error(),
	})
	if retryErr != nil {
		return fmt.Errorf("cleanup failed: %v; schedule retry: %w", cause, retryErr)
	}

	return fmt.Errorf("cleanup failed and will retry: %w", cause)
}

func (reconciler *Reconciler) merge(
	ctx context.Context,
	request pendingci.Request,
	observation pendingci.Observation,
) error {
	if err := reconciler.effects.MergeAtHead(ctx, request, observation.HeadSHA); err != nil {
		var restoreErr error
		if request.ArtifactKind == pendingci.ArtifactCheck {
			if checkEffects, ok := reconciler.effects.(checkMergeEffects); ok {
				restoreErr = checkEffects.RestoreBlockingCheck(ctx, request)
			}
		}
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

		return fmt.Errorf("merge failed and remains armed: %w", errors.Join(err, restoreErr))
	}

	return reconciler.finish(
		ctx, request, pendingci.LifecycleMerged,
		"CI passed and pull request merged", observation.ObservedAt,
	)
}
