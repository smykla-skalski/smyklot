package panel

import (
	"net/http"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func (s *Server) getRootTargetUsers(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.requireRootWorkspaceManager(w, r, false)
	if !ok {
		return
	}
	s.listWorkspaceUsers(w, r, manager)
}

func (s *Server) postRootTargetUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	manager, ok := s.requireRootWorkspaceManager(w, r, true)
	if !ok {
		return
	}
	s.addWorkspaceUser(w, r, manager)
}

func (s *Server) putRootTargetUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	manager, ok := s.requireRootWorkspaceManager(w, r, true)
	if !ok {
		return
	}
	s.updateWorkspaceUser(w, r, manager)
}

func (s *Server) getRootTargetUserDecisions(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.requireRootWorkspaceManager(w, r, false)
	if !ok {
		return
	}
	s.listWorkspaceUserDecisions(w, r, manager.TargetID)
}

func (s *Server) getRootTargetInvitations(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.requireRootWorkspaceManager(w, r, false)
	if !ok {
		return
	}
	s.listInvitations(w, r, &manager.TargetID)
}

func (s *Server) postRootTargetInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	manager, ok := s.requireRootWorkspaceManager(w, r, true)
	if !ok {
		return
	}
	var input createInvitationRequest
	if !decodeJSON(w, r, &input) || !validCreateInvitation(w, input) {
		return
	}
	account, err := s.resolvePanelUser(r, manager.TargetID, input.Login)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "github_user_unavailable", "GitHub user could not be resolved")
		return
	}
	if s.refusedSelfInvitation(w, manager.Actor, account) {
		return
	}
	if !s.canInviteToTarget(
		r, manager.Actor, manager.ActorUser, manager.Access, account, *input.Role,
	) {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot grant this invitation role")
		return
	}
	s.createInvitation(w, r, invitationDraft{
		ActorID:             manager.Actor.ID,
		AccountID:           account.ID,
		TargetID:            &manager.TargetID,
		Role:                input.Role,
		Days:                input.ExpiresInDays,
		ElevationID:         manager.ElevationID,
		SessionTokenHash:    manager.SessionTokenHash,
		AcknowledgeDeclined: input.AcknowledgeDeclined,
	})
}

func (s *Server) reissueRootTargetInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	invitation, manager, ok := s.requireRootWorkspaceInvitationManager(w, r)
	if !ok {
		return
	}
	s.reissueManagedInvitation(
		w, r, invitation, manager.Actor, manager.ElevationID, manager.SessionTokenHash,
	)
}

func (s *Server) deleteRootTargetInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	invitation, manager, ok := s.requireRootWorkspaceInvitationManager(w, r)
	if !ok {
		return
	}
	s.revokeManagedInvitation(
		w, r, invitation, manager.Actor, manager.ElevationID, manager.SessionTokenHash,
	)
}

func (s *Server) getRootTargetAudit(w http.ResponseWriter, r *http.Request) {
	context, ok := s.requireRootTarget(w, r, false)
	if !ok {
		return
	}
	page, err := parseHistoryPage(r.URL.Query(), auditHistoryOrders...)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_history_query", err.Error())
		return
	}
	s.getRootAuditPage(w, r, page, &context.Target.ID)
}

func (s *Server) getRootTargetFailures(w http.ResponseWriter, r *http.Request) {
	context, ok := s.requireRootTarget(w, r, false)
	if !ok {
		return
	}
	s.getWorkspaceFailurePage(w, r, context.Target.ID)
}

func (s *Server) requireRootWorkspaceManager(
	w http.ResponseWriter,
	r *http.Request,
	write bool,
) (workspaceUserManager, bool) {
	context, ok := s.requireRootTarget(w, r, write)
	if !ok {
		return workspaceUserManager{}, false
	}
	user, err := s.store.GetPanelUser(r.Context(), context.Account.ID)
	if err != nil {
		s.writeInternal(w, err)
		return workspaceUserManager{}, false
	}

	return workspaceUserManager{
		Actor:            context.Account,
		ActorUser:        user,
		Access:           context.Access,
		TargetID:         context.Target.ID,
		ElevationID:      elevationID(context.Elevation),
		SessionTokenHash: context.SessionHash,
		RootWrite:        write,
	}, true
}

func (s *Server) requireRootWorkspaceInvitationManager(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Invitation, workspaceUserManager, bool) {
	manager, ok := s.requireRootWorkspaceManager(w, r, true)
	if !ok {
		return storage.Invitation{}, workspaceUserManager{}, false
	}
	invitation, err := s.store.GetInvitation(
		r.Context(), r.PathValue("invitation"), s.now().UTC(),
	)
	if err != nil {
		s.writeStorageError(w, err)
		return storage.Invitation{}, workspaceUserManager{}, false
	}
	if invitation.TargetID == nil || *invitation.TargetID != manager.TargetID {
		s.writeError(w, http.StatusNotFound, "not_found", "workspace invitation not found")
		return storage.Invitation{}, workspaceUserManager{}, false
	}
	if invitation.Role == nil || !s.canInviteToTarget(
		r,
		manager.Actor,
		manager.ActorUser,
		manager.Access,
		invitation.Account,
		*invitation.Role,
	) {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot manage this invitation")
		return storage.Invitation{}, workspaceUserManager{}, false
	}

	return invitation, manager, true
}
