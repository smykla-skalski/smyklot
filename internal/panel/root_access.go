package panel

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const rootAccessUsersPath = "users"

func (s *Server) getRootAccess(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("access") != rootAccessUsersPath {
		s.writeError(w, http.StatusNotFound, "not_found", "Root access view was not found")
		return
	}
	actor, actorUser, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	page, err := parseRootPanelUserPage(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	users, err := s.store.ListRootPanelUserPage(r.Context(), page)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rootPanelUserPageDTO(users, actor, actorUser.SystemRole))
}

func parseRootPanelUserPage(values url.Values) (storage.RootPanelUserPageRequest, error) {
	page := storage.RootPanelUserPageRequest{
		Limit: DefaultPageSize, Order: storage.RootPanelUserNameAscending,
		Query: strings.TrimSpace(values.Get("q")),
	}
	if err := parseAccessPageBase(values, page.Query, &page.Offset, &page.Limit); err != nil {
		return storage.RootPanelUserPageRequest{}, fmt.Errorf("invalid Root user page: %w", err)
	}
	order := storage.RootPanelUserOrder(values.Get("sort"))
	switch order {
	case "", storage.RootPanelUserNameAscending:
	case storage.RootPanelUserNameDescending, storage.RootPanelUserRoleAscending,
		storage.RootPanelUserRoleDescending, storage.RootPanelUserLoginNewest,
		storage.RootPanelUserLoginOldest:
		page.Order = order
	default:
		return storage.RootPanelUserPageRequest{}, fmt.Errorf("invalid Root user sort order")
	}
	for _, raw := range values["system_role"] {
		role := storage.SystemRole(raw)
		if role != storage.SystemRoleNone && role != storage.SystemRoleRoot &&
			role != storage.SystemRoleSuperRoot {
			return storage.RootPanelUserPageRequest{}, fmt.Errorf("invalid system role")
		}
		if !slices.Contains(page.SystemRoles, role) {
			page.SystemRoles = append(page.SystemRoles, role)
		}
	}
	for _, raw := range values["status"] {
		status := storage.PanelUserStatus(raw)
		if status != storage.PanelUserActive && status != storage.PanelUserBanned &&
			status != storage.PanelUserRemoved {
			return storage.RootPanelUserPageRequest{}, fmt.Errorf("invalid Root user status")
		}
		if !slices.Contains(page.Statuses, status) {
			page.Statuses = append(page.Statuses, status)
		}
	}

	return page, nil
}
