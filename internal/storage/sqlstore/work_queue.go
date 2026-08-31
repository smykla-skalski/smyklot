package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

const (
	queueActorSystem     = "system"
	queueBlockedDisabled = "Workload disabled by policy"
	queueEventCreated    = "created"
	queueSourceDelivery  = "delivery"
	queueSourcePendingCI = "pending_ci"
	queueSourceRecurring = "recurring"
	queryAllRows         = "1 = 1"
	queryTargetIDEquals  = "target_id = ?"
)

const queueItemColumns = `
    id, kind, lane, target_id, repository_id, source_kind, source_id,
    title, summary, state, priority, priority_overridden,
    window_mode, immediate_dispatch, profile_id,
    not_before, cadence_anchor_at, eligible_at, estimated_start_at, blocked_reason,
    progress_current, progress_total, attempt, lease_expires_at,
    requested_by, reason, details, revision, created_at, updated_at,
    started_at, finished_at`

func (s *Store) ListWorkQueue(
	ctx context.Context,
	filter workqueue.Filter,
) (workqueue.Page, error) {
	clauses, arguments := queueFilters(filter)
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	limit, offset, bounded := queueSelectionBounds(filter)
	var total int
	if bounded {
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM queue_items"+where, arguments...).Scan(&total); err != nil {
			return workqueue.Page{}, fmt.Errorf("count queue items: %w", err)
		}
	}
	query := "SELECT" + queueItemColumns + `
FROM queue_items` + where + `
ORDER BY CASE WHEN finished_at IS NULL THEN 0 ELSE 1 END,
         CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END,
         eligible_at, updated_at DESC, id`
	if bounded {
		query += "\nLIMIT ? OFFSET ?"
		arguments = append(arguments, limit+1, offset)
	}
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return workqueue.Page{}, fmt.Errorf("list queue items: %w", err)
	}
	items, err := collectRows(rows, scanQueueItem)
	if err != nil {
		return workqueue.Page{}, fmt.Errorf("read queue items: %w", err)
	}
	if !bounded {
		total = len(items)
	}
	var positionErr error
	if queueSummarySnapshotComplete(filter) {
		positionErr = s.addQueuePositionsFromSnapshot(ctx, items)
	} else {
		positionErr = s.addQueuePositions(ctx, items)
	}
	if positionErr != nil {
		return workqueue.Page{}, positionErr
	}
	if err := s.nameQueueSubjects(ctx, items); err != nil {
		return workqueue.Page{}, err
	}
	if filter.DispatchOrder {
		sortQueueItemsForDispatch(items)
		items, next := paginateQueueItems(items, limit, offset)

		return s.queuePageWithFacets(ctx, filter, items, next, total)
	}
	next := 0
	if len(items) > limit {
		items = items[:limit]
		next = offset + limit
	}

	return s.queuePageWithFacets(ctx, filter, items, next, total)
}

func queueSelectionBounds(filter workqueue.Filter) (limit, offset int, bounded bool) {
	limit, offset = pageLimit(filter.Limit), max(filter.Offset, 0)

	return limit, offset, !filter.DispatchOrder
}

func (s *Store) queuePageWithFacets(
	ctx context.Context,
	filter workqueue.Filter,
	items []workqueue.Item,
	next, total int,
) (workqueue.Page, error) {
	if filter.Summary {
		stateCounts, err := s.queueStateCounts(ctx, filter.TargetID)
		if err != nil {
			return workqueue.Page{}, err
		}

		return workqueue.Page{
			Items: items, NextOffset: next, Total: total,
			Facets: emptyQueueFacets(), StateCounts: stateCounts,
		}, nil
	}
	facets, err := s.queueFacets(ctx, filter.TargetID)
	if err != nil {
		return workqueue.Page{}, err
	}

	return workqueue.Page{Items: items, NextOffset: next, Total: total, Facets: facets}, nil
}

