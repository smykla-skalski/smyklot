package panel

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	maxAccessReasonLength = 500
	panelUsersResource    = "users"
)

type addUserRequest struct {
	Login    string             `json:"login"`
	Role     *storage.PanelRole `json:"role"`
	TargetID string             `json:"target_id"`
}

type updateUserRequest struct {
	GlobalRole       *storage.PanelRole       `json:"global_role"`
	Status           *storage.PanelUserStatus `json:"status"`
	BanReason        *string                  `json:"ban_reason"`
	ExpectedRevision *int64                   `json:"expected_revision"`
}

type nullablePanelRole struct {
	Value   *storage.PanelRole
	Present bool
}

func (value *nullablePanelRole) UnmarshalJSON(data []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Value = nil

		return nil
	}
	var role storage.PanelRole
	if err := json.Unmarshal(data, &role); err != nil {
		return err
	}
	value.Value = &role

	return nil
}

type updateTargetUserRequest struct {
	Role             nullablePanelRole `json:"role"`
	Suspended        *bool             `json:"suspended"`
	SuspensionReason *string           `json:"suspension_reason"`
	ExpectedRevision *int64            `json:"expected_revision"`
}

func (s *Server) getUsers(w http.ResponseWriter, r *http.Request) {
	actor, actorUser, ok := s.requireGlobalUserManager(w, r)
	if !ok {
		return
	}
	page, err := parsePanelUserPage(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	users, err := s.store.ListPanelUserPage(r.Context(), page)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, panelUserPageDTO(users, func(user storage.PanelUser) bool {
		return canManageGlobalUser(actor, actorUser, user, user.GlobalRole)
	}))
}

func (s *Server) postUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	actor, actorUser, ok := s.requireGlobalUserManager(w, r)
	if !ok {
		return
	}
	var input addUserRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Role == nil || !validGlobalPanelRole(*input.Role) ||
		strings.TrimSpace(input.Login) == "" || strings.TrimSpace(input.TargetID) == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "login, role, and installation are required")
		return
	}
	if *input.Role == storage.PanelRoleOwner && !actorUser.Root {
		s.writeError(w, http.StatusForbidden, "forbidden", "only the root owner can appoint owners")
		return
	}
	account, err := s.resolvePanelUser(r, input.TargetID, input.Login)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "github_user_unavailable", "GitHub user could not be resolved")
		return
	}
	if account.ID == actor.ID {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot change your own access")
		return
	}
	created, err := s.store.CreatePanelUser(r.Context(), storage.PanelUserCreate{
		AccountID: account.ID, GlobalRole: *input.Role, ActorAccountID: actor.ID,
		ChangedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventResync})
	writeJSON(w, http.StatusCreated, panelUserDTO(created, true))
}

func (s *Server) putUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	actor, actorUser, ok := s.requireGlobalUserManager(w, r)
	if !ok {
		return
	}
	var input updateUserRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.GlobalRole == nil || input.Status == nil || input.ExpectedRevision == nil ||
		!validGlobalPanelRole(*input.GlobalRole) || !validPanelUserStatus(*input.Status) {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "user policy is incomplete")
		return
	}
	reason, ok := validAccessReason(w, input.BanReason)
	if !ok {
		return
	}
	subject, err := s.store.GetPanelUser(r.Context(), r.PathValue("account"))
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	if !canManageGlobalUser(actor, actorUser, subject, *input.GlobalRole) {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot change this user's access")
		return
	}
	updated, err := s.store.UpdatePanelUser(r.Context(), storage.PanelUserChange{
		AccountID: subject.Account.ID, ActorAccountID: actor.ID,
		GlobalRole: *input.GlobalRole, Status: *input.Status, BanReason: reason,
		ExpectedRevision: *input.ExpectedRevision, ChangedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	if updated.Status == storage.PanelUserBanned || updated.Status == storage.PanelUserRemoved {
		s.revokePanelUser(r, updated)
	} else {
		s.events.announce(panelEvent{Type: panelEventResync})
	}
	writeJSON(w, http.StatusOK, panelUserDTO(updated, true))
}

func (s *Server) getTargetUsers(w http.ResponseWriter, r *http.Request) {
	actor, actorUser, actorAccess, ok := s.requireTargetUserManager(w, r)
	if !ok {
		return
	}
	page, err := parsePanelUserPage(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	users, err := s.store.ListTargetPanelUserPage(r.Context(), r.PathValue("target"), page)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, targetPanelUserPageDTO(users, func(user storage.TargetPanelUser) bool {
		return canManageTargetUser(
			actor, actorUser, actorAccess, user.User, user.Access, user.Access.Role,
		)
	}))
}

func (s *Server) getUserDecisions(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireGlobalUserManager(w, r); !ok {
		return
	}
	s.listUserDecisions(w, r, nil)
}

func (s *Server) getTargetUserDecisions(w http.ResponseWriter, r *http.Request) {
	if _, _, _, ok := s.requireTargetUserManager(w, r); !ok {
		return
	}
	targetID := r.PathValue("target")
	s.listUserDecisions(w, r, &targetID)
}

func (s *Server) listUserDecisions(w http.ResponseWriter, r *http.Request, targetID *string) {
	if _, err := s.store.GetPanelUser(r.Context(), r.PathValue("account")); err != nil {
		s.writeStorageError(w, err)
		return
	}
	items, err := s.store.ListAccessDecisions(
		r.Context(), r.PathValue("account"), targetID, MaxPageSize,
	)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"decisions": accessDecisionsDTO(items)})
}

