package panel

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

type fakeBypassDirectory struct {
	calls               int
	target, kind, query string
	err                 error
}

func (fake *fakeBypassDirectory) LookupBypassActors(_ context.Context, target, kind, query string) ([]github.BypassActorIdentity, bool, error) {
	fake.calls++
	fake.target, fake.kind, fake.query = target, kind, query
	return []github.BypassActorIdentity{{ActorID: 1197525, ActorType: "Integration", Name: "Smyklot", Slug: "smyklot", AvatarURL: new("https://avatars.githubusercontent.com/in/1197525")}}, true, fake.err
}

func TestBypassActorLookupIsScopedAndResolvesNames(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)
	directory := &fakeBypassDirectory{}
	harness.server.bypassActors = directory
	path := "/panel/api/v1/targets/github:installation:10/bypass-actors?type=Integration&q=smyklot"
	response := harness.request(t, http.MethodGet, path, nil, nil)
	requireResponse(t, response, "unauthenticated directory", http.StatusUnauthorized)
	response = harness.request(t, http.MethodGet, path, nil, createOrdinarySession(t, harness))
	requireResponse(t, response, "unowned directory", http.StatusNotFound)
	if directory.calls != 0 {
		t.Fatal("directory queried before authorization")
	}
	response = harness.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, response, "named app", http.StatusOK, `"name":"Smyklot"`, `"avatar_url":"https://avatars.githubusercontent.com/in/1197525"`)
	if directory.target != "github:installation:10" || directory.kind != "Integration" || directory.query != "smyklot" {
		t.Fatalf("lookup scope = %+v", directory)
	}
	directory.err = errors.New("private provider URL and credentials")
	response = harness.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, response, "provider failure", http.StatusBadGateway, `"code":"github_actor_unavailable"`)
	if contains := response.Body.String(); strings.Contains(contains, directory.err.Error()) {
		t.Fatal("provider details leaked")
	}
	for _, query := range []string{"?type=DeployKey&q=name", "?type=Integration&q=../secret", "?q=smyklot"} {
		before := directory.calls
		response = harness.request(t, http.MethodGet, "/panel/api/v1/targets/github:installation:10/bypass-actors"+query, nil, session)
		requireResponse(t, response, "invalid query", http.StatusBadRequest)
		if directory.calls != before {
			t.Fatal("invalid query reached GitHub")
		}
	}
}
