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
	Login         string                    `json:"login"`
	Role          *storage.InstallationRole `json:"role"`
	ExpiresInDays int                       `json:"expires_in_days"`

	// AcknowledgeDeclined answers the refusal below rather than arriving with the first
	// attempt: a manager sends, is told the identity said no last time, and decides.
	AcknowledgeDeclined bool `json:"acknowledge_declined"`
}

// invitationDraft is one offer about to be written, gathered rather than passed one argument at a
// time - the three entry points differ only in scope and in how the write is authorised.
type invitationDraft struct {
	ActorID             string
	AccountID           string
	TargetID            *string
	Role                *storage.InstallationRole
	SystemRole          *storage.SystemRole
	Days                int
	ElevationID         *string
	SessionTokenHash    string
	AcknowledgeDeclined bool
}

type reissueInvitationRequest struct {
	ExpiresInDays int `json:"expires_in_days"`
}

type invitationResponse struct {
	ID          string                    `json:"id"`
	Account     accountResponse           `json:"account"`
	TargetID    *string                   `json:"target_id,omitempty"`
	TargetName  *string                   `json:"target_name,omitempty"`
	TargetLogin *string                   `json:"target_login,omitempty"`
	TargetKind  *string                   `json:"target_kind,omitempty"`
	Role        *storage.InstallationRole `json:"role,omitempty"`
	SystemRole  *storage.SystemRole       `json:"system_role,omitempty"`
	Status      storage.InvitationStatus  `json:"status"`
	ExpiresAt   time.Time                 `json:"expires_at"`
	CreatedBy   accountResponse           `json:"created_by"`
	CreatedAt   time.Time                 `json:"created_at"`
	RespondedAt *time.Time                `json:"responded_at,omitempty"`
	InviteURL   string                    `json:"invite_url,omitempty"`
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
	if targetID != nil && slices.Contains(page.Roles, storage.InstallationRoleOwner) {
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
	if !validCreateInvitation(w, input) {
		return
	}
	targetID := r.PathValue("target")
	account, err := s.resolvePanelUser(r, targetID, input.Login)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "github_user_unavailable", "GitHub user could not be resolved")
		return
	}
	if s.refusedSelfInvitation(w, actor, account) {
		return
	}
	if !s.canInviteToTarget(r, actor, actorUser, actorAccess, account, *input.Role) {
		s.writeError(w, http.StatusForbidden, "forbidden", "you cannot grant this invitation role")
		return
	}
	s.createInvitation(w, r, invitationDraft{
		ActorID: actor.ID, AccountID: account.ID, TargetID: &targetID, Role: input.Role,
		Days: input.ExpiresInDays, AcknowledgeDeclined: input.AcknowledgeDeclined,
	})
}

// refusedSelfInvitation answers the one refusal a manager can walk into without meaning to.
//
// Inviting yourself is already impossible - canInviteToTarget and canManageTargetUser both refuse
// an actor who is their own subject - but it arrived as "you cannot grant this invitation role",
// which reads as a limit on the role rather than on who is being named. Said first, and said as
// itself, the panel can show it against the login field where it was typed.
func (s *Server) refusedSelfInvitation(
	w http.ResponseWriter,
	actor, subject storage.Account,
) bool {
	if actor.ID != subject.ID {
		return false
	}
	s.writeError(w, http.StatusForbidden, "self_invitation", "you cannot invite yourself")

	return true
}

func (s *Server) createInvitation(w http.ResponseWriter, r *http.Request, draft invitationDraft) {
	id, token, err := s.newInvitationSecrets()
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	now := s.now().UTC()
	invitation, err := s.store.CreateInvitation(r.Context(), storage.InvitationCreate{
		ID: id, TokenHash: tokenHash(token), AccountID: draft.AccountID, TargetID: draft.TargetID,
		Role: draft.Role, SystemRole: draft.SystemRole, ElevationID: draft.ElevationID,
		SessionTokenHash: draft.SessionTokenHash,
		ExpiresAt:        now.Add(time.Duration(draft.Days) * 24 * time.Hour),
		CreatedByAccount: draft.ActorID, CreatedAt: now,
		AcknowledgeDeclined: draft.AcknowledgeDeclined,
	})
	if err != nil {
		s.writeInvitationMutationError(w, draft.TargetID, draft.ElevationID, err)
		return
	}
	s.announceInvitation(invitation)
	writeJSON(w, http.StatusCreated, invitationDTO(invitation, s.invitationURL(token)))
}

