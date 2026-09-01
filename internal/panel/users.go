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
	Login string                    `json:"login"`
	Role  *storage.InstallationRole `json:"role"`
}

type nullableWorkspaceRole struct {
	Value   *storage.InstallationRole
	Present bool
}

func (value *nullableWorkspaceRole) UnmarshalJSON(data []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Value = nil

		return nil
	}
	var role storage.InstallationRole
	if err := json.Unmarshal(data, &role); err != nil {
		return err
	}
	value.Value = &role

	return nil
}

type updateTargetUserRequest struct {
	Role             nullableWorkspaceRole `json:"role"`
	Suspended        *bool                 `json:"suspended"`
	SuspensionReason *string               `json:"suspension_reason"`
	ExpectedRevision *int64                `json:"expected_revision"`
}

type workspaceUserManager struct {
	Actor            storage.Account
	ActorUser        storage.PanelUser
	Access           storage.TargetAccess
	TargetID         string
	ElevationID      *string
	SessionTokenHash string
	RootWrite        bool
}

func (s *Server) getTargetUsers(w http.ResponseWriter, r *http.Request) {
	actor, actorUser, actorAccess, ok := s.requireTargetUserManager(w, r)
	if !ok {
		return
	}
	s.listWorkspaceUsers(w, r, workspaceUserManager{
		Actor: actor, ActorUser: actorUser, Access: actorAccess, TargetID: r.PathValue("target"),
	})
}

func (s *Server) listWorkspaceUsers(
	w http.ResponseWriter,
	r *http.Request,
	manager workspaceUserManager,
) {
	page, err := parsePanelUserPage(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	users, err := s.store.ListTargetPanelUserPage(
		r.Context(), manager.TargetID, s.now().UTC(), page,
	)
	if err != nil {
		s.writeWorkspaceMutationError(w, manager, err)
		return
	}
	writeJSON(w, http.StatusOK, targetPanelUserPageDTO(users, func(user storage.TargetPanelUser) bool {
		return canManageTargetUser(
			manager.Actor, manager.ActorUser, manager.Access, user.User, user.Access, user.Access.Role,
		)
	}))
}

func (s *Server) getTargetUserDecisions(w http.ResponseWriter, r *http.Request) {
	if _, _, _, ok := s.requireTargetUserManager(w, r); !ok {
		return
	}
	s.listWorkspaceUserDecisions(w, r, r.PathValue("target"))
}

func (s *Server) listWorkspaceUserDecisions(
	w http.ResponseWriter,
	r *http.Request,
	targetID string,
) {
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
	s.addWorkspaceUser(w, r, workspaceUserManager{
		Actor: actor, ActorUser: actorUser, Access: actorAccess, TargetID: r.PathValue("target"),
	})
}

func (s *Server) addWorkspaceUser(
	w http.ResponseWriter,
	r *http.Request,
	manager workspaceUserManager,
) {
	var input addUserRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Role == nil || !validGrantedTargetRole(*input.Role) || strings.TrimSpace(input.Login) == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "login and target role are required")
		return
	}
	targetID := manager.TargetID
	account, err := s.resolvePanelUser(r, targetID, input.Login)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "github_user_unavailable", "GitHub user could not be resolved")
		return
	}
	if account.ID == manager.Actor.ID {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot change your own access")
		return
	}
	subject, err := s.ensurePanelUser(r, account, manager.Actor.ID)
	if err != nil {
		s.writeWorkspaceMutationError(w, manager, err)
		return
	}
	current, err := s.store.ResolveTargetAccess(
		r.Context(), subject.Account.ID, targetID, s.now().UTC(),
	)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	if !canManageTargetUser(
		manager.Actor, manager.ActorUser, manager.Access, subject, current, *input.Role,
	) {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot grant this workspace role")
		return
	}
	override, err := s.store.SetTargetAccess(r.Context(), storage.TargetAccessChange{
		TargetID: targetID, SubjectAccountID: subject.Account.ID, ActorAccountID: manager.Actor.ID,
		ElevationID: manager.ElevationID, SessionTokenHash: manager.SessionTokenHash,
		Role: input.Role, ExpectedRevision: 0, ChangedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeWorkspaceMutationError(w, manager, err)
		return
	}
	access, err := s.store.ResolveTargetAccess(
		r.Context(), subject.Account.ID, targetID, s.now().UTC(),
	)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventAccessChanged, TargetID: targetID})
	writeJSON(w, http.StatusCreated, targetPanelUserDTO(storage.TargetPanelUser{
		User: subject, Override: &override, Access: access,
	}, true))
}

