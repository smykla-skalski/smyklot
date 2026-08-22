package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestRepoCacheCollapsesConcurrentMisses(t *testing.T) {
	t.Parallel()

	// This used to assert that a load which started earlier could not overwrite
	// a newer one, by running two at once. Two can no longer run at once: the
	// cache admits one refresh per key, which is the stronger property and the
	// one worth pinning. A configuration file now costs up to five requests to
	// read, and a cold start is exactly when every delivery for a repository
	// arrives together.
	const callers = 16

	started := make(chan struct{})
	release := make(chan struct{})

	var loadCalls atomic.Int64

	cache := newRepoCache(time.Hour, func(
		context.Context,
		*github.Client,
		string,
		string,
		*string,
	) (string, error) {
		if loadCalls.Add(1) == 1 {
			close(started)
			<-release
		}

		return "service", nil
	})

	results := make(chan string, callers)

	var wg sync.WaitGroup

	// The first caller blocks inside the load, so the rest arrive while it is
	// in flight - which is the only moment collapsing can be observed.
	wg.Add(1)

	go func() {
		defer wg.Done()

		value, err := cache.Get(t.Context(), nil, "owner", "repository")
		if err != nil {
			t.Error(err)
		}

		results <- value
	}()

	<-started

	for range callers - 1 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			value, err := cache.Get(t.Context(), nil, "owner", "repository")
			if err != nil {
				t.Error(err)
			}

			results <- value
		}()
	}

	// Give the waiters a moment to reach the cache before the load returns, so
	// they join the call in flight rather than finding a filled cache.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(results)

	for value := range results {
		if value != "service" {
			t.Errorf("caller saw %q, want service", value)
		}
	}

	if calls := loadCalls.Load(); calls != 1 {
		t.Errorf("load ran %d times for %d concurrent callers, want 1", calls, callers)
	}
}

// A later read still replaces an earlier one; collapsing is only for callers
// that overlap.
func TestRepoCacheReloadsAfterItsEntryExpires(t *testing.T) {
	t.Parallel()

	var loadCalls atomic.Int64

	cache := newRepoCache(time.Nanosecond, func(
		context.Context,
		*github.Client,
		string,
		string,
		*string,
	) (string, error) {
		if loadCalls.Add(1) == 1 {
			return "service", nil
		}

		return "action", nil
	})

	first, err := cache.Get(t.Context(), nil, "owner", "repository")
	if err != nil {
		t.Fatal(err)
	}

	if first != "service" {
		t.Fatalf("first read = %q, want service", first)
	}

	second, err := cache.Get(t.Context(), nil, "owner", "repository")
	if err != nil {
		t.Fatal(err)
	}

	if second != "action" {
		t.Fatalf("second read = %q, want action", second)
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
		_ *string,
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
