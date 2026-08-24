package sqlstore

import (
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestChooseQueueDispatchUsesWeightedPriorityFairness(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	items := []workqueue.Item{
		dispatchFixture("urgent", workqueue.PriorityUrgent, "target", now),
		dispatchFixture("high", workqueue.PriorityHigh, "target", now),
		dispatchFixture("normal", workqueue.PriorityNormal, "target", now),
		dispatchFixture("low", workqueue.PriorityLow, "target", now),
	}
	state := queueDispatchState{}
	counts := make(map[workqueue.Priority]int)
	for range priorityCycle {
		choice, ok := chooseQueueDispatch(items, state)
		if !ok {
			t.Fatal("chooseQueueDispatch() found no candidate")
		}
		counts[choice.item.Priority]++
		state.priorityCursor = choice.nextCursor
	}
	want := map[workqueue.Priority]int{
		workqueue.PriorityUrgent: 8,
		workqueue.PriorityHigh:   4,
		workqueue.PriorityNormal: 2,
		workqueue.PriorityLow:    1,
	}
	for priority, expected := range want {
		if counts[priority] != expected {
			t.Errorf("dispatch count for %s = %d, want %d", priority, counts[priority], expected)
		}
	}
}

func TestChooseQueueDispatchRotatesTargetsAndKeepsOldestFirst(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	items := []workqueue.Item{
		dispatchFixture("a-new", workqueue.PriorityNormal, "a", now),
		dispatchFixture("a-old", workqueue.PriorityNormal, "a", now.Add(-time.Minute)),
		dispatchFixture("b", workqueue.PriorityNormal, "b", now.Add(-time.Hour)),
	}
	state := queueDispatchState{priorityCursor: 3}
	first, ok := chooseQueueDispatch(items, state)
	if !ok || first.item.ID != "a-old" {
		t.Fatalf("first dispatch = %q, found = %t, want a-old", first.item.ID, ok)
	}

	state.priorityCursor = first.nextCursor
	state.targetCursor = *first.item.TargetID
	second, ok := chooseQueueDispatch(items, state)
	if !ok || second.item.ID != "b" {
		t.Fatalf("second dispatch = %q, found = %t, want b", second.item.ID, ok)
	}
}

func TestEstimateQueuePositionFollowsWeightedTurns(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	items := make([]workqueue.Item, 0, 11)
	for index := range 10 {
		items = append(items, dispatchFixture(
			"urgent-"+string(rune('a'+index)), workqueue.PriorityUrgent, "target", now,
		))
	}
	target := dispatchFixture("low", workqueue.PriorityLow, "target", now)
	items = append(items, target)

	position := estimateQueuePosition(items, target, queueDispatchState{}, time.Minute, now)
	if position.ahead != 4 {
		t.Fatalf("work ahead = %d, want 4 weighted urgent turns", position.ahead)
	}
	if want := now.Add(4 * time.Minute); !position.estimated.Equal(want) {
		t.Fatalf("estimated start = %s, want %s", position.estimated, want)
	}
}

func dispatchFixture(
	id string,
	priority workqueue.Priority,
	target string,
	eligible time.Time,
) workqueue.Item {
	return workqueue.Item{
		ID: id, Lane: workqueue.LaneMaintenance, TargetID: &target,
		Priority: priority, EligibleAt: eligible, CreatedAt: eligible,
	}
}
