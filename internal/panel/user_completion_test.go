package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

type fakeCandidates struct {
	accounts []storage.Account
	err      error
	calls    int
}

func (f *fakeCandidates) ListTargetCandidates(
	context.Context,
	string,
) ([]storage.Account, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}

	return f.accounts, nil
}

func candidateAccount(subject, login, name string) storage.Account {
	return storage.Account{
		ID:          "github:test:user:" + subject,
		Provider:    "github:test",
		SubjectID:   subject,
		Login:       login,
		DisplayName: name,
	}
}

func suggestionLogins(t *testing.T, body []byte) []string {
	t.Helper()
	var response struct {
		Items []struct {
			Login string `json:"login"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode suggestions: %v", err)
	}
	logins := make([]string, 0, len(response.Items))
	for _, item := range response.Items {
		logins = append(logins, item.Login)
	}

	return logins
}

func TestUserSuggestionsCompleteFromTheOrganizationRoster(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "panel-owner")
	session := harness.signIn(t)
	harness.server.candidates = &fakeCandidates{accounts: []storage.Account{
		candidateAccount("100", "marta", "Marta Nowak"),
		candidateAccount("101", "marek", "Marek Kowalski"),
		candidateAccount("102", "tomasz", "Tomasz Mars"),
		candidateAccount("103", "ada", "Marzena Adamska"),
	}}

	response := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/user-suggestions?q=mar",
		nil,
		session,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	/* Logins starting with the query come first, then a display name starting
	   with it, then anything merely containing it - which is the order somebody
	   typing the first letters of a name is looking for. */
	logins := suggestionLogins(t, response.Body.Bytes())
	want := []string{"marek", "marta", "ada", "tomasz"}
	if len(logins) != len(want) {
		t.Fatalf("logins = %v, want %v", logins, want)
	}
	for index, login := range want {
		if logins[index] != login {
			t.Fatalf("logins = %v, want %v", logins, want)
		}
	}
}

func TestUserSuggestionsSayNothingForAShortQuery(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "panel-owner")
	session := harness.signIn(t)
	candidates := &fakeCandidates{accounts: []storage.Account{
		candidateAccount("100", "marta", "Marta Nowak"),
	}}
	harness.server.candidates = candidates

	for _, query := range []string{"", "m", "%20"} {
		response := harness.request(
			t,
			http.MethodGet,
			"/panel/api/v1/targets/github:installation:10/user-suggestions?q="+query,
			nil,
			session,
		)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		if logins := suggestionLogins(t, response.Body.Bytes()); len(logins) != 0 {
			t.Fatalf("query %q returned %v, want nothing", query, logins)
		}
	}

	// One letter matches most of any roster, so the roster is never even read.
	if candidates.calls != 0 {
		t.Fatalf("roster read %d times for queries too short to answer", candidates.calls)
	}
}

func TestUserSuggestionsStayEmptyWhenTheRosterCannotBeRead(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "panel-owner")
	session := harness.signIn(t)
	harness.server.candidates = &fakeCandidates{err: errors.New("organization unreachable")}

	/* Completion is an offer. A roster the panel could not read leaves the field
	   as it was before completion existed, which still works, so this must not
	   reach the reader as a failure. */
	response := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/user-suggestions?q=mar",
		nil,
		session,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if logins := suggestionLogins(t, response.Body.Bytes()); len(logins) != 0 {
		t.Fatalf("logins = %v, want nothing", logins)
	}
}

func TestUserSuggestionsWithoutADirectoryAreEmpty(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "panel-owner")
	session := harness.signIn(t)

	// A deployment that cannot list anybody still adds users by typed login.
	response := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/user-suggestions?q=mar",
		nil,
		session,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if logins := suggestionLogins(t, response.Body.Bytes()); len(logins) != 0 {
		t.Fatalf("logins = %v, want nothing", logins)
	}
}

func TestUserSuggestionsNeedTheStandingToAddSomebody(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "panel-owner")
	harness.server.candidates = &fakeCandidates{accounts: []storage.Account{
		candidateAccount("100", "marta", "Marta Nowak"),
	}}

	/* Naming everyone in an organization is not public information, and somebody
	   who cannot add a person has no reason to be handed the list of people they
	   could have added. */
	response := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/user-suggestions?q=mar",
		nil,
		nil,
	)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestUserSuggestionsCapWhatTheyOffer(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "panel-owner")
	session := harness.signIn(t)
	roster := make([]storage.Account, 0, maxUserSuggestions*3)
	for index := range maxUserSuggestions * 3 {
		roster = append(roster, candidateAccount(
			string(rune('a'+index)),
			"marek-"+string(rune('a'+index)),
			"Marek",
		))
	}
	harness.server.candidates = &fakeCandidates{accounts: roster}

	response := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/user-suggestions?q=marek",
		nil,
		session,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if logins := suggestionLogins(t, response.Body.Bytes()); len(logins) != maxUserSuggestions {
		t.Fatalf("offered %d logins, want %d", len(logins), maxUserSuggestions)
	}
}

func TestUserSuggestionsSkipPeopleWhoAlreadyHaveAccess(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "panel-owner")
	session := harness.signIn(t)

	/* The signed-in owner is on this workspace already. Offering them as
	   somebody to add is an invitation to a conflict, and the panel refuses the
	   change anyway. */
	harness.server.candidates = &fakeCandidates{accounts: []storage.Account{
		candidateAccount("1", "panel-owner", "Panel Owner"),
		candidateAccount("100", "panel-newcomer", "Panel Newcomer"),
	}}

	response := harness.request(
		t,
		http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/user-suggestions?q=panel",
		nil,
		session,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	logins := suggestionLogins(t, response.Body.Bytes())
	if len(logins) != 1 || logins[0] != "panel-newcomer" {
		t.Fatalf("logins = %v, want only panel-newcomer", logins)
	}
}
