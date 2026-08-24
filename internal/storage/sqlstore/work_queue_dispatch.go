package sqlstore

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// priorityCycle is smooth weighted fairness: urgent:high:normal:low = 8:4:2:1.
// Interleaving the bands keeps a lower-priority turn from arriving as one
// large burst after all of the urgent turns.
var priorityCycle = [...]workqueue.Priority{
	workqueue.PriorityUrgent,
	workqueue.PriorityHigh,
	workqueue.PriorityUrgent,
	workqueue.PriorityNormal,
	workqueue.PriorityUrgent,
	workqueue.PriorityHigh,
	workqueue.PriorityUrgent,
	workqueue.PriorityLow,
	workqueue.PriorityUrgent,
	workqueue.PriorityHigh,
	workqueue.PriorityUrgent,
	workqueue.PriorityNormal,
	workqueue.PriorityUrgent,
	workqueue.PriorityHigh,
	workqueue.PriorityUrgent,
}

type queueDispatchState struct {
	priorityCursor int
	targetCursor   string
}

type queueDispatchChoice struct {
	item       workqueue.Item
	nextCursor int
}

func (s *Store) NextQueueAvailability(
	ctx context.Context,
	lane workqueue.Lane,
	now time.Time,
) (*time.Time, error) {
	return nextQueueAvailability(ctx, s.db, lane, now)
}

func nextQueueAvailability(
	ctx context.Context,
	runner runner,
	lane workqueue.Lane,
	now time.Time,
) (*time.Time, error) {
	var available StoredTime
	err := runner.QueryRowContext(ctx, `
SELECT MIN(
    CASE
        WHEN state = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at > eligible_at
            THEN lease_expires_at
        ELSE eligible_at
    END
)
FROM queue_items
WHERE lane = ? AND state IN ('scheduled', 'ready', 'retrying', 'running')`, lane).Scan(&available)
	if err != nil {
		return nil, fmt.Errorf("read next %s queue availability: %w", lane, err)
	}
	if !available.Valid() {
		return nil, nil
	}
	at := available.Time()
	if at.Before(now) {
		at = now
	}

	return &at, nil
}

