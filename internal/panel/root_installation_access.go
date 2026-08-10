package panel

import (
	"net/http"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func (s *Server) getRootTargetUsers(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.requireRootInstallationManager(w, r, false)
	if !ok {
		return
	}
	s.listInstallationUsers(w, r, manager)
}

func (s *Server) postRootTargetUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	manager, ok := s.requireRootInstallationManager(w, r, true)
	if !ok {
		return
	}
	s.addInstallationUser(w, r, manager)
}

func (s *Server) putRootTargetUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	manager, ok := s.requireRootInstallationManager(w, r, true)
	if !ok {
		return
	}
	s.updateInstallationUser(w, r, manager)
}

func (s *Server) getRootTargetUserDecisions(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.requireRootInstallationManager(w, r, false)
	if !ok {
		return
	}
	s.listInstallationUserDecisions(w, r, manager.TargetID)
}

func (s *Server) getRootTargetInvitations(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.requireRootInstallationManager(w, r, false)
	if !ok {
		return
	}
	s.listInvitations(w, r, &manager.TargetID)
}

func (s *Server) postRootTargetInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	manager, ok := s.requireRootInstallationManager(w, r, true)
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
	if !s.canInviteToTarget(
		r, manager.Actor, manager.ActorUser, manager.Access, account, *input.Role,
	) {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot grant this invitation role")
		return
	}
	s.createInvitation(
		w,
		r,
		manager.Actor.ID,
		account.ID,
		&manager.TargetID,
		input.Role,
		nil,
		input.ExpiresInDays,
		manager.ElevationID,
		manager.SessionTokenHash,
	)
}

func (s *Server) reissueRootTargetInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	invitation, manager, ok := s.requireRootInstallationInvitationManager(w, r)
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
	invitation, manager, ok := s.requireRootInstallationInvitationManager(w, r)
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
	s.getInstallationFailurePage(w, r, context.Target.ID)
}

func (s *Server) requireRootInstallationManager(
	w http.ResponseWriter,
	r *http.Request,
	write bool,
) (installationUserManager, bool) {
	context, ok := s.requireRootTarget(w, r, write)
	if !ok {
		return installationUserManager{}, false
	}
	user, err := s.store.GetPanelUser(r.Context(), context.Account.ID)
	if err != nil {
		s.writeInternal(w, err)
		return installationUserManager{}, false
	}

	return installationUserManager{
		Actor:            context.Account,
		ActorUser:        user,
		Access:           context.Access,
		TargetID:         context.Target.ID,
		ElevationID:      elevationID(context.Elevation),
		SessionTokenHash: context.SessionHash,
		RootWrite:        write,
	}, true
}

func (s *Server) requireRootInstallationInvitationManager(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Invitation, installationUserManager, bool) {
	manager, ok := s.requireRootInstallationManager(w, r, true)
	if !ok {
		return storage.Invitation{}, installationUserManager{}, false
	}
	invitation, err := s.store.GetInvitation(
		r.Context(), r.PathValue("invitation"), s.now().UTC(),
	)
	if err != nil {
		s.writeStorageError(w, err)
		return storage.Invitation{}, installationUserManager{}, false
	}
	if invitation.TargetID == nil || *invitation.TargetID != manager.TargetID {
		s.writeError(w, http.StatusNotFound, "not_found", "installation invitation not found")
		return storage.Invitation{}, installationUserManager{}, false
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
		return storage.Invitation{}, installationUserManager{}, false
	}

	return invitation, manager, true
}
