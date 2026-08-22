package gate

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

const (
	workerCount = 4
	lease       = 5 * time.Minute
	RetryDelay  = 5 * time.Second
)

type leaseStore interface {
	LeaseDue(context.Context, time.Time, time.Time) (pendingci.LeaseResult, error)
	RetuneQuietPeriod(context.Context, pendingci.RetuneQuietPeriodRequest) (int64, error)
}

type processor interface {
	Process(context.Context, pendingci.Request) error
}

// Scheduler owns one fallback timer and a bounded worker pool. A
// webhook only wakes this dispatcher; it never runs reconciliation inline.
type Scheduler struct {
	store     leaseStore
	processor processor
	logger    *slog.Logger
	now       func() time.Time
	wake      chan struct{}
	retuneMu  sync.Mutex
	retune    *pendingci.RetuneQuietPeriodRequest
	retuneGen uint64
}

func newScheduler(
	store leaseStore,
	processor processor,
	logger *slog.Logger,
) *Scheduler {
	return &Scheduler{
		store: store, processor: processor, logger: logger,
		now: func() time.Time { return time.Now().UTC() }, wake: make(chan struct{}, 1),
	}
}

func (scheduler *Scheduler) Wake() {
	select {
	case scheduler.wake <- struct{}{}:
	default:
	}
}

// RetunePassingQuiet durably reschedules stable-passing requests. The latest
// value wins, and a storage failure remains pending for the dispatcher's retry
// loop instead of leaving rows on their old deadlines.
func (scheduler *Scheduler) RetunePassingQuiet(value time.Duration) {
	scheduler.retuneMu.Lock()
	scheduler.retuneGen++
	scheduler.retune = &pendingci.RetuneQuietPeriodRequest{
		PassingQuiet: value, ChangedAt: scheduler.now(), InheritedOnly: true,
	}
	scheduler.retuneMu.Unlock()
	scheduler.Wake()
}

func (scheduler *Scheduler) Run(ctx context.Context) {
	jobs := make(chan pendingci.Request, workerCount)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go scheduler.worker(ctx, jobs, &workers)
	}
	scheduler.dispatch(ctx, jobs)
	close(jobs)
	workers.Wait()
}

func (scheduler *Scheduler) dispatch(
	ctx context.Context,
	jobs chan<- pendingci.Request,
) {
	for {
		now := scheduler.now()
		if err := scheduler.applyQuietPeriodRetune(ctx); err != nil {
			scheduler.logger.Error("pending CI quiet-period retune failed", "error", err)
			retryAt := now.Add(RetryDelay)
			if !scheduler.wait(ctx, &retryAt) {
				return
			}
			continue
		}
		lease, err := scheduler.store.LeaseDue(ctx, now, now.Add(lease))
		if err != nil {
			scheduler.logger.Error("pending CI lease failed", "error", err)
			retryAt := now.Add(RetryDelay)
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

func (scheduler *Scheduler) applyQuietPeriodRetune(ctx context.Context) error {
	scheduler.retuneMu.Lock()
	if scheduler.retune == nil {
		scheduler.retuneMu.Unlock()

		return nil
	}
	request := *scheduler.retune
	generation := scheduler.retuneGen
	scheduler.retuneMu.Unlock()

	changed, err := scheduler.store.RetuneQuietPeriod(ctx, request)
	if err != nil {
		return err
	}
	if changed > 0 {
		scheduler.logger.Info(
			"retuned pending CI quiet-period deadlines",
			"requests", changed,
			"quiet_period", request.PassingQuiet,
		)
	}

	scheduler.retuneMu.Lock()
	if scheduler.retuneGen == generation {
		scheduler.retune = nil
	}
	scheduler.retuneMu.Unlock()

	return nil
}

func (scheduler *Scheduler) worker(
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

func (scheduler *Scheduler) wait(ctx context.Context, availableAt *time.Time) bool {
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
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-scheduler.wake:
		return true
	case <-timer.C:
		return true
	}
}
