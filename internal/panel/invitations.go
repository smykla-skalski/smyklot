package panel

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const defaultInviteDays = 7

type createInvitationRequest struct {
	Login         string             `json:"login"`
	Role          *storage.PanelRole `json:"role"`
	TargetID      string             `json:"target_id"`
	ExpiresInDays int                `json:"expires_in_days"`
}

type reissueInvitationRequest struct {
	ExpiresInDays int `json:"expires_in_days"`
}

type invitationResponse struct {
	ID          string                   `json:"id"`
	Account     accountResponse          `json:"account"`
	TargetID    *string                  `json:"target_id,omitempty"`
	TargetName  *string                  `json:"target_name,omitempty"`
	Role        storage.PanelRole        `json:"role"`
	Status      storage.InvitationStatus `json:"status"`
	ExpiresAt   time.Time                `json:"expires_at"`
	CreatedBy   accountResponse          `json:"created_by"`
	CreatedAt   time.Time                `json:"created_at"`
	RespondedAt *time.Time               `json:"responded_at,omitempty"`
	InviteURL   string                   `json:"invite_url,omitempty"`
}

func (s *Server) getInvitations(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireGlobalUserManager(w, r); !ok {
		return
	}
	s.listInvitations(w, r, nil)
}

func (s *Server) getTargetInvitations(w http.ResponseWriter, r *http.Request) {
	if _, _, _, ok := s.requireTargetUserManager(w, r); !ok {
		return
	}
	targetID := r.PathValue("target")
	s.listInvitations(w, r, &targetID)
}

func (s *Server) listInvitations(w http.ResponseWriter, r *http.Request, targetID *string) {
	page, err := parseInvitationPage(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if targetID != nil && slices.Contains(page.Roles, storage.PanelRoleOwner) {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "owner is not a target invitation role")
		return
	}
	invitations, err := s.store.ListInvitationPage(r.Context(), targetID, s.now().UTC(), page)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invitationPageDTO(invitations))
}

func (s *Server) postInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	actor, actorUser, ok := s.requireGlobalUserManager(w, r)
	if !ok {
		return
	}
	var input createInvitationRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validCreateInvitation(w, input, true) {
		return
	}
	if *input.Role == storage.PanelRoleOwner && !actorUser.Root {
		s.writeError(w, http.StatusForbidden, "forbidden", "only the root owner can invite owners")
		return
	}
	account, err := s.resolvePanelUser(r, input.TargetID, input.Login)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "github_user_unavailable", "GitHub user could not be resolved")
		return
	}
	if account.ID == actor.ID {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot invite yourself")
		return
	}
	s.createInvitation(w, r, actor.ID, account.ID, nil, *input.Role, input.ExpiresInDays)
}

func (s *Server) postTargetInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	actor, actorUser, actorAccess, ok := s.requireTargetUserManager(w, r)
	if !ok {
		return
	}
	var input createInvitationRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validCreateInvitation(w, input, false) {
		return
	}
	targetID := r.PathValue("target")
	account, err := s.resolvePanelUser(r, targetID, input.Login)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "github_user_unavailable", "GitHub user could not be resolved")
		return
	}
	if !s.canInviteToTarget(r, actor, actorUser, actorAccess, account, *input.Role) {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot grant this invitation role")
		return
	}
	s.createInvitation(w, r, actor.ID, account.ID, &targetID, *input.Role, input.ExpiresInDays)
}