func queueSummarySnapshotComplete(filter workqueue.Filter) bool {
	if !filter.Summary || !filter.DispatchOrder || len(filter.States) != 5 {
		return false
	}
	active := map[workqueue.State]bool{
		workqueue.StateScheduled: true, workqueue.StateBlocked: true,
		workqueue.StateReady: true, workqueue.StateRunning: true,
		workqueue.StateRetrying: true,
	}
	for _, state := range filter.States {
		if !active[state] {
			return false
		}
		delete(active, state)
	}
	if len(active) != 0 {
		return false
	}

	return filter.TargetID == nil &&
		filter.RepositoryID == nil && filter.ProfileID == nil && len(filter.Kinds) == 0 &&
		len(filter.Priorities) == 0 && filter.CreatedAfter == nil && filter.CreatedBefore == nil &&
		filter.FinishedAfter == nil && strings.TrimSpace(filter.Search) == ""
}

func paginateQueueItems(items []workqueue.Item, limit, offset int) ([]workqueue.Item, int) {
	start := min(offset, len(items))
	end := min(start+limit, len(items))
	next := 0
	if end < len(items) {
		next = end
	}

	return items[start:end], next
}

func sortQueueItemsForDispatch(items []workqueue.Item) {
	sort.SliceStable(items, func(left, right int) bool {
		leftRank, rightRank := dispatchListRank(items[left]), dispatchListRank(items[right])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		leftAt, rightAt := queueItemEstimate(items[left]), queueItemEstimate(items[right])
		if !leftAt.Equal(rightAt) {
			return leftAt.Before(rightAt)
		}
		if items[left].WorkAhead != items[right].WorkAhead {
			return items[left].WorkAhead < items[right].WorkAhead
		}

		return items[left].ID < items[right].ID
	})
}

func dispatchListRank(item workqueue.Item) int {
	switch item.State {
	case workqueue.StateRunning:
		return 0
	case workqueue.StateBlocked:
		return 2
	default:
		return 1
	}
}

func queueItemEstimate(item workqueue.Item) time.Time {
	if item.EstimatedStartAt != nil {
		return *item.EstimatedStartAt
	}

	return item.EligibleAt
}

func queueFilters(filter workqueue.Filter) ([]string, []any) {
	var clauses []string
	var arguments []any
	if filter.TargetID != nil {
		clauses = append(clauses, queryTargetIDEquals)
		arguments = append(arguments, *filter.TargetID)
	}
	if filter.RepositoryID != nil {
		clauses = append(clauses, "repository_id = ?")
		arguments = append(arguments, *filter.RepositoryID)
	}
	if filter.ProfileID != nil {
		if *filter.ProfileID == "immediate" {
			clauses = append(clauses, "profile_id IS NULL")
		} else {
			clauses = append(clauses, "profile_id = ?")
			arguments = append(arguments, *filter.ProfileID)
		}
	}
	if len(filter.States) > 0 {
		clause, values := queueInClause("state", filter.States)
		clauses, arguments = append(clauses, clause), append(arguments, values...)
	}
	if len(filter.Kinds) > 0 {
		clause, values := queueInClause("kind", filter.Kinds)
		clauses, arguments = append(clauses, clause), append(arguments, values...)
	}
	if len(filter.Priorities) > 0 {
		clause, values := queueInClause("priority", filter.Priorities)
		clauses, arguments = append(clauses, clause), append(arguments, values...)
	}
	if filter.CreatedAfter != nil {
		clauses = append(clauses, "created_at >= ?")
		arguments = append(arguments, *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		clauses = append(clauses, "created_at < ?")
		arguments = append(arguments, *filter.CreatedBefore)
	}
	if filter.FinishedAfter != nil {
		clauses = append(clauses, "finished_at >= ?")
		arguments = append(arguments, *filter.FinishedAfter)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		/* Folded on both sides rather than through LIKE's own rules: SQLite
		   matches ASCII case-insensitively and PostgreSQL does not, so the same
		   search answered differently depending on the engine. */
		pattern := "%" + strings.ToLower(search) + "%"
		clauses = append(clauses, "(LOWER(title) LIKE ? OR LOWER(COALESCE(summary, '')) LIKE ?)")
		arguments = append(arguments, pattern, pattern)
	}

	return clauses, arguments
}

func (s *Store) queueFacets(ctx context.Context, targetID *string) (workqueue.Facets, error) {
	facets := emptyQueueFacets()
	queries := []struct {
		expression string
		append     func(string)
	}{
		{"target_id", func(value string) { facets.Targets = append(facets.Targets, value) }},
		{"repository_id", func(value string) { facets.Repositories = append(facets.Repositories, value) }},
		{"COALESCE(profile_id, 'immediate')", func(value string) { facets.Profiles = append(facets.Profiles, value) }},
		{"state", func(value string) { facets.States = append(facets.States, workqueue.State(value)) }},
		{"kind", func(value string) { facets.Kinds = append(facets.Kinds, workqueue.Kind(value)) }},
		{"priority", func(value string) { facets.Priorities = append(facets.Priorities, workqueue.Priority(value)) }},
	}
	for _, query := range queries {
		if err := s.readQueueFacet(ctx, query.expression, targetID, query.append); err != nil {
			return workqueue.Facets{}, err
		}
	}

	return facets, nil
}

func emptyQueueFacets() workqueue.Facets {
	return workqueue.Facets{
		Targets: []string{}, Repositories: []string{}, Profiles: []string{},
		States: []workqueue.State{}, Kinds: []workqueue.Kind{},
		Priorities: []workqueue.Priority{},
	}
}

func (s *Store) queueStateCounts(
	ctx context.Context,
	targetID *string,
) (map[workqueue.State]int, error) {
	query := "SELECT state, COUNT(*) FROM queue_items"
	var arguments []any
	if targetID != nil {
		query += " WHERE " + queryTargetIDEquals
		arguments = append(arguments, *targetID)
	}
	query += " GROUP BY state"
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("count queue states: %w", err)
	}
	defer func() { _ = rows.Close() }()
	counts := make(map[workqueue.State]int)
	for rows.Next() {
		var state workqueue.State
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, fmt.Errorf("read queue state count: %w", err)
		}
		counts[state] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read queue state counts: %w", err)
	}

	return counts, nil
}

