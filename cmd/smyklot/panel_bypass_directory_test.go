package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestBypassInstallationStatusDoesNotInventRepositoryAccess(t *testing.T) {
	t.Parallel()
	now := time.Now()
	directory := &bypassDirectory{currentAppID: 1, installationsKnown: true, installations: []github.AppInstallationAccess{
		{AppID: 2, RepositorySelection: "all"},
		{AppID: 3, RepositorySelection: "selected"},
		{AppID: 4, RepositorySelection: "all", SuspendedAt: &now},
		{AppID: 5, RepositorySelection: "unrecognized"},
	}}
	for id, want := range map[int64]string{1: "unknown", 2: "all_repositories", 3: "selected_repositories", 4: "suspended", 5: "unknown", 6: "not_installed"} {
		if got := bypassInstallationStatus(github.BypassActorIdentity{ActorID: id, ActorType: "Integration"}, directory); got != want {
			t.Errorf("app %d = %q, want %q", id, got, want)
		}
	}
	directory.currentAppID = 3
	if got := bypassInstallationStatus(github.BypassActorIdentity{ActorID: 3, ActorType: "Integration"}, directory); got != "selected_repositories" {
		t.Fatalf("current app selection overridden: %q", got)
	}
	directory.installationsKnown = false
	if got := bypassInstallationStatus(github.BypassActorIdentity{ActorID: 6, ActorType: "Integration"}, directory); got != "unknown" {
		t.Fatalf("unknown inventory = %q", got)
	}
	if got := bypassInstallationStatus(github.BypassActorIdentity{ActorID: 6, ActorType: "Team"}, directory); got != "" {
		t.Fatalf("team installation = %q", got)
	}
}

func TestBypassSuggestionsMatchNamesAndSlugsWithinActorKind(t *testing.T) {
	t.Parallel()
	items := []github.BypassActorIdentity{
		{ActorID: 1, ActorType: "Integration", Name: "A release helper", Slug: "helper"},
		{ActorID: 2, ActorType: "Integration", Name: "Release bot", Slug: "release"},
		{ActorID: 3, ActorType: "Team", Name: "Release team", Slug: "release"},
	}
	before := append([]github.BypassActorIdentity(nil), items...)
	matches := matchingBypassActors(items, "Integration", "ReLeAsE")
	if len(matches) != 2 || matches[0].ActorID != 2 || matches[1].ActorID != 1 {
		t.Fatalf("matches = %+v", matches)
	}
	if !reflect.DeepEqual(items, before) {
		t.Fatal("search mutated the cached roster")
	}
	if matches := matchingBypassActors(items, "Integration", "missing"); len(matches) != 0 {
		t.Fatalf("unrelated match: %+v", matches)
	}
}

func TestBypassDirectoryKeepsSuccessfulSourcesWhenOneFails(t *testing.T) {
	t.Parallel()
	items, complete := collectBypassIdentities(t.Context(), []bypassIdentitySource{
		func(context.Context) ([]github.BypassActorIdentity, error) {
			return []github.BypassActorIdentity{{ActorID: 17, Name: "Release"}}, nil
		},
		func(context.Context) ([]github.BypassActorIdentity, error) {
			return nil, errors.New("permission denied")
		},
	})
	if complete || len(items) != 1 || items[0].ActorID != 17 {
		t.Fatalf("partial sources = %+v, %v", items, complete)
	}
}
