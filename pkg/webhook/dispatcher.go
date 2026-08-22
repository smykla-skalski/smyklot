package webhook

import (
	"context"
	"errors"
	"time"
)

// invalidPayload is what a leased row that will not parse is failed with. The
// payload itself is not repeated into the reason: it is untrusted, and it is
// already on the row.
const invalidPayload = "stored webhook payload is invalid"

// lease turns committed inbox rows into bounded worker jobs.
//
// The HTTP side only wakes this loop; queue pressure can therefore delay
// execution but can never discard an acknowledged webhook, because the work is
// in the inbox rather than in memory.
func (p *Pipeline) lease(ctx context.Context) {
	for {
		now := p.opts.Now()
		result, err := p.inbox.Lease(ctx, now, now.Add(p.opts.Timeouts.Lease))
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			p.opts.Logger.Error("delivery inbox read failed", "error", err)
			retryAt := p.opts.Now().Add(leaseRetryDelay)
			if !p.wait(ctx, &retryAt) {
				return
			}

			continue
		}
		if result.Work == nil {
			if !p.wait(ctx, result.AvailableAt) {
				return
			}

			continue
		}

		delivery, err := p.decode(*result.Work)
		if err != nil {
			p.rejectInvalid(ctx, *result.Work, err)

			continue
		}

		select {
		case <-ctx.Done():
			return
		case p.jobs <- delivery:
		}
	}
}

// decode rebuilds a Delivery from a leased row.
//
// The row stores the raw body rather than a decoded struct, so this is where
// the payload is read - by whichever process leased it, which need not be the
// one that accepted it.
func (p *Pipeline) decode(work Work) (Delivery, error) {
	source, err := ParseSource(work.Payload)
	if err != nil {
		return Delivery{}, err
	}

	logger := p.opts.Logger.With(
		"delivery_id", work.DeliveryID,
		"event", eventLabel(work.Event, p.opts.known),
		"repo", source.Repository.FullName,
		"action", source.Action,
	)

	return Delivery{
		Event: work.Event, ID: work.DeliveryID, Source: source,
		Payload: work.Payload, Key: work.Key, ClaimID: work.ClaimID,
		Attempt: work.Attempt, Logger: logger,
	}, nil
}

// rejectInvalid settles a row whose payload will not parse.
//
// Non-retryable, and the handler is never called: a body that is not JSON now
// will not be JSON on the next attempt, and the row is retained so a redelivery
// of the same thing does not start the cycle again.
func (p *Pipeline) rejectInvalid(ctx context.Context, work Work, cause error) {
	p.opts.Logger.Error("stored delivery could not be decoded",
		"delivery_id", work.DeliveryID, "claim_id", work.ClaimID, "error", cause)

	err := p.inbox.Fail(ctx, Failure{
		ClaimID: work.ClaimID, Stage: StageDecode, Reason: invalidPayload,
		Retryable: false, At: p.opts.Now(),
	})
	if err != nil {
		p.opts.Logger.Error("invalid delivery could not be finalized",
			"delivery_id", work.DeliveryID, "claim_id", work.ClaimID, "error", err)
	}
}

// wait sleeps until there is something to do: a wake from an accepted delivery,
// the instant the inbox said work becomes available, or shutdown.
func (p *Pipeline) wait(ctx context.Context, availableAt *time.Time) bool {
	if availableAt == nil {
		select {
		case <-ctx.Done():
			return false
		case <-p.woken:
			return true
		}
	}

	delay := availableAt.Sub(p.opts.Now())
	if delay < 0 {
		delay = 0
	}
	timer := time.NewTimer(delay)
	defer stopTimer(timer)

	select {
	case <-ctx.Done():
		return false
	case <-p.woken:
		return true
	case <-timer.C:
		return true
	}
}

// work is one worker: it runs deliveries until the queue closes.
func (p *Pipeline) work() {
	defer p.workers.Done()

	for delivery := range p.jobs {
		p.run(delivery)
	}
}