func (s *Store) readQueueFacet(
	ctx context.Context,
	expression string,
	targetID *string,
	appendValue func(string),
) error {
	query := "SELECT DISTINCT " + expression + " FROM queue_items"
	var arguments []any
	if targetID != nil {
		query += " WHERE target_id = ?"
		arguments = append(arguments, *targetID)
	}
	query += " ORDER BY 1"
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return fmt.Errorf("list queue facet %s: %w", expression, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var value sql.NullString
		if err := rows.Scan(&value); err != nil {
			return fmt.Errorf("read queue facet %s: %w", expression, err)
		}
		if value.Valid && value.String != "" {
			appendValue(value.String)
		}
	}

	return rows.Err()
}

func queueInClause[T ~string](column string, values []T) (string, []any) {
	marks := make([]string, len(values))
	arguments := make([]any, len(values))
	for index, value := range values {
		marks[index], arguments[index] = "?", value
	}

	return column + " IN (" + strings.Join(marks, ", ") + ")", arguments
}

func (s *Store) addQueuePositions(ctx context.Context, items []workqueue.Item) error {
	positions := make(map[string]queuePosition)
	for _, lane := range []workqueue.Lane{
		workqueue.LaneWebhook, workqueue.LanePendingCI, workqueue.LaneMaintenance,
	} {
		computed, err := s.queuePositions(ctx, lane, time.Now().UTC())
		if err != nil {
			return err
		}
		for id, position := range computed {
			positions[id] = position
		}
	}

	return s.decorateQueuePositions(ctx, items, positions)
}

