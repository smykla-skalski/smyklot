package webhook

import (
	"context"
	"errors"
	"time"
)

const invalidPayload = "stored webhook payload is invalid"

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

	return p.opts.decorate(Delivery{
		Event: work.Event, ID: work.DeliveryID, Source: source,
		Payload: work.Payload, Key: work.Key, ClaimID: work.ClaimID,
		Attempt: work.Attempt, Logger: logger,
	}), nil
}

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
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-p.woken:
		return true
	case <-timer.C:
		return true
	}
}

func (p *Pipeline) work() {
	defer p.workers.Done()

	for delivery := range p.jobs {
		p.run(delivery)
	}
}

func (p *Pipeline) run(delivery Delivery) {
	ctx, cancel := context.WithTimeout(context.Background(), p.opts.Timeouts.Job)
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

	retryable := false
	if delivery.Attempt > 1 {
		_, retryable = p.opts.Retry(err, 1)
	}
	p.finalize(ctx, delivery, OutcomeFailed, func(finalizeCtx context.Context) error {
		return p.inbox.Fail(finalizeCtx, Failure{
			ClaimID: delivery.ClaimID, Stage: StageExecute,
			Reason: err.Error(), Retryable: retryable, At: p.opts.Now(),
		})
	})
	delivery.Logger.Error("delivery failed", "error", err, "duration", elapsed.String())
}

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
				timer.Stop()

				return
			case <-timer.C:
			}
		}
	}()
}
