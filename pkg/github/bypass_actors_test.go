package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveBypassAppUsesActualNodeAndLogo(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apps/smyklot":
			_, _ = fmt.Fprint(w, `{"node_id":"opaque-app-node","owner":{"avatar_url":"https://wrong.example/owner"}}`)
		case "/graphql":
			var request struct{ Variables map[string]string }
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.Variables["id"] != "opaque-app-node" {
				t.Errorf("invented node identity: %v", request.Variables)
			}
			_, _ = fmt.Fprint(w, `{"data":{"node":{"__typename":"App","databaseId":1197525,"name":"Smyklot","slug":"smyklot","logoUrl":"https://avatars.githubusercontent.com/in/1197525"}}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)
	client, err := NewClient("installation", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := client.ResolveBypassActor(t.Context(), "owner", "Integration", "smyklot")
	if err != nil {
		t.Fatal(err)
	}
	if actor.ActorID != 1197525 || actor.Name != "Smyklot" || actor.AvatarURL == nil || *actor.AvatarURL != "https://avatars.githubusercontent.com/in/1197525" {
		t.Fatalf("identity = %+v", actor)
	}
}

func TestBypassDirectoryPagesRulesAndActors(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query     string
			Variables map[string]any
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(request.Query, "node(id:"):
			if request.Variables["id"] != "rule-one" || request.Variables["after"] != "actors-next" {
				t.Errorf("actor cursor = %v", request.Variables)
			}
			_, _ = fmt.Fprint(w, `{"data":{"node":{"bypassActors":{"nodes":[{"actor":{"__typename":"Team","databaseId":9,"name":"Release team","slug":"release"}}],"pageInfo":{"hasNextPage":false}}}}}`)
		case request.Variables["after"] == nil:
			_, _ = fmt.Fprint(w, `{"data":{"repository":{"rulesets":{"nodes":[{"id":"rule-one","bypassActors":{"nodes":[{"actor":{"__typename":"App","databaseId":17,"name":"Release","slug":"release","logoUrl":"http://unsafe.example/logo"}}],"pageInfo":{"hasNextPage":true,"endCursor":"actors-next"}}}],"pageInfo":{"hasNextPage":true,"endCursor":"rules-next"}}}}}`)
		default:
			if request.Variables["after"] != "rules-next" {
				t.Errorf("rule cursor = %v", request.Variables)
			}
			_, _ = fmt.Fprint(w, `{"data":{"repository":{"rulesets":{"nodes":[{"id":"rule-two","bypassActors":{"nodes":[{"actor":null},{"actor":{"__typename":"User","databaseId":8,"login":"bart"}}],"pageInfo":{"hasNextPage":false}}}],"pageInfo":{"hasNextPage":false}}}}}`)
		}
	}))
	t.Cleanup(api.Close)
	client, err := NewClient("installation", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	actors, err := client.ListRepositoryBypassIdentities(t.Context(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(actors) != 3 || actors[0].ActorType != "Integration" || actors[1].Name != "Release team" || actors[2].Name != "bart" {
		t.Fatalf("actors = %+v", actors)
	}
	if actors[0].AvatarURL != nil {
		t.Fatal("unsafe image scheme was passed to panel")
	}
}

func TestBypassDirectoryDoesNotTreatProviderErrorsAsAnEmptyList(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"data":{"repository":null}}`,
		`{"errors":[{"message":"not authorized"}],"data":{"repository":null}}`,
		`{"data":{"repository":{"rulesets":{"nodes":[],"pageInfo":{"hasNextPage":true,"endCursor":null}}}}}`,
	} {
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, body) }))
		t.Cleanup(api.Close)
		client, err := NewClient("installation", api.URL)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.ListRepositoryBypassIdentities(t.Context(), "owner", "repo"); err == nil {
			t.Fatalf("accepted incomplete response %s", body)
		}
	}
}
