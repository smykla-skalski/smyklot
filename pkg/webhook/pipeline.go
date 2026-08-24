// Package webhook receives GitHub webhooks: it verifies the signature, refuses
// what nobody asked for, deduplicates redeliveries, and runs what is left on a
// bounded worker pool, keeping the work durable across a restart.
package webhook

import (
	"context"
	"errors"
	"net/http"
	"sync"

	ghwebhook "github.com/jferrl/go-githubauth/webhook"
)

var (
	ErrNoSecret = errors.New("webhook secret is empty")

	ErrNoInbox = errors.New("webhook inbox is nil")

	ErrNoHandler = errors.New("webhook handler is nil")

	ErrNoEvents = errors.New("webhook options name no events")

	ErrDrainTimeout = errors.New("gave up waiting for in-flight deliveries")
)

type Pipeline struct {
	inbox   Inbox
	handle  Handler
	opts    resolved
	handler http.Handler

	jobs  chan Delivery
	woken chan struct{}

	lifecycleMu sync.Mutex
	cancelLease context.CancelFunc
	leaseDone   chan struct{}
	workers     sync.WaitGroup
	started     bool
	queueClosed bool

	finalizeCtx    context.Context
	cancelFinalize context.CancelFunc
	finalizeMu     sync.Mutex
	finalizeClosed bool
	finalizing     sync.WaitGroup
}

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
	if len(opts.Events) == 0 {
		return nil, ErrNoEvents
	}

	resolvedOpts := opts.resolve()
	finalizeCtx, cancelFinalize := context.WithCancel(context.Background())

	pipeline := &Pipeline{
		inbox: inbox, handle: handle, opts: resolvedOpts,
		jobs:           make(chan Delivery, resolvedOpts.QueueDepth),
		woken:          make(chan struct{}, 1),
		finalizeCtx:    finalizeCtx,
		cancelFinalize: cancelFinalize,
	}
	pipeline.handler = ghwebhook.Middleware(
		secret, ghwebhook.WithErrorHandler(pipeline.unsigned),
	)(receiver{pipeline: pipeline})

	return pipeline, nil
}

func (p *Pipeline) Receiver() http.Handler {
	return p.handler
}

func (p *Pipeline) Start(ctx context.Context) {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	if p.started || p.queueClosed {
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

func (p *Pipeline) QueueDepth() int {
	return len(p.jobs)
}

// Wake asks the durable dispatcher to re-check eligibility immediately.
func (p *Pipeline) Wake() { p.wake() }

func (p *Pipeline) closeQueue() {
	p.lifecycleMu.Lock()
	if p.queueClosed {
		p.lifecycleMu.Unlock()

		return
	}
	p.queueClosed = true
	cancel, done := p.cancelLease, p.leaseDone
	p.lifecycleMu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}

	close(p.jobs)
}

func (p *Pipeline) wake() {
	select {
	case p.woken <- struct{}{}:
	default:
	}
}
