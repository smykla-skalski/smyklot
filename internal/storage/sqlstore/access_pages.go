package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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

// ListTargetPanelUserPage returns one filtered installation-scoped user page.
func (s *Store) ListTargetPanelUserPage(
	ctx context.Context,
	targetID string,
	now time.Time,
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

	clauses, arguments, err := panelUserPageFilters(page)
	if err != nil {
		return storage.TargetPanelUserPage{}, err
	}
	join := `
LEFT JOIN target_roles tr ON tr.account_id = pu.account_id AND tr.target_id = ?
LEFT JOIN target_owners target_owner
  ON target_owner.account_id = pu.account_id AND target_owner.target_id = ?`
	arguments = append([]any{targetID, targetID}, arguments...)
	total, err := countPanelUsers(ctx, s.db, join, clauses, arguments)
	if err != nil {
		return storage.TargetPanelUserPage{}, fmt.Errorf("count target panel users: %w", err)
	}
	ids, nextOffset, err := listPanelUserIDs(ctx, s.db, join, clauses, arguments, page)
	if err != nil {
		return storage.TargetPanelUserPage{}, err
	}
	items := make([]storage.TargetPanelUser, 0, len(ids))
	for _, id := range ids {
		item, readErr := s.targetPanelUser(ctx, id, targetID, now)
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
  AND `+optionalScopeClause("aa.target_id")+`
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
) ([]string, []any, error) {
	clauses := []string{
		"pu.status <> 'removed'",
		"(target_owner.account_id IS NOT NULL OR tr.account_id IS NOT NULL)",
	}
	arguments := make([]any, 0)
	if page.Query != "" {
		clauses = append(clauses, containsAnyClause("a.login", "a.display_name"))
		arguments = append(arguments, containsArguments(page.Query, 2)...)
	}
	if len(page.Roles) > 0 {
		roleClauses, roleArguments, err := panelUserRoleFilters(page.Roles)
		if err != nil {
			return nil, nil, err
		}
		clauses = append(clauses, "("+strings.Join(roleClauses, " OR ")+")")
		arguments = append(arguments, roleArguments...)
	}
	if len(page.States) > 0 {
		stateClauses, err := panelUserStateFilters(page.States)
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

func panelUserRoleFilters(roles []storage.InstallationRole) ([]string, []any, error) {
	expression := targetEffectiveRoleSQL()
	clauses := make([]string, 0, len(roles))
	arguments := make([]any, 0, len(roles))
	for _, role := range roles {
		if !validInstallationRole(role) {
			return nil, nil, fmt.Errorf("unsupported panel user role %q", role)
		}
		clauses = append(clauses, expression+" = ?")
		arguments = append(arguments, role)
	}

	return clauses, arguments, nil
}

func panelUserStateFilters(states []storage.PanelUserListState) ([]string, error) {
	clauses := make([]string, 0, len(states))
	for _, state := range states {
		clause, err := panelUserStateFilter(state)
		if err != nil {
			return nil, err
		}
		if clause != "" {
			clauses = append(clauses, clause)
		}
	}

	return clauses, nil
}

func panelUserStateFilter(state storage.PanelUserListState) (string, error) {
	switch state {
	case storage.PanelUserListActive:
		return "(pu.status = 'active' AND COALESCE(tr.suspended, FALSE) = FALSE)", nil
	case storage.PanelUserListBanned:
		return "pu.status = 'banned'", nil
	case storage.PanelUserListSuspended:
		return "(pu.status = 'active' AND tr.suspended = TRUE)", nil
	default:
		return "", fmt.Errorf("unsupported panel user state %q", state)
	}
}

func targetEffectiveRoleSQL() string {
	return `(CASE
WHEN pu.status <> 'active' OR COALESCE(tr.suspended, FALSE) = TRUE THEN 'none'
WHEN target_owner.account_id IS NOT NULL THEN 'owner'
WHEN pu.system_role IN ('root', 'super_root') THEN 'none'
ELSE COALESCE(tr.role, 'none')
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
	queryer runner,
	join string,
	clauses []string,
	arguments []any,
	page storage.PanelUserPageRequest,
) ([]string, int, error) {
	order, err := panelUserOrder(page.Order)
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

func panelUserOrder(order storage.PanelUserOrder) (string, error) {
	role := targetEffectiveRoleSQL()
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

func (s *Store) targetPanelUser(
	ctx context.Context,
	accountID, targetID string,
	now time.Time,
) (storage.TargetPanelUser, error) {
	user, err := getPanelUser(ctx, s.db, accountID)
	if err != nil {
		return storage.TargetPanelUser{}, err
	}
	override, err := getTargetAccessOverride(ctx, s.db, accountID, targetID)
	var overridePointer *storage.TargetAccessOverride
	if err == nil {
		overridePointer = &override
	} else if !errors.Is(err, sql.ErrNoRows) {
		return storage.TargetPanelUser{}, err
	}
	access, err := s.ResolveTargetAccess(ctx, accountID, targetID, now)
	if err != nil {
		return storage.TargetPanelUser{}, err
	}

	return storage.TargetPanelUser{User: user, Override: overridePointer, Access: access}, nil
}

func scanAccessDecision(scanner rowScanner) (storage.AccessDecision, error) {
	var decision storage.AccessDecision
	var targetID, avatarURL sql.NullString
	var createdAt, actorUpdatedAt StoredTime
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
	decision.CreatedAt = createdAt.Time()
	decision.Actor.UpdatedAt = actorUpdatedAt.Time()

	return decision, nil
}
