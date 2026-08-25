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

func TestEstimateQueuePositionsOrdersTheBacklogInOnePass(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	items := make([]workqueue.Item, 0, 2_000)
	for index := range 2_000 {
		priority := workqueue.PriorityNormal
		if index%5 == 0 {
			priority = workqueue.PriorityUrgent
		}
		target := string(rune('a' + index%20))
		items = append(items, dispatchFixture(
			"item-"+time.Duration(index).String(), priority, target,
			now.Add(time.Duration(index%4)*time.Minute),
		))
	}

	positions := estimateQueuePositions(items, queueDispatchState{}, time.Second, now)
	if len(positions) != len(items) {
		t.Fatalf("estimated %d queue items, want %d", len(positions), len(items))
	}
	seen := make([]bool, len(items))
	for _, position := range positions {
		if position.ahead < 0 || position.ahead >= len(items) {
			t.Fatalf("work ahead %d is outside the backlog", position.ahead)
		}
		if seen[position.ahead] {
			t.Fatalf("work ahead %d was assigned twice", position.ahead)
		}
		seen[position.ahead] = true
	}
}

func TestEstimateQueuePositionsDoesNotLetFuturePriorityJumpReadyWork(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ready := dispatchFixture("ready", workqueue.PriorityLow, "target", now)
	future := dispatchFixture("future", workqueue.PriorityUrgent, "target", now.Add(time.Hour))

	positions := estimateQueuePositions(
		[]workqueue.Item{future, ready}, queueDispatchState{}, time.Minute, now,
	)
	if positions[ready.ID].ahead != 0 {
		t.Fatalf("ready work ahead = %d, want 0", positions[ready.ID].ahead)
	}
	if positions[future.ID].ahead != 1 {
		t.Fatalf("future work ahead = %d, want 1", positions[future.ID].ahead)
	}
}

func TestEstimateQueuePositionsAdmitsWorkBeforeTheNextVirtualSlot(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	first := dispatchFixture("first-low", workqueue.PriorityLow, "target", now)
	second := dispatchFixture("second-low", workqueue.PriorityLow, "target", now)
	urgent := dispatchFixture(
		"future-urgent", workqueue.PriorityUrgent, "target", now.Add(time.Minute),
	)

	positions := estimateQueuePositions(
		[]workqueue.Item{first, second, urgent}, queueDispatchState{}, 2*time.Minute, now,
	)
	if positions[urgent.ID].ahead != 1 {
		t.Fatalf("future urgent work ahead = %d, want 1", positions[urgent.ID].ahead)
	}
	if positions[urgent.ID].estimated != now.Add(2*time.Minute) {
		t.Fatalf(
			"future urgent estimate = %s, want %s",
			positions[urgent.ID].estimated,
			now.Add(2*time.Minute),
		)
	}
}

func TestEstimateQueuePositionsKeepsWorkerBusyAcrossIdleGap(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	first := dispatchFixture(
		"future-first", workqueue.PriorityNormal, "target", now.Add(time.Hour),
	)
	second := dispatchFixture(
		"future-second", workqueue.PriorityNormal, "target", now.Add(time.Hour+30*time.Second),
	)

	positions := estimateQueuePositions(
		[]workqueue.Item{first, second}, queueDispatchState{}, time.Minute, now,
	)
	if got, want := positions[first.ID].estimated, now.Add(time.Hour); !got.Equal(want) {
		t.Fatalf("first estimate = %s, want %s", got, want)
	}
	if got, want := positions[second.ID].estimated, now.Add(time.Hour+time.Minute); !got.Equal(want) {
		t.Fatalf("second estimate = %s, want %s", got, want)
	}
}

func TestEstimateQueuePositionsMatchesReadyDispatcherOrder(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	priorities := []workqueue.Priority{
		workqueue.PriorityUrgent,
		workqueue.PriorityHigh,
		workqueue.PriorityNormal,
		workqueue.PriorityLow,
	}
	items := make([]workqueue.Item, 0, 120)
	for index := range 120 {
		item := dispatchFixture(
			"ready-"+time.Duration(index).String(), priorities[index%len(priorities)],
			string(rune('a'+index%7)), now.Add(-time.Duration(index%3)*time.Minute),
		)
		item.Immediate = index%19 == 0
		items = append(items, item)
	}
	state := queueDispatchState{priorityCursor: 5, targetCursor: "c"}
	positions := estimateQueuePositions(items, state, time.Second, now)
	remaining := append([]workqueue.Item(nil), items...)
	for ahead := 0; len(remaining) > 0; ahead++ {
		choice, ok := chooseQueueDispatch(remaining, state)
		if !ok {
			t.Fatal("dispatcher found no ready work")
		}
		if positions[choice.item.ID].ahead != ahead {
			t.Fatalf(
				"%s work ahead = %d, want %d",
				choice.item.ID, positions[choice.item.ID].ahead, ahead,
			)
		}
		state.priorityCursor = choice.nextCursor
		state.targetCursor = queueItemTarget(choice.item)
		remaining = removeDispatchFixture(remaining, choice.item.ID)
	}
}

func removeDispatchFixture(items []workqueue.Item, id string) []workqueue.Item {
	for index := range items {
		if items[index].ID == id {
			return append(items[:index], items[index+1:]...)
		}
	}

	return items
}

func TestQueueSummarySnapshotRequiresFullActiveRootScope(t *testing.T) {
	t.Parallel()
	filter := workqueue.Filter{
		States: []workqueue.State{
			workqueue.StateScheduled, workqueue.StateBlocked, workqueue.StateReady,
			workqueue.StateRunning, workqueue.StateRetrying,
		},
		DispatchOrder: true,
		Summary:       true,
	}
	if !queueSummarySnapshotComplete(filter) {
		t.Fatal("full active Root summary should provide a complete scheduler snapshot")
	}
	filter.States = []workqueue.State{workqueue.StateReady}
	if queueSummarySnapshotComplete(filter) {
		t.Fatal("partial state summary must not estimate from an incomplete snapshot")
	}
}

func TestDispatchOrderReadsOneCompleteSnapshot(t *testing.T) {
	t.Parallel()
	filter := workqueue.Filter{Limit: 3, Offset: 40, DispatchOrder: true}
	limit, offset, bounded := queueSelectionBounds(filter)
	if bounded {
		t.Fatal("dispatch order must not limit a snapshot from an earlier count")
	}
	if limit != 3 || offset != 40 {
		t.Fatalf("pagination = (%d, %d), want (3, 40)", limit, offset)
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
