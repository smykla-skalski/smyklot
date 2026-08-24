package sqlstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

type linkedQueueItem struct {
	ID            string
	Kind          workqueue.Kind
	Lane          workqueue.Lane
	TargetID      string
	RepositoryID  *string
	SourceKind    string
	SourceID      string
	Title         string
	Summary       string
	State         workqueue.State
	NotBefore     time.Time
	Details       any
	ActorID       string
	ProgressTotal int
}

func insertLinkedQueueItem(ctx context.Context, tx *transaction, link linkedQueueItem) error {
	policy, err := getEffectiveQueuePolicy(ctx, tx, link.Kind, &link.TargetID)
	if err != nil {
		return fmt.Errorf("read linked queue policy: %w", err)
	}
	details, err := json.Marshal(link.Details)
	if err != nil {
		return fmt.Errorf("encode linked queue details: %w", err)
	}
	item := workqueue.Item{
		ID: link.ID, Kind: link.Kind, Lane: link.Lane, TargetID: &link.TargetID,
		RepositoryID: link.RepositoryID, SourceKind: link.SourceKind, SourceID: link.SourceID,
		Title: link.Title, Summary: link.Summary, State: link.State,
		Priority: policy.DefaultPriority, WindowMode: workqueue.WindowRespect,
		NotBefore: link.NotBefore, EligibleAt: link.NotBefore, Details: details,
		ProgressTotal: link.ProgressTotal,
		Revision:      1, CreatedAt: link.NotBefore, UpdatedAt: link.NotBefore,
	}
	if !link.Kind.Windowed() {
		item.WindowMode = workqueue.WindowBypass
	} else {
		item.ProfileID = &policy.ProfileID
		profile, profileErr := getScheduleProfile(ctx, tx, policy.ProfileID)
		if profileErr != nil {
			return fmt.Errorf("read linked queue profile: %w", profileErr)
		}
		item.EligibleAt, err = workqueue.NextEligible(profile, item.NotBefore)
		if err != nil {
			return err
		}
	}
	if !policy.Enabled && !item.State.Terminal() {
		item.State = workqueue.StateBlocked
		item.BlockedReason = queueBlockedDisabled
	}
	if err := insertQueueItem(ctx, tx, item); err != nil {
		return err
	}

	return insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: item.ID, ActorID: queueEventActor(link.ActorID), Kind: queueEventCreated, State: item.State,
		Summary: "Queued " + item.Title, CreatedAt: item.CreatedAt,
	})
}

func transitionLinkedQueueItem(
	ctx context.Context,
	tx *transaction,
	id string,
	state workqueue.State,
	at time.Time,
	summary string,
	actorID string,
) error {
	finishedAt := any(nil)
	if state.Terminal() {
		finishedAt = at
	}
	result, err := tx.ExecContext(ctx, `
UPDATE queue_items SET state = ?, immediate_dispatch = FALSE,
    lease_expires_at = NULL, finished_at = ?, updated_at = ?, revision = revision + 1
WHERE id = ?`, state, finishedAt, at, id)
	if err != nil {
		return fmt.Errorf("transition linked queue item: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read linked queue transition: %w", err)
	}
	if changed == 0 {
		return nil
	}

	return insertQueueEvent(ctx, tx, workqueue.Event{
		ActorID: queueEventActor(actorID),
		ItemID:  id, Kind: "source_transition", State: state,
		Summary: summary, CreatedAt: at,
	})
}

func leaseLinkedQueueItem(
	ctx context.Context,
	tx *transaction,
	id string,
	at time.Time,
	until time.Time,
	stage string,
) error {
	result, err := tx.ExecContext(ctx, `
UPDATE queue_items SET state = 'running', immediate_dispatch = FALSE,
    attempt = attempt + 1, lease_expires_at = ?,
    started_at = COALESCE(started_at, ?), blocked_reason = '',
    updated_at = ?, revision = revision + 1
WHERE id = ?`, until, at, at, id)
	if err != nil {
		return fmt.Errorf("lease linked queue item: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return fmt.Errorf("lease linked queue item changed %d rows: %w", changed, err)
	}

	return insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: id, ActorID: queueEventActor(queueActorSystem), Kind: "started", State: workqueue.StateRunning,
		Summary: stage, CreatedAt: at,
	})
}

