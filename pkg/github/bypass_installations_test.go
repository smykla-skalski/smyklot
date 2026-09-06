package github

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstallationInventoryPagesAndRetainsAccess(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orgs/workspace/installations" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("page") == "1" {
			_, _ = fmt.Fprint(w, `{"total_count":2,"installations":[{"app_id":17,"app_slug":"release","repository_selection":"all"}]}`)
		} else {
			_, _ = fmt.Fprint(w, `{"total_count":2,"installations":[{"app_id":18,"app_slug":"other","repository_selection":"selected","suspended_at":"2026-09-01T12:00:00Z"}]}`)
		}
	}))
	t.Cleanup(api.Close)
	client, err := NewClient("installation", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	items, err := client.ListOrganizationAppInstallations(t.Context(), "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].RepositorySelection != "all" || items[1].RepositorySelection != "selected" || items[1].SuspendedAt == nil {
		t.Fatalf("inventory = %+v", items)
	}
}

func TestInstallationInventoryCannotProveAbsenceFromIncompleteData(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{}`, `{"total_count":0}`, `{"installations":[]}`, `{"total_count":-1,"installations":[]}`,
		`{"total_count":1,"installations":[]}`, `{"total_count":1,"installations":[{"app_id":0}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, body) }))
			t.Cleanup(api.Close)
			client, err := NewClient("installation", api.URL)
			if err != nil {
				t.Fatal(err)
			}
			if items, err := client.ListOrganizationAppInstallations(t.Context(), "workspace"); err == nil || items != nil {
				t.Fatalf("incomplete inventory accepted: %+v, %v", items, err)
			}
		})
	}
}

func TestSecretTeamCannotBeAddedAsBypass(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":{"organization":{"team":{"databaseId":17,"name":"Secret","slug":"secret","privacy":"SECRET"}}}}`)
	}))
	t.Cleanup(api.Close)
	client, err := NewClient("installation", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ResolveBypassActor(t.Context(), "workspace", "Team", "secret"); err == nil {
		t.Fatal("secret team accepted")
	}
}
