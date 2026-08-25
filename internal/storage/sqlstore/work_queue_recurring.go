package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func (s *Store) ClaimRecurringWork(
	ctx context.Context,
	claim workqueue.RecurringClaim,
) (workqueue.Item, bool, error) {
	if err := validateRecurringClaim(claim); err != nil {
		return workqueue.Item{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Item{}, false, fmt.Errorf("begin recurring claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	item, err := s.ensureRecurringOccurrenceTx(ctx, tx, claim)
	if err != nil {
		return workqueue.Item{}, false, err
	}
	if item.ID == "" {
		return workqueue.Item{}, false, nil
	}
	choice, available, err := s.nextQueueDispatch(ctx, tx, workqueue.LaneMaintenance, claim.Now)
	if err != nil {
		return workqueue.Item{}, false, err
	}
	if !available || choice.item.ID != item.ID {
		if err := tx.Commit(); err != nil {
			return workqueue.Item{}, false, fmt.Errorf("commit recurring schedule: %w", err)
		}

		return item, false, nil
	}
	claimed, err := claimRecurringItem(ctx, tx, &item, claim)
	if err != nil {
		return workqueue.Item{}, false, err
	}
	if claimed {
		if err := advanceQueueDispatch(ctx, tx, choice, claim.Now); err != nil {
			return workqueue.Item{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return workqueue.Item{}, false, fmt.Errorf("commit recurring claim: %w", err)
	}

	return item, claimed, nil
}

// EnsureRecurringWork creates or coalesces one durable occurrence without
// leasing it. Callers use this to publish every candidate before the shared
// dispatcher chooses, so the first pass after startup follows the same
// weighted and tenant-fair ordering as every later pass.
func (s *Store) EnsureRecurringWork(
	ctx context.Context,
	claim workqueue.RecurringClaim,
) (workqueue.Item, error) {
	if err := validateRecurringClaim(claim); err != nil {
		return workqueue.Item{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Item{}, fmt.Errorf("begin recurring schedule: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	item, err := s.ensureRecurringOccurrenceTx(ctx, tx, claim)
	if err != nil {
		return workqueue.Item{}, err
	}
	if item.ID == "" {
		return workqueue.Item{}, nil
	}
	if err := tx.Commit(); err != nil {
		return workqueue.Item{}, fmt.Errorf("commit recurring schedule: %w", err)
	}

	return item, nil
}

// SupersedeMissingRecurringWork retires durable occurrences whose installation
// or repository no longer appears in the scheduler's current catalog. A live
// lease is allowed to finish; an expired lease is safe to retire on this pass.
func (s *Store) SupersedeMissingRecurringWork(
	ctx context.Context,
	claims []workqueue.RecurringClaim,
	now time.Time,
) ([]workqueue.Item, error) {
	if now.IsZero() {
		return nil, errors.New("recurring reconciliation time is required")
	}
	live := make(map[string]struct{}, len(claims))
	for _, claim := range claims {
		if err := validateRecurringClaim(claim); err != nil {
			return nil, err
		}
		live[recurringSourceID(claim)] = struct{}{}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin recurring reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := s.lockQueueDispatchState(ctx, tx, workqueue.LaneMaintenance); err != nil {
		return nil, err
	}
	items, err := activeRecurringItems(ctx, tx, now, s.dialect.RowLock())
	if err != nil {
		return nil, err
	}
	superseded := make([]workqueue.Item, 0)
	for _, item := range items {
		if _, found := live[item.SourceID]; found {
			continue
		}
		updated, changed, err := supersedeRecurringItem(ctx, tx, item, now)
		if err != nil {
			return nil, err
		}
		if changed {
			superseded = append(superseded, updated)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit recurring reconciliation: %w", err)
	}

	return superseded, nil
}

func activeRecurringItems(
	ctx context.Context,
	tx *transaction,
	now time.Time,
	lock string,
) ([]workqueue.Item, error) {
	rows, err := tx.QueryContext(ctx, "SELECT"+queueItemColumns+`
FROM queue_items
WHERE source_kind = ?
  AND state NOT IN ('succeeded', 'failed', 'cancelled', 'superseded')
  AND (state <> 'running' OR lease_expires_at IS NULL OR lease_expires_at <= ?)
ORDER BY id`+lock, queueSourceRecurring, now)
	if err != nil {
		return nil, fmt.Errorf("list active recurring queue items: %w", err)
	}
	items, err := collectRows(rows, scanQueueItem)
	if err != nil {
		return nil, fmt.Errorf("read active recurring queue items: %w", err)
	}

	return items, nil
}

func supersedeRecurringItem(
	ctx context.Context,
	tx *transaction,
	item workqueue.Item,
	now time.Time,
) (workqueue.Item, bool, error) {
	const reason = "Workload scope is no longer available"
	result, err := tx.ExecContext(ctx, `
UPDATE queue_items SET state = 'superseded', immediate_dispatch = FALSE,
    blocked_reason = ?, lease_expires_at = NULL, finished_at = ?, updated_at = ?,
    revision = revision + 1
WHERE id = ? AND state NOT IN ('succeeded', 'failed', 'cancelled', 'superseded')
  AND (state <> 'running' OR lease_expires_at IS NULL OR lease_expires_at <= ?)`,
		reason, now, now, item.ID, now,
	)
	if err != nil {
		return workqueue.Item{}, false, fmt.Errorf("supersede missing recurring item: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return workqueue.Item{}, false, fmt.Errorf("read recurring supersede result: %w", err)
	}
	if changed == 0 {
		return item, false, nil
	}
	item.State, item.Immediate, item.BlockedReason = workqueue.StateSuperseded, false, reason
	item.LeaseExpiresAt, item.FinishedAt = nil, &now
	item.UpdatedAt, item.Revision = now, item.Revision+1
	if err := insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: item.ID, ActorID: queueEventActor(queueActorSystem), Kind: "source_unavailable",
		State: item.State, Summary: reason, CreatedAt: now,
	}); err != nil {
		return workqueue.Item{}, false, err
	}

	return item, true, nil
}

func (s *Store) ensureRecurringOccurrenceTx(
	ctx context.Context,
	tx *transaction,
	claim workqueue.RecurringClaim,
) (workqueue.Item, error) {
	if _, err := s.lockQueueDispatchState(ctx, tx, workqueue.LaneMaintenance); err != nil {
		return workqueue.Item{}, err
	}
	sourceID := recurringSourceID(claim)
	item, err := latestRecurringItem(ctx, tx, sourceID, s.dialect.RowLock())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workqueue.Item{}, fmt.Errorf("read recurring queue item: %w", err)
	}
	missing := errors.Is(err, sql.ErrNoRows)
	policy, err := getEffectiveQueuePolicy(ctx, tx, claim.Kind, claim.TargetID)
	if err != nil {
		return workqueue.Item{}, err
	}
	if !policy.Enabled {
		return workqueue.Item{}, nil
	}
	if missing || item.State.Terminal() {
		item, err = s.createRecurringOccurrence(ctx, tx, claim, policy, item, sourceID)
		if err != nil {
			return workqueue.Item{}, err
		}
	} else if err := s.coalesceRecurringOccurrence(ctx, tx, &item, claim, policy); err != nil {
		return workqueue.Item{}, err
	}

	return item, nil
}

// RequestRecurringWork pulls one recurring occurrence forward without losing
// its cadence anchor. It is the durable implementation of a one-off Run now
// command when no existing queue row is available to address directly.
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
	if item.State == workqueue.StateRunning {
		return workqueue.Item{}, storage.ErrConflict
	}
	action := workqueue.ItemAction{
		Type: workqueue.ActionRunNow, ExpectedRevision: item.Revision,
		ActorID: request.ActorID, Reason: request.Reason, ChangedAt: request.Now,
	}
	updated, summary, err := runQueueNow(item, action)
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
	if err := tx.Commit(); err != nil {
		return workqueue.Item{}, fmt.Errorf("commit recurring request: %w", err)
	}

	return updated, nil
}

func (s *Store) coalesceRecurringOccurrence(
	ctx context.Context,
	tx *transaction,
	item *workqueue.Item,
	claim workqueue.RecurringClaim,
	policy workqueue.Policy,
) error {
	previousAnchor := item.NotBefore
	if item.CadenceAnchorAt != nil {
		previousAnchor = *item.CadenceAnchorAt
	}
	if item.Immediate || item.Attempt > 0 ||
		(item.State != workqueue.StateScheduled && item.State != workqueue.StateReady) ||
		policy.Cadence <= 0 ||
		previousAnchor.Add(policy.Cadence).After(claim.Now) {
		return nil
	}
	anchor := recurringAnchor(previousAnchor, policy.Cadence, claim.Now)
	if !anchor.After(previousAnchor) {
		return nil
	}
	profile, err := getScheduleProfile(ctx, tx, policy.ProfileID)
	if err != nil {
		return noRows(err)
	}
	eligible, err := workqueue.NextEligible(profile, anchor)
	if err != nil {
		return err
	}
	state := stateForEligibility(eligible, claim.Now)
	if _, err := tx.ExecContext(ctx, `
UPDATE queue_items SET state = ?, not_before = ?, cadence_anchor_at = ?, eligible_at = ?,
    updated_at = ?, revision = revision + 1 WHERE id = ?`,
		state, anchor, anchor, eligible, claim.Now, item.ID,
	); err != nil {
		return fmt.Errorf("coalesce recurring queue item: %w", err)
	}
	item.State, item.NotBefore, item.CadenceAnchorAt, item.EligibleAt = state, anchor, &anchor, eligible
	item.UpdatedAt, item.Revision = claim.Now, item.Revision+1

	return insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: item.ID, ActorID: queueEventActor(queueActorSystem), Kind: "coalesced", State: state,
		Summary: "Coalesced missed occurrences", CreatedAt: claim.Now,
	})
}

func validateRecurringClaim(claim workqueue.RecurringClaim) error {
	if !claim.Kind.Valid() || claim.Kind == workqueue.KindWebhookDelivery ||
		claim.Kind == workqueue.KindPendingCI || claim.Kind == workqueue.KindSyncApply ||
		claim.Kind == workqueue.KindScheduleChange {
		return errors.New("recurring workload kind is invalid")
	}
	if strings.TrimSpace(claim.Title) == "" || claim.Now.IsZero() || claim.LeaseDuration <= 0 {
		return errors.New("recurring workload title, time, and lease are required")
	}

	return nil
}

func recurringSourceID(claim workqueue.RecurringClaim) string {
	parts := []string{string(claim.Kind), "global", "all"}
	if claim.TargetID != nil {
		parts[1] = *claim.TargetID
	}
	if claim.RepositoryID != nil {
		parts[2] = *claim.RepositoryID
	}

	return strings.Join(parts, ":")
}

func latestRecurringItem(
	ctx context.Context,
	runner runner,
	sourceID, lock string,
) (workqueue.Item, error) {
	return scanQueueItem(runner.QueryRowContext(ctx, "SELECT"+queueItemColumns+`
FROM queue_items
WHERE source_kind = ? AND source_id = ?
ORDER BY CASE
    WHEN state IN ('succeeded', 'failed', 'cancelled', 'superseded') THEN 1
    ELSE 0
  END,
  COALESCE(cadence_anchor_at, not_before) DESC,
  created_at DESC
LIMIT 1`+lock, queueSourceRecurring, sourceID))
}

func (s *Store) createRecurringOccurrence(
	ctx context.Context,
	tx *transaction,
	claim workqueue.RecurringClaim,
	policy workqueue.Policy,
	previous workqueue.Item,
	sourceID string,
) (workqueue.Item, error) {
	previousAnchor := previous.NotBefore
	if previous.CadenceAnchorAt != nil {
		previousAnchor = *previous.CadenceAnchorAt
	}
	anchor := recurringAnchor(previousAnchor, policy.Cadence, claim.Now)
	profile, err := getScheduleProfile(ctx, tx, policy.ProfileID)
	if err != nil {
		return workqueue.Item{}, noRows(err)
	}
	eligible, err := workqueue.NextEligible(profile, anchor)
	if err != nil {
		return workqueue.Item{}, err
	}
	profileID := policy.ProfileID
	item := workqueue.Item{
		ID:   "recurring:" + sourceID + ":" + strconv.FormatInt(anchor.UnixNano(), 10),
		Kind: claim.Kind, Lane: workqueue.LaneMaintenance,
		TargetID: claim.TargetID, RepositoryID: claim.RepositoryID,
		SourceKind: queueSourceRecurring, SourceID: sourceID, Title: claim.Title,
		State: stateForEligibility(eligible, claim.Now), Priority: policy.DefaultPriority,
		WindowMode: workqueue.WindowRespect, ProfileID: &profileID,
		NotBefore: anchor, CadenceAnchorAt: &anchor, EligibleAt: eligible, Revision: 1,
		CreatedAt: claim.Now, UpdatedAt: claim.Now,
	}
	if err := insertQueueItem(ctx, tx, item); err != nil {
		if s.dialect.UniqueViolation(err) {
			return workqueue.Item{}, storage.ErrConflict
		}
		return workqueue.Item{}, err
	}
	if err := insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: item.ID, ActorID: queueEventActor(queueActorSystem), Kind: queueEventCreated, State: item.State,
		Summary: "Queued " + item.Title, CreatedAt: claim.Now,
	}); err != nil {
		return workqueue.Item{}, err
	}
	return item, nil
}

func (s *Store) scheduleNextRecurringOccurrence(
	ctx context.Context,
	tx *transaction,
	item workqueue.Item,
	at time.Time,
) error {
	policy, err := getEffectiveQueuePolicy(ctx, tx, item.Kind, item.TargetID)
	if err != nil {
		return err
	}
	if !policy.Enabled || policy.Cadence <= 0 {
		return nil
	}
	item.State = workqueue.StateSucceeded
	claim := workqueue.RecurringClaim{
		Kind: item.Kind, TargetID: item.TargetID, RepositoryID: item.RepositoryID,
		Title: item.Title, Now: at, LeaseDuration: time.Minute,
	}
	_, err = s.createRecurringOccurrence(ctx, tx, claim, policy, item, item.SourceID)

	return err
}

func recurringAnchor(previous time.Time, cadence time.Duration, now time.Time) time.Time {
	if previous.IsZero() || cadence <= 0 {
		return now
	}
	next := previous.Add(cadence)
	if next.After(now) {
		return next
	}

	return next.Add(now.Sub(next) / cadence * cadence)
}

func claimRecurringItem(
	ctx context.Context,
	tx *transaction,
	item *workqueue.Item,
	claim workqueue.RecurringClaim,
) (bool, error) {
	if item.State == workqueue.StateRunning && item.LeaseExpiresAt != nil &&
		item.LeaseExpiresAt.After(claim.Now) {
		return false, nil
	}
	if item.EligibleAt.After(claim.Now) && !item.Immediate {
		return false, nil
	}
	lease := claim.Now.Add(claim.LeaseDuration)
	if err := leaseLinkedQueueItem(
		ctx, tx, item.ID, claim.Now, lease, "Started "+item.Title,
	); err != nil {
		return false, err
	}
	item.State, item.Immediate, item.BlockedReason = workqueue.StateRunning, false, ""
	item.Attempt++
	item.LeaseExpiresAt, item.UpdatedAt = &lease, claim.Now
	item.Revision++
	if item.StartedAt == nil {
		item.StartedAt = &claim.Now
	}

	return true, nil
}

func (s *Store) FinishRecurringWork(
	ctx context.Context,
	id string,
	completion workqueue.RecurringCompletion,
	at time.Time,
) (workqueue.Item, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Item{}, fmt.Errorf("begin recurring finish: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	item, err := getQueueItem(ctx, tx, id, s.dialect.RowLock())
	if err != nil {
		return workqueue.Item{}, noRows(err)
	}
	if item.SourceKind != queueSourceRecurring || item.State != workqueue.StateRunning {
		return workqueue.Item{}, storage.ErrConflict
	}
	state, eligible, summary, finished := s.recurringOutcome(
		ctx, tx, item, completion, at,
	)
	if _, err := tx.ExecContext(ctx, `
UPDATE queue_items SET state = ?, eligible_at = ?, blocked_reason = ?,
    lease_expires_at = NULL, finished_at = ?, updated_at = ?, revision = revision + 1
WHERE id = ?`, state, eligible, completion.Failure, finished, at, id); err != nil {
		return workqueue.Item{}, fmt.Errorf("finish recurring queue item: %w", err)
	}
	if err := insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: id, ActorID: queueEventActor(queueActorSystem), Kind: "finished",
		State: state, Summary: summary, CreatedAt: at,
	}); err != nil {
		return workqueue.Item{}, err
	}
	if state == workqueue.StateSucceeded {
		if err := s.scheduleNextRecurringOccurrence(ctx, tx, item, at); err != nil {
			return workqueue.Item{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return workqueue.Item{}, fmt.Errorf("commit recurring finish: %w", err)
	}
	item.State, item.EligibleAt, item.BlockedReason = state, eligible, completion.Failure
	item.LeaseExpiresAt, item.FinishedAt, item.UpdatedAt = nil, finished, at
	item.Revision++

	return item, nil
}

func (s *Store) recurringOutcome(
	ctx context.Context,
	tx *transaction,
	item workqueue.Item,
	completion workqueue.RecurringCompletion,
	at time.Time,
) (workqueue.State, time.Time, string, *time.Time) {
	if completion.Failure == "" {
		if completion.SuccessSummary == "" {
			completion.SuccessSummary = item.Title + " completed"
		}

		return workqueue.StateSucceeded, item.EligibleAt, completion.SuccessSummary, &at
	}
	if completion.Blocked {
		return workqueue.StateBlocked, item.EligibleAt, item.Title + " is blocked", nil
	}
	if !completion.Retryable {
		return workqueue.StateFailed, item.EligibleAt, item.Title + " failed", &at
	}
	policy, err := getEffectiveQueuePolicy(ctx, tx, item.Kind, item.TargetID)
	if err != nil || policy.RetryDelay <= 0 {
		return workqueue.StateFailed, item.EligibleAt, item.Title + " failed", &at
	}
	eligible := at.Add(policy.RetryDelay)
	if item.ProfileID != nil {
		if profile, profileErr := getScheduleProfile(ctx, tx, *item.ProfileID); profileErr == nil {
			if next, nextErr := workqueue.NextEligible(profile, eligible); nextErr == nil {
				eligible = next
			}
		}
	}

	return workqueue.StateRetrying, eligible, item.Title + " will retry", nil
}
