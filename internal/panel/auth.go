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
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	sessionCookieName = "smyklot_panel_session"
	stateCookieName   = "smyklot_panel_oauth_state"
	inviteCookieName  = "smyklot_panel_invitation"
	returnCookieName  = "smyklot_panel_return"
	tokenBytes        = 32
	oauthStateVersion = byte(1)
	oauthStateContext = "smyklot-panel-oauth-state"
	inviteContext     = "smyklot-panel-invitation"
	returnContext     = "smyklot-panel-return"
	// Longer than any address the panel can produce, and short enough that a
	// cookie carrying one cannot be used as storage.
	maxReturnPath = 512
)

// safeReturnPath reports whether a path is somewhere in THIS panel, and returns
// it when it is.
//
// The whole point of the check is that this value is a redirect target arriving
// from the browser, so anything that could leave the origin is an open redirect -
// the classic phishing primitive, where a link that genuinely begins
// `https://smyklot.com/...` lands on somebody else's sign-in form. Refusing is
// always safe: the reader gets the landing page, which is where they went before
// this existed.
//
// Four ways out of the origin, and each is refused by shape rather than by
// pattern-matching a blocklist:
//
//   - an absolute URL, `https://evil.example/` - has a scheme or a host;
//   - a protocol-relative one, `//evil.example/` - no scheme, and browsers still
//     leave; `net/url` parses it WITH a host, which is what catches it;
//   - a backslash, `/\evil.example`, which several browsers normalise to `//`
//     before they resolve it, so it never reaches Go's idea of a host;
//   - anything outside the panel's own base path, which is not an escape but is
//     not ours to send a reader to either.
//
// THE PATH IS CLEANED BEFORE IT IS COMPARED, and that ordering is the whole of
// the fourth check. `http.Redirect` runs `path.Clean` on the target itself
// before it writes the header, so a prefix test against the raw value is a test
// against something the browser never sees: `/panel/../../evil.example` starts
// with `/panel/` and arrives as `/evil.example`. Same origin, so not an open
// redirect - but the panel is not necessarily the only thing mounted on it, and
// a guarantee that reads as containment has to contain.
//
// The fragment is dropped rather than refused: it never reaches the server, so a
// value carrying one is a browser's, not an attacker's, and the panel restores
// its own address anyway.
func (s *Server) safeReturnPath(value string) (string, bool) {
	if value == "" || len(value) > maxReturnPath {
		return "", false
	}
	if strings.ContainsAny(value, "\\\x00\r\n\t") {
		return "", false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" ||
		parsed.User != nil {
		return "", false
	}
	cleaned := path.Clean(parsed.EscapedPath())
	/* `Clean` drops a trailing slash, which the panel's own router treats as no
	   part of the address anyway, and turns "" into ".". Neither is a path. */
	if !strings.HasPrefix(cleaned, "/") {
		return "", false
	}
	/* ONE SLASH, asked of the value that is actually redirected to.
	   ------------------------------------------------------------------------
	   Nothing reaches this today, and that is the point of writing it here. A
	   protocol-relative target is refused twice over already - `url.Parse` reads
	   `//evil.example` as a HOST, and `path.Clean` collapses `///evil.example`
	   to one slash - and a backslash anywhere is rejected outright. But every one
	   of those questions is asked of `value` or resolved by a transformation,
	   while what reaches `http.Redirect` is `cleaned`, two steps later.

	   CodeQL flags exactly that gap and is right to. A guard that holds because
	   of what an earlier variable happened to look like is one refactor away from
	   holding by luck: drop the `Clean`, or let a future caller pass a path
	   straight in, and the second position is nobody's job again. Asked here, of
	   these bytes, it cannot drift from what is sent. */
	if len(cleaned) > 1 && (cleaned[1] == '/' || cleaned[1] == '\\') {
		return "", false
	}
	if s.cfg.BasePath != "" && !strings.HasPrefix(cleaned, s.cfg.BasePath+"/") {
		return "", false
	}
	if parsed.RawQuery != "" {
		cleaned += "?" + parsed.RawQuery
	}

	return cleaned, true
}

// signedReturnPath binds a return path to this browser, the way an invitation
// intent is bound. Tampering buys nothing on its own - the path is re-checked on
// the way out - but a value the server did not write has no business steering
// where a sign-in lands.
func signedReturnPath(path, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(returnContext))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(path))

	return path + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// readReturnPath answers where a finished sign-in should land, or "" for the
