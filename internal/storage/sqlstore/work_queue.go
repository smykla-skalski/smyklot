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
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM queue_items"+where, arguments...).Scan(&total); err != nil {
		return workqueue.Page{}, fmt.Errorf("count queue items: %w", err)
	}
	limit := pageLimit(filter.Limit)
	offset := max(filter.Offset, 0)
	arguments = append(arguments, limit+1, offset)
	rows, err := s.db.QueryContext(ctx, "SELECT"+queueItemColumns+`
FROM queue_items`+where+`
ORDER BY CASE WHEN finished_at IS NULL THEN 0 ELSE 1 END,
         CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END,
         eligible_at, updated_at DESC, id
LIMIT ? OFFSET ?`, arguments...)
	if err != nil {
		return workqueue.Page{}, fmt.Errorf("list queue items: %w", err)
	}
	items, err := collectRows(rows, scanQueueItem)
	if err != nil {
		return workqueue.Page{}, fmt.Errorf("read queue items: %w", err)
	}
	next := 0
	if len(items) > limit {
		items = items[:limit]
		next = offset + limit
	}
	if err := s.addQueuePositions(ctx, items); err != nil {
		return workqueue.Page{}, err
	}
	facets, err := s.queueFacets(ctx, filter.TargetID)
	if err != nil {
		return workqueue.Page{}, err
	}

	return workqueue.Page{Items: items, NextOffset: next, Total: total, Facets: facets}, nil
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

	return clauses, arguments
}

func (s *Store) queueFacets(ctx context.Context, targetID *string) (workqueue.Facets, error) {
	facets := workqueue.Facets{
		Targets: []string{}, Repositories: []string{}, Profiles: []string{},
		States: []workqueue.State{}, Kinds: []workqueue.Kind{},
		Priorities: []workqueue.Priority{},
	}
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
	profiles := make(map[string]workqueue.Profile)
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
	positions := make(map[string]queuePosition, len(items))
	for _, item := range items {
		positions[item.ID] = estimateQueuePosition(items, item, state, duration, now)
	}

	return positions, nil
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
	if target.State == workqueue.StateRunning && target.LeaseExpiresAt != nil &&
		target.LeaseExpiresAt.After(now) {
		estimated := now
		if target.StartedAt != nil {
			estimated = *target.StartedAt
		}

		return queuePosition{estimated: estimated}
	}
	running := 0
	remaining := make([]workqueue.Item, 0, len(all))
	for _, item := range all {
		if item.ID != target.ID && item.State == workqueue.StateRunning &&
			item.LeaseExpiresAt != nil && item.LeaseExpiresAt.After(now) {
			running++
			continue
		}
		if item.Immediate || !item.EligibleAt.After(target.EligibleAt) {
			remaining = append(remaining, item)
		}
	}
	ahead := running
	for len(remaining) > 0 {
		choice, ok := chooseQueueDispatch(remaining, state)
		if !ok {
			break
		}
		if choice.item.ID == target.ID {
			break
		}
		ahead++
		state.priorityCursor = choice.nextCursor
		if choice.item.TargetID == nil {
			state.targetCursor = ""
		} else {
			state.targetCursor = *choice.item.TargetID
		}
		remaining = removeQueueItem(remaining, choice.item.ID)
	}
	estimated := target.EligibleAt
	if estimated.Before(now) {
		estimated = now
	}
	estimated = estimated.Add(time.Duration(ahead/target.Lane.Workers()) * duration)

	return queuePosition{ahead: ahead, estimated: estimated}
}

func removeQueueItem(items []workqueue.Item, id string) []workqueue.Item {
	for index := range items {
		if items[index].ID == id {
			return append(items[:index], items[index+1:]...)
		}
	}

	return items
}

func (s *Store) GetQueueItem(ctx context.Context, id string) (workqueue.Item, error) {
	item, err := getQueueItem(ctx, s.db, id, "")
	if errors.Is(err, sql.ErrNoRows) {
		return workqueue.Item{}, storage.ErrNotFound
	}
	if err != nil {
		return workqueue.Item{}, fmt.Errorf("get queue item: %w", err)
	}
	if err := s.addQueuePositions(ctx, []workqueue.Item{item}); err != nil {
		return workqueue.Item{}, err
	}

	return item, nil
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
