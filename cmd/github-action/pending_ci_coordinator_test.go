package main

import (
	"context"
	"testing"
	"time"
)

func TestPendingCICoordinatorSerializesRepositoryWork(t *testing.T) {
	t.Parallel()
	coordinator := newPendingCICoordinator()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- coordinator.Exclusive(context.Background(), "repository-1", func() error {
			close(firstStarted)
			<-releaseFirst

			return nil
		})
	}()
	<-firstStarted

	otherStarted := make(chan struct{})
	otherDone := make(chan error, 1)
	go func() {
		otherDone <- coordinator.Exclusive(context.Background(), "repository-2", func() error {
			close(otherStarted)

			return nil
		})
	}()
	select {
	case <-otherStarted:
	case <-time.After(time.Second):
		t.Fatal("independent repository was blocked")
	}
	if err := <-otherDone; err != nil {
		t.Fatal(err)
	}

	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- coordinator.Exclusive(context.Background(), "repository-1", func() error {
			close(secondStarted)

			return nil
		})
	}()
	select {
	case <-secondStarted:
		t.Fatal("same repository entered concurrent work")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("same repository did not resume")
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}

func TestPendingCICoordinatorHonorsCancellation(t *testing.T) {
	t.Parallel()
	coordinator := newPendingCICoordinator()
	release := make(chan struct{})
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- coordinator.Exclusive(context.Background(), "repository", func() error {
			close(started)
			<-release

			return nil
		})
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := coordinator.Exclusive(ctx, "repository", func() error {
		called = true

		return nil
	})
	if err == nil {
		t.Fatal("cancelled acquisition succeeded")
	}
	if called {
		t.Fatal("cancelled operation ran")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
