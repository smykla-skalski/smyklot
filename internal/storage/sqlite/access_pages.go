package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const accessDecisionSelect = `
SELECT
    aa.id,
    aa.target_id,
    aa.action,
    aa.summary,
    aa.created_at,
    actor.id,
    actor.provider,
    actor.subject_id,
    actor.login,
    actor.display_name,
    actor.avatar_url,
    actor.updated_at
FROM access_audit_entries aa
JOIN accounts actor ON actor.id = aa.actor_account_id`

// ListPanelUserPage returns one filtered account-wide user page.
func (s *Store) ListPanelUserPage(
	ctx context.Context,
	page storage.PanelUserPageRequest,
) (storage.PanelUserPage, error) {
	clauses, arguments, err := panelUserPageFilters(page, false)
	if err != nil {
		return storage.PanelUserPage{}, err
	}
	total, err := countPanelUsers(ctx, s.db, "", clauses, arguments)
	if err != nil {
		return storage.PanelUserPage{}, fmt.Errorf("count panel users: %w", err)
	}
	ids, nextOffset, err := listPanelUserIDs(ctx, s.db, "", clauses, arguments, page, false)
	if err != nil {
		return storage.PanelUserPage{}, err
	}
	items := make([]storage.PanelUser, 0, len(ids))
	for _, id := range ids {
		user, readErr := getPanelUser(ctx, s.db, id)
		if readErr != nil {
			return storage.PanelUserPage{}, fmt.Errorf("read panel user page item: %w", readErr)
		}
		items = append(items, user)
	}

	return storage.PanelUserPage{Items: items, NextOffset: nextOffset, Total: total}, nil
}

// ListTargetPanelUserPage returns one filtered installation-scoped user page.
func (s *Store) ListTargetPanelUserPage(
	ctx context.Context,
	targetID string,
	page storage.PanelUserPageRequest,
) (storage.TargetPanelUserPage, error) {
	var available bool
	if err := s.db.QueryRowContext(
		ctx,
		"SELECT available FROM targets WHERE id = ?",
		targetID,
	).Scan(&available); err != nil {
		return storage.TargetPanelUserPage{}, fmt.Errorf("read target user page target: %w", noRows(err))
	}
	if !available {
		return storage.TargetPanelUserPage{}, fmt.Errorf("read target user page target: %w", storage.ErrNotFound)
	}

	clauses, arguments, err := panelUserPageFilters(page, true)
	if err != nil {
		return storage.TargetPanelUserPage{}, err
	}
	join := " LEFT JOIN target_roles tr ON tr.account_id = pu.account_id AND tr.target_id = ?"
	arguments = append([]any{targetID}, arguments...)
	total, err := countPanelUsers(ctx, s.db, join, clauses, arguments)
	if err != nil {
		return storage.TargetPanelUserPage{}, fmt.Errorf("count target panel users: %w", err)
	}
	ids, nextOffset, err := listPanelUserIDs(ctx, s.db, join, clauses, arguments, page, true)
	if err != nil {
		return storage.TargetPanelUserPage{}, err
	}
	items := make([]storage.TargetPanelUser, 0, len(ids))
	for _, id := range ids {
		item, readErr := targetPanelUser(ctx, s.db, id, targetID)
		if readErr != nil {
			return storage.TargetPanelUserPage{}, fmt.Errorf("read target user page item: %w", readErr)
		}
		items = append(items, item)
	}

	return storage.TargetPanelUserPage{Items: items, NextOffset: nextOffset, Total: total}, nil
}

// ListAccessDecisions returns the newest immutable decisions for one identity
// in exactly one global or installation scope.
func (s *Store) ListAccessDecisions(
	ctx context.Context,
	accountID string,
	targetID *string,
	limit int,
) ([]storage.AccessDecision, error) {
	rows, err := s.db.QueryContext(ctx, accessDecisionSelect+`
WHERE aa.subject_account_id = ?
  AND ((aa.target_id IS NULL AND ? IS NULL) OR aa.target_id = ?)
ORDER BY aa.id DESC
LIMIT ?`, accountID, targetID, targetID, pageLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list access decisions: %w", err)
	}
	items, err := collectRows(rows, scanAccessDecision)
	if err != nil {
		return nil, fmt.Errorf("read access decisions: %w", err)
	}

	return items, nil
}

