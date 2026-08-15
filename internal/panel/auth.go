package panel

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	sessionCookieName = "smyklot_panel_session"
	stateCookieName   = "smyklot_panel_oauth_state"
	inviteCookieName  = "smyklot_panel_invitation"
	tokenBytes        = 32
	oauthStateVersion = byte(1)
	oauthStateContext = "smyklot-panel-oauth-state"
	inviteContext     = "smyklot-panel-invitation"
)

type invitationAction string

const (
	invitationAccept  invitationAction = "accept"
	invitationDecline invitationAction = "decline"
)

func (s *Server) startSignIn(w http.ResponseWriter, r *http.Request) {
	s.clearCookie(w, inviteCookieName)
	if token := r.URL.Query().Get("invite"); token != "" {
		action := invitationAction(r.URL.Query().Get("action"))
		if !validInvitationToken(token) ||
			(action != invitationAccept && action != invitationDecline) {
			s.writePageError(w, r, http.StatusBadRequest, "invalid_invitation", "invitation action is invalid")
			return
		}
		invitation, inviteErr := s.store.GetInvitationByToken(
			r.Context(), tokenHash(token), s.now().UTC(),
		)
		if inviteErr != nil || invitation.Status != storage.InvitationPending {
			status, code, message := invitationErrorStatus(inviteErr)
			if inviteErr == nil {
				status, code, message = http.StatusConflict, "invitation_used", "this invitation is no longer pending"
			}
			s.writePageError(w, r, status, code, message)
			return
		}
		s.setCookie(
			w,
			inviteCookieName,
			signedInvitationIntent(action, token, s.cfg.ClientSecret),
			s.cfg.StateTTL,
		)
	}
	state, err := newOAuthState(s.random, s.now().UTC(), s.cfg.ClientSecret)
	if err != nil {
		s.writePageInternal(w, r, err)
		return
	}
	s.setCookie(w, stateCookieName, state, s.cfg.StateTTL)
	http.Redirect(w, r, s.signIn.AuthorizeURL(state), http.StatusFound)
}

func (s *Server) finishSignIn(w http.ResponseWriter, r *http.Request) {
	s.clearCookie(w, stateCookieName)
	s.clearCookie(w, inviteCookieName)
	query := r.URL.Query()
	if query.Get("error") != "" {
		s.writePageError(w, r, http.StatusUnauthorized, "sign_in_failed", "GitHub sign-in was not completed")
		return
	}
	state := query.Get("state")
	code := query.Get("code")
	if state == "" || code == "" {
		s.writePageError(w, r, http.StatusBadRequest, "sign_in_failed", "GitHub callback is incomplete")
		return
	}
	cookie, cookieErr := r.Cookie(stateCookieName)
	if cookieErr != nil ||
		subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 ||
		!validOAuthState(state, s.now().UTC(), s.cfg.StateTTL, s.cfg.ClientSecret) {
		s.writePageError(w, r, http.StatusUnauthorized, "sign_in_failed", "GitHub sign-in belongs to another browser")
		return
	}

	account, err := s.signIn.ExchangeIdentity(r.Context(), code)
	if err != nil {
		s.writePageError(w, r, http.StatusBadGateway, "sign_in_failed", "GitHub sign-in could not be verified")
		return
	}
	account.UpdatedAt = s.now().UTC()
	if err := s.store.UpsertAccount(r.Context(), account); err != nil {
		s.writePageInternal(w, r, err)
		return
	}
	invited, handled := s.respondToInvitation(w, r, account)
	if handled {
		return
	}
	authorized, err := s.authorizeAccount(r, account)
	if err != nil {
		s.writePageInternal(w, r, err)
		return
	}
	authorized = authorized || invited
	if !authorized {
		s.writePageError(w, r, http.StatusForbidden, "forbidden", "this GitHub account cannot access the panel")
		return
	}
	_, err = s.catalog.SyncCatalog(r.Context())
	if err != nil {
		s.writePageError(w, r, http.StatusBadGateway, "catalog_unavailable", "GitHub installations could not be synchronized")
		return
	}
	session, err := s.createSession(r, account.ID)
	if err != nil {
		s.writePageInternal(w, r, err)
		return
	}
	s.setCookie(w, sessionCookieName, session, s.sessionTTL())
	http.Redirect(w, r, s.cfg.landingPath(), http.StatusFound)
}

// respondToInvitation applies the browser-bound invitation intent, when one is
// present. handled means the helper already wrote a terminal response.
func (s *Server) respondToInvitation(
	w http.ResponseWriter,
	r *http.Request,
	account storage.Account,
) (invited, handled bool) {
	intent, hasInvitation, err := readInvitationIntent(r, s.cfg.ClientSecret)
	if err != nil {
		s.writePageError(w, r, http.StatusUnauthorized, "invalid_invitation", "invitation response belongs to another browser")

		return false, true
	}
	if !hasInvitation {
		return false, false
	}
	invitation, err := s.store.RespondToInvitation(r.Context(), storage.InvitationResponse{
		TokenHash: tokenHash(intent.token), AccountID: account.ID,
		Accept: intent.action == invitationAccept, At: s.now().UTC(),
	})
	if err != nil {
		status, code, message := invitationErrorStatus(err)
		s.writePageError(w, r, status, code, message)

		return false, true
	}
	s.announceInvitation(invitation)
	if intent.action == invitationDecline {
		http.Redirect(w, r, s.invitationURL(intent.token)+"?declined=1", http.StatusFound)

		return false, true
	}

	return true, false
}

