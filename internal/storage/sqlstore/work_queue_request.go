package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// RequestRecurringWork pulls one recurring occurrence forward without losing
// its cadence anchor. It is the durable implementation of a one-off Run now
// command when no existing queue row is available to address directly.
// A repeated RequestKey returns its immutable acceptance snapshot, not the live
// queue state. Read GetQueueItem separately for current execution progress.
func (s *Store) RequestRecurringWork(
	ctx context.Context,
	request workqueue.RecurringRequest,
) (workqueue.Item, error) {
	claim := workqueue.RecurringClaim{
		Kind: request.Kind, TargetID: request.TargetID, RepositoryID: request.RepositoryID,
		Title: request.Title, Now: request.Now, LeaseDuration: time.Minute,
	}
	if err := validateRecurringClaim(claim); err != nil {
		return workqueue.Item{}, err
	}
	if !validRecurringRequestKey(request.RequestKey) {
		return workqueue.Item{}, storage.ErrConflict
	}
	if strings.TrimSpace(request.ActorID) == "" || strings.TrimSpace(request.Reason) == "" {
		return workqueue.Item{}, errors.New("recurring request actor and reason are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Item{}, fmt.Errorf("begin recurring request: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := s.lockQueueDispatchState(ctx, tx, workqueue.LaneMaintenance); err != nil {
		return workqueue.Item{}, err
	}
	if err := s.authorizeSyncCheckRequest(ctx, tx, request); err != nil {
		return workqueue.Item{}, err
	}
	if request.RequestKey != "" {
		accepted, err := recurringRequestReceipt(ctx, tx, request)
		if err == nil {
			return accepted, nil
		}
		if !errors.Is(err, storage.ErrNotFound) {
			return workqueue.Item{}, err
		}
	}

	if err := s.prepareSyncCheckRequest(ctx, tx, request); err != nil {
		return workqueue.Item{}, err
	}
	item, err := s.recurringRequestCandidate(ctx, tx, request, claim)
	if err != nil {
		return workqueue.Item{}, err
	}
	if item.State == workqueue.StateRunning {
		return workqueue.Item{}, storage.ErrConflict
	}
	action := workqueue.ItemAction{
		Type: workqueue.ActionRunNow, ExpectedRevision: item.Revision,
		ActorID: request.ActorID, Reason: request.Reason, ChangedAt: request.Now,
	}
	updated, summary, err := s.applyQueueAction(ctx, tx, item, action)
	if err != nil {
		return workqueue.Item{}, err
	}
	if err := updateQueueItemForAction(ctx, tx, item, updated); err != nil {
		return workqueue.Item{}, err
	}
	actor := request.ActorID
	if err := insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: item.ID, ActorID: &actor, Kind: "action.run_now",
		State: updated.State, Summary: summary, CreatedAt: request.Now,
	}); err != nil {
		return workqueue.Item{}, err
	}
	if err := insertRecurringRequestReceipt(ctx, tx, request, updated); err != nil {
		return workqueue.Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return workqueue.Item{}, fmt.Errorf("commit recurring request: %w", err)
	}

	return updated, nil
}

func (s *Store) recurringRequestCandidate(ctx context.Context, tx *transaction, request workqueue.RecurringRequest, claim workqueue.RecurringClaim) (workqueue.Item, error) {
	sourceID := recurringSourceID(claim)
	item, err := latestRecurringItem(ctx, tx, sourceID, s.dialect.RowLock())
	missing := errors.Is(err, sql.ErrNoRows)
	if err != nil && !missing {
		return workqueue.Item{}, fmt.Errorf("read requested recurring item: %w", err)
	}
	policy, err := getEffectiveQueuePolicy(ctx, tx, request.Kind, request.TargetID)
	if err != nil {
		return workqueue.Item{}, err
	}
	if missing || item.State.Terminal() {
		item, err = s.createRecurringOccurrence(ctx, tx, claim, policy, item, sourceID)
		if err != nil {
			return workqueue.Item{}, err
		}
	}
	return item, nil
}
