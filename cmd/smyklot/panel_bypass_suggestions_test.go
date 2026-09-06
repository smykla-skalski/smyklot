package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestBypassSuggestionsResolveExactSlugBesideCachedPartialMatches(t *testing.T) {
	t.Parallel()
	requests := make(chan string, 4)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		switch r.URL.Path {
		case "/apps/release":
			_, _ = fmt.Fprint(w, `{"node_id":"release-app"}`)
		case "/graphql":
			_, _ = fmt.Fprint(w, `{"data":{"node":{"__typename":"App","databaseId":17,"name":"Release","slug":"release"}}}`)
		default:
			t.Errorf("unexpected lookup %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)
	client, err := github.NewClient("installation", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	directory := &bypassDirectory{items: []github.BypassActorIdentity{{ActorID: 18, ActorType: "Integration", Name: "Release helper", Slug: "release-helper"}}}
	for range 2 {
		items, err := lookupBypassSuggestions(t.Context(), client, "workspace", directory, "Integration", "release")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 || items[0].ActorID != 17 {
			t.Fatalf("exact app hidden: %+v", items)
		}
	}
	if len(requests) != 2 {
		t.Fatalf("exact app was not cached: %d requests", len(requests))
	}
}

func TestBypassSuggestionsKeepPartialMatchesWhenExactSlugDoesNotExist(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	t.Cleanup(api.Close)
	client, err := github.NewClient("installation", api.URL)
	if err != nil {
		t.Fatal(err)
	}
	directory := &bypassDirectory{items: []github.BypassActorIdentity{{ActorID: 18, ActorType: "Integration", Name: "Release helper", Slug: "release-helper"}}}
	items, err := lookupBypassSuggestions(t.Context(), client, "workspace", directory, "Integration", "rele")
	if err != nil || len(items) != 1 || items[0].ActorID != 18 {
		t.Fatalf("partial matches lost: %+v, %v", items, err)
	}
}