func alreadyHasAccessMessage(targetID *string) string {
	if targetID == nil {
		return "this account is already in Smyklot; change its system role instead of inviting it"
	}

	return "this user already has access to this workspace; change their role instead"
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
	s.reissueManagedInvitation(w, r, invitation, actor, nil, "")
}

func (s *Server) reissueManagedInvitation(
	w http.ResponseWriter,
	r *http.Request,
	invitation storage.Invitation,
	actor storage.Account,
	elevationID *string,
	sessionTokenHash string,
) {
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
		ElevationID: elevationID, SessionTokenHash: sessionTokenHash,
		ExpiresAt:        now.Add(time.Duration(input.ExpiresInDays) * 24 * time.Hour),
		CreatedByAccount: actor.ID, CreatedAt: now,
	})
	if err != nil {
		s.writeInvitationMutationError(w, invitation.TargetID, elevationID, err)
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
	s.revokeManagedInvitation(w, r, invitation, actor, nil, "")
}

func (s *Server) revokeManagedInvitation(
	w http.ResponseWriter,
	r *http.Request,
	invitation storage.Invitation,
	actor storage.Account,
	elevationID *string,
	sessionTokenHash string,
) {
	updated, err := s.store.RevokeInvitation(r.Context(), storage.InvitationRevoke{
		ID: invitation.ID, ActorAccountID: actor.ID, ElevationID: elevationID,
		SessionTokenHash: sessionTokenHash, RevokedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeInvitationMutationError(w, invitation.TargetID, elevationID, err)
		return
	}
	s.announceInvitation(updated)
	writeJSON(w, http.StatusOK, invitationDTO(updated, ""))
}

// writeInvitationMutationError names the two refusals the panel can do something about.
//
// Both are conflicts, and both would otherwise arrive as the generic one - "settings changed in
// another session; reload the latest values", or under elevation the stale-Owners message - which
// is untrue and unactionable here. They are answered before either mapping, and for every write
// that can raise them: an offer can become meaningless while it sits unanswered, so renewing one
// is refused on the same ground as making it.
func (s *Server) writeInvitationMutationError(
	w http.ResponseWriter,
	targetID, elevationID *string,
	err error,
) {
	switch {
	case errors.Is(err, storage.ErrAlreadyMember):
		s.writeError(w, http.StatusConflict, "already_has_access", alreadyHasAccessMessage(targetID))
	case errors.Is(err, storage.ErrDeclinedEarlier):
		s.writeError(
			w,
			http.StatusConflict,
			"invitation_declined",
			"this user declined the last invitation; confirm to send another",
		)
	case elevationID != nil:
		s.writeRootWriteError(w, err)
	default:
		s.writeStorageError(w, err)
	}
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
	targetID := r.PathValue("target")
	if invitation.TargetID == nil || *invitation.TargetID != targetID {
		s.writeError(w, http.StatusNotFound, "not_found", "workspace invitation not found")
		return storage.Invitation{}, storage.Account{}, false
	}
	actor, actorUser, access, ok := s.requireTargetUserManager(w, r)
	if !ok || invitation.Role == nil ||
		!s.canInviteToTarget(r, actor, actorUser, access, invitation.Account, *invitation.Role) {
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
	desired storage.InstallationRole,
) bool {
	if actor.ID == subjectAccount.ID || !validGrantedTargetRole(desired) {
		return false
	}
	subject, err := s.store.GetPanelUser(r.Context(), subjectAccount.ID)
	if errors.Is(err, storage.ErrNotFound) {
		return actorAccess.Role == storage.InstallationRoleOwner || actorUser.SystemRole.IsRoot() ||
			actorAccess.Role == storage.InstallationRoleAdmin && desired != storage.InstallationRoleAdmin
	}
	if err != nil || subject.Status == storage.PanelUserBanned || subject.SystemRole.IsRoot() {
		return false
	}
	subjectAccess, err := s.store.ResolveTargetAccess(
		r.Context(), subjectAccount.ID, actorAccessTarget(r), s.now().UTC(),
	)
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
		TargetName: invitation.TargetName, TargetLogin: invitation.TargetLogin,
		TargetKind: invitation.TargetKind,
		Role:       invitation.Role, SystemRole: invitation.SystemRole,
		Status:    invitation.Status,
		ExpiresAt: invitation.ExpiresAt, CreatedBy: accountDTO(invitation.CreatedBy),
		CreatedAt: invitation.CreatedAt, RespondedAt: invitation.RespondedAt, InviteURL: inviteURL,
	}
}

func validCreateInvitation(w http.ResponseWriter, input createInvitationRequest) bool {
	validRole := input.Role != nil && validGrantedTargetRole(*input.Role)
	if strings.TrimSpace(input.Login) == "" || !validRole ||
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
