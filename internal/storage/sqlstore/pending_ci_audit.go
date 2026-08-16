package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type pendingCIEventWriter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type pendingCIEventFactory func(
	before pendingci.Request,
	after pendingci.Request,
) []pendingci.Event

func recordPendingCIEvent(
	ctx context.Context,
	writer pendingCIEventWriter,
	event pendingci.Event,
) error {
	_, err := writer.ExecContext(ctx, `
INSERT INTO pending_ci_events (
    request_id, kind, trigger_kind, event_name, event_key,
    delivery_id, state, summary, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.RequestID,
		event.Kind,
		event.Trigger,
		event.EventName,
		event.EventKey,
		event.DeliveryID,
		event.State,
		event.Summary,
		event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("record pending CI audit event: %w", err)
	}

	return nil
}

func (s *Store) ListHistory(
	ctx context.Context,
	filter pendingci.HistoryFilter,
) ([]pendingci.Request, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit == 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, pendingCISelect+`
WHERE lifecycle <> ?
ORDER BY finished_at DESC, id DESC LIMIT ?`, pendingci.LifecycleArmed, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending CI history: %w", err)
	}
	items, err := collectRows(rows, scanPendingCI)
	if err != nil {
		return nil, fmt.Errorf("read pending CI history: %w", err)
	}

	return items, nil
}

func (s *Store) ListEvents(
	ctx context.Context,
	filter pendingci.EventFilter,
) ([]pendingci.Event, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit == 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, request_id, kind, trigger_kind, event_name, event_key,
       delivery_id, state, summary, created_at
FROM (
    SELECT id, request_id, kind, trigger_kind, event_name, event_key,
           delivery_id, state, summary, created_at
    FROM pending_ci_events
    WHERE request_id = ?
    ORDER BY id DESC
    LIMIT ?
) recent
ORDER BY id`, filter.RequestID, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending CI audit events: %w", err)
	}
	items, err := collectRows(rows, scanPendingCIEvent)
	if err != nil {
		return nil, fmt.Errorf("read pending CI audit events: %w", err)
	}

	return items, nil
}

func scanPendingCIEvent(scanner rowScanner) (pendingci.Event, error) {
	var event pendingci.Event
	var createdAt StoredTime
	err := scanner.Scan(
		&event.ID,
		&event.RequestID,
		&event.Kind,
		&event.Trigger,
		&event.EventName,
		&event.EventKey,
		&event.DeliveryID,
		&event.State,
		&event.Summary,
		&createdAt,
	)
	if err != nil {
		return pendingci.Event{}, err
	}
	event.CreatedAt = createdAt.Time()

	return event, nil
}

func pendingCIAuditEvent(
	requestID int64,
	kind pendingci.EventKind,
	trigger pendingci.Trigger,
	state string,
	summary string,
	createdAt time.Time,
) pendingci.Event {
	return pendingci.Event{
		RequestID: requestID, Kind: kind, Trigger: trigger,
		State: state, Summary: summary, CreatedAt: createdAt,
	}
}

func normalizedTrigger(trigger, fallback pendingci.Trigger) pendingci.Trigger {
	if trigger != "" {
		return trigger
	}
	if fallback != "" {
		return fallback
	}

	return pendingci.TriggerFallback
}

func (s *Store) updatePendingCIWithEvents(
	ctx context.Context,
	id int64,
	action string,
	query string,
	events pendingCIEventFactory,
	args ...any,
) (pendingci.Request, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pendingci.Request{}, fmt.Errorf("begin %s: %w", action, err)
	}
	defer func() { _ = tx.Rollback() }()

	before, err := getPendingCIFrom(ctx, tx, id)
	if err != nil {
		return pendingci.Request{}, err
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return pendingci.Request{}, fmt.Errorf("%s: %w", action, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return pendingci.Request{}, fmt.Errorf("read %s result: %w", action, err)
	}
	if changed != 1 {
		return pendingci.Request{}, storage.ErrConflict
	}
	after, err := getPendingCIFrom(ctx, tx, id)
	if err != nil {
		return pendingci.Request{}, err
	}
	for _, event := range events(before, after) {
		if err := recordPendingCIEvent(ctx, tx, event); err != nil {
			return pendingci.Request{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return pendingci.Request{}, fmt.Errorf("commit %s: %w", action, err)
	}

	return after, nil
}

func getPendingCIFrom(
	ctx context.Context,
	reader rowQuerier,
	id int64,
) (pendingci.Request, error) {
	request, err := scanPendingCI(reader.QueryRowContext(
		ctx, pendingCISelect+" WHERE id = ?", id,
	))
	if err != nil {
		return pendingci.Request{}, fmt.Errorf("get pending CI request: %w", noRows(err))
	}

	return request, nil
}