func panelUserPageFilters(
	page storage.PanelUserPageRequest,
	target bool,
) ([]string, []any, error) {
	clauses := []string{"pu.status <> 'removed'"}
	arguments := make([]any, 0)
	if target {
		clauses = append(clauses, "(pu.root = 1 OR pu.global_role <> 'none' OR tr.account_id IS NOT NULL)")
	}
	if page.Query != "" {
		clauses = append(clauses, `(instr(lower(a.login), lower(?)) > 0
OR instr(lower(a.display_name), lower(?)) > 0)`)
		arguments = append(arguments, page.Query, page.Query)
	}
	if len(page.Roles) > 0 {
		roleClauses, roleArguments, err := panelUserRoleFilters(page.Roles, target)
		if err != nil {
			return nil, nil, err
		}
		clauses = append(clauses, "("+strings.Join(roleClauses, " OR ")+")")
		arguments = append(arguments, roleArguments...)
	}
	if len(page.States) > 0 {
		stateClauses, err := panelUserStateFilters(page.States, target)
		if err != nil {
			return nil, nil, err
		}
		if len(stateClauses) == 0 {
			clauses = append(clauses, "0 = 1")
		} else {
			clauses = append(clauses, "("+strings.Join(stateClauses, " OR ")+")")
		}
	}

	return clauses, arguments, nil
}

func panelUserRoleFilters(roles []storage.PanelRole, target bool) ([]string, []any, error) {
	expression := "pu.global_role"
	if target {
		expression = targetEffectiveRoleSQL()
	}
	clauses := make([]string, 0, len(roles))
	arguments := make([]any, 0, len(roles))
	for _, role := range roles {
		if !validGlobalRole(role) {
			return nil, nil, fmt.Errorf("unsupported panel user role %q", role)
		}
		clauses = append(clauses, expression+" = ?")
		arguments = append(arguments, role)
	}

	return clauses, arguments, nil
}

func panelUserStateFilters(states []storage.PanelUserListState, target bool) ([]string, error) {
	clauses := make([]string, 0, len(states))
	for _, state := range states {
		clause, err := panelUserStateFilter(state, target)
		if err != nil {
			return nil, err
		}
		if clause != "" {
			clauses = append(clauses, clause)
		}
	}

	return clauses, nil
}

func panelUserStateFilter(state storage.PanelUserListState, target bool) (string, error) {
	switch state {
	case storage.PanelUserListActive:
		if target {
			return "(pu.status = 'active' AND COALESCE(tr.suspended, 0) = 0)", nil
		}
		return "pu.status = 'active'", nil
	case storage.PanelUserListBanned:
		return "pu.status = 'banned'", nil
	case storage.PanelUserListSuspended:
		if target {
			return "(pu.status = 'active' AND tr.suspended = 1)", nil
		}
		return "", nil
	default:
		return "", fmt.Errorf("unsupported panel user state %q", state)
	}
}

func targetEffectiveRoleSQL() string {
	return `(CASE
WHEN pu.status <> 'active' OR COALESCE(tr.suspended, 0) = 1 THEN 'none'
WHEN pu.root = 1 OR pu.global_role = 'owner' THEN 'owner'
ELSE COALESCE(tr.role, pu.global_role)
END)`
}

