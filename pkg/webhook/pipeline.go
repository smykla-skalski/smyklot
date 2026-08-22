// Package webhook receives GitHub webhooks: it verifies the signature, refuses
// what nobody asked for, deduplicates redeliveries, and runs what is left on a
// bounded worker pool, keeping the work durable across a restart.
//
// A pipeline is three things stitched together. The Receiver answers GitHub
// before any work runs, because GitHub gives a delivery ten seconds and does
// not retry one that times out. The Inbox is where an accepted delivery lives
// until it has been executed - a port, so a consumer brings their own table, or
// MemoryInbox if a lost delivery is a shrug. And the dispatcher leases from
// that inbox and hands work to a Handler.
//
// It knows GitHub - HMAC-SHA256, X-Hub-Signature-256, X-GitHub-Delivery,
// installation ids - and nothing about any particular App. What to do with a
// delivery is the Handler's business; what to skip is the Screen's; what to
// count is the Observer's.
//
// Signature verification is not reimplemented - github.com/jferrl/
// go-githubauth/webhook already does it in constant time and ships the header
// constants this package re-exports.
package webhook

import (
	"context"
	"errors"
	"net/http"
	"sync"
)

// Errors New and Shutdown return.
var (
	// ErrNoSecret is returned by New. A pipeline with no secret would accept
	// anything anyone posted at it.
	ErrNoSecret = errors.New("webhook secret is empty")

	// ErrNoInbox is returned by New. There is nowhere to put an accepted
	// delivery without one.
	ErrNoInbox = errors.New("webhook inbox is nil")

	// ErrNoHandler is returned by New.
	ErrNoHandler = errors.New("webhook handler is nil")

	// ErrDrainTimeout is returned by Shutdown when work was still running when
	// the deadline passed. It is a report rather than a failure: the inbox
	// still owns those deliveries and the next process leases them again.
	ErrDrainTimeout = errors.New("gave up waiting for in-flight deliveries")
)

// Pipeline is a signed, deduplicated, durable, retrying webhook pipeline.
type Pipeline struct {
	secret  []byte
	inbox   Inbox
	handle  Handler
	opts    resolved
	handler http.Handler

	jobs  chan Delivery
	woken chan struct{}

	// jobCtx outlives the request that accepted a delivery and survives
	// shutdown being signalled, so work already handed to a worker still
	// finishes.
	jobCtx context.Context

	lifecycleMu sync.Mutex
	cancelLease context.CancelFunc
	leaseDone   chan struct{}
	workers     sync.WaitGroup
	started     bool

	queueMu     sync.Mutex
	queueClosed bool

	// finalizeCtx owns outcome writes that outlive their worker attempt.
	finalizeCtx    context.Context
	cancelFinalize context.CancelFunc
	finalizeMu     sync.Mutex
	finalizeClosed bool
	finalizing     sync.WaitGroup
}

// New builds a pipeline.
//
// The secret is a positional argument rather than an Options field because
// there is no safe default for it: an App that forgets a field serves unsigned
// webhooks and finds out from an incident.
func New(secret []byte, inbox Inbox, handle Handler, opts Options) (*Pipeline, error) {
	if len(secret) == 0 {
		return nil, ErrNoSecret
	}
	if inbox == nil {
		return nil, ErrNoInbox
	}
	if handle == nil {
		return nil, ErrNoHandler
	}

	resolvedOpts := opts.resolve()
	finalizeCtx, cancelFinalize := context.WithCancel(context.Background())

	pipeline := &Pipeline{
		secret: secret, inbox: inbox, handle: handle, opts: resolvedOpts,
		jobs:           make(chan Delivery, resolvedOpts.QueueDepth),
		woken:          make(chan struct{}, 1),
		jobCtx:         context.Background(),
		finalizeCtx:    finalizeCtx,
		cancelFinalize: cancelFinalize,
	}
	pipeline.handler = Middleware(secret, WithErrorHandler(pipeline.unsigned))(
		receiver{pipeline: pipeline},
	)

	return pipeline, nil
}

// Receiver is the HTTP handler for the webhook route. Mount it on the one path
// GitHub posts to; it verifies every request it is given.
func (p *Pipeline) Receiver() http.Handler {
	return p.handler
}

// Start launches the lease loop and the worker pool.
//
// Work already accepted outlives ctx: cancelling it stops new leases, it does
// not abandon a delivery a worker has been handed. Use Shutdown to wait for
// those.
func (p *Pipeline) Start(ctx context.Context) {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	if p.started {
		return
	}
	p.started = true

	for range p.opts.Workers {
		p.workers.Add(1)
		go p.work()
	}

	leaseCtx, cancel := context.WithCancel(ctx)
	p.cancelLease = cancel
	p.leaseDone = make(chan struct{})
	go func() {
		defer close(p.leaseDone)
		p.lease(leaseCtx)
	}()
}

// Shutdown stops leasing, drains what is queued, and joins the outcome writes
// that outlived their worker.
func (p *Pipeline) Shutdown(ctx context.Context) error {
	p.closeQueue()

	drained := make(chan struct{})
	go func() {
		p.workers.Wait()
		close(drained)
	}()

	drainCtx, cancel := context.WithTimeout(ctx, p.opts.Timeouts.Drain)
	defer cancel()

	var err error
	select {
	case <-drained:
	case <-drainCtx.Done():
		err = ErrDrainTimeout
	}

	p.finalizeMu.Lock()
	if !p.finalizeClosed {
		p.finalizeClosed = true
		p.cancelFinalize()
	}
	p.finalizeMu.Unlock()
	p.finalizing.Wait()

	return err
}

// QueueDepth is how much leased work is waiting for a worker. A consumer
// registers a gauge over it; a number that sits near the configured depth is
// the first sign that the workers cannot keep up.
func (p *Pipeline) QueueDepth() int {
	return len(p.jobs)
}

// closeQueue stops the lease loop and then closes the queue, in that order:
// closing first would let a lease in flight send on a closed channel.
func (p *Pipeline) closeQueue() {
	p.queueMu.Lock()
	if p.queueClosed {
		p.queueMu.Unlock()

		return
	}
	p.queueClosed = true
	p.queueMu.Unlock()

	p.lifecycleMu.Lock()
	cancel, done := p.cancelLease, p.leaseDone
	p.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}

	close(p.jobs)
}

// wake asks the lease loop to look now rather than at its next deadline.
func (p *Pipeline) wake() {
	select {
	case p.woken <- struct{}{}:
	default:
	}
}
