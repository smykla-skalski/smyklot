package github_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestRepositoryIdentityUsesImmutableID(t *testing.T) {
	for _, returnedID := range []int64{11, 12} {
		t.Run(fmt.Sprint(returnedID), func(t *testing.T) {
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/repositories/11" {
					t.Errorf("repository identity request = %s %s", r.Method, r.URL)
				}
				_, _ = fmt.Fprintf(w, `{"id":%d,"name":"renamed","full_name":"acme/renamed","owner":{"login":"acme"},"default_branch":"release","private":true}`, returnedID)
			}))
			defer endpoint.Close()
			client, err := github.NewClient("test-token", endpoint.URL)
			if err != nil {
				t.Fatal(err)
			}
			repository, err := client.GetRepositoryByID(t.Context(), 11)
			if returnedID != 11 {
				if err == nil {
					t.Fatal("accepted a different repository identity")
				}
				return
			}
			if err != nil || repository.ID != 11 || repository.FullName != "acme/renamed" ||
				repository.Owner != "acme" || repository.Name != "renamed" || repository.DefaultBranch != "release" || !repository.Private {
				t.Fatalf("current identity = %+v (%v)", repository, err)
			}
		})
	}
}

func TestRepositoryIdentityRejectsInvalidIDBeforeHTTP(t *testing.T) {
	var client *github.Client
	for _, id := range []int64{0, -1} {
		if _, err := client.GetRepositoryByID(t.Context(), id); err == nil {
			t.Fatalf("invalid identity %d was accepted", id)
		}
	}
}