func countPanelUsers(
	ctx context.Context,
	queryer rowQuerier,
	join string,
	clauses []string,
	arguments []any,
) (int, error) {
	var total int
	err := queryer.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM panel_users pu
JOIN accounts a ON a.id = pu.account_id`+join+`
WHERE `+strings.Join(clauses, " AND "), arguments...).Scan(&total)

	return total, err
}

func listPanelUserIDs(
	ctx context.Context,
	queryer *sql.DB,
	join string,
	clauses []string,
	arguments []any,
	page storage.PanelUserPageRequest,
	target bool,
) ([]string, int, error) {
	order, err := panelUserOrder(page.Order, target)
	if err != nil {
		return nil, 0, err
	}
	limit := pageLimit(page.Limit)
	offset := max(page.Offset, 0)
	queryArguments := append(append([]any{}, arguments...), limit+1, offset)
	// #nosec G202 -- join, clauses, and order come only from fixed internal constants.
	rows, err := queryer.QueryContext(ctx, `
SELECT pu.account_id
FROM panel_users pu
JOIN accounts a ON a.id = pu.account_id`+join+`
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY `+order+`
LIMIT ? OFFSET ?`, queryArguments...)
	if err != nil {
		return nil, 0, fmt.Errorf("list panel user page: %w", err)
	}
	ids, err := collectRows(rows, func(scanner rowScanner) (string, error) {
		var id string
		err := scanner.Scan(&id)
		return id, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("read panel user page: %w", err)
	}
	nextOffset := 0
	if len(ids) > limit {
		ids = ids[:limit]
		nextOffset = offset + limit
	}

	return ids, nextOffset, nil
}

func panelUserOrder(order storage.PanelUserOrder, target bool) (string, error) {
	role := "pu.global_role"
	if target {
		role = targetEffectiveRoleSQL()
	}
	roleLevel := `(CASE ` + role + `
WHEN 'none' THEN 0
WHEN 'viewer' THEN 1
WHEN 'editor' THEN 2
WHEN 'admin' THEN 3
WHEN 'owner' THEN 4
ELSE -1
END)`
	switch order {
	case "", storage.PanelUserNameAscending:
		return "lower(a.display_name) ASC, lower(a.login) ASC, a.id ASC", nil
	case storage.PanelUserNameDescending:
		return "lower(a.display_name) DESC, lower(a.login) DESC, a.id DESC", nil
	case storage.PanelUserRoleAscending:
		return roleLevel + " ASC, lower(a.display_name) ASC, a.id ASC", nil
	case storage.PanelUserRoleDescending:
		return roleLevel + " DESC, lower(a.display_name) ASC, a.id ASC", nil
	case storage.PanelUserUpdatedNewest:
		return "pu.updated_at DESC, a.id DESC", nil
	case storage.PanelUserUpdatedOldest:
		return "pu.updated_at ASC, a.id ASC", nil
	case storage.PanelUserLoginNewest:
		return "(pu.last_login_at IS NULL) ASC, pu.last_login_at DESC, a.id DESC", nil
	case storage.PanelUserLoginOldest:
		return "(pu.last_login_at IS NULL) ASC, pu.last_login_at ASC, a.id ASC", nil
	default:
		return "", fmt.Errorf("unsupported panel user order %q", order)
	}
}

func targetPanelUser(
	ctx context.Context,
	queryer rowQuerier,
	accountID, targetID string,
) (storage.TargetPanelUser, error) {
	user, err := getPanelUser(ctx, queryer, accountID)
	if err != nil {
		return storage.TargetPanelUser{}, err
	}
	override, err := getTargetAccessOverride(ctx, queryer, accountID, targetID)
	var overridePointer *storage.TargetAccessOverride
	var targetRole sql.NullString
	var suspended bool
	if err == nil {
		overridePointer = &override
		suspended = override.Suspended
		if override.Role != nil {
			targetRole = sql.NullString{String: string(*override.Role), Valid: true}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return storage.TargetPanelUser{}, err
	}
	access := resolvedTargetAccess(user.Root, user.Status, user.GlobalRole, targetRole, suspended)
	if overridePointer != nil {
		access.SuspensionReason = overridePointer.SuspensionReason
	}
	access.Capabilities = storage.EffectiveCapabilities(access.Role, user.Root)

	return storage.TargetPanelUser{User: user, Override: overridePointer, Access: access}, nil
}

func scanAccessDecision(scanner rowScanner) (storage.AccessDecision, error) {
	var decision storage.AccessDecision
	var targetID, avatarURL sql.NullString
	var createdAt, actorUpdatedAt string
	err := scanner.Scan(
		&decision.ID,
		&targetID,
		&decision.Action,
		&decision.Summary,
		&createdAt,
		&decision.Actor.ID,
		&decision.Actor.Provider,
		&decision.Actor.SubjectID,
		&decision.Actor.Login,
		&decision.Actor.DisplayName,
		&avatarURL,
		&actorUpdatedAt,
	)
	if err != nil {
		return storage.AccessDecision{}, err
	}
	decision.TargetID = stringPointer(targetID)
	decision.Actor.AvatarURL = stringPointer(avatarURL)
	decision.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return storage.AccessDecision{}, err
	}
	decision.Actor.UpdatedAt, err = parseTime(actorUpdatedAt)

	return decision, err
}
