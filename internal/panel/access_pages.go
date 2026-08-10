package panel

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

type accessDecisionResponse struct {
	ID        string          `json:"id"`
	Actor     accountResponse `json:"actor"`
	Action    string          `json:"action"`
	Summary   string          `json:"summary"`
	CreatedAt time.Time       `json:"created_at"`
}

func parsePanelUserPage(values url.Values) (storage.PanelUserPageRequest, error) {
	page := storage.PanelUserPageRequest{
		Limit: DefaultPageSize,
		Order: storage.PanelUserNameAscending,
		Query: strings.TrimSpace(values.Get("q")),
	}
	if err := parseAccessPageBase(
		values,
		page.Query,
		&page.Offset,
		&page.Limit,
	); err != nil {
		return storage.PanelUserPageRequest{}, fmt.Errorf("invalid user page: %w", err)
	}
	switch order := storage.PanelUserOrder(values.Get("sort")); order {
	case "", storage.PanelUserNameAscending:
	case storage.PanelUserNameDescending,
		storage.PanelUserRoleAscending,
		storage.PanelUserRoleDescending,
		storage.PanelUserUpdatedNewest,
		storage.PanelUserUpdatedOldest,
		storage.PanelUserLoginNewest,
		storage.PanelUserLoginOldest:
		page.Order = order
	default:
		return storage.PanelUserPageRequest{}, fmt.Errorf("invalid user sort order")
	}
	for _, raw := range values["role"] {
		role := storage.InstallationRole(raw)
		if !validTargetUserFilterRole(role) || slices.Contains(page.Roles, role) {
			if slices.Contains(page.Roles, role) {
				continue
			}
			return storage.PanelUserPageRequest{}, fmt.Errorf("invalid user role")
		}
		page.Roles = append(page.Roles, role)
	}
	for _, raw := range values["status"] {
		state := storage.PanelUserListState(raw)
		if state != storage.PanelUserListActive && state != storage.PanelUserListBanned &&
			state != storage.PanelUserListSuspended {
			return storage.PanelUserPageRequest{}, fmt.Errorf("invalid user status")
		}
		if !slices.Contains(page.States, state) {
			page.States = append(page.States, state)
		}
	}

	return page, nil
}

func parseInvitationPage(values url.Values) (storage.InvitationPageRequest, error) {
	page := storage.InvitationPageRequest{
		Limit: DefaultPageSize,
		Order: storage.InvitationCreatedNewest,
		Query: strings.TrimSpace(values.Get("q")),
	}
	if err := parseAccessPageBase(
		values,
		page.Query,
		&page.Offset,
		&page.Limit,
	); err != nil {
		return storage.InvitationPageRequest{}, fmt.Errorf("invalid invitation page: %w", err)
	}
	switch order := storage.InvitationOrder(values.Get("sort")); order {
	case "", storage.InvitationCreatedNewest:
	case storage.InvitationCreatedOldest,
		storage.InvitationExpirySoonest,
		storage.InvitationExpiryLatest,
		storage.InvitationNameAscending,
		storage.InvitationNameDescending,
		storage.InvitationRoleAscending,
		storage.InvitationRoleDescending:
		page.Order = order
	default:
		return storage.InvitationPageRequest{}, fmt.Errorf("invalid invitation sort order")
	}
	for _, raw := range values["role"] {
		role := storage.InstallationRole(raw)
		if !validGrantedTargetRole(role) {
			return storage.InvitationPageRequest{}, fmt.Errorf("invalid invitation role")
		}
		if !slices.Contains(page.Roles, role) {
			page.Roles = append(page.Roles, role)
		}
	}
	for _, raw := range values["status"] {
		status := storage.InvitationStatus(raw)
		if status != storage.InvitationPending && status != storage.InvitationAccepted &&
			status != storage.InvitationDeclined && status != storage.InvitationRevoked &&
			status != storage.InvitationExpired {
			return storage.InvitationPageRequest{}, fmt.Errorf("invalid invitation status")
		}
		if !slices.Contains(page.Statuses, status) {
			page.Statuses = append(page.Statuses, status)
		}
	}

	return page, nil
}

func parseAccessPageBase(
	values url.Values,
	query string,
	offset, limit *int,
) error {
	if len(query) > 200 || strings.ContainsFunc(query, unicode.IsControl) {
		return fmt.Errorf("invalid search")
	}
	if raw := values.Get("cursor"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return fmt.Errorf("invalid cursor")
		}
		*offset = value
	}
	if raw := values.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 || value > MaxPageSize {
			return fmt.Errorf("invalid page size")
		}
		*limit = value
	}

	return nil
}

func targetPanelUserPageDTO(
	page storage.TargetPanelUserPage,
	manageable func(storage.TargetPanelUser) bool,
) pageResponse[panelUserResponse] {
	items := make([]panelUserResponse, 0, len(page.Items))
	for _, user := range page.Items {
		items = append(items, targetPanelUserDTO(user, manageable(user)))
	}

	return pageResponse[panelUserResponse]{
		Items: items, NextCursor: offsetCursor(page.NextOffset), Total: page.Total,
	}
}

func invitationPageDTO(page storage.InvitationPage) pageResponse[invitationResponse] {
	items := make([]invitationResponse, 0, len(page.Items))
	for _, invitation := range page.Items {
		items = append(items, invitationDTO(invitation, ""))
	}

	return pageResponse[invitationResponse]{
		Items: items, NextCursor: offsetCursor(page.NextOffset), Total: page.Total,
	}
}

func accessDecisionsDTO(items []storage.AccessDecision) []accessDecisionResponse {
	result := make([]accessDecisionResponse, 0, len(items))
	for _, decision := range items {
		result = append(result, accessDecisionResponse{
			ID: strconv.FormatInt(decision.ID, 10), Actor: accountDTO(decision.Actor),
			Action: decision.Action, Summary: decision.Summary,
			CreatedAt: decision.CreatedAt,
		})
	}

	return result
}