func (s *Store) nextQueueDispatch(
	ctx context.Context,
	tx *transaction,
	lane workqueue.Lane,
	now time.Time,
) (queueDispatchChoice, bool, error) {
	state, err := s.lockQueueDispatchState(ctx, tx, lane)
	if err != nil {
		return queueDispatchChoice{}, false, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT"+queueItemColumns+`
FROM queue_items
WHERE lane = ?
  AND state IN ('scheduled', 'ready', 'retrying', 'running')
  AND (immediate_dispatch = TRUE OR eligible_at <= ?)
  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`+s.dialect.RowLock(),
		lane, now, now,
	)
	if err != nil {
		return queueDispatchChoice{}, false, fmt.Errorf("list dispatch candidates: %w", err)
	}
	items, err := collectRows(rows, scanQueueItem)
	if err != nil {
		return queueDispatchChoice{}, false, fmt.Errorf("read dispatch candidates: %w", err)
	}
	if len(items) == 0 {
		return queueDispatchChoice{}, false, nil
	}
	items, err = s.deferClosedWindowItems(ctx, tx, items, now)
	if err != nil {
		return queueDispatchChoice{}, false, err
	}
	if len(items) == 0 {
		return queueDispatchChoice{}, false, nil
	}

	choice, ok := chooseQueueDispatch(items, state)

	return choice, ok, nil
}

func (s *Store) deferClosedWindowItems(
	ctx context.Context,
	tx *transaction,
	items []workqueue.Item,
	now time.Time,
) ([]workqueue.Item, error) {
	eligible := make([]workqueue.Item, 0, len(items))
	profiles := make(map[string]workqueue.Profile)
	for _, item := range items {
		if item.Immediate || !item.Kind.Windowed() || item.WindowMode == workqueue.WindowBypass {
			eligible = append(eligible, item)
			continue
		}
		if item.ProfileID == nil {
			return nil, fmt.Errorf("dispatch queue item %q has no schedule profile", item.ID)
		}
		profile, found := profiles[*item.ProfileID]
		if !found {
			var err error
			profile, err = getScheduleProfile(ctx, tx, *item.ProfileID)
			if err != nil {
				return nil, fmt.Errorf("read dispatch schedule profile: %w", err)
			}
			profiles[profile.ID] = profile
		}
		next, err := workqueue.NextEligible(profile, now)
		if err != nil {
			return nil, fmt.Errorf("calculate dispatch window for %q: %w", item.ID, err)
		}
		if next.Equal(now) {
			eligible = append(eligible, item)
			continue
		}
		if err := deferQueueItemToWindow(ctx, tx, item, profile, next, now); err != nil {
			return nil, err
		}
	}

	return eligible, nil
}

func deferQueueItemToWindow(
	ctx context.Context,
	tx *transaction,
	item workqueue.Item,
	profile workqueue.Profile,
	next time.Time,
	now time.Time,
) error {
	reason := "Waiting for the " + profile.Name + " window"
	result, err := tx.ExecContext(ctx, `
UPDATE queue_items SET state = 'scheduled', eligible_at = ?, blocked_reason = ?,
    lease_expires_at = NULL, updated_at = ?, revision = revision + 1
WHERE id = ? AND revision = ?`, next, reason, now, item.ID, item.Revision)
	if err != nil {
		return fmt.Errorf("defer queue item to next window: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deferred queue item result: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("defer queue item %q changed %d rows", item.ID, changed)
	}

	return insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: item.ID, Kind: "window_missed", State: workqueue.StateScheduled,
		Summary:   "Window closed before dispatch; moved to the next " + profile.Name + " opening",
		CreatedAt: now,
	})
}

func (s *Store) lockQueueDispatchState(
	ctx context.Context,
	tx *transaction,
	lane workqueue.Lane,
) (queueDispatchState, error) {
	var state queueDispatchState
	err := tx.QueryRowContext(ctx, `
SELECT priority_cursor, target_cursor FROM queue_dispatch_state WHERE lane = ?`+
		s.dialect.RowLock(), lane).Scan(&state.priorityCursor, &state.targetCursor)
	if err != nil {
		return queueDispatchState{}, fmt.Errorf("lock %s queue dispatch state: %w", lane, err)
	}

	return state, nil
}

func chooseQueueDispatch(
	items []workqueue.Item,
	state queueDispatchState,
) (queueDispatchChoice, bool) {
	immediate := matchingPriority(items, "", true)
	if len(immediate) > 0 {
		return queueDispatchChoice{
			item:       chooseQueueTarget(immediate, state.targetCursor),
			nextCursor: state.priorityCursor,
		}, true
	}
	for offset := range priorityCycle {
		index := (state.priorityCursor + offset) % len(priorityCycle)
		eligible := matchingPriority(items, priorityCycle[index], false)
		if len(eligible) == 0 {
			continue
		}

		return queueDispatchChoice{
			item:       chooseQueueTarget(eligible, state.targetCursor),
			nextCursor: (index + 1) % len(priorityCycle),
		}, true
	}

	return queueDispatchChoice{}, false
}

func matchingPriority(
	items []workqueue.Item,
	priority workqueue.Priority,
	immediate bool,
) []workqueue.Item {
	matched := make([]workqueue.Item, 0, len(items))
	for _, item := range items {
		if item.Immediate == immediate && (immediate || item.Priority == priority) {
			matched = append(matched, item)
		}
	}

	return matched
}

func chooseQueueTarget(items []workqueue.Item, previous string) workqueue.Item {
	targets := make([]string, 0, len(items))
	byTarget := make(map[string][]workqueue.Item)
	for _, item := range items {
		target := ""
		if item.TargetID != nil {
			target = *item.TargetID
		}
		if _, exists := byTarget[target]; !exists {
			targets = append(targets, target)
		}
		byTarget[target] = append(byTarget[target], item)
	}
	sort.Strings(targets)
	index := sort.SearchStrings(targets, previous)
	if index < len(targets) && targets[index] == previous {
		index++
	}
	if index == len(targets) {
		index = 0
	}
	selected := byTarget[targets[index]]
	sort.Slice(selected, func(left, right int) bool {
		if !selected[left].EligibleAt.Equal(selected[right].EligibleAt) {
			return selected[left].EligibleAt.Before(selected[right].EligibleAt)
		}
		if !selected[left].CreatedAt.Equal(selected[right].CreatedAt) {
			return selected[left].CreatedAt.Before(selected[right].CreatedAt)
		}

		return selected[left].ID < selected[right].ID
	})

	return selected[0]
}

func advanceQueueDispatch(
	ctx context.Context,
	tx *transaction,
	choice queueDispatchChoice,
	at time.Time,
) error {
	target := ""
	if choice.item.TargetID != nil {
		target = *choice.item.TargetID
	}
	result, err := tx.ExecContext(ctx, `
UPDATE queue_dispatch_state
SET priority_cursor = ?, target_cursor = ?, updated_at = ?
WHERE lane = ?`, choice.nextCursor, target, at, choice.item.Lane)
	if err != nil {
		return fmt.Errorf("advance %s queue dispatch state: %w", choice.item.Lane, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read queue dispatch advance: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("advance %s queue dispatch state changed %d rows", choice.item.Lane, changed)
	}

	return nil
}
