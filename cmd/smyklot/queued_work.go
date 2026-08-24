package main

import (
	"context"
	"errors"
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
	item, claimed, err := s.store.ClaimRecurringWork(ctx, workqueue.RecurringClaim{
		Kind: work.kind, TargetID: work.targetID, RepositoryID: work.repositoryID,
		Title: work.title, Now: now, LeaseDuration: recurringWorkLease,
	})
	if err != nil || !claimed {
		return false, err
	}
	s.announceRecurringWork(work)
	successSummary, runErr := run()
	failure := ""
	if runErr != nil {
		failure = runErr.Error()
	}
	_, finishErr := s.store.FinishRecurringWork(
		ctx, item.ID, failure, successSummary, time.Now().UTC(),
	)
	s.announceRecurringWork(work)

	return true, errors.Join(runErr, finishErr)
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