func (s *Store) addQueuePositionsFromSnapshot(
	ctx context.Context,
	items []workqueue.Item,
) error {
	positions := make(map[string]queuePosition)
	now := time.Now().UTC()
	for _, lane := range []workqueue.Lane{
		workqueue.LaneWebhook, workqueue.LanePendingCI, workqueue.LaneMaintenance,
	} {
		laneItems := make([]workqueue.Item, 0)
		for _, item := range items {
			if item.Lane != lane || item.State == workqueue.StateBlocked {
				continue
			}
			laneItems = append(laneItems, item)
		}
		if len(laneItems) == 0 {
			continue
		}
		state, err := s.readQueueDispatchState(ctx, lane)
		if err != nil {
			return err
		}
		duration, err := s.estimatedLaneDuration(ctx, lane)
		if err != nil {
			return err
		}
		for id, position := range estimateQueuePositions(laneItems, state, duration, now) {
			positions[id] = position
		}
	}

	return s.decorateQueuePositions(ctx, items, positions)
}

/*
Names the repository each row is about.

One query per distinct repository rather than a join, for the reason the profile
above takes the same shape: a page holds fifty rows and a handful of repositories,
and the read is answered from the map after the first of each. A repository that
has since been removed leaves the name empty, which is what a row with nothing to
say about its subject should say.
*/
func (s *Store) nameQueueSubjects(ctx context.Context, items []workqueue.Item) error {
	names := make(map[string]string)
	for index := range items {
		id := items[index].RepositoryID
		if id == nil {
			continue
		}
		name, known := names[*id]
		if !known {
			row := s.db.QueryRowContext(ctx, "SELECT full_name FROM repositories WHERE id = ?", *id)
			switch err := row.Scan(&name); {
			case errors.Is(err, sql.ErrNoRows):
				name = ""
			case err != nil:
				return fmt.Errorf("name queue subject: %w", err)
			}
			names[*id] = name
		}
		items[index].RepositoryName = name
	}

	return nil
}

func (s *Store) decorateQueuePositions(
	ctx context.Context,
	items []workqueue.Item,
	positions map[string]queuePosition,
) error {
	profiles := make(map[string]workqueue.Profile)
	for index := range items {
		if items[index].ProfileID != nil {
			profile, ok := profiles[*items[index].ProfileID]
			if !ok {
				var err error
				profile, err = s.GetScheduleProfile(ctx, *items[index].ProfileID)
				if err != nil {
					return err
				}
				profiles[profile.ID] = profile
			}
			items[index].ProfileName = profile.Name
			items[index].ProfileTimezone = profile.Timezone
		}
		if items[index].State.Terminal() {
			continue
		}
		if position, ok := positions[items[index].ID]; ok {
			items[index].WorkAhead = position.ahead
			items[index].EstimatedStartAt = &position.estimated
		} else if items[index].EstimatedStartAt == nil {
			estimate := items[index].EligibleAt
			items[index].EstimatedStartAt = &estimate
		}
	}

	return nil
}

type queuePosition struct {
	ahead     int
	estimated time.Time
}

func (s *Store) queuePositions(
	ctx context.Context,
	lane workqueue.Lane,
	now time.Time,
) (map[string]queuePosition, error) {
	items, err := s.activeQueueLane(ctx, lane)
	if err != nil || len(items) == 0 {
		return map[string]queuePosition{}, err
	}
	state, err := s.readQueueDispatchState(ctx, lane)
	if err != nil {
		return nil, err
	}
	duration, err := s.estimatedLaneDuration(ctx, lane)
	if err != nil {
		return nil, err
	}

	return estimateQueuePositions(items, state, duration, now), nil
}

