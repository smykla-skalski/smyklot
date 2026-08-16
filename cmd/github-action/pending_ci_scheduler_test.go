package main

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

func TestPendingCISchedulerWakePreemptsFallbackTimer(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &schedulerTestStore{now: now, firstLease: make(chan struct{})}
	processor := &schedulerTestProcessor{processed: make(chan pendingci.Request, 1)}
	scheduler := newPendingCIScheduler(
		store,
		processor,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	scheduler.now = func() time.Time { return now }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()

	select {
	case <-store.firstLease:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not inspect durable work")
	}
	scheduler.Wake()
	select {
	case request := <-processor.processed:
		if request.ID != 7 {
			t.Fatalf("processed request %d, want 7", request.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("webhook wake did not preempt fallback timer")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop")
	}
}

func TestPendingCISchedulerRetunesBeforeLeasing(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &schedulerTestStore{
		now: now, firstLease: make(chan struct{}),
		retuned: make(chan pendingci.RetuneQuietPeriodRequest, 1),
	}
	scheduler := newPendingCIScheduler(
		store,
		&schedulerTestProcessor{processed: make(chan pendingci.Request, 1)},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	scheduler.now = func() time.Time { return now }
	scheduler.RetunePassingQuiet(45 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()

	select {
	case request := <-store.retuned:
		if request.PassingQuiet != 45*time.Second || request.ChangedAt != now {
			t.Fatalf("retune request = %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not retune durable quiet-period deadlines")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop")
	}
}

type schedulerTestStore struct {
	mu         sync.Mutex
	now        time.Time
	calls      int
	firstLease chan struct{}
	retuned    chan pendingci.RetuneQuietPeriodRequest
}

func (store *schedulerTestStore) RetuneQuietPeriod(
	_ context.Context,
	request pendingci.RetuneQuietPeriodRequest,
) (int64, error) {
	if store.retuned != nil {
		store.retuned <- request
	}

	return 1, nil
}

func (store *schedulerTestStore) LeaseDue(
	_ context.Context,
	_ time.Time,
	_ time.Time,
) (pendingci.LeaseResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.calls++
	switch store.calls {
	case 1:
		close(store.firstLease)
		available := store.now.Add(time.Hour)

		return pendingci.LeaseResult{AvailableAt: &available}, nil
	case 2:
		return pendingci.LeaseResult{Request: &pendingci.Request{ID: 7}}, nil
	default:
		return pendingci.LeaseResult{}, nil
	}
}

type schedulerTestProcessor struct {
	processed chan pendingci.Request
}

func (processor *schedulerTestProcessor) Process(
	_ context.Context,
	request pendingci.Request,
) error {
	processor.processed <- request

	return nil
}
