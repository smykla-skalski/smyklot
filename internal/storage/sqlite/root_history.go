package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const rootAuditSelect = `
SELECT
    event.id, event.category, event.target_id, event.elevation_id,
    event.action, event.summary, event.created_at,
    actor.id, actor.provider, actor.subject_id, actor.login,
    actor.display_name, actor.avatar_url, actor.updated_at,
    target_account.id, target_account.provider, target_account.subject_id,
    target_account.login, target_account.display_name, target_account.avatar_url,
    target_account.updated_at,
    subject.id, subject.provider, subject.subject_id, subject.login,
    subject.display_name, subject.avatar_url, subject.updated_at
FROM app_audit_events event
JOIN accounts actor ON actor.id = event.actor_account_id
LEFT JOIN targets target ON target.id = event.target_id
LEFT JOIN accounts target_account ON target_account.id = target.account_id
LEFT JOIN accounts subject ON subject.id = event.subject_account_id`

// ListRootAudit returns filtered application-wide audit history.
func (s *Store) ListRootAudit(
	ctx context.Context,
	page storage.RootAuditPageRequest,
) (storage.RootAuditPage, error) {
	limit := pageLimit(page.Limit)
	clauses, arguments, err := rootAuditFilters(page)
	if err != nil {
		return storage.RootAuditPage{}, err
	}
	total, err := countHistory(ctx, s.db, rootAuditSelect, clauses, arguments)
	if err != nil {
		return storage.RootAuditPage{}, fmt.Errorf("count Root audit events: %w", err)
	}
	order, err := rootAuditOrder(page.Order)
	if err != nil {
		return storage.RootAuditPage{}, err
	}
	offset := max(page.Offset, 0)
	arguments = append(arguments, limit+1, offset)
	query := rootAuditSelect + " WHERE " + strings.Join(clauses, " AND ") +
		" ORDER BY " + order + " LIMIT ? OFFSET ?" // #nosec G202 -- fixed clauses and order.
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return storage.RootAuditPage{}, fmt.Errorf("list Root audit events: %w", err)
	}
	items, err := collectRows(rows, scanRootAuditEvent)
	if err != nil {
		return storage.RootAuditPage{}, fmt.Errorf("read Root audit events: %w", err)
	}

	return rootAuditPage(items, limit, total, offset), nil
}

func rootAuditFilters(page storage.RootAuditPageRequest) ([]string, []any, error) {
	clauses := []string{"1 = 1"}
	arguments := []any{}
	if page.Query != "" {
		clauses = append(clauses, `(instr(lower(event.action), lower(?)) > 0
OR instr(lower(event.summary), lower(?)) > 0
OR instr(lower(actor.login), lower(?)) > 0
OR instr(lower(actor.display_name), lower(?)) > 0
OR instr(lower(COALESCE(target_account.login, '')), lower(?)) > 0
OR instr(lower(COALESCE(target_account.display_name, '')), lower(?)) > 0
OR instr(lower(COALESCE(subject.login, '')), lower(?)) > 0
OR instr(lower(COALESCE(event.elevation_id, '')), lower(?)) > 0)`)
		for range 8 {
			arguments = append(arguments, page.Query)
		}
	}
	if len(page.Categories) > 0 {
		placeholders := make([]string, 0, len(page.Categories))
		for _, category := range page.Categories {
			if !validAuditCategory(category) {
				return nil, nil, fmt.Errorf("unsupported Root audit category %q", category)
			}
			placeholders = append(placeholders, "?")
			arguments = append(arguments, category)
		}
		clauses = append(clauses, "event.category IN ("+strings.Join(placeholders, ",")+")")
	}
	if page.TargetID != nil {
		clauses = append(clauses, "event.target_id = ?")
		arguments = append(arguments, *page.TargetID)
	}

	return clauses, arguments, nil
}