func (s *Server) postTargetUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	actor, actorUser, actorAccess, ok := s.requireTargetUserManager(w, r)
	if !ok {
		return
	}
	var input addUserRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Role == nil || !validGrantedTargetRole(*input.Role) || strings.TrimSpace(input.Login) == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "login and target role are required")
		return
	}
	targetID := r.PathValue("target")
	account, err := s.resolvePanelUser(r, targetID, input.Login)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "github_user_unavailable", "GitHub user could not be resolved")
		return
	}
	if account.ID == actor.ID {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot change your own access")
		return
	}
	subject, err := s.ensurePanelUser(r, account, actor.ID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	current, err := s.store.ResolveTargetAccess(r.Context(), subject.Account.ID, targetID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	if !canManageTargetUser(actor, actorUser, actorAccess, subject, current, *input.Role) {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot grant this installation role")
		return
	}
	override, err := s.store.SetTargetAccess(r.Context(), storage.TargetAccessChange{
		TargetID: targetID, SubjectAccountID: subject.Account.ID, ActorAccountID: actor.ID,
		Role: input.Role, ExpectedRevision: 0, ChangedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	access, err := s.store.ResolveTargetAccess(r.Context(), subject.Account.ID, targetID)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventAccessChanged, TargetID: targetID})
	writeJSON(w, http.StatusCreated, targetPanelUserDTO(storage.TargetPanelUser{
		User: subject, Override: &override, Access: access,
	}, true))
}

func (s *Server) putTargetUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	actor, actorUser, actorAccess, ok := s.requireTargetUserManager(w, r)
	if !ok {
		return
	}
	var input updateTargetUserRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !input.Role.Present || input.Suspended == nil || input.ExpectedRevision == nil ||
		(input.Role.Value != nil && !validTargetPanelRole(*input.Role.Value)) {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "installation user policy is incomplete")
		return
	}
	reason, ok := validAccessReason(w, input.SuspensionReason)
	if !ok {
		return
	}
	targetID := r.PathValue("target")
	subject, err := s.store.GetPanelUser(r.Context(), r.PathValue("account"))
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	current, err := s.store.ResolveTargetAccess(r.Context(), subject.Account.ID, targetID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	desired := subject.GlobalRole
	if input.Role.Value != nil {
		desired = *input.Role.Value
	}
	if *input.Suspended {
		desired = storage.PanelRoleNone
	}
	if !canManageTargetUser(actor, actorUser, actorAccess, subject, current, desired) {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot change this user's installation access")
		return
	}
	override, err := s.store.SetTargetAccess(r.Context(), storage.TargetAccessChange{
		TargetID: targetID, SubjectAccountID: subject.Account.ID, ActorAccountID: actor.ID,
		Role: input.Role.Value, Suspended: *input.Suspended, SuspensionReason: reason,
		ExpectedRevision: *input.ExpectedRevision, ChangedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	access, err := s.store.ResolveTargetAccess(r.Context(), subject.Account.ID, targetID)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventAccessChanged, TargetID: targetID})
	writeJSON(w, http.StatusOK, targetPanelUserDTO(storage.TargetPanelUser{
		User: subject, Override: &override, Access: access,
	}, true))
}

func (s *Server) requireGlobalUserManager(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Account, storage.PanelUser, bool) {
	account, ok := s.requireViewer(w, r)
	if !ok {
		return storage.Account{}, storage.PanelUser{}, false
	}
	user, err := s.store.GetPanelUser(r.Context(), account.ID)
	if err != nil {
		s.writeInternal(w, err)
		return storage.Account{}, storage.PanelUser{}, false
	}
	if !storage.EffectiveCapabilities(user.GlobalRole, user.Root).ManageGlobalUsers {
		s.writeError(w, http.StatusForbidden, "forbidden", "global user management requires Owner access")
		return storage.Account{}, storage.PanelUser{}, false
	}

	return account, user, true
}

