package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

const (
	pendingCIWorkerCount = 4
	pendingCILease       = 5 * time.Minute
	pendingCIRetryDelay  = 5 * time.Second
)

type pendingCILeaseStore interface {
	LeaseDue(context.Context, time.Time, time.Time) (pendingci.LeaseResult, error)
}

type pendingCIProcessor interface {
	Process(context.Context, pendingci.Request) error
}

// pendingCIScheduler owns one fallback timer and a bounded worker pool. A
// webhook only wakes this dispatcher; it never runs reconciliation inline.
type pendingCIScheduler struct {
	store     pendingCILeaseStore
	processor pendingCIProcessor
	logger    *slog.Logger
	now       func() time.Time
	wake      chan struct{}
}

func newPendingCIScheduler(
	store pendingCILeaseStore,
	processor pendingCIProcessor,
	logger *slog.Logger,
) *pendingCIScheduler {
	return &pendingCIScheduler{
		store: store, processor: processor, logger: logger,
		now: func() time.Time { return time.Now().UTC() }, wake: make(chan struct{}, 1),
	}
}

func (scheduler *pendingCIScheduler) Wake() {
	select {
	case scheduler.wake <- struct{}{}:
	default:
	}
}

func (scheduler *pendingCIScheduler) Run(ctx context.Context) {
	jobs := make(chan pendingci.Request, pendingCIWorkerCount)
	var workers sync.WaitGroup
	for range pendingCIWorkerCount {
		workers.Add(1)
		go scheduler.worker(ctx, jobs, &workers)
	}
	scheduler.dispatch(ctx, jobs)
	close(jobs)
	workers.Wait()
}

func (scheduler *pendingCIScheduler) dispatch(
	ctx context.Context,
	jobs chan<- pendingci.Request,
) {
	for {
		now := scheduler.now()
		lease, err := scheduler.store.LeaseDue(ctx, now, now.Add(pendingCILease))
		if err != nil {
			scheduler.logger.Error("pending CI lease failed", "error", err)
			retryAt := now.Add(pendingCIRetryDelay)
			if !scheduler.wait(ctx, &retryAt) {
				return
			}
			continue
		}
		if lease.Request == nil {
			if !scheduler.wait(ctx, lease.AvailableAt) {
				return
			}
			continue
		}
		select {
		case jobs <- *lease.Request:
		case <-ctx.Done():
			return
		}
	}
}

func (scheduler *pendingCIScheduler) worker(
	ctx context.Context,
	jobs <-chan pendingci.Request,
	workers *sync.WaitGroup,
) {
	defer workers.Done()
	for request := range jobs {
		if err := scheduler.processor.Process(ctx, request); err != nil {
			scheduler.logger.Error(
				"pending CI reconciliation failed",
				"request", request.ID,
				"repository", request.RepositoryFullName,
				"pull_request", request.PullRequest,
				"error", err,
			)
		}
		scheduler.Wake()
	}
}

func (scheduler *pendingCIScheduler) wait(ctx context.Context, availableAt *time.Time) bool {
	if availableAt == nil {
		select {
		case <-ctx.Done():
			return false
		case <-scheduler.wake:
			return true
		}
	}
	delay := availableAt.Sub(scheduler.now())
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer stopTimer(timer)
	select {
	case <-ctx.Done():
		return false
	case <-scheduler.wake:
		return true
	case <-timer.C:
		return true
	}
}