func (s *Store) activeQueueLane(
	ctx context.Context,
	lane workqueue.Lane,
) ([]workqueue.Item, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT"+queueItemColumns+`
FROM queue_items
WHERE lane = ? AND state IN ('scheduled', 'ready', 'retrying', 'running')`, lane)
	if err != nil {
		return nil, fmt.Errorf("list active %s queue: %w", lane, err)
	}
	items, err := collectRows(rows, scanQueueItem)
	if err != nil {
		return nil, fmt.Errorf("read active %s queue: %w", lane, err)
	}

	return items, nil
}

func (s *Store) readQueueDispatchState(
	ctx context.Context,
	lane workqueue.Lane,
) (queueDispatchState, error) {
	var state queueDispatchState
	if err := s.db.QueryRowContext(ctx, `
SELECT priority_cursor, target_cursor FROM queue_dispatch_state WHERE lane = ?`, lane).
		Scan(&state.priorityCursor, &state.targetCursor); err != nil {
		return queueDispatchState{}, fmt.Errorf("read %s queue dispatch state: %w", lane, err)
	}

	return state, nil
}

func (s *Store) estimatedLaneDuration(
	ctx context.Context,
	lane workqueue.Lane,
) (time.Duration, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT started_at, finished_at FROM queue_items
WHERE lane = ? AND started_at IS NOT NULL AND finished_at IS NOT NULL
ORDER BY finished_at DESC LIMIT 101`, lane)
	if err != nil {
		return 0, fmt.Errorf("list %s queue durations: %w", lane, err)
	}
	defer func() { _ = rows.Close() }()
	var durations []time.Duration
	for rows.Next() {
		var started, finished StoredTime
		if err := rows.Scan(&started, &finished); err != nil {
			return 0, fmt.Errorf("read %s queue duration: %w", lane, err)
		}
		if elapsed := finished.Time().Sub(started.Time()); elapsed > 0 {
			durations = append(durations, elapsed)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(durations) == 0 {
		switch lane {
		case workqueue.LaneWebhook:
			return 2 * time.Second, nil
		case workqueue.LanePendingCI:
			return 30 * time.Second, nil
		default:
			return time.Minute, nil
		}
	}
	sort.Slice(durations, func(left, right int) bool { return durations[left] < durations[right] })

	return durations[len(durations)/2], nil
}

func estimateQueuePosition(
	all []workqueue.Item,
	target workqueue.Item,
	state queueDispatchState,
	duration time.Duration,
	now time.Time,
) queuePosition {
	positions := estimateQueuePositions(all, state, duration, now)
	if position, ok := positions[target.ID]; ok {
		return position
	}

	estimated := target.EligibleAt
	if estimated.Before(now) {
		estimated = now
	}

	return queuePosition{estimated: estimated}
}

func estimateQueuePositions(
	items []workqueue.Item,
	state queueDispatchState,
	duration time.Duration,
	now time.Time,
) map[string]queuePosition {
	positions, waiting, immediate, running := splitQueuePositionItems(items, now)
	scheduler := newQueuePositionScheduler(state)
	sortQueuePositionItems(immediate)
	for _, item := range immediate {
		scheduler.add(item)
	}
	sortQueuePositionItems(waiting)
	workers := 1
	if len(items) > 0 {
		workers = items[0].Lane.Workers()
	}
	estimateWaitingQueuePositions(positions, waiting, scheduler, running, workers, duration, now)

	return positions
}

func splitQueuePositionItems(
	items []workqueue.Item,
	now time.Time,
) (map[string]queuePosition, []workqueue.Item, []workqueue.Item, int) {
	positions := make(map[string]queuePosition, len(items))
	waiting := make([]workqueue.Item, 0, len(items))
	immediate := make([]workqueue.Item, 0, len(items))
	running := 0
	for _, item := range items {
		if item.State == workqueue.StateRunning && item.LeaseExpiresAt != nil &&
			item.LeaseExpiresAt.After(now) {
			estimated := now
			if item.StartedAt != nil {
				estimated = *item.StartedAt
			}
			positions[item.ID] = queuePosition{estimated: estimated}
			running++

			continue
		}
		if item.Immediate {
			immediate = append(immediate, item)

			continue
		}
		waiting = append(waiting, item)
	}

	return positions, waiting, immediate, running
}

func estimateWaitingQueuePositions(
	positions map[string]queuePosition,
	waiting []workqueue.Item,
	scheduler *queuePositionScheduler,
	running int,
	workers int,
	duration time.Duration,
	now time.Time,
) {
	nextWaiting := 0
	nextWaiting = addReadyQueuePositionItems(scheduler, waiting, nextWaiting, now)
	workerAvailable := queueWorkerAvailability(workers, running, duration, now)
	queuedAhead := 0
	for scheduler.pending > 0 || nextWaiting < len(waiting) {
		worker := earliestQueueWorker(workerAvailable)
		slotAt := workerAvailable[worker]
		if nextWaiting < len(waiting) {
			nextWaiting = addReadyQueuePositionItems(scheduler, waiting, nextWaiting, slotAt)
		}
		if scheduler.pending == 0 {
			if waiting[nextWaiting].EligibleAt.After(slotAt) {
				slotAt = waiting[nextWaiting].EligibleAt
			}
			nextWaiting = addReadyQueuePositionItems(
				scheduler,
				waiting,
				nextWaiting,
				slotAt,
			)
		}
		item, ok := scheduler.next()
		if !ok {
			break
		}
		ahead := running + queuedAhead
		estimated := slotAt
		if estimated.Before(item.EligibleAt) {
			estimated = item.EligibleAt
		}
		positions[item.ID] = queuePosition{ahead: ahead, estimated: estimated}
		workerAvailable[worker] = estimated.Add(duration)
		queuedAhead++
	}
}

func queueWorkerAvailability(
	workers int,
	running int,
	duration time.Duration,
	now time.Time,
) []time.Time {
	available := make([]time.Time, workers)
	for index := range available {
		available[index] = now
		if index < running {
			available[index] = now.Add(duration)
		}
	}

	return available
}

func earliestQueueWorker(available []time.Time) int {
	earliest := 0
	for index := 1; index < len(available); index++ {
		if available[index].Before(available[earliest]) {
			earliest = index
		}
	}

	return earliest
}

func addReadyQueuePositionItems(
	scheduler *queuePositionScheduler,
	waiting []workqueue.Item,
	next int,
	at time.Time,
) int {
	for next < len(waiting) && !waiting[next].EligibleAt.After(at) {
		scheduler.add(waiting[next])
		next++
	}

	return next
}

type queuePositionScheduler struct {
	state      queueDispatchState
	immediate  queuePositionBand
	priorities map[workqueue.Priority]*queuePositionBand
	pending    int
}

func newQueuePositionScheduler(state queueDispatchState) *queuePositionScheduler {
	return &queuePositionScheduler{
		state: state,
		priorities: map[workqueue.Priority]*queuePositionBand{
			workqueue.PriorityUrgent: {},
			workqueue.PriorityHigh:   {},
			workqueue.PriorityNormal: {},
			workqueue.PriorityLow:    {},
		},
	}
}

func (s *queuePositionScheduler) add(item workqueue.Item) {
	if item.Immediate {
		s.immediate.add(item)
	} else {
		s.priorities[item.Priority].add(item)
	}
	s.pending++
}

func (s *queuePositionScheduler) next() (workqueue.Item, bool) {
	if s.immediate.count > 0 {
		item := s.immediate.pop(s.state.targetCursor)
		s.state.targetCursor = queueItemTarget(item)
		s.pending--

		return item, true
	}
	for offset := range priorityCycle {
		index := (s.state.priorityCursor + offset) % len(priorityCycle)
		band := s.priorities[priorityCycle[index]]
		if band.count == 0 {
			continue
		}
		item := band.pop(s.state.targetCursor)
		s.state.priorityCursor = (index + 1) % len(priorityCycle)
		s.state.targetCursor = queueItemTarget(item)
		s.pending--

		return item, true
	}

	return workqueue.Item{}, false
}

type queuePositionBand struct {
	byTarget map[string][]workqueue.Item
	targets  []string
	count    int
}

func (b *queuePositionBand) add(item workqueue.Item) {
	if b.byTarget == nil {
		b.byTarget = make(map[string][]workqueue.Item)
	}
	target := queueItemTarget(item)
	if _, found := b.byTarget[target]; !found {
		index := sort.SearchStrings(b.targets, target)
		b.targets = append(b.targets, "")
		copy(b.targets[index+1:], b.targets[index:])
		b.targets[index] = target
	}
	b.byTarget[target] = append(b.byTarget[target], item)
	b.count++
}

func (b *queuePositionBand) pop(previous string) workqueue.Item {
	index := sort.Search(len(b.targets), func(index int) bool {
		return b.targets[index] > previous
	})
	if index == len(b.targets) {
		index = 0
	}
	target := b.targets[index]
	queued := b.byTarget[target]
	item := queued[0]
	if len(queued) == 1 {
		delete(b.byTarget, target)
		b.targets = append(b.targets[:index], b.targets[index+1:]...)
	} else {
		b.byTarget[target] = queued[1:]
	}
	b.count--

	return item
}

func sortQueuePositionItems(items []workqueue.Item) {
	sort.Slice(items, func(left, right int) bool {
		if !items[left].EligibleAt.Equal(items[right].EligibleAt) {
			return items[left].EligibleAt.Before(items[right].EligibleAt)
		}
		if !items[left].CreatedAt.Equal(items[right].CreatedAt) {
			return items[left].CreatedAt.Before(items[right].CreatedAt)
		}

		return items[left].ID < items[right].ID
	})
}

func queueItemTarget(item workqueue.Item) string {
	if item.TargetID == nil {
		return ""
	}

	return *item.TargetID
}

func (s *Store) GetQueueItem(ctx context.Context, id string) (workqueue.Item, error) {
	item, err := getQueueItem(ctx, s.db, id, "")
	if errors.Is(err, sql.ErrNoRows) {
		return workqueue.Item{}, storage.ErrNotFound
	}
	if err != nil {
		return workqueue.Item{}, fmt.Errorf("get queue item: %w", err)
	}
	/* Decorated through the slice and returned FROM it. Passing a literal and
	   returning `item` handed the caller the row as it was read: the window it
	   waits in, the work ahead of it and the repository it is about are all
	   written onto the slice's own copy. */
	decorated := []workqueue.Item{item}
	if err := s.addQueuePositions(ctx, decorated); err != nil {
		return workqueue.Item{}, err
	}
	if err := s.nameQueueSubjects(ctx, decorated); err != nil {
		return workqueue.Item{}, err
	}

	return decorated[0], nil
}

func getQueueItem(
	ctx context.Context,
	runner runner,
	id string,
	lock string,
) (workqueue.Item, error) {
	return scanQueueItem(runner.QueryRowContext(ctx, "SELECT"+queueItemColumns+`
FROM queue_items WHERE id = ?`+lock, id))
}

func scanQueueItem(scanner rowScanner) (workqueue.Item, error) {
	var item workqueue.Item
	var targetID, repositoryID, profileID, requestedBy sql.NullString
	var details string
	var notBefore, cadenceAnchor, eligibleAt, estimated, lease, created, updated, started, finished StoredTime
	if err := scanner.Scan(
		&item.ID, &item.Kind, &item.Lane, &targetID, &repositoryID,
		&item.SourceKind, &item.SourceID, &item.Title, &item.Summary,
		&item.State, &item.Priority, &item.PriorityOverride,
		&item.WindowMode, &item.Immediate, &profileID,
		&notBefore, &cadenceAnchor, &eligibleAt, &estimated, &item.BlockedReason,
		&item.ProgressCurrent, &item.ProgressTotal, &item.Attempt, &lease,
		&requestedBy, &item.Reason, &details, &item.Revision,
		&created, &updated, &started, &finished,
	); err != nil {
		return workqueue.Item{}, err
	}
	item.TargetID, item.RepositoryID = stringPointer(targetID), stringPointer(repositoryID)
	item.ProfileID, item.RequestedBy = stringPointer(profileID), stringPointer(requestedBy)
	item.Details = json.RawMessage(details)
	item.NotBefore, item.EligibleAt = notBefore.Time(), eligibleAt.Time()
	item.CadenceAnchorAt = cadenceAnchor.Pointer()
	item.EstimatedStartAt, item.LeaseExpiresAt = estimated.Pointer(), lease.Pointer()
	item.CreatedAt, item.UpdatedAt = created.Time(), updated.Time()
	item.StartedAt, item.FinishedAt = started.Pointer(), finished.Pointer()

	return item, nil
}

func (s *Store) CreateQueueItem(ctx context.Context, item workqueue.Item) (workqueue.Item, error) {
	if err := validateQueueItem(item); err != nil {
		return workqueue.Item{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Item{}, fmt.Errorf("begin queue item create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	item.Revision = 1
	if len(item.Details) == 0 {
		item.Details = json.RawMessage(`{}`)
	}
	if err := insertQueueItem(ctx, tx, item); err != nil {
		if s.dialect.UniqueViolation(err) {
			return workqueue.Item{}, storage.ErrConflict
		}
		return workqueue.Item{}, err
	}
	if err := insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: item.ID, ActorID: item.RequestedBy, Kind: queueEventCreated, State: item.State,
		Summary: "Queued " + item.Title, CreatedAt: item.CreatedAt,
	}); err != nil {
		return workqueue.Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return workqueue.Item{}, fmt.Errorf("commit queue item create: %w", err)
	}

	return item, nil
}

func validateQueueItem(item workqueue.Item) error {
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Title) == "" {
		return errors.New("queue item id and title are required")
	}
	if !item.Kind.Valid() || !item.Priority.Valid() {
		return errors.New("queue item kind or priority is invalid")
	}
	if item.NotBefore.IsZero() || item.EligibleAt.IsZero() || item.CreatedAt.IsZero() {
		return errors.New("queue item schedule is required")
	}
	if item.WindowMode == workqueue.WindowRespect && item.ProfileID == nil {
		return errors.New("windowed queue item needs a profile")
	}

	return nil
}

func insertQueueItem(ctx context.Context, tx *transaction, item workqueue.Item) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO queue_items (`+queueItemColumns+`)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Kind, item.Lane, item.TargetID, item.RepositoryID,
		item.SourceKind, item.SourceID, item.Title, item.Summary, item.State,
		item.Priority, item.PriorityOverride, item.WindowMode, item.Immediate, item.ProfileID,
		item.NotBefore, item.CadenceAnchorAt, item.EligibleAt, item.EstimatedStartAt, item.BlockedReason,
		item.ProgressCurrent, item.ProgressTotal, item.Attempt, item.LeaseExpiresAt,
		item.RequestedBy, item.Reason, string(item.Details), item.Revision,
		item.CreatedAt, item.UpdatedAt, item.StartedAt, item.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("insert queue item: %w", err)
	}

	return nil
}