func (s *Server) writeWorkspaceMutationError(
	w http.ResponseWriter,
	manager workspaceUserManager,
	err error,
) {
	if manager.RootWrite {
		s.writeRootWriteError(w, err)
		return
	}
	s.writeStorageError(w, err)
}

func (s *Server) putTargetUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	actor, actorUser, actorAccess, ok := s.requireTargetUserManager(w, r)
	if !ok {
		return
	}
	s.updateWorkspaceUser(w, r, workspaceUserManager{
		Actor: actor, ActorUser: actorUser, Access: actorAccess, TargetID: r.PathValue("target"),
	})
}

func (s *Server) updateWorkspaceUser(
	w http.ResponseWriter,
	r *http.Request,
	manager workspaceUserManager,
) {
	var input updateTargetUserRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !input.Role.Present || input.Suspended == nil || input.ExpectedRevision == nil ||
		(input.Role.Value != nil && !validTargetWorkspaceRole(*input.Role.Value)) {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "the workspace user policy is incomplete")
		return
	}
	reason, ok := validAccessReason(w, input.SuspensionReason)
	if !ok {
		return
	}
	targetID := manager.TargetID
	subject, err := s.store.GetPanelUser(r.Context(), r.PathValue("account"))
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	current, err := s.store.ResolveTargetAccess(
		r.Context(), subject.Account.ID, targetID, s.now().UTC(),
	)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	desired := storage.InstallationRoleNone
	if input.Role.Value != nil {
		desired = *input.Role.Value
	}
	if *input.Suspended {
		desired = storage.InstallationRoleNone
	}
	if !canManageTargetUser(
		manager.Actor, manager.ActorUser, manager.Access, subject, current, desired,
	) {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot change this user's access to the workspace")
		return
	}
	override, err := s.store.SetTargetAccess(r.Context(), storage.TargetAccessChange{
		TargetID: targetID, SubjectAccountID: subject.Account.ID, ActorAccountID: manager.Actor.ID,
		ElevationID: manager.ElevationID, SessionTokenHash: manager.SessionTokenHash,
		Role: input.Role.Value, Suspended: *input.Suspended, SuspensionReason: reason,
		ExpectedRevision: *input.ExpectedRevision, ChangedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeWorkspaceMutationError(w, manager, err)
		return
	}
	access, err := s.store.ResolveTargetAccess(
		r.Context(), subject.Account.ID, targetID, s.now().UTC(),
	)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventAccessChanged, TargetID: targetID})
	writeJSON(w, http.StatusOK, targetPanelUserDTO(storage.TargetPanelUser{
		User: subject, Override: &override, Access: access,
	}, true))
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
		s.writeError(w, http.StatusForbidden, "forbidden", "managing a workspace's users requires Admin access")
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
		AccountID: account.ID, ActorAccountID: actorID, ChangedAt: s.now().UTC(),
	})
}

func canManageTargetUser(
	actor storage.Account,
	actorUser storage.PanelUser,
	actorAccess storage.TargetAccess,
	subject storage.PanelUser,
	subjectAccess storage.TargetAccess,
	desiredRole storage.InstallationRole,
) bool {
	if actor.ID == subject.Account.ID || subject.Status != storage.PanelUserActive ||
		subject.SystemRole.IsRoot() {
		return false
	}
	if actorAccess.Role == storage.InstallationRoleOwner || actorUser.SystemRole.IsRoot() {
		return desiredRole != storage.InstallationRoleOwner
	}
	if actorAccess.Role != storage.InstallationRoleAdmin || subjectAccess.Role == storage.InstallationRoleAdmin {
		return false
	}
	return desiredRole == storage.InstallationRoleNone || desiredRole == storage.InstallationRoleViewer ||
		desiredRole == storage.InstallationRoleEditor
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

func validTargetWorkspaceRole(role storage.InstallationRole) bool {
	return role == storage.InstallationRoleNone || role == storage.InstallationRoleViewer ||
		role == storage.InstallationRoleEditor || role == storage.InstallationRoleAdmin
}

func validTargetUserFilterRole(role storage.InstallationRole) bool {
	return validTargetWorkspaceRole(role) || role == storage.InstallationRoleOwner
}

func validGrantedTargetRole(role storage.InstallationRole) bool {
	return role == storage.InstallationRoleViewer || role == storage.InstallationRoleEditor ||
		role == storage.InstallationRoleAdmin
}