func scheduleApprovedSyncPlan(
	ctx context.Context,
	tx *transaction,
	planID string,
	at time.Time,
	actorID string,
) error {
	item, err := getQueueItem(ctx, tx, "sync-plan:"+planID, "")
	if err != nil {
		return fmt.Errorf("read sync plan queue item: %w", err)
	}
	eligibleAt := at
	if item.WindowMode == workqueue.WindowRespect && item.ProfileID != nil {
		profile, profileErr := getScheduleProfile(ctx, tx, *item.ProfileID)
		if profileErr != nil {
			return profileErr
		}
		eligibleAt, err = workqueue.NextEligible(profile, at)
		if err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE queue_items SET state = 'scheduled', not_before = ?, eligible_at = ?,
    blocked_reason = '', updated_at = ?, revision = revision + 1
WHERE id = ?`, at, eligibleAt, at, item.ID); err != nil {
		return fmt.Errorf("schedule approved sync plan: %w", err)
	}

	return insertQueueEvent(ctx, tx, workqueue.Event{
		ActorID: queueEventActor(actorID),
		ItemID:  item.ID, Kind: "approved", State: workqueue.StateScheduled,
		Summary: "Sync plan approved", CreatedAt: at,
	})
}

func syncPendingCIQueue(
	ctx context.Context,
	tx runner,
	request pendingci.Request,
) error {
	item, err := getQueueItem(ctx, tx, "pending-ci:"+strconv.FormatInt(request.ID, 10), "")
	if err != nil {
		return fmt.Errorf("read pending CI queue item: %w", err)
	}
	state, blockedReason, err := pendingCIQueueState(ctx, tx, item, request)
	if err != nil {
		return err
	}
	finished := request.FinishedAt
	if request.CleanupPending {
		finished = nil
	}
	eligibleAt := request.NextCheckAt
	if !item.Immediate && item.WindowMode == workqueue.WindowRespect && item.ProfileID != nil {
		profile, profileErr := getScheduleProfile(ctx, tx, *item.ProfileID)
		if profileErr != nil {
			return profileErr
		}
		eligibleAt, err = workqueue.NextEligible(profile, request.NextCheckAt)
		if err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `
UPDATE queue_items SET state = ?, not_before = ?, eligible_at = ?,
    blocked_reason = ?, lease_expires_at = ?, finished_at = ?,
    updated_at = ?, revision = revision + 1
WHERE source_kind = 'pending_ci' AND source_id = ?`,
		state, request.NextCheckAt, eligibleAt, blockedReason,
		request.LeaseExpiresAt, finished, request.UpdatedAt, strconv.FormatInt(request.ID, 10),
	)
	if err != nil {
		return fmt.Errorf("synchronize pending CI queue item: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return err
	}
	summary := "Pending CI check rescheduled"
	if request.Reason != "" {
		summary += ": " + request.Reason
	}
	if state.Terminal() {
		summary = "Pending CI request " + string(state)
	} else if state == workqueue.StateBlocked && blockedReason == queueBlockedDisabled {
		summary = "Pending CI checking blocked by policy"
	}

	return insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: item.ID, ActorID: queueEventActor(queueActorSystem), Kind: "source_transition",
		State: state, Summary: summary, CreatedAt: request.UpdatedAt,
	})
}

func pendingCIQueueState(
	ctx context.Context,
	tx runner,
	item workqueue.Item,
	request pendingci.Request,
) (workqueue.State, string, error) {
	state := workqueue.StateScheduled
	switch request.Lifecycle {
	case pendingci.LifecycleMerged:
		state = workqueue.StateSucceeded
	case pendingci.LifecycleCancelled:
		state = workqueue.StateCancelled
	case pendingci.LifecycleSuperseded:
		state = workqueue.StateSuperseded
	}
	if request.CleanupPending {
		return workqueue.StateRetrying, request.Reason, nil
	}
	if state.Terminal() {
		return state, request.Reason, nil
	}
	policy, err := getEffectiveQueuePolicy(ctx, tx, workqueue.KindPendingCI, item.TargetID)
	if err != nil {
		return "", "", fmt.Errorf("read pending CI queue policy: %w", err)
	}
	if !policy.Enabled {
		return workqueue.StateBlocked, queueBlockedDisabled, nil
	}

	return state, request.Reason, nil
}

func queueEventActor(actorID string) *string {
	if actorID == "" || actorID == queueActorSystem {
		return nil
	}

	return &actorID
}

func syncPendingCIQueueWhere(
	ctx context.Context,
	tx runner,
	clause string,
	arguments ...any,
) error {
	rows, err := tx.QueryContext(ctx, pendingCISelect+" WHERE "+clause, arguments...)
	if err != nil {
		return fmt.Errorf("list pending CI queue changes: %w", err)
	}
	requests, err := collectRows(rows, scanPendingCI)
	if err != nil {
		return fmt.Errorf("read pending CI queue changes: %w", err)
	}
	for _, request := range requests {
		if err := syncPendingCIQueue(ctx, tx, request); err != nil {
			return err
		}
	}

	return nil
}
