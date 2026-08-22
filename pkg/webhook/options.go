package webhook

import (
	"context"
	"log/slog"
	"time"
)

type Handler func(ctx context.Context, delivery Delivery) error

type Screen func(delivery Delivery) (bool, error)

type Observer struct {
	Received  func(event, outcome string)
	Executed  func(delivery Delivery, elapsed time.Duration, err error)
	Finalized func(delivery Delivery, outcome Outcome)
}

type Timeouts struct {
	Job          time.Duration
	Finalization time.Duration
	Lease        time.Duration
	Drain        time.Duration
}

const (
	defaultWorkers      = 8
	defaultQueueDepth   = 256
	defaultJob          = 5 * time.Minute
	defaultFinalization = 5 * time.Second
	defaultDrain        = 30 * time.Second

	leaseRetryDelay        = time.Second
	finalizationRetryDelay = time.Second
)

type Options struct {
	Events     []string
	Screen     Screen
	Retry      Retryable
	Workers    int
	QueueDepth int
	Timeouts   Timeouts
	Logger     *slog.Logger
	Observer   Observer
	Attrs      func(Delivery) []slog.Attr
	Now        func() time.Time
}

type resolved struct {
	Options

	known map[string]struct{}
}

func (o Options) resolve() resolved {
	if o.Workers <= 0 {
		o.Workers = defaultWorkers
	}
	if o.QueueDepth <= 0 {
		o.QueueDepth = defaultQueueDepth
	}
	if o.Timeouts.Job <= 0 {
		o.Timeouts.Job = defaultJob
	}
	if o.Timeouts.Finalization <= 0 {
		o.Timeouts.Finalization = defaultFinalization
	}
	if o.Timeouts.Drain <= 0 {
		o.Timeouts.Drain = defaultDrain
	}
	if o.Timeouts.Lease <= 0 {
		o.Timeouts.Lease = o.Timeouts.Job*
			time.Duration(o.QueueDepth/o.Workers+2) + o.Timeouts.Finalization
	}
	if o.Retry == nil {
		o.Retry = DefaultRetry
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	if o.Now == nil {
		o.Now = func() time.Time { return time.Now().UTC() }
	}

	known := make(map[string]struct{}, len(o.Events))
	for _, event := range o.Events {
		known[event] = struct{}{}
	}

	return resolved{Options: o, known: known}
}

func (r resolved) accepts(event string) bool {
	_, ok := r.known[event]

	return ok
}

func (r resolved) received(event, outcome string) {
	if r.Observer.Received != nil {
		r.Observer.Received(eventLabel(event, r.known), outcome)
	}
}

func (r resolved) executed(delivery Delivery, elapsed time.Duration, err error) {
	if r.Observer.Executed != nil {
		r.Observer.Executed(delivery, elapsed, err)
	}
}

func (r resolved) finalized(delivery Delivery, outcome Outcome) {
	if r.Observer.Finalized != nil {
		r.Observer.Finalized(delivery, outcome)
	}
}

func (r resolved) decorate(delivery Delivery) Delivery {
	if r.Attrs == nil {
		return delivery
	}
	attrs := r.Attrs(delivery)
	if len(attrs) == 0 {
		return delivery
	}

	delivery.Logger = slog.New(delivery.Logger.Handler().WithAttrs(attrs))

	return delivery
}