func rootAuditOrder(order storage.HistoryOrder) (string, error) {
	switch order {
	case "", storage.HistoryNewest:
		return "event.id DESC", nil
	case storage.HistoryOldest:
		return "event.id ASC", nil
	case storage.HistoryActorAscending:
		return "lower(actor.display_name) ASC, event.id DESC", nil
	case storage.HistoryActorDescending:
		return "lower(actor.display_name) DESC, event.id DESC", nil
	case storage.HistoryTargetAscending:
		return "lower(COALESCE(target_account.display_name, 'Smyklot')) ASC, event.id DESC", nil
	case storage.HistoryTargetDescending:
		return "lower(COALESCE(target_account.display_name, 'Smyklot')) DESC, event.id DESC", nil
	case storage.HistoryChangeAscending:
		return "lower(event.summary) ASC, event.id DESC", nil
	case storage.HistoryChangeDescending:
		return "lower(event.summary) DESC, event.id DESC", nil
	default:
		return "", fmt.Errorf("unsupported Root audit order %q", order)
	}
}

func validAuditCategory(category storage.AuditCategory) bool {
	switch category {
	case storage.AuditCategoryConfiguration, storage.AuditCategoryAccess,
		storage.AuditCategoryOwnership, storage.AuditCategoryElevation,
		storage.AuditCategoryNotification, storage.AuditCategoryRuntime:
		return true
	default:
		return false
	}
}

func scanRootAuditEvent(scanner rowScanner) (storage.AppAuditEvent, error) {
	var event storage.AppAuditEvent
	var targetID, elevationID sql.NullString
	var actorAvatar, targetAvatar, subjectAvatar sql.NullString
	var targetIDValue, targetProvider, targetSubject, targetLogin, targetName sql.NullString
	var targetUpdated, subjectID, subjectProvider, subjectSubject sql.NullString
	var subjectLogin, subjectName, subjectUpdated sql.NullString
	var createdAt, actorUpdated string
	if err := scanner.Scan(
		&event.ID, &event.Category, &targetID, &elevationID,
		&event.Action, &event.Summary, &createdAt,
		&event.Actor.ID, &event.Actor.Provider, &event.Actor.SubjectID, &event.Actor.Login,
		&event.Actor.DisplayName, &actorAvatar, &actorUpdated,
		&targetIDValue, &targetProvider, &targetSubject, &targetLogin, &targetName,
		&targetAvatar, &targetUpdated,
		&subjectID, &subjectProvider, &subjectSubject, &subjectLogin, &subjectName,
		&subjectAvatar, &subjectUpdated,
	); err != nil {
		return storage.AppAuditEvent{}, err
	}
	event.ElevationID = stringPointer(elevationID)
	event.Actor.AvatarURL = stringPointer(actorAvatar)
	var err error
	event.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return storage.AppAuditEvent{}, err
	}
	event.Actor.UpdatedAt, err = parseTime(actorUpdated)
	if err != nil {
		return storage.AppAuditEvent{}, err
	}
	if targetID.Valid && targetIDValue.Valid {
		target, targetErr := nullableAuditAccount(
			targetIDValue, targetProvider, targetSubject, targetLogin, targetName,
			targetAvatar, targetUpdated,
		)
		if targetErr != nil {
			return storage.AppAuditEvent{}, targetErr
		}
		event.Target = target
	}
	if subjectID.Valid {
		subject, subjectErr := nullableAuditAccount(
			subjectID, subjectProvider, subjectSubject, subjectLogin, subjectName,
			subjectAvatar, subjectUpdated,
		)
		if subjectErr != nil {
			return storage.AppAuditEvent{}, subjectErr
		}
		event.Subject = subject
	}

	return event, nil
}

func nullableAuditAccount(
	id, provider, subject, login, name, avatar, updated sql.NullString,
) (*storage.Account, error) {
	account := &storage.Account{
		ID: id.String, Provider: provider.String, SubjectID: subject.String,
		Login: login.String, DisplayName: name.String, AvatarURL: stringPointer(avatar),
	}
	var err error
	account.UpdatedAt, err = parseTime(updated.String)

	return account, err
}