func (s *Server) requireTargetUserManager(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Account, storage.PanelUser, storage.TargetAccess, bool) {
	account, _, access, ok := s.requireTarget(w, r, false)
	if !ok {
		return storage.Account{}, storage.PanelUser{}, storage.TargetAccess{}, false
	}
	if !access.Capabilities.ManageTargetUsers {
		s.writeError(w, http.StatusForbidden, "forbidden", "installation user management requires Admin access")
		return storage.Account{}, storage.PanelUser{}, storage.TargetAccess{}, false
	}
	user, err := s.store.GetPanelUser(r.Context(), account.ID)
	if err != nil {
		s.writeInternal(w, err)
		return storage.Account{}, storage.PanelUser{}, storage.TargetAccess{}, false
	}

	return account, user, access, true
}

func (s *Server) resolvePanelUser(
	r *http.Request,
	targetID, login string,
) (storage.Account, error) {
	account, err := s.users.ResolveUser(r.Context(), targetID, strings.TrimSpace(login))
	if err != nil {
		return storage.Account{}, err
	}
	if err := s.store.UpsertAccount(r.Context(), account); err != nil {
		return storage.Account{}, err
	}

	return account, nil
}

func (s *Server) ensurePanelUser(
	r *http.Request,
	account storage.Account,
	actorID string,
) (storage.PanelUser, error) {
	user, err := s.store.GetPanelUser(r.Context(), account.ID)
	if err == nil {
		if user.Status == storage.PanelUserBanned {
			return storage.PanelUser{}, storage.ErrConflict
		}
		if user.Status != storage.PanelUserRemoved {
			return user, nil
		}
	} else if !errors.Is(err, storage.ErrNotFound) {
		return storage.PanelUser{}, err
	}

	return s.store.CreatePanelUser(r.Context(), storage.PanelUserCreate{
		AccountID: account.ID, GlobalRole: storage.PanelRoleNone,
		ActorAccountID: actorID, ChangedAt: s.now().UTC(),
	})
}

func (s *Server) revokePanelUser(r *http.Request, user storage.PanelUser) {
	reason := "Your panel access was revoked"
	if user.Status == storage.PanelUserBanned {
		reason = "Your panel access was suspended"
		if user.BanReason != nil {
			reason = *user.BanReason
		}
	}
	_, _ = s.store.RevokeAccountSessions(
		r.Context(), user.Account.ID, string(user.Status), reason, s.now().UTC(),
	)
	s.events.revokeAccount(user.Account.ID, string(user.Status), reason)
	s.events.announce(panelEvent{Type: panelEventResync})
}

func canManageGlobalUser(
	actor storage.Account,
	actorUser, subject storage.PanelUser,
	desiredRole storage.PanelRole,
) bool {
	if actor.ID == subject.Account.ID || subject.Root {
		return false
	}
	if actorUser.Root {
		return true
	}
	return subject.GlobalRole != storage.PanelRoleOwner && desiredRole != storage.PanelRoleOwner
}

func canManageTargetUser(
	actor storage.Account,
	actorUser storage.PanelUser,
	actorAccess storage.TargetAccess,
	subject storage.PanelUser,
	subjectAccess storage.TargetAccess,
	desiredRole storage.PanelRole,
) bool {
	if actor.ID == subject.Account.ID || subject.Status != storage.PanelUserActive || subject.Root ||
		subject.GlobalRole == storage.PanelRoleOwner {
		return false
	}
	if actorAccess.Role == storage.PanelRoleOwner || actorUser.Root {
		return desiredRole != storage.PanelRoleOwner
	}
	if actorAccess.Role != storage.PanelRoleAdmin ||
		subject.GlobalRole == storage.PanelRoleAdmin ||
		subjectAccess.Role == storage.PanelRoleAdmin {
		return false
	}
	return desiredRole == storage.PanelRoleNone || desiredRole == storage.PanelRoleViewer ||
		desiredRole == storage.PanelRoleEditor
}

func validAccessReason(w http.ResponseWriter, raw *string) (*string, bool) {
	if raw == nil {
		return nil, true
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, true
	}
	if len(value) > maxAccessReasonLength || strings.ContainsFunc(value, unicode.IsControl) {
		writeError(w, http.StatusBadRequest, "invalid_request", "reason must be plain text up to 500 characters")
		return nil, false
	}

	return &value, true
}

func validGlobalPanelRole(role storage.PanelRole) bool {
	return validTargetPanelRole(role) || role == storage.PanelRoleOwner
}

func validTargetPanelRole(role storage.PanelRole) bool {
	return role == storage.PanelRoleNone || role == storage.PanelRoleViewer ||
		role == storage.PanelRoleEditor || role == storage.PanelRoleAdmin
}

func validGrantedTargetRole(role storage.PanelRole) bool {
	return role == storage.PanelRoleViewer || role == storage.PanelRoleEditor ||
		role == storage.PanelRoleAdmin
}

func validPanelUserStatus(status storage.PanelUserStatus) bool {
	return status == storage.PanelUserActive || status == storage.PanelUserBanned ||
		status == storage.PanelUserRemoved
}