func (s *Server) createInvitation(
	w http.ResponseWriter,
	r *http.Request,
	actorID, accountID string,
	targetID *string,
	role storage.PanelRole,
	days int,
) {
	id, token, err := s.newInvitationSecrets()
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	now := s.now().UTC()
	invitation, err := s.store.CreateInvitation(r.Context(), storage.InvitationCreate{
		ID: id, TokenHash: tokenHash(token), AccountID: accountID, TargetID: targetID,
		Role: role, ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour),
		CreatedByAccount: actorID, CreatedAt: now,
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.announceInvitation(invitation)
	writeJSON(w, http.StatusCreated, invitationDTO(invitation, s.invitationURL(token)))
}

func (s *Server) reviewInvitation(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	invitation, err := s.store.GetInvitationByToken(r.Context(), tokenHash(token), s.now().UTC())
	if err != nil && !errors.Is(err, storage.ErrExpired) {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invitationDTO(invitation, ""))
}

func (s *Server) reissueInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	invitation, actor, ok := s.requireInvitationManager(w, r)
	if !ok {
		return
	}
	var input reissueInvitationRequest
	if !decodeJSON(w, r, &input) || !validInviteDays(w, input.ExpiresInDays) {
		return
	}
	_, token, err := s.newInvitationSecrets()
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	now := s.now().UTC()
	updated, err := s.store.ReissueInvitation(r.Context(), storage.InvitationReissue{
		ID: invitation.ID, TokenHash: tokenHash(token),
		ExpiresAt:        now.Add(time.Duration(input.ExpiresInDays) * 24 * time.Hour),
		CreatedByAccount: actor.ID, CreatedAt: now,
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.announceInvitation(updated)
	writeJSON(w, http.StatusOK, invitationDTO(updated, s.invitationURL(token)))
}

func (s *Server) deleteInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	invitation, actor, ok := s.requireInvitationManager(w, r)
	if !ok {
		return
	}
	updated, err := s.store.RevokeInvitation(r.Context(), storage.InvitationRevoke{
		ID: invitation.ID, ActorAccountID: actor.ID, RevokedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.announceInvitation(updated)
	writeJSON(w, http.StatusOK, invitationDTO(updated, ""))
}

func (s *Server) requireInvitationManager(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Invitation, storage.Account, bool) {
	invitation, err := s.store.GetInvitation(r.Context(), r.PathValue("invitation"), s.now().UTC())
	if err != nil {
		s.writeStorageError(w, err)
		return storage.Invitation{}, storage.Account{}, false
	}
	if invitation.TargetID == nil {
		actor, actorUser, ok := s.requireGlobalUserManager(w, r)
		if !ok || invitation.Role == storage.PanelRoleOwner && !actorUser.Root ||
			actor.ID == invitation.Account.ID {
			if ok {
				s.writeError(w, http.StatusForbidden, "forbidden", "you cannot manage this invitation")
			}
			return storage.Invitation{}, storage.Account{}, false
		}
		return invitation, actor, true
	}
	r.SetPathValue("target", *invitation.TargetID)
	actor, actorUser, access, ok := s.requireTargetUserManager(w, r)
	if !ok || !s.canInviteToTarget(r, actor, actorUser, access, invitation.Account, invitation.Role) {
		if ok {
			s.writeError(w, http.StatusForbidden, "forbidden", "you cannot manage this invitation")
		}
		return storage.Invitation{}, storage.Account{}, false
	}

	return invitation, actor, true
}

func (s *Server) canInviteToTarget(
	r *http.Request,
	actor storage.Account,
	actorUser storage.PanelUser,
	actorAccess storage.TargetAccess,
	subjectAccount storage.Account,
	desired storage.PanelRole,
) bool {
	if actor.ID == subjectAccount.ID || !validGrantedTargetRole(desired) {
		return false
	}
	subject, err := s.store.GetPanelUser(r.Context(), subjectAccount.ID)
	if errors.Is(err, storage.ErrNotFound) {
		return actorAccess.Role == storage.PanelRoleOwner || actorUser.Root ||
			actorAccess.Role == storage.PanelRoleAdmin && desired != storage.PanelRoleAdmin
	}
	if err != nil || subject.Status == storage.PanelUserBanned || subject.Root ||
		subject.GlobalRole == storage.PanelRoleOwner {
		return false
	}
	subjectAccess, err := s.store.ResolveTargetAccess(r.Context(), subjectAccount.ID, actorAccessTarget(r))
	if err != nil {
		return false
	}

	return canManageTargetUser(actor, actorUser, actorAccess, subject, subjectAccess, desired)
}

func actorAccessTarget(r *http.Request) string {
	return r.PathValue("target")
}

func (s *Server) newInvitationSecrets() (string, string, error) {
	id, err := randomToken(s.random)
	if err != nil {
		return "", "", err
	}
	token, err := randomToken(s.random)

	return id, token, err
}

func (s *Server) invitationURL(token string) string {
	return s.cfg.PublicOrigin + s.cfg.BasePath + "/invite/" + url.PathEscape(token)
}

func (s *Server) announceInvitation(invitation storage.Invitation) {
	if invitation.TargetID == nil {
		s.events.announce(panelEvent{Type: panelEventResync})
		return
	}
	s.events.announce(panelEvent{Type: "invitation.changed", TargetID: *invitation.TargetID})
}

func invitationDTO(invitation storage.Invitation, inviteURL string) invitationResponse {
	return invitationResponse{
		ID: invitation.ID, Account: accountDTO(invitation.Account), TargetID: invitation.TargetID,
		TargetName: invitation.TargetName, Role: invitation.Role, Status: invitation.Status,
		ExpiresAt: invitation.ExpiresAt, CreatedBy: accountDTO(invitation.CreatedBy),
		CreatedAt: invitation.CreatedAt, RespondedAt: invitation.RespondedAt, InviteURL: inviteURL,
	}
}

func validCreateInvitation(w http.ResponseWriter, input createInvitationRequest, global bool) bool {
	validRole := input.Role != nil && validGrantedTargetRole(*input.Role)
	if global && input.Role != nil {
		validRole = validGlobalPanelRole(*input.Role) && *input.Role != storage.PanelRoleNone
	}
	validTarget := !global || strings.TrimSpace(input.TargetID) != ""
	if strings.TrimSpace(input.Login) == "" || !validRole || !validTarget ||
		!validInviteDays(w, input.ExpiresInDays) {
		if input.ExpiresInDays == 1 || input.ExpiresInDays == 7 || input.ExpiresInDays == 30 {
			writeError(w, http.StatusBadRequest, "invalid_request", "invitation policy is incomplete")
		}
		return false
	}

	return true
}

func validInviteDays(w http.ResponseWriter, days int) bool {
	if days == 1 || days == defaultInviteDays || days == 30 {
		return true
	}
	writeError(w, http.StatusBadRequest, "invalid_request", "invitation expiry must be 1, 7, or 30 days")

	return false
}

func invitationErrorStatus(err error) (int, string, string) {
	switch {
	case errors.Is(err, storage.ErrIdentityMismatch):
		return http.StatusForbidden, "wrong_identity", "this invitation belongs to another GitHub account"
	case errors.Is(err, storage.ErrExpired):
		return http.StatusGone, "invitation_expired", "this invitation expired"
	case errors.Is(err, storage.ErrConflict), errors.Is(err, storage.ErrRevoked):
		return http.StatusConflict, "invitation_used", "this invitation is no longer pending"
	case errors.Is(err, storage.ErrNotFound):
		return http.StatusNotFound, "invitation_not_found", "this invitation does not exist"
	default:
		return http.StatusInternalServerError, "internal", "the invitation response failed"
	}
}