func (s *Store) ListQueueEvents(
	ctx context.Context,
	itemID string,
	limit int,
) ([]workqueue.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, queue_item_id, actor_account_id, kind, state, summary, details, created_at
FROM (
  SELECT id, queue_item_id, actor_account_id, kind, state, summary, details, created_at
  FROM queue_events WHERE queue_item_id = ? ORDER BY id DESC LIMIT ?
) recent ORDER BY id`, itemID, queueEventLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list queue events: %w", err)
	}
	events, err := collectRows(rows, scanQueueEvent)
	if err != nil {
		return nil, fmt.Errorf("read queue events: %w", err)
	}

	return events, nil
}

func queueEventLimit(limit int) int {
	if limit <= 0 {
		return 100
	}

	return min(limit, 500)
}

func scanQueueEvent(scanner rowScanner) (workqueue.Event, error) {
	var event workqueue.Event
	var actor sql.NullString
	var details string
	var created StoredTime
	if err := scanner.Scan(
		&event.ID, &event.ItemID, &actor, &event.Kind, &event.State,
		&event.Summary, &details, &created,
	); err != nil {
		return workqueue.Event{}, err
	}
	event.ActorID, event.CreatedAt = stringPointer(actor), created.Time()
	event.Actor = queueActorSystem
	if event.ActorID != nil {
		event.Actor = *event.ActorID
	}
	event.Details = json.RawMessage(details)

	return event, nil
}

func insertQueueEvent(ctx context.Context, runner runner, event workqueue.Event) error {
	details := event.Details
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}
	_, err := runner.ExecContext(ctx, `
INSERT INTO queue_events (
    queue_item_id, actor_account_id, kind, state, summary, details, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.ItemID, event.ActorID, event.Kind, event.State,
		event.Summary, string(details), event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert queue event: %w", err)
	}

	return nil
}
