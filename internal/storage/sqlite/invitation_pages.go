package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// ListInvitationPage returns one filtered invitation-management page.
func (s *Store) ListInvitationPage(
	ctx context.Context,
	targetID *string,
	now time.Time,
	page storage.InvitationPageRequest,
) (storage.InvitationPage, error) {
	clauses, arguments, err := invitationPageFilters(targetID, now, page)
	if err != nil {
		return storage.InvitationPage{}, err
	}
	fromIndex := strings.Index(invitationSelect, "FROM ")
	if fromIndex < 0 {
		return storage.InvitationPage{}, fmt.Errorf("invitation select does not contain a FROM clause")
	}
	var total int
	if err := s.db.QueryRowContext(
		ctx,
		"SELECT COUNT(*) "+invitationSelect[fromIndex:]+" WHERE "+strings.Join(clauses, " AND "),
		arguments...,
	).Scan(&total); err != nil {
		return storage.InvitationPage{}, fmt.Errorf("count invitations: %w", err)
	}
	order, err := invitationPageOrder(page.Order)
	if err != nil {
		return storage.InvitationPage{}, err
	}
	limit := pageLimit(page.Limit)
	offset := max(page.Offset, 0)
	queryArguments := append(append([]any{}, arguments...), limit+1, offset)
	// #nosec G202 -- clauses and order come only from fixed internal constants.
	rows, err := s.db.QueryContext(
		ctx,
		invitationSelect+" WHERE "+strings.Join(clauses, " AND ")+" ORDER BY "+order+" LIMIT ? OFFSET ?",
		queryArguments...,
	)
	if err != nil {
		return storage.InvitationPage{}, fmt.Errorf("list invitation page: %w", err)
	}
	items, err := collectRows(rows, func(scanner rowScanner) (storage.Invitation, error) {
		return scanInvitation(scanner, now)
	})
	if err != nil {
		return storage.InvitationPage{}, fmt.Errorf("read invitation page: %w", err)
	}
	result := storage.InvitationPage{Items: items, Total: total}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		result.NextOffset = offset + limit
	}

	return result, nil
}

func invitationPageFilters(
	targetID *string,
	now time.Time,
	page storage.InvitationPageRequest,
) ([]string, []any, error) {
	clauses := []string{"((ui.target_id IS NULL AND ? IS NULL) OR ui.target_id = ?)"}
	arguments := []any{targetID, targetID}
	if page.Query != "" {
		clauses = append(clauses, `(instr(lower(invited.login), lower(?)) > 0
OR instr(lower(invited.display_name), lower(?)) > 0
OR instr(lower(creator.login), lower(?)) > 0)`)
		arguments = append(arguments, page.Query, page.Query, page.Query)
	}
	if len(page.Roles) > 0 {
		parts := make([]string, 0, len(page.Roles))
		for _, role := range page.Roles {
			if !validInvitationRole(role, targetID) {
				return nil, nil, fmt.Errorf("unsupported invitation role %q", role)
			}
			parts = append(parts, "ui.role = ?")
			arguments = append(arguments, role)
		}
		clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
	}
	if len(page.Statuses) > 0 {
		statusExpression := "(CASE WHEN ui.status = 'pending' AND ui.expires_at <= ? THEN 'expired' ELSE ui.status END)"
		parts := make([]string, 0, len(page.Statuses))
		for _, status := range page.Statuses {
			if !validInvitationStatus(status) {
				return nil, nil, fmt.Errorf("unsupported invitation status %q", status)
			}
			parts = append(parts, statusExpression+" = ?")
			arguments = append(arguments, formatTime(now), status)
		}
		clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
	}

	return clauses, arguments, nil
}

func invitationPageOrder(order storage.InvitationOrder) (string, error) {
	roleLevel := `(CASE ui.role
WHEN 'viewer' THEN 1
WHEN 'editor' THEN 2
WHEN 'admin' THEN 3
WHEN 'owner' THEN 4
ELSE 0
END)`
	switch order {
	case "", storage.InvitationCreatedNewest:
		return "ui.created_at DESC, ui.id DESC", nil
	case storage.InvitationCreatedOldest:
		return "ui.created_at ASC, ui.id ASC", nil
	case storage.InvitationExpirySoonest:
		return "ui.expires_at ASC, ui.id ASC", nil
	case storage.InvitationExpiryLatest:
		return "ui.expires_at DESC, ui.id DESC", nil
	case storage.InvitationNameAscending:
		return "lower(invited.display_name) ASC, lower(invited.login) ASC, ui.id ASC", nil
	case storage.InvitationNameDescending:
		return "lower(invited.display_name) DESC, lower(invited.login) DESC, ui.id DESC", nil
	case storage.InvitationRoleAscending:
		return roleLevel + " ASC, lower(invited.display_name) ASC, ui.id ASC", nil
	case storage.InvitationRoleDescending:
		return roleLevel + " DESC, lower(invited.display_name) ASC, ui.id ASC", nil
	default:
		return "", fmt.Errorf("unsupported invitation order %q", order)
	}
}

func validInvitationStatus(status storage.InvitationStatus) bool {
	switch status {
	case storage.InvitationPending,
		storage.InvitationAccepted,
		storage.InvitationDeclined,
		storage.InvitationRevoked,
		storage.InvitationExpired:
		return true
	default:
		return false
	}
}
