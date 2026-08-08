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
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	sessionCookieName = "smyklot_panel_session"
	stateCookieName   = "smyklot_panel_oauth_state"
	tokenBytes        = 32
	oauthStateVersion = byte(1)
	oauthStateContext = "smyklot-panel-oauth-state"
)

func (s *Server) startSignIn(w http.ResponseWriter, r *http.Request) {
	state, err := newOAuthState(s.random, s.now().UTC(), s.cfg.ClientSecret)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	s.setCookie(w, stateCookieName, state, s.cfg.StateTTL)
	http.Redirect(w, r, s.signIn.AuthorizeURL(state), http.StatusFound)
}

func (s *Server) finishSignIn(w http.ResponseWriter, r *http.Request) {
	s.clearCookie(w, stateCookieName)
	query := r.URL.Query()
	if query.Get("error") != "" {
		s.writeError(w, http.StatusUnauthorized, "sign_in_failed", "GitHub sign-in was not completed")
		return
	}
	state := query.Get("state")
	code := query.Get("code")
	if state == "" || code == "" {
		s.writeError(w, http.StatusBadRequest, "sign_in_failed", "GitHub callback is incomplete")
		return
	}
	cookie, cookieErr := r.Cookie(stateCookieName)
	if cookieErr != nil ||
		subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 ||
		!validOAuthState(state, s.now().UTC(), s.cfg.StateTTL, s.cfg.ClientSecret) {
		s.writeError(w, http.StatusUnauthorized, "sign_in_failed", "GitHub sign-in belongs to another browser")
		return
	}

	account, err := s.signIn.ExchangeIdentity(r.Context(), code)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "sign_in_failed", "GitHub sign-in could not be verified")
		return
	}
	account.UpdatedAt = s.now().UTC()
	if err := s.store.UpsertAccount(r.Context(), account); err != nil {
		s.writeInternal(w, err)
		return
	}
	owner, err := s.authorizeOwner(r, account)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	if !owner {
		s.writeError(w, http.StatusForbidden, "forbidden", "this GitHub account does not own the panel")
		return
	}
	_, err = s.catalog.SyncCatalog(r.Context())
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "catalog_unavailable", "GitHub installations could not be synchronized")
		return
	}
	session, err := s.createSession(r, account.ID)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	s.setCookie(w, sessionCookieName, session, s.cfg.SessionTTL)
	http.Redirect(w, r, s.cfg.landingPath(), http.StatusFound)
}

func (s *Server) authorizeOwner(r *http.Request, account storage.Account) (bool, error) {
	owner, err := s.store.IsOwner(r.Context(), account.ID)
	if err != nil || owner {
		return owner, err
	}
	if !strings.EqualFold(account.Login, s.cfg.OwnerLogin) {
		return false, nil
	}

	return s.store.ClaimOwner(r.Context(), account.ID)
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
		ExpiresAt: now.Add(s.cfg.SessionTTL),
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
		if err := s.store.DeleteSession(r.Context(), hash); err != nil {
			s.writeInternal(w, err)
			return
		}
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
