package main

import (
	"context"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestRepoCacheDoesNotRegressToAnOlderLoad(t *testing.T) {
	t.Parallel()
	olderStarted := make(chan struct{})
	releaseOlder := make(chan struct{})
	loadCalls := 0
	cache := newRepoCache(time.Hour, func(
		context.Context,
		*github.Client,
		string,
		string,
	) (string, error) {
		loadCalls++
		if loadCalls == 1 {
			close(olderStarted)
			<-releaseOlder

			return "service", nil
		}

		return "action", nil
	})

	olderResult := make(chan string, 1)
	go func() {
		value, err := cache.Get(t.Context(), nil, "owner", "repository")
		if err != nil {
			olderResult <- "error: " + err.Error()

			return
		}
		olderResult <- value
	}()
	<-olderStarted

	newer, err := cache.Get(t.Context(), nil, "owner", "repository")
	if err != nil {
		t.Fatal(err)
	}
	if newer != "action" {
		t.Fatalf("newer load = %q, want action", newer)
	}
	close(releaseOlder)
	if older := <-olderResult; older != "action" {
		t.Fatalf("older load returned %q after newer commit, want action", older)
	}
	cached, err := cache.Get(t.Context(), nil, "owner", "repository")
	if err != nil {
		t.Fatal(err)
	}
	if cached != "action" {
		t.Fatalf("cached value = %q, want action", cached)
	}
	if loadCalls != 2 {
		t.Fatalf("load calls = %d, want 2", loadCalls)
	}
}

func TestRepoCacheKeepsIdentityAcrossRepositoryRename(t *testing.T) {
	t.Parallel()
	loadCalls := make([]string, 0, 2)
	cache := newRepoCache(time.Hour, func(
		_ context.Context,
		_ *github.Client,
		owner string,
		repository string,
	) (string, error) {
		name := repoFullName(owner, repository)
		loadCalls = append(loadCalls, name)
		if name == "new/repository" {
			return "action", nil
		}

		return "service", nil
	})
	const repositoryID = "github:repository:7"
	initial, err := cache.GetByKey(
		t.Context(), nil, repositoryID, "old", "repository",
	)
	if err != nil {
		t.Fatal(err)
	}
	if initial != "service" {
		t.Fatalf("initial value = %q, want service", initial)
	}

	cache.mu.Lock()
	entry := cache.entries[repositoryID]
	entry.fetched = time.Time{}
	cache.entries[repositoryID] = entry
	cache.mu.Unlock()

	renamed, err := cache.GetByKey(
		t.Context(), nil, repositoryID, "new", "repository",
	)
	if err != nil {
		t.Fatal(err)
	}
	if renamed != "action" {
		t.Fatalf("renamed value = %q, want action", renamed)
	}
	fromOldCoordinates, err := cache.GetByKey(
		t.Context(), nil, repositoryID, "old", "repository",
	)
	if err != nil {
		t.Fatal(err)
	}
	if fromOldCoordinates != "action" {
		t.Fatalf("old coordinates returned %q, want action", fromOldCoordinates)
	}
	if len(loadCalls) != 2 {
		t.Fatalf("load calls = %v, want one load per repository name", loadCalls)
	}
}