func rootAuditPage(
	items []storage.AppAuditEvent,
	limit, total, offset int,
) storage.RootAuditPage {
	page := storage.RootAuditPage{Items: items, Total: total}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextOffset = offset + limit
	}

	return page
}

const rootFailureSelect = `
SELECT
    d.id, d.delivery_id, d.target_id, d.repository_full_name, d.event,
    d.stage, d.reason, d.retryable, d.finished_at,
    a.id, a.provider, a.subject_id, a.login, a.display_name, a.avatar_url, a.updated_at
FROM deliveries d
JOIN targets t ON t.id = d.target_id
JOIN accounts a ON a.id = t.account_id`

// ListRootFailures returns filtered delivery failures across all installations.
func (s *Store) ListRootFailures(
	ctx context.Context,
	page storage.FailurePageRequest,
) (storage.RootFailurePage, error) {
	limit := pageLimit(page.Limit)
	clauses, arguments := rootFailureFilters(page)
	total, err := countHistory(ctx, s.db, rootFailureSelect, clauses, arguments)
	if err != nil {
		return storage.RootFailurePage{}, fmt.Errorf("count Root failures: %w", err)
	}
	order, err := rootFailureOrder(page.Order)
	if err != nil {
		return storage.RootFailurePage{}, err
	}
	offset := max(page.Offset, 0)
	arguments = append(arguments, limit+1, offset)
	query := rootFailureSelect + " WHERE " + strings.Join(clauses, " AND ") +
		" ORDER BY " + order + " LIMIT ? OFFSET ?" // #nosec G202 -- fixed clauses and order.
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return storage.RootFailurePage{}, fmt.Errorf("list Root failures: %w", err)
	}
	items, err := collectRows(rows, scanRootFailure)
	if err != nil {
		return storage.RootFailurePage{}, fmt.Errorf("read Root failures: %w", err)
	}
	result := storage.RootFailurePage{Items: items, Total: total}
	if len(items) > limit {
		result.Items = items[:limit]
		result.NextOffset = offset + limit
	}

	return result, nil
}

func rootFailureOrder(order storage.HistoryOrder) (string, error) {
	switch order {
	case "", storage.HistoryNewest:
		return "d.id DESC", nil
	case storage.HistoryOldest:
		return "d.id ASC", nil
	case storage.HistoryStatusAscending:
		return "d.retryable ASC, d.id DESC", nil
	case storage.HistoryStatusDescending:
		return "d.retryable DESC, d.id DESC", nil
	case storage.HistoryRepositoryAscending:
		return "d.repository_full_name COLLATE NOCASE ASC, d.id DESC", nil
	case storage.HistoryRepositoryDescending:
		return "d.repository_full_name COLLATE NOCASE DESC, d.id DESC", nil
	default:
		return "", fmt.Errorf("unsupported Root failure order %q", order)
	}
}

func rootFailureFilters(page storage.FailurePageRequest) ([]string, []any) {
	clauses := []string{"d.status = ?"}
	arguments := []any{storage.DeliveryFailed}
	if page.Query != "" {
		clauses = append(clauses, `(instr(lower(d.delivery_id), lower(?)) > 0
OR instr(lower(d.repository_full_name), lower(?)) > 0
OR instr(lower(d.event), lower(?)) > 0
OR instr(lower(d.stage), lower(?)) > 0
OR instr(lower(d.reason), lower(?)) > 0
OR instr(lower(a.login), lower(?)) > 0
OR instr(lower(a.display_name), lower(?)) > 0)`)
		for range 7 {
			arguments = append(arguments, page.Query)
		}
	}
	if page.Retryable != nil {
		clauses = append(clauses, "d.retryable = ?")
		arguments = append(arguments, *page.Retryable)
	}

	return clauses, arguments
}