// landing page. Every failure is silent: a reader whose cookie was dropped wants
// the panel, not an error about how they got there.
func (s *Server) readReturnPath(r *http.Request) string {
	cookie, err := r.Cookie(returnCookieName)
	if err != nil {
		return ""
	}
	cut := strings.LastIndex(cookie.Value, ".")
	if cut <= 0 {
		return ""
	}
	path := cookie.Value[:cut]
	if subtle.ConstantTimeCompare(
		[]byte(cookie.Value), []byte(signedReturnPath(path, s.cfg.ClientSecret)),
	) != 1 {
		return ""
	}
	// Checked again on the way out: the rule that matters is where the browser is
	// sent, and a signature only says the server wrote it earlier.
	safe, ok := s.safeReturnPath(path)
	if !ok {
		return ""
	}

	return safe
}

// validInvitationToken reports whether a token could be one this server issued:
// tokenBytes of randomness, base64url, unpadded.
//
// Deliberately its own check rather than the route table's matcher for the same
// shape. This one guards a query parameter and a signed cookie, which arrive
// whatever the panel's routes happen to be, and a security check that reads its
// rule out of the frontend's build output is a security check with a build step
// between it and the thing it protects.
func validInvitationToken(token string) bool {
	if len(token) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token)

	return err == nil
}

type invitationAction string

const (
	invitationAccept  invitationAction = "accept"
	invitationDecline invitationAction = "decline"
)

func (s *Server) startSignIn(w http.ResponseWriter, r *http.Request) {
	s.clearCookie(w, inviteCookieName)
	s.clearCookie(w, returnCookieName)
	// Where the reader was going before they were asked who they are. A pasted
	// address is the ordinary way into a deep page, and losing it sends somebody
	// who asked for one workspace's plan to the front page of everything.
	if path, ok := s.safeReturnPath(r.URL.Query().Get("return_to")); ok {
		s.setCookie(w, returnCookieName, signedReturnPath(path, s.cfg.ClientSecret), s.cfg.StateTTL)
	}
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
		s.failSignIn(w, r, http.StatusUnauthorized, "sign_in_failed")
		return
	}
	state := query.Get("state")
	code := query.Get("code")
	if state == "" || code == "" {
		s.failSignIn(w, r, http.StatusBadRequest, "sign_in_failed")
		return
	}
	cookie, cookieErr := r.Cookie(stateCookieName)
	if cookieErr != nil ||
		subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 ||
		!validOAuthState(state, s.now().UTC(), s.cfg.StateTTL, s.cfg.ClientSecret) {
		s.failSignIn(w, r, http.StatusUnauthorized, "sign_in_failed")
		return
	}

	account, err := s.signIn.ExchangeIdentity(r.Context(), code)
	if err != nil {
		s.failSignIn(w, r, http.StatusBadGateway, "sign_in_failed")
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
		s.failSignIn(w, r, http.StatusForbidden, "forbidden")
		return
	}
	_, err = s.catalog.SyncCatalog(r.Context())
	if err != nil {
		s.failSignIn(w, r, http.StatusBadGateway, "catalog_unavailable")
		return
	}
	s.wakePendingCIGates()
	session, err := s.createSession(r, account.ID)
	if err != nil {
		s.writePageInternal(w, r, err)
		return
	}
	s.setCookie(w, sessionCookieName, session, s.sessionTTL())
	landing := s.cfg.landingPath()
	if back := s.readReturnPath(r); back != "" {
		landing = back
	}
	s.clearCookie(w, returnCookieName)
	/* The target came off a cookie, so a scanner is right to look twice. What it
	   cannot see is that `readReturnPath` verifies the signature AND re-runs
	   `safeReturnPath` on the way out, which is the check that decides this: a
	   path inside this panel, no scheme, no host, nothing protocol-relative. The
	   refusals are held by TestPanelSignInReturnsToTheAddressAsked, one case per
	   way out of the origin. */
	http.Redirect(w, r, landing, http.StatusFound) //nolint:gosec // G710: validated by safeReturnPath, twice.
}

// signInFailedParam carries a failed sign-in back to the front door.
const signInFailedParam = "signin_failed"

// failSignIn sends a reader back to the sign-in card with the reason, rather than
// to an error page of its own.
//
// A sign-in that did not work ends where signing in begins: the card already
// holds the button, so putting the reason beside it makes the answer and the
// retry one thing. The full-page version put them on different screens, and the
// way back from it was a link that started the whole flow again from the top.
//
// What crosses is the status and the code, not the prose. The panel already
// keeps one table of words for these, keyed by exactly that pair, and it is the
// same table the error PAGES read - so a reworded sign-in failure is reworded
// once. Anything the frontend does not recognise falls back to its own sentence,
// which is why nothing here needs to be kept in step by hand.
func (s *Server) failSignIn(w http.ResponseWriter, r *http.Request, status int, code string) {
	query := url.Values{}
	query.Set(signInFailedParam, strconv.Itoa(status)+":"+code)
	s.clearCookie(w, returnCookieName)
	http.Redirect(w, r, s.cfg.landingPath()+"?"+query.Encode(), http.StatusFound)
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