func (s *Server) authorizeAccount(r *http.Request, account storage.Account) (bool, error) {
	if account.SubjectID == strconv.FormatInt(s.cfg.SuperRootID, 10) {
		if err := s.store.ReconcileSuperRoot(r.Context(), account.ID, s.now().UTC()); err != nil {
			return false, err
		}

		return true, nil
	}
	user, err := s.store.GetPanelUser(r.Context(), account.ID)
	if errors.Is(err, storage.ErrNotFound) {
		return s.store.ActivateDerivedOwner(r.Context(), account.ID, s.now().UTC())
	}
	if err != nil {
		return false, err
	}

	return user.Status == storage.PanelUserActive, nil
}

func (s *Server) createSession(r *http.Request, accountID string) (string, error) {
	token, err := randomToken(s.random)
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	if err := s.store.CreateSession(r.Context(), storage.Session{
		TokenHash: tokenHash(token),
		AccountID: accountID,
		CreatedAt: now,
		ExpiresAt: now.Add(s.sessionTTL()),
	}, MaxSessions); err != nil {
		return "", err
	}

	return token, nil
}

func (s *Server) signOut(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	_, hash, err := s.viewer(r)
	if err == nil {
		if err := s.store.DeleteSession(
			r.Context(), hash, storage.ElevationRevoked, s.now().UTC(),
		); err != nil {
			s.writeInternal(w, err)
			return
		}
		s.events.revokeSession(hash, "signed_out", "You signed out")
	} else if !errors.Is(err, storage.ErrNotFound) && !errors.Is(err, storage.ErrExpired) {
		s.writeInternal(w, err)
		return
	}
	s.clearCookie(w, sessionCookieName)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is conditional only for explicit HTTP local development.
		Name:     name,
		Value:    value,
		Path:     s.cfg.cookiePath(),
		MaxAge:   int(ttl.Seconds()),
		Secure:   strings.HasPrefix(s.cfg.PublicOrigin, "https://"),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is conditional only for explicit HTTP local development.
		Name:     name,
		Value:    "",
		Path:     s.cfg.cookiePath(),
		MaxAge:   -1,
		Secure:   strings.HasPrefix(s.cfg.PublicOrigin, "https://"),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func randomToken(source io.Reader) (string, error) {
	bytes := make([]byte, tokenBytes)
	if _, err := io.ReadFull(source, bytes); err != nil {
		return "", fmt.Errorf("generate panel token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func newOAuthState(source io.Reader, issuedAt time.Time, secret string) (string, error) {
	payload := make([]byte, 1+8+tokenBytes)
	payload[0] = oauthStateVersion
	if _, err := binary.Encode(payload[1:9], binary.BigEndian, issuedAt.Unix()); err != nil {
		return "", fmt.Errorf("encode OAuth state timestamp: %w", err)
	}
	if _, err := io.ReadFull(source, payload[9:]); err != nil {
		return "", fmt.Errorf("generate OAuth state: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(append(payload, oauthStateMAC(payload, secret)...)), nil
}

func validOAuthState(state string, now time.Time, ttl time.Duration, secret string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil || len(decoded) != 1+8+tokenBytes+sha256.Size || decoded[0] != oauthStateVersion {
		return false
	}
	payload, actualMAC := decoded[:1+8+tokenBytes], decoded[1+8+tokenBytes:]
	if !hmac.Equal(actualMAC, oauthStateMAC(payload, secret)) {
		return false
	}
	var issuedAtUnix int64
	if _, err := binary.Decode(payload[1:9], binary.BigEndian, &issuedAtUnix); err != nil {
		return false
	}
	issuedAt := time.Unix(issuedAtUnix, 0).UTC()

	return !issuedAt.After(now) && now.Before(issuedAt.Add(ttl))
}

func oauthStateMAC(payload []byte, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(oauthStateContext))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(payload)

	return mac.Sum(nil)
}

func tokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))

	return hex.EncodeToString(hash[:])
}

type invitationIntent struct {
	action invitationAction
	token  string
}

func signedInvitationIntent(action invitationAction, token, secret string) string {
	payload := string(action) + "." + token
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(inviteContext))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(payload))

	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func readInvitationIntent(r *http.Request, secret string) (invitationIntent, bool, error) {
	cookie, err := r.Cookie(inviteCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return invitationIntent{}, false, nil
	}
	if err != nil {
		return invitationIntent{}, false, err
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 {
		return invitationIntent{}, false, errors.New("invalid invitation intent")
	}
	action := invitationAction(parts[0])
	token := parts[1]
	if !validInvitationToken(token) ||
		(action != invitationAccept && action != invitationDecline) {
		return invitationIntent{}, false, errors.New("invalid invitation intent")
	}
	expected := signedInvitationIntent(action, token, secret)
	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(expected)) != 1 {
		return invitationIntent{}, false, errors.New("invalid invitation intent")
	}

	return invitationIntent{action: action, token: token}, true, nil
}
