package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

const (
	// One lease covers the longest possible wait behind the bounded in-memory
	// handoff plus its own execution. Startup recovery ignores the old lease, so
	// this protects a live slow queue without delaying crash recovery.
	deliveryLeaseDuration   = jobTimeout*time.Duration(queueDepth/workerCount+2) + deliveryFinalizationTimeout
	deliveryStoreRetryDelay = time.Second
	invalidDeliveryPayload  = "stored webhook payload is invalid"
)

// deliveryInbox is the narrow persistence boundary required by webhook
// dispatch. It deliberately excludes panel, catalog, authentication, and CI
// state operations.
type deliveryInbox interface {
	LeaseDelivery(context.Context, time.Time, time.Time) (storage.DeliveryLeaseResult, error)
	FailDelivery(context.Context, storage.DeliveryFailureChange) error
}

type deliveryDecoder func(storage.DeliveryWork) (job, error)

// deliveryDispatcher turns committed inbox rows into bounded worker jobs. HTTP
// acceptance only wakes it; queue pressure can therefore delay execution but
// can never discard an acknowledged webhook.
type deliveryDispatcher struct {
	store  deliveryInbox
	jobs   chan<- job
	decode deliveryDecoder
	logger *slog.Logger
	wake   chan struct{}

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
}

func newDeliveryDispatcher(
	store deliveryInbox,
	jobs chan<- job,
	decode deliveryDecoder,
	logger *slog.Logger,
) *deliveryDispatcher {
	return &deliveryDispatcher{
		store: store, jobs: jobs, decode: decode, logger: logger,
		wake: make(chan struct{}, 1),
	}
}

func (d *deliveryDispatcher) Start(parent context.Context) {
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	if d.cancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(parent)
	d.cancel = cancel
	d.done = make(chan struct{})
	go func() {
		defer close(d.done)
		d.run(ctx)
	}()
}

func (d *deliveryDispatcher) Stop() {
	d.lifecycleMu.Lock()
	cancel, done := d.cancel, d.done
	d.lifecycleMu.Unlock()
	if cancel == nil {
		return
	}

	cancel()
	<-done
}

func (d *deliveryDispatcher) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *deliveryDispatcher) run(ctx context.Context) {
	for {
		now := time.Now().UTC()
		lease, err := d.store.LeaseDelivery(ctx, now, now.Add(deliveryLeaseDuration))
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			d.logger.Error("durable delivery inbox read failed", "error", err)
			retryAt := time.Now().Add(deliveryStoreRetryDelay)
			if !d.wait(ctx, &retryAt) {
				return
			}

			continue
		}
		if lease.Work == nil {
			if !d.wait(ctx, lease.AvailableAt) {
				return
			}

			continue
		}

		j, err := d.decode(*lease.Work)
		if err != nil {
			d.rejectInvalid(ctx, *lease.Work, err)

			continue
		}
		select {
		case <-ctx.Done():
			return
		case d.jobs <- j:
		}
	}
}

func (d *deliveryDispatcher) rejectInvalid(
	ctx context.Context,
	work storage.DeliveryWork,
	cause error,
) {
	d.logger.Error(
		"durable delivery could not be decoded",
		"delivery_id", work.DeliveryID,
		"claim_id", work.ID,
		"error", cause,
	)
	err := d.store.FailDelivery(ctx, storage.DeliveryFailureChange{
		ClaimID:   work.ID,
		Stage:     "decode",
		Reason:    invalidDeliveryPayload,
		Retryable: false,
		FailedAt:  time.Now().UTC(),
	})
	if err != nil {
		d.logger.Error(
			"invalid durable delivery could not be finalized",
			"delivery_id", work.DeliveryID,
			"claim_id", work.ID,
			"error", err,
		)
	}
}

func (d *deliveryDispatcher) wait(ctx context.Context, availableAt *time.Time) bool {
	if availableAt == nil {
		select {
		case <-ctx.Done():
			return false
		case <-d.wake:
			return true
		}
	}

	delay := time.Until(*availableAt)
	if delay < 0 {
		delay = 0
	}
	timer := time.NewTimer(delay)
	defer stopTimer(timer)
	select {
	case <-ctx.Done():
		return false
	case <-d.wake:
		return true
	case <-timer.C:
		return true
	}
}

func (s *server) deliveryJob(work storage.DeliveryWork) (job, error) {
	event, err := webhook.ParseIssueComment(work.Payload)
	if err != nil {
		return job{}, fmt.Errorf("parse persisted issue comment: %w", err)
	}
	if !event.Actionable() {
		return job{}, errors.New("persisted issue comment is not actionable")
	}
	if err := validateCommentInput(runtimeConfigFor(event, s.cfg)); err != nil {
		return job{}, fmt.Errorf("validate persisted issue comment: %w", err)
	}

	logger := s.logger.With(
		"delivery_id", work.DeliveryID,
		"event", webhook.EventIssueComment,
		"repo", event.Repository.FullName,
		"pr", event.Issue.Number,
		"action", event.Action,
		"attempt", work.Attempt,
	)

	return job{
		event:      event,
		key:        work.ClaimKey,
		deliveryID: work.DeliveryID,
		claimID:    work.ID,
		attempt:    work.Attempt,
		logger:     logger,
	}, nil
}
