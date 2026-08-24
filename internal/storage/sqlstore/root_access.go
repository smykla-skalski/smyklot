package sqlstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// UpdateSystemRole promotes or demotes a non-Super-Root account atomically.
func (s *Store) UpdateSystemRole(
	ctx context.Context,
	change storage.SystemRoleChange,
) (storage.PanelUser, error) {
	if change.SystemRole != storage.SystemRoleNone && change.SystemRole != storage.SystemRoleRoot {
		return storage.PanelUser{}, errors.New("unsupported system role")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("begin system role update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := getPanelUser(ctx, tx, change.AccountID)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("read system role subject: %w", noRows(err))
	}
	if current.Revision != change.ExpectedRevision || current.SystemRole == storage.SystemRoleSuperRoot ||
		current.Status != storage.PanelUserActive {
		return storage.PanelUser{}, storage.ErrConflict
	}
	result, err := tx.ExecContext(ctx, `
UPDATE panel_users
SET system_role = ?, revision = revision + 1, updated_at = ?
WHERE account_id = ? AND revision = ?`, change.SystemRole,
		change.ChangedAt, change.AccountID, change.ExpectedRevision)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("update system role: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return storage.PanelUser{}, storage.ErrConflict
	}
	if err := insertAccessAudit(
		ctx, tx, nil, change.ActorAccountID, change.AccountID, "system_role.changed",
		"changed system role to "+string(change.SystemRole), change.ChangedAt,
	); err != nil {
		return storage.PanelUser{}, err
	}
	updated, err := getPanelUser(ctx, tx, change.AccountID)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("read updated system role: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return storage.PanelUser{}, fmt.Errorf("commit system role update: %w", err)
	}

	return updated, nil
}

// ListRootPanelUserPage returns accounts across the whole application,
// including soft-removed identities retained for audit continuity.
func (s *Store) ListRootPanelUserPage(
	ctx context.Context,
	page storage.RootPanelUserPageRequest,
) (storage.RootPanelUserPage, error) {
	clauses, arguments, err := rootPanelUserFilters(page)
	if err != nil {
		return storage.RootPanelUserPage{}, err
	}
	order, err := rootPanelUserOrderSQL(page.Order)
	if err != nil {
		return storage.RootPanelUserPage{}, err
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM panel_users pu
JOIN accounts a ON a.id = pu.account_id`+where, arguments...).Scan(&total); err != nil {
		return storage.RootPanelUserPage{}, fmt.Errorf("count Root panel users: %w", err)
	}
	// #nosec G202 -- clauses and ordering are assembled only from fixed internal constants.
	rows, err := s.db.QueryContext(ctx, `
SELECT
    pu.account_id,
    (SELECT COUNT(*) FROM target_owners own WHERE own.account_id = pu.account_id),
    (SELECT COUNT(*) FROM target_roles role
      WHERE role.account_id = pu.account_id AND role.role IS NOT NULL AND role.role <> 'none')
FROM panel_users pu
JOIN accounts a ON a.id = pu.account_id`+where+`
ORDER BY `+order+`
LIMIT ? OFFSET ?`, append(arguments, pageLimit(page.Limit)+1, max(page.Offset, 0))...)
	if err != nil {
		return storage.RootPanelUserPage{}, fmt.Errorf("list Root panel users: %w", err)
	}
	items, err := collectRows(rows, scanRootPanelUserCounts)
	if err != nil {
		return storage.RootPanelUserPage{}, fmt.Errorf("read Root panel users: %w", err)
	}
	nextOffset := 0
	limit := pageLimit(page.Limit)
	if len(items) > limit {
		items = items[:limit]
		nextOffset = max(page.Offset, 0) + limit
	}
	for index := range items {
		user, readErr := getPanelUser(ctx, s.db, items[index].User.Account.ID)
		if readErr != nil {
			return storage.RootPanelUserPage{}, fmt.Errorf("read Root panel user: %w", readErr)
		}
		items[index].User = user
	}

	return storage.RootPanelUserPage{Items: items, NextOffset: nextOffset, Total: total}, nil
}

func rootPanelUserFilters(
	page storage.RootPanelUserPageRequest,
) ([]string, []any, error) {
	clauses := []string{queryAllRows}
	arguments := make([]any, 0)
	if page.Query != "" {
		clauses = append(clauses, containsAnyClause("a.login", "a.display_name"))
		arguments = append(arguments, containsArguments(page.Query, 2)...)
	}
	if len(page.SystemRoles) > 0 {
		values := make([]string, 0, len(page.SystemRoles))
		for _, role := range page.SystemRoles {
			if role != storage.SystemRoleNone && role != storage.SystemRoleRoot &&
				role != storage.SystemRoleSuperRoot {
				return nil, nil, fmt.Errorf("unsupported system role %q", role)
			}
			values = append(values, "?")
			arguments = append(arguments, role)
		}
		clauses = append(clauses, "pu.system_role IN ("+strings.Join(values, ", ")+")")
	}
	if len(page.Statuses) > 0 {
		values := make([]string, 0, len(page.Statuses))
		for _, status := range page.Statuses {
			if !validPanelUserStatus(status) {
				return nil, nil, fmt.Errorf("unsupported panel user status %q", status)
			}
			values = append(values, "?")
			arguments = append(arguments, status)
		}
		clauses = append(clauses, "pu.status IN ("+strings.Join(values, ", ")+")")
	}

	return clauses, arguments, nil
}

func rootPanelUserOrderSQL(order storage.RootPanelUserOrder) (string, error) {
	roleLevel := `(CASE pu.system_role
WHEN 'none' THEN 0
WHEN 'root' THEN 1
WHEN 'super_root' THEN 2
ELSE -1 END)`
	switch order {
	case "", storage.RootPanelUserNameAscending:
		return "lower(a.display_name) ASC, lower(a.login) ASC, a.id ASC", nil
	case storage.RootPanelUserNameDescending:
		return "lower(a.display_name) DESC, lower(a.login) DESC, a.id DESC", nil
	case storage.RootPanelUserRoleAscending:
		return roleLevel + " ASC, lower(a.display_name) ASC, a.id ASC", nil
	case storage.RootPanelUserRoleDescending:
		return roleLevel + " DESC, lower(a.display_name) ASC, a.id ASC", nil
	case storage.RootPanelUserLoginNewest:
		return "(pu.last_login_at IS NULL) ASC, pu.last_login_at DESC, a.id DESC", nil
	case storage.RootPanelUserLoginOldest:
		return "(pu.last_login_at IS NULL) ASC, pu.last_login_at ASC, a.id ASC", nil
	default:
		return "", fmt.Errorf("unsupported Root panel user order %q", order)
	}
}

func scanRootPanelUserCounts(scanner rowScanner) (storage.RootPanelUser, error) {
	var item storage.RootPanelUser
	err := scanner.Scan(
		&item.User.Account.ID,
		&item.OwnedInstallationCount,
		&item.AssignedInstallationCount,
	)

	return item, err
}
