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