// run executes one delivery and records what happened.
func (p *Pipeline) run(delivery Delivery) {
	ctx, cancel := context.WithTimeout(p.jobCtx, p.opts.Timeouts.Job)
	defer cancel()

	started := p.opts.Now()
	err := p.handle(ctx, delivery)
	elapsed := p.opts.Now().Sub(started)
	p.opts.executed(delivery, elapsed, err)

	if err == nil {
		p.finalize(ctx, delivery, OutcomeSucceeded, func(finalizeCtx context.Context) error {
			return p.inbox.Complete(finalizeCtx, delivery.ClaimID, p.opts.Now())
		})
		delivery.Logger.Info("delivery executed", "duration", elapsed.String())

		return
	}

	if delay, again := p.opts.Retry(err, delivery.Attempt); again {
		p.finalize(ctx, delivery, OutcomeRetrying, func(finalizeCtx context.Context) error {
			retryErr := p.inbox.Retry(finalizeCtx, Retry{
				ClaimID: delivery.ClaimID, Stage: StageExecute,
				Reason: err.Error(), At: p.opts.Now().Add(delay),
			})
			if retryErr == nil {
				p.wake()
			}

			return retryErr
		})
		delivery.Logger.Warn("delivery will be retried",
			"error", err, "duration", elapsed.String(),
			"attempt", delivery.Attempt, "retry_in", delay.String())

		return
	}

	p.finalize(ctx, delivery, OutcomeFailed, func(finalizeCtx context.Context) error {
		return p.inbox.Fail(finalizeCtx, Failure{
			ClaimID: delivery.ClaimID, Stage: StageExecute,
			Reason: err.Error(), Retryable: false, At: p.opts.Now(),
		})
	})
	delivery.Logger.Error("delivery failed", "error", err, "duration", elapsed.String())
}

// finalize records an outcome, and keeps trying in the background if the inbox
// will not take it.
//
// A claim stays owned until its outcome is written, so a transient database
// stall delays redelivery instead of stranding the claim for the rest of the
// process lifetime. Shutdown joins these before the caller closes its store.
func (p *Pipeline) finalize(
	ctx context.Context,
	delivery Delivery,
	outcome Outcome,
	write func(context.Context) error,
) {
	finalizeCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx), p.opts.Timeouts.Finalization,
	)
	err := write(finalizeCtx)
	cancel()
	if err == nil {
		p.opts.finalized(delivery, outcome)

		return
	}

	delivery.Logger.Error("delivery outcome could not be persisted",
		"outcome", string(outcome), "error", err)
	p.retryFinalization(delivery, outcome, write)
}

// retryFinalization keeps writing an outcome until it lands or the pipeline
// shuts down.
func (p *Pipeline) retryFinalization(
	delivery Delivery,
	outcome Outcome,
	write func(context.Context) error,
) {
	p.finalizeMu.Lock()
	if p.finalizeClosed {
		p.finalizeMu.Unlock()

		return
	}
	p.finalizing.Add(1)
	retryCtx := p.finalizeCtx
	p.finalizeMu.Unlock()

	go func() {
		defer p.finalizing.Done()

		for attempts := 1; ; attempts++ {
			select {
			case <-retryCtx.Done():
				return
			default:
			}

			attemptCtx, cancel := context.WithTimeout(retryCtx, p.opts.Timeouts.Finalization)
			err := write(attemptCtx)
			cancel()
			if err == nil {
				delivery.Logger.Info("delivery outcome persisted after retry",
					"outcome", string(outcome), "attempts", attempts)
				p.opts.finalized(delivery, outcome)

				return
			}

			timer := time.NewTimer(finalizationRetryDelay)
			select {
			case <-retryCtx.Done():
				stopTimer(timer)

				return
			case <-timer.C:
			}
		}
	}()
}

// stopTimer stops a timer and drains it if it had already fired.
func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
