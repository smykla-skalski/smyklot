package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const auditSelect = `
SELECT
    ae.id,
    ae.target_id,
    ae.repository_id,
    ae.repository_full_name,
    ae.action,
    ae.summary,
    ae.created_at,
    a.id,
    a.provider,
    a.subject_id,
    a.login,
    a.display_name,
    a.avatar_url,
    a.updated_at
FROM audit_entries ae
JOIN accounts a ON a.id = ae.actor_account_id`

// ListAudit returns one filtered page of immutable target history.
func (s *Store) ListAudit(
	ctx context.Context,
	targetID string,
	page storage.AuditPageRequest,
) (storage.AuditPage, error) {
	limit := pageLimit(page.Limit)
	clauses, arguments, err := auditFilters(targetID, page)
	if err != nil {
		return storage.AuditPage{}, err
	}
	total, err := countHistory(ctx, s.db, auditSelect, clauses, arguments)
	if err != nil {
		return storage.AuditPage{}, fmt.Errorf("count audit entries: %w", err)
	}
	order, err := auditPageOrder(page.Order)
	if err != nil {
		return storage.AuditPage{}, err
	}
	offset := max(page.Offset, 0)
	arguments = append(arguments, limit+1, offset)
	// #nosec G202 -- clauses and direction come only from fixed internal constants;
	// every request value remains a bound parameter.
	query := auditSelect + " WHERE " + strings.Join(clauses, " AND ") +
		" ORDER BY " + order + " LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return storage.AuditPage{}, fmt.Errorf("list audit entries: %w", err)
	}

	items, err := collectRows(rows, scanAuditEntry)
	if err != nil {
		return storage.AuditPage{}, fmt.Errorf("read audit entries: %w", err)
	}

	return auditPage(items, limit, total, offset), nil
}

func auditPageOrder(order storage.HistoryOrder) (string, error) {
	switch order {
	case "", storage.HistoryNewest:
		return "ae.id DESC", nil
	case storage.HistoryOldest:
		return "ae.id ASC", nil
	case storage.HistoryActorAscending:
		return "lower(a.display_name) ASC, lower(a.login) ASC, ae.id DESC", nil
	case storage.HistoryActorDescending:
		return "lower(a.display_name) DESC, lower(a.login) DESC, ae.id DESC", nil
	case storage.HistoryTargetAscending:
		return "lower(COALESCE(ae.repository_full_name, 'Account')) ASC, ae.id DESC", nil
	case storage.HistoryTargetDescending:
		return "lower(COALESCE(ae.repository_full_name, 'Account')) DESC, ae.id DESC", nil
	case storage.HistoryChangeAscending:
		return "lower(ae.summary) ASC, lower(ae.action) ASC, ae.id DESC", nil
	case storage.HistoryChangeDescending:
		return "lower(ae.summary) DESC, lower(ae.action) DESC, ae.id DESC", nil
	default:
		return "", fmt.Errorf("unsupported audit order %q", order)
	}
}

func auditFilters(
	targetID string,
	page storage.AuditPageRequest,
) ([]string, []any, error) {
	clauses := []string{"ae.target_id = ?"}
	arguments := []any{targetID}
	if page.Query != "" {
		columns := []string{
			"ae.action",
			"ae.summary",
			"COALESCE(ae.repository_full_name, '')",
			"a.login",
			"a.display_name",
		}
		clauses = append(clauses, containsAnyClause(columns...))
		arguments = append(arguments, containsArguments(page.Query, len(columns))...)
	}
	switch page.Scope {
	case "", storage.AuditAll:
	case storage.AuditAccount:
		clauses = append(clauses, "ae.repository_id IS NULL")
	case storage.AuditRepositories:
		clauses = append(clauses, "ae.repository_id IS NOT NULL")
	default:
		return nil, nil, fmt.Errorf("unsupported audit scope %q", page.Scope)
	}
	switch page.Change {
	case "", storage.AuditChangeAll:
	case storage.AuditChangeEnablement:
		clauses = append(clauses, "ae.action IN ('repository.enabled', 'repository.disabled')")
	case storage.AuditChangeRepository:
		clauses = append(clauses, "ae.action LIKE 'repository.%' AND ae.action NOT IN ('repository.enabled', 'repository.disabled')")
	case storage.AuditChangeAccount:
		clauses = append(clauses, "ae.action LIKE 'target.%'")
	default:
		return nil, nil, fmt.Errorf("unsupported audit change %q", page.Change)
	}

	return clauses, arguments, nil
}

func scanAuditEntry(scanner rowScanner) (storage.AuditEntry, error) {
	var entry storage.AuditEntry
	var repositoryID, repositoryFullName, avatarURL sql.NullString
	var createdAt, accountUpdatedAt StoredTime

	err := scanner.Scan(
		&entry.ID,
		&entry.TargetID,
		&repositoryID,
		&repositoryFullName,
		&entry.Action,
		&entry.Summary,
		&createdAt,
		&entry.Actor.ID,
		&entry.Actor.Provider,
		&entry.Actor.SubjectID,
		&entry.Actor.Login,
		&entry.Actor.DisplayName,
		&avatarURL,
		&accountUpdatedAt,
	)
	if err != nil {
		return storage.AuditEntry{}, err
	}

	entry.RepositoryID = stringPointer(repositoryID)
	entry.RepositoryFullName = stringPointer(repositoryFullName)
	entry.Actor.AvatarURL = stringPointer(avatarURL)

	entry.CreatedAt = createdAt.Time()
	entry.Actor.UpdatedAt = accountUpdatedAt.Time()

	return entry, nil
}

func auditPage(items []storage.AuditEntry, limit, total, offset int) storage.AuditPage {
	page := storage.AuditPage{Items: items, Total: total}
	if len(items) <= limit {
		return page
	}

	page.Items = items[:limit]
	page.NextOffset = offset + limit

	return page
}
