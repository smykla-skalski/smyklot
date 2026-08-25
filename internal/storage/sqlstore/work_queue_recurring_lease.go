package sqlstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// ClaimNextRecurringWork leases the one recurring occurrence selected by the
// shared maintenance scheduler. Publishing happens before this call, so the
// dispatcher can make one global choice instead of re-running the same choice
// once for every possible maintenance job.
func (s *Store) ClaimNextRecurringWork(
	ctx context.Context,
	lease workqueue.RecurringLease,
) (workqueue.Item, bool, error) {
	if lease.Now.IsZero() {
		return workqueue.Item{}, false, errors.New("recurring lease time is required")
	}
	if lease.LeaseDuration <= 0 {
		return workqueue.Item{}, false, errors.New("recurring lease duration must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Item{}, false, fmt.Errorf("begin next recurring claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	choice, available, err := s.nextQueueDispatch(
		ctx, tx, workqueue.LaneMaintenance, lease.Now,
	)
	if err != nil {
		return workqueue.Item{}, false, err
	}
	if !available || choice.item.SourceKind != queueSourceRecurring {
		if err := tx.Commit(); err != nil {
			return workqueue.Item{}, false, fmt.Errorf("commit recurring dispatch check: %w", err)
		}

		return workqueue.Item{}, false, nil
	}
	item := choice.item
	claimed, err := claimRecurringItem(ctx, tx, &item, workqueue.RecurringClaim{
		Kind: item.Kind, TargetID: item.TargetID, RepositoryID: item.RepositoryID,
		Title: item.Title, Now: lease.Now, LeaseDuration: lease.LeaseDuration,
	})
	if err != nil {
		return workqueue.Item{}, false, err
	}
	if claimed {
		if err := advanceQueueDispatch(ctx, tx, choice, lease.Now); err != nil {
			return workqueue.Item{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return workqueue.Item{}, false, fmt.Errorf("commit next recurring claim: %w", err)
	}

	return item, claimed, nil
}
