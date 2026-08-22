package webhook

import (
	"context"
	"log/slog"
	"time"
)

// Handler runs one delivery. Returning an error hands it to the retry policy;
// returning nil completes it.
//
// A handler is called at most once per attempt but may be called more than once
// per delivery, so it has to be safe to repeat.
type Handler func(ctx context.Context, delivery Delivery) error

// Screen decides whether a parsed delivery is worth persisting.
//
// It runs before the claim, and that is the point of it: most of what GitHub
// sends is noise - a comment the App itself wrote, a comment on an issue that
// is not a pull request, a check run nobody subscribed to - and a screen that
// says no costs one parse instead of one row. A false answers 204; an error
// answers 400.
type Screen func(delivery Delivery) (bool, error)

// Observer receives what a deployment wants to count.
//
// Callbacks rather than a metrics registry: a Prometheus dependency here would
// put this library's opinion about namespaces into every consumer's process.
// Every field is optional, and each is called from the goroutine that did the
// work, so a slow one slows the pipeline.
type Observer struct {
	// Received fires once per HTTP request, with the sanitized event name and
	// one of the Outcome* request outcomes.
	Received func(event, outcome string)

	// Executed fires when the handler returns, before the outcome is written.
	// It is where a latency histogram belongs: finalization can be retried out
	// of band and would otherwise distort the measurement.
	Executed func(delivery Delivery, elapsed time.Duration, err error)

	// Finalized fires once the outcome has reached the inbox, and only then. A
	// consumer that has to tell somebody a delivery finished does it here, so
	// it cannot announce work the inbox does not agree happened.
	Finalized func(delivery Delivery, outcome Outcome)
}

// Timeouts bound every stage a delivery passes through. Every zero value is a
// working default.
type Timeouts struct {
	// Job caps one handler call.
	Job time.Duration

	// Finalization gives the outcome write its own window after the handler's
	// context has already expired, so a delivery that timed out still records
	// that it did.
	Finalization time.Duration

	// Lease is how long a leased delivery is reserved. Zero derives it from
	// Job, Workers and QueueDepth: one lease has to cover the longest possible
	// wait behind the bounded handoff plus its own execution, or a delivery
	// sitting in a full queue gets leased a second time while the first copy is
	// still waiting.
	Lease time.Duration

	// Drain caps how long Shutdown waits for queued work.
	Drain time.Duration
}

// Defaults, applied to any zero field of Options or Timeouts.
const (
	defaultWorkers      = 8
	defaultQueueDepth   = 256
	defaultJob          = 5 * time.Minute
	defaultFinalization = 5 * time.Second
	defaultDrain        = 30 * time.Second

	// leaseRetryDelay is how long the lease loop waits after the inbox refuses
	// to answer, so a database stall does not become a spin.
	leaseRetryDelay = time.Second

	// finalizationRetryDelay paces the out-of-band retry that keeps a claim
	// owned until its outcome is recorded.
	finalizationRetryDelay = time.Second
)

// Options are the pipeline's knobs.
//
// A struct rather than a pile of With* functions, because that is what this
// repository already does and because a reader can see the whole surface at
// once. Every zero value is a working default.
type Options struct {
	// Events are the events worth doing anything with. Anything else is
	// answered 204 as soon as the signature checks out, without the payload
	// being parsed, screened or claimed.
	//
	// It also fixes the metric label set: the event header is not covered by
	// the signature, so a name that is not on this list is reported as "other"
	// rather than minting a time series per request. Empty accepts every event
	// except ping, which the pipeline answers itself.
	Events []string

	// Screen filters a delivery before it costs a row. Without one, every
	// delivery whose event is in scope is claimed.
	Screen Screen

	// Retry replaces DefaultRetry.
	Retry Retryable

	// Workers bounds how many deliveries run at once. The work is almost
	// entirely waiting on the GitHub API, so this belongs well above the core
	// count.
	Workers int

	// QueueDepth bounds how many leased deliveries wait for a worker. Past
	// this the lease loop stops leasing, which leaves the work in the inbox
	// rather than in memory.
	QueueDepth int

	Timeouts Timeouts
	Logger   *slog.Logger
	Observer Observer

	// Now is the clock. Tests pass a fake one.
	Now func() time.Time
}

// resolved is Options with every default filled in, so nothing downstream has
// to ask whether a field was set.
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

// accepts reports whether a delivery of this event is worth reading a body for.
// An empty Events list accepts everything.
func (r resolved) accepts(event string) bool {
	if len(r.known) == 0 {
		return true
	}
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
