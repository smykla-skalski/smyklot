package panel

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestInvitationRecoveryPreservesOnlyVerifiedIntent(t *testing.T) {
	for _, tampered := range []bool{false, true} {
		name := "verified wrong account"
		if tampered {
			name = "unverified browser intent"
		}
		t.Run(name, func(t *testing.T) {
			checkInvitationRecovery(t, tampered)
		})
	}
}

func checkInvitationRecovery(t *testing.T, tampered bool) {
	t.Helper()
	h := newPanelHarness(t, "owner")
	owner := h.signIn(t)
	invitee := storage.Account{ID: "github:test:user:invited", Provider: "github:test", SubjectID: "invited", Login: "invited", DisplayName: "Invited User", UpdatedAt: h.now}
	h.server.users = fakeUserResolver{account: invitee}
	created := h.request(t, http.MethodPost, "/panel/api/v1/targets/github:installation:10/invitations", strings.NewReader(`{"login":"invited","role":"editor","expires_in_days":7}`), owner)
	requireResponse(t, created, "create invitation", http.StatusCreated)
	var invitation struct {
		InviteURL string `json:"invite_url"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &invitation); err != nil {
		t.Fatal(err)
	}
	inviteURL, err := url.Parse(invitation.InviteURL)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimPrefix(inviteURL.Path, "/panel/invite/")
	start := httptest.NewRecorder()
	h.handler.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/panel/auth/github/start?invite="+token+"&action=accept", nil))
	requireResponse(t, start, "start invitation", http.StatusFound)
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callback := httptest.NewRequest(http.MethodGet, "/panel/auth/github/callback?code=code&state="+url.QueryEscape(location.Query().Get("state")), nil)
	callback.Header.Set("Accept", "text/html")
	callback.AddCookie(responseCookie(t, start, stateCookieName))
	intent := responseCookie(t, start, inviteCookieName)
	if tampered {
		intent.Value += "x"
	}
	callback.AddCookie(intent)
	response := httptest.NewRecorder()
	h.handler.ServeHTTP(response, callback)
	status := http.StatusForbidden
	if tampered {
		status = http.StatusUnauthorized
	}
	requireResponse(t, response, "refused invitation", status)
	body := html.UnescapeString(response.Body.String())
	if tampered {
		if strings.Contains(body, "invitation_token") || strings.Contains(body, token) {
			t.Fatal("unverified token exposed as recovery context")
		}
	} else if !strings.Contains(body, `"invitation_token":"`+token+`"`) {
		t.Fatal("verified invitation context lost")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("invitation error must not be cached")
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookieName && cookie.Value != "" {
			t.Fatal("failed invitation created a session")
		}
	}
	review := h.request(t, http.MethodGet, "/panel/api/v1/invites/"+token, nil, nil)
	requireResponse(t, review, "invitation remains pending", http.StatusOK, `"status":"pending"`)
	h.acceptInvitation(t, invitee, token)
	review = h.request(t, http.MethodGet, "/panel/api/v1/invites/"+token, nil, nil)
	requireResponse(t, review, "correct identity can still accept", http.StatusOK, `"status":"accepted"`)
}
