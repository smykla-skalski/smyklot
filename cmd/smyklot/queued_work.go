package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

const recurringWorkLease = 15 * time.Minute

type recurringWork struct {
	kind         workqueue.Kind
	targetID     *string
	repositoryID *string
	title        string
}

func (s *server) runRecurringWork(
	ctx context.Context,
	work recurringWork,
	run func() error,
) (bool, error) {
	return s.runRecurringWorkWithSummary(ctx, work, func() (string, error) {
		return "", run()
	})
}

func (s *server) runRecurringWorkWithSummary(
	ctx context.Context,
	work recurringWork,
	run func() (string, error),
) (bool, error) {
	now := time.Now().UTC()
	item, claimed, err := s.claimRecurringWork(ctx, workqueue.RecurringClaim{
		Kind: work.kind, TargetID: work.targetID, RepositoryID: work.repositoryID,
		Title: work.title, Now: now, LeaseDuration: recurringWorkLease,
	})
	if err != nil || !claimed {
		return false, err
	}
	err = s.runClaimedRecurringWorkWithSummary(ctx, item, work, run)

	return true, err
}

func (s *server) claimRecurringWork(
	ctx context.Context,
	claim workqueue.RecurringClaim,
) (workqueue.Item, bool, error) {
	release, allowed := s.beginBackgroundWork()
	if !allowed {
		return workqueue.Item{}, false, nil
	}
	defer release()

	return s.store.ClaimRecurringWork(ctx, claim)
}

func (s *server) runClaimedRecurringWorkWithSummary(
	ctx context.Context,
	item workqueue.Item,
	work recurringWork,
	run func() (string, error),
) error {
	s.announceRecurringWork(work)
	successSummary, runErr := run()
	_, finishErr := s.store.FinishRecurringWork(
		ctx, item.ID, recurringCompletion(successSummary, runErr), time.Now().UTC(),
	)
	s.announceRecurringWork(work)

	// A workload error is already durable as retrying or failed queue state.
	// Only a failure to persist that outcome should stop the shared dispatcher;
	// otherwise one repository failure would back off unrelated maintenance.
	return finishErr
}

func recurringCompletion(
	successSummary string,
	runErr error,
) workqueue.RecurringCompletion {
	if runErr == nil {
		return workqueue.RecurringCompletion{SuccessSummary: successSummary}
	}
	var blocker interface{ QueueBlockReason() string }
	if errors.As(runErr, &blocker) {
		return workqueue.RecurringCompletion{
			Failure: strings.TrimSpace(blocker.QueueBlockReason()), Blocked: true,
		}
	}
	var classified interface{ Retryable() bool }
	retryable := true
	if errors.As(runErr, &classified) {
		retryable = classified.Retryable()
	}

	return workqueue.RecurringCompletion{Failure: runErr.Error(), Retryable: retryable}
}

func (s *server) announceRecurringWork(work recurringWork) {
	if s.panel == nil {
		return
	}
	targetID := ""
	if work.targetID != nil {
		targetID = *work.targetID
	}
	s.panel.AnnounceQueue(targetID)
}
