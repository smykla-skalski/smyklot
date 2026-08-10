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

const rootAccessInvitationsPath = "invitations"

type createRootInvitationRequest struct {
	Login         string `json:"login"`
	ExpiresInDays int    `json:"expires_in_days"`
}

type updateRootUserRequest struct {
	SystemRole       *storage.SystemRole      `json:"system_role"`
	Status           *storage.PanelUserStatus `json:"status"`
	Reason           *string                  `json:"reason"`
	ExpectedRevision *int64                   `json:"expected_revision"`
}

func (s *Server) getRootAccess(w http.ResponseWriter, r *http.Request) {
	actor, actorUser, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	if r.PathValue("access") == rootAccessInvitationsPath {
		s.listRootInvitations(w, r)
		return
	}
	if r.PathValue("access") != rootAccessUsersPath {
		s.writeError(w, http.StatusNotFound, "not_found", "Root access view was not found")
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

func (s *Server) listRootInvitations(w http.ResponseWriter, r *http.Request) {
	page, err := parseInvitationPage(r.URL.Query())
	if err != nil || len(page.Roles) > 0 {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "invalid Root invitation page")
		return
	}
	invitations, err := s.store.ListRootInvitationPage(r.Context(), s.now().UTC(), page)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invitationPageDTO(invitations))
}

func (s *Server) postRootInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	actor, ok := s.requireSuperRoot(w, r)
	if !ok {
		return
	}
	var input createRootInvitationRequest
	if !decodeJSON(w, r, &input) || strings.TrimSpace(input.Login) == "" ||
		!validInviteDays(w, input.ExpiresInDays) {
		return
	}
	account, err := s.users.ResolveRootUser(r.Context(), strings.TrimSpace(input.Login))
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "github_user_unavailable", "GitHub user could not be resolved")
		return
	}
	if account.ID == actor.ID {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot invite yourself")
		return
	}
	if err := s.store.UpsertAccount(r.Context(), account); err != nil {
		s.writeInternal(w, err)
		return
	}
	role := storage.SystemRoleRoot
	s.createInvitation(
		w, r, actor.ID, account.ID, nil, nil, &role, input.ExpiresInDays, nil, "",
	)
}

func (s *Server) reissueRootInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	invitation, actor, ok := s.requireRootInvitationManager(w, r)
	if ok {
		s.reissueManagedInvitation(w, r, invitation, actor, nil, "")
	}
}

func (s *Server) deleteRootInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	invitation, actor, ok := s.requireRootInvitationManager(w, r)
	if ok {
		s.revokeManagedInvitation(w, r, invitation, actor, nil, "")
	}
}

func (s *Server) requireRootInvitationManager(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Invitation, storage.Account, bool) {
	actor, ok := s.requireSuperRoot(w, r)
	if !ok {
		return storage.Invitation{}, storage.Account{}, false
	}
	invitation, err := s.store.GetInvitation(r.Context(), r.PathValue("invitation"), s.now().UTC())
	if err != nil {
		s.writeStorageError(w, err)
		return storage.Invitation{}, storage.Account{}, false
	}
	if invitation.TargetID != nil || invitation.SystemRole == nil ||
		*invitation.SystemRole != storage.SystemRoleRoot {
		s.writeError(w, http.StatusNotFound, "not_found", "Root invitation not found")
		return storage.Invitation{}, storage.Account{}, false
	}

	return invitation, actor, true
}

func (s *Server) requireSuperRoot(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Account, bool) {
	actor, actorUser, ok := s.requireRoot(w, r)
	if !ok {
		return storage.Account{}, false
	}
	if actorUser.SystemRole != storage.SystemRoleSuperRoot {
		s.writeError(w, http.StatusForbidden, "forbidden", "Super Root access is required")
		return storage.Account{}, false
	}

	return actor, true
}

func (s *Server) putRootUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	actor, actorUser, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	var input updateRootUserRequest
	if !decodeJSON(w, r, &input) || input.ExpectedRevision == nil ||
		(input.SystemRole == nil) == (input.Status == nil) {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "one Root user change is required")
		return
	}
	subject, err := s.store.GetPanelUser(r.Context(), r.PathValue("account"))
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	if subject.Account.ID == actor.ID || subject.SystemRole == storage.SystemRoleSuperRoot {
		s.writeError(w, http.StatusForbidden, "forbidden", "this Root account cannot be changed")
		return
	}
	if input.SystemRole != nil {
		s.changeRootSystemRole(w, r, actor, actorUser, subject, input)
		return
	}
	s.changeRootUserStatus(w, r, actor, subject, input)
}

func (s *Server) changeRootSystemRole(
	w http.ResponseWriter,
	r *http.Request,
	actor storage.Account,
	actorUser, subject storage.PanelUser,
	input updateRootUserRequest,
) {
	role := *input.SystemRole
	if actorUser.SystemRole != storage.SystemRoleSuperRoot ||
		(role != storage.SystemRoleNone && role != storage.SystemRoleRoot) {
		s.writeError(w, http.StatusForbidden, "forbidden", "Super Root access is required")
		return
	}
	_, err := s.store.UpdateSystemRole(r.Context(), storage.SystemRoleChange{
		AccountID: subject.Account.ID, ActorAccountID: actor.ID, SystemRole: role,
		ExpectedRevision: *input.ExpectedRevision, ChangedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventResync})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) changeRootUserStatus(
	w http.ResponseWriter,
	r *http.Request,
	actor storage.Account,
	subject storage.PanelUser,
	input updateRootUserRequest,
) {
	status := *input.Status
	if subject.SystemRole.IsRoot() || (status != storage.PanelUserActive &&
		status != storage.PanelUserBanned && status != storage.PanelUserRemoved) {
		s.writeError(w, http.StatusForbidden, "forbidden", "Root lifecycle cannot be changed")
		return
	}
	reason, valid := validAccessReason(w, input.Reason)
	if !valid {
		return
	}
	_, err := s.store.UpdatePanelUser(r.Context(), storage.PanelUserChange{
		AccountID: subject.Account.ID, ActorAccountID: actor.ID, Status: status,
		BanReason: reason, ExpectedRevision: *input.ExpectedRevision,
		ChangedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	if status == storage.PanelUserBanned || status == storage.PanelUserRemoved {
		code := "account_banned"
		message := "Your Smyklot account was banned"
		if status == storage.PanelUserRemoved {
			code, message = "account_removed", "Your Smyklot account was removed"
		}
		hashes, err := s.store.RevokeAccountSessions(
			r.Context(), subject.Account.ID, code, message, s.now().UTC(),
		)
		if err != nil {
			s.writeInternal(w, err)
			return
		}
		for _, hash := range hashes {
			s.events.revokeSession(hash, code, message)
		}
	}
	s.events.announce(panelEvent{Type: panelEventResync})
	w.WriteHeader(http.StatusNoContent)
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
