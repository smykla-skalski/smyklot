package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/githubapp"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/metrics"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

const (
	// workerCount bounds how many deliveries execute at once. The work is
	// almost entirely waiting on the GitHub API, so this is well above the
	// core count
	workerCount = 8

	// queueDepth bounds how many deliveries wait for a worker. Past this the
	// service says so rather than growing without limit, and GitHub's delivery
	// log records the refusal
	queueDepth = 256

	// jobTimeout caps one delivery's execution. Well beyond the handful of API
	// calls a command makes, even with retries
	jobTimeout = 5 * time.Minute

	// deliveryFinalizationTimeout gives the durable claim a short, independent
	// window to record the outcome after execution itself times out
	deliveryFinalizationTimeout = 5 * time.Second

	// shutdownTimeout caps how long in-flight requests get to finish
	shutdownTimeout = 15 * time.Second

	// drainTimeout caps how long queued deliveries get to finish after the
	// listener stops
	drainTimeout = 30 * time.Second

	// readHeaderTimeout guards against a client that opens a connection and
	// dribbles headers
	readHeaderTimeout = 10 * time.Second

	// readTimeout bounds the whole request, body included.
	//
	// Without it a client can trickle a body indefinitely, and it can do so
	// unauthenticated: the signature is checked only after the body has been
	// read in full. Such a request would outlive shutdownTimeout and leave a
	// handler running after Shutdown has already given up on it
	readTimeout = 30 * time.Second

	// idleTimeout bounds how long a kept-alive connection may sit unused
	idleTimeout = 60 * time.Second

	// codeownersTTL is how long a repository's CODEOWNERS is trusted before it
	// is read again.
	//
	// Comfortably longer than the sweep interval on purpose: at the same
	// cadence every tick would land on a just-expired entry and the cache would
	// buy nothing for the caller that reads these most
	codeownersTTL = time.Hour

	// repoConfigTTL is how long .github/smyklot.yaml is trusted. Far shorter
	// than CODEOWNERS, because that file decides whether this process acts on
	// the repository at all.
	//
	// A repository rolling back to the Action commits `runner: action`, and the
	// Action reads that on its very next run because a workflow starts a fresh
	// process every time. Anything cached here is how long this process keeps
	// acting on a repository that has already moved, with both of them
	// answering the same comment - the one thing the setting exists to stop.
	//
	// Shorter than the sweep interval, so the sweep re-reads the file every
	// tick. That is one contents request per repository per tick against an
	// installation's 5000 an hour, and it is worth it. Deliveries arriving
	// together still share one read, which is the burst this ever protected
	repoConfigTTL = 30 * time.Second

	// maxDeliveryIDLength caps the unverified delivery identifier before it
	// reaches a log line. GitHub's are UUIDs, well under this
	maxDeliveryIDLength = 64

	// eventOther is the metric label for any event the service does not
	// execute. The event header is not covered by the signature, so passing it
	// through would let a caller mint a new time series per request
	eventOther = "other"
)

// server executes PR commands from GitHub webhook deliveries.
type server struct {
	cfg    *serveConfig
	tokens *githubapp.TokenStore
	store  storage.Store
	panel  *adminpanel.Server

	logger   *slog.Logger
	redactor *logging.Redactor

	registry *prometheus.Registry
	metrics  *metrics.Metrics

	// readiness answers whether GitHub is reachable, kept current by probeLoop
	readiness *readiness

	// failures holds the deliveries that were accepted and then failed, which
	// GitHub's own delivery log records as successes
	failures *failureLog

	// configs and owners hold the two files every repository is read for. The
	// sweep touches both for every repository on every tick
	configs *repoCache[repositoryConfigFile]
	owners  *repoCache[string]

	deduper *webhook.Deduper

	jobs chan job

	// queueMu guards jobs against being closed while a handler is mid-send.
	//
	// http.Server.Shutdown leaves a handler that is still running alone once
	// its deadline passes, so a handler can reach the send after Run has moved
	// on to closing the queue. A send on a closed channel panics rather than
	// taking the select's default branch, which would eat the delivery and
	// strand its claim. Senders hold this for read, the close takes it for
	// write, so the two cannot overlap
	queueMu     sync.RWMutex
	queueClosed bool

	// catalogMu orders complete GitHub catalog snapshots and the per-installation
	// snapshots discovered by the sweep. Network reads are covered too, so an
	// older read can never commit after a newer one.
	catalogMu sync.Mutex

	// jobCtx outlives the request that enqueued a job and survives shutdown
	// being signalled, so a delivery already in the queue still completes
	jobCtx context.Context

	// deliveryRetryCtx owns outcome writes that outlive their worker attempt.
	// The mutex prevents Close from racing a new retry into the WaitGroup.
	deliveryRetryCtx    context.Context
	cancelDeliveryRetry context.CancelFunc
	deliveryRetryMu     sync.Mutex
	deliveryRetryClosed bool
	deliveryRetries     sync.WaitGroup
}

// job is one delivery waiting to be executed.
type job struct {
	event      *webhook.IssueCommentEvent
	key        string
	deliveryID string
	claimID    int64

	// logger already carries this delivery's identifiers, so every line the
	// work produces can be traced back to the delivery that caused it
	logger *slog.Logger
}

func newServer(cfg *serveConfig) (*server, error) {
	tokens, err := githubapp.NewTokenStore(
		cfg.appClientID, cfg.appPrivateKey, cfg.apiBaseURL, githubapp.DefaultMintTimeout)
	if err != nil {
		return nil, NewGitHubError(ErrGitHubAppAuth, err)
	}

	// The two values that must never reach a log line or the failures
	// endpoint, taught to the one place that can catch them
	secrets := [][]byte{cfg.webhookSecret, cfg.appPrivateKey}
	if cfg.panel != nil {
		secrets = append(secrets, []byte(cfg.panel.clientSecret))
	}
	redactor := logging.NewRedactor(secrets...)

	registry := metrics.NewRegistry()

	out := cfg.logWriter
	if out == nil {
		out = os.Stdout
	}

	deliveryRetryCtx, cancelDeliveryRetry := context.WithCancel(context.Background())
	srv := &server{
		cfg:                 cfg,
		tokens:              tokens,
		logger:              logging.New(out, cfg.logFormat, cfg.logLevel, redactor),
		redactor:            redactor,
		registry:            registry,
		metrics:             metrics.New(registry),
		configs:             newRepoCache(repoConfigTTL, fetchRepositoryConfig),
		owners:              newRepoCache(codeownersTTL, fetchCodeowners),
		readiness:           newReadiness(),
		failures:            newFailureLog(maxRecordedFailures),
		deduper:             webhook.NewDeduper(webhook.DefaultTTL, webhook.DefaultMaxEntries, nil),
		jobs:                make(chan job, queueDepth),
		jobCtx:              context.Background(),
		deliveryRetryCtx:    deliveryRetryCtx,
		cancelDeliveryRetry: cancelDeliveryRetry,
	}

	metrics.RegisterQueue(registry, func() float64 { return float64(len(srv.jobs)) }, queueDepth)
	metrics.RegisterReadiness(registry, func() bool { return srv.readiness.state().Ready })
	if err := srv.initPanel(context.Background()); err != nil {
		cancelDeliveryRetry()
		return nil, err
	}

	return srv, nil
}

// Close releases optional durable service resources.
func (s *server) Close() error {
	s.stopDeliveryFinalizationRetries()
	if s.store != nil {
		return s.store.Close()
	}

	return nil
}

// handler builds the service's public routes.
//
// Only the webhook route sits behind signature verification; a probe has no
// signature to offer and reveals nothing. Everything an operator reads lives on
// the admin listener instead.
func (s *server) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET "+healthPath, func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusOK, "ok\n")
	})

	verify := webhook.Middleware(s.cfg.webhookSecret, webhook.WithErrorHandler(s.rejectUnsigned))
	mux.Handle("POST "+s.cfg.webhookPath, verify(http.HandlerFunc(s.handleDelivery)))
	if s.panel != nil {
		if s.cfg.panel.basePath == "" {
			mux.Handle("/", s.panel.Handler())
		} else {
			mux.Handle(s.cfg.panel.basePath, s.panel.Handler())
			mux.Handle(s.cfg.panel.basePath+"/", s.panel.Handler())
		}
	}

	return mux
}

// rejectUnsigned answers a delivery whose signature does not check out.
//
// Counted rather than only refused: a service quietly rejecting everything
// because its secret was rotated looks exactly like a service nobody is using,
// and this is the one number that tells them apart.
func (s *server) rejectUnsigned(w http.ResponseWriter, r *http.Request, err error) {
	event := eventLabel(r.Header.Get(webhook.EventHeader))

	s.count(event, metrics.OutcomeUnsigned)
	s.logger.Warn("rejected delivery with bad signature",
		"delivery_id", safeDeliveryID(r.Header.Get(webhook.DeliveryHeader)),
		"event", event,
		"error", err)

	http.Error(w, "invalid signature", http.StatusUnauthorized)
}

// Run serves until ctx is cancelled, then drains what is already in flight.
func (s *server) Run(ctx context.Context) error {
	// Work already accepted must survive the shutdown signal, or a rolling
	// update leaves a pull request approved but never merged
	s.jobCtx = context.WithoutCancel(ctx)

	// A listener that dies must stop the sweep and the probe too, not leave
	// them running on behalf of a service that is no longer serving
	runCtx, stopBackground := context.WithCancel(ctx)
	defer stopBackground()

	workers := s.startWorkers()
	background := s.startBackground(runCtx)

	webhooks := s.newHTTPServer(s.cfg.listenAddress, s.handler())
	admin := s.newHTTPServer(s.cfg.adminAddress, s.adminHandler())

	s.logger.Info("listening",
		"address", s.cfg.listenAddress,
		"webhook_path", s.cfg.webhookPath,
		"admin_address", s.cfg.adminAddress)

	shutdownErr := s.serveUntilDone(ctx, webhooks, admin)

	// Stop accepting first, then drain. Shutdown abandons a handler that is
	// still running once its deadline passes, so closeQueue rather than a bare
	// close: a late handler must be refused, not met with a closed channel
	stopBackground()
	s.closeQueue()
	s.drain(workers)
	<-background

	return shutdownErr
}

// newHTTPServer builds one listener with the timeouts every listener needs.
func (s *server) newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// serveUntilDone runs both listeners until one fails or ctx is cancelled, then
// shuts both down.
//
// Both, whichever way it got here: an admin listener that died would otherwise
// leave the service serving webhooks with no way to say how it is doing.
func (s *server) serveUntilDone(ctx context.Context, listeners ...*http.Server) error {
	stopped := make(chan error, len(listeners))

	for _, srv := range listeners {
		go func() {
			err := srv.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}

			stopped <- err
		}()
	}

	var shutdownErr error

	select {
	case shutdownErr = <-stopped:
		if shutdownErr != nil {
			s.logger.Error("listener stopped", "error", shutdownErr)
		}

	case <-ctx.Done():
		s.logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	for _, srv := range listeners {
		shutdownErr = errors.Join(shutdownErr, srv.Shutdown(shutdownCtx))
	}

	return shutdownErr
}

// startBackground runs the sweep and the readiness probe until ctx is
// cancelled, and reports when both have stopped.
func (s *server) startBackground(ctx context.Context) <-chan struct{} {
	var running sync.WaitGroup

	running.Add(2)

	go func() {
		defer running.Done()

		s.pollLoop(ctx)
	}()

	go func() {
		defer running.Done()

		s.probeLoop(ctx)
	}()

	if s.store != nil {
		running.Add(1)

		go func() {
			defer running.Done()

			s.panelMaintenanceLoop(ctx)
		}()
	}

	stopped := make(chan struct{})

	go func() {
		running.Wait()
		close(stopped)
	}()

	return stopped
}

// handleDelivery decides what a verified delivery deserves and answers GitHub.
//
// It answers before the work runs. GitHub gives a delivery ten seconds and does
// not retry one that times out, so executing inline would lose commands
// whenever a merge took longer than that.
func (s *server) handleDelivery(w http.ResponseWriter, r *http.Request) {
	eventName := r.Header.Get(webhook.EventHeader)
	event := eventLabel(eventName)

	// GitHub sends a ping when the webhook is first configured, and expects an
	// answer before it will send anything else
	if eventName == webhook.EventPing {
		s.ignore(w, event, http.StatusOK)

		return
	}

	// A command only ever arrives on a comment. Subscribing to more events than
	// that is the operator's business, but nothing else has anything to execute
	if eventName != webhook.EventIssueComment {
		s.ignore(w, event, http.StatusNoContent)

		return
	}

	deliveryID := safeDeliveryID(r.Header.Get(webhook.DeliveryHeader))

	// Attached once, here, so every later line about this delivery carries it
	ctx := logging.With(logging.Into(r.Context(), s.logger), "delivery_id", deliveryID, "event", event)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.reject(ctx, w, event, "cannot read body", err)

		return
	}

	e, err := webhook.ParseIssueComment(body)
	if err != nil {
		s.reject(ctx, w, event, "malformed payload", err)

		return
	}

	if !e.Actionable() {
		s.ignore(w, event, http.StatusNoContent)

		return
	}

	ctx = logging.With(ctx,
		"repo", e.Repository.FullName, "pr", e.Issue.Number, "action", e.Action)

	if err := validateCommentInput(runtimeConfigFor(e, s.cfg)); err != nil {
		s.reject(ctx, w, event, "unusable payload", err)

		return
	}
	s.dispatch(ctx, w, job{
		event:      e,
		key:        e.IdempotencyKey(),
		deliveryID: deliveryID,
		logger:     logging.From(ctx),
	})
}

// ignore answers a delivery that carried nothing to execute.
func (s *server) ignore(w http.ResponseWriter, event string, status int) {
	s.count(event, metrics.OutcomeIgnored)
	w.WriteHeader(status)
}

// reject answers a delivery that could not be read or used.
func (s *server) reject(ctx context.Context, w http.ResponseWriter, event, message string, cause error) {
	s.count(event, metrics.OutcomeInvalid)
	logging.From(ctx).Error(message, "error", cause)
	http.Error(w, message, http.StatusBadRequest)
}

// dispatch claims a delivery and queues it, or explains why it did neither.
func (s *server) dispatch(ctx context.Context, w http.ResponseWriter, j job) {
	// Claiming before queueing is what makes a redelivery harmless: the second
	// copy never reaches a worker
	claim, err := s.beginDelivery(ctx, &j)
	if err != nil {
		s.count(webhook.EventIssueComment, metrics.OutcomeRefused)
		logging.From(ctx).Error("delivery claim failed", "error", err)
		http.Error(w, "not accepted", http.StatusServiceUnavailable)

		return
	}
	if claim == storage.DeliveryClaimInProgress {
		s.count(webhook.EventIssueComment, metrics.OutcomeRefused)
		logging.From(ctx).Info("delivery is still being processed")
		http.Error(w, "not accepted", http.StatusServiceUnavailable)

		return
	}
	if claim == storage.DeliveryClaimRetained {
		s.count(webhook.EventIssueComment, metrics.OutcomeDuplicate)
		logging.From(ctx).Info("delivery already handled")
		w.WriteHeader(http.StatusOK)

		return
	}

	if s.enqueue(j) {
		s.count(webhook.EventIssueComment, metrics.OutcomeAccepted)
		w.WriteHeader(http.StatusAccepted)

		return
	}

	// Releasing the claim keeps the delivery retryable rather than dropping it
	// silently: GitHub records the refusal and a redelivery gets a fresh try
	finalize := func(finalizationCtx context.Context) error {
		return s.abandonDelivery(finalizationCtx, j)
	}
	finalizationCtx, cancelFinalization := deliveryFinalizationContext(ctx)
	releaseErr := finalize(finalizationCtx)
	cancelFinalization()
	if releaseErr != nil {
		logging.From(ctx).Error("delivery claim could not be released", "error", releaseErr)
		s.retryDeliveryFinalization(j, "abandonment", finalize)
	}
	s.count(webhook.EventIssueComment, metrics.OutcomeRefused)
	logging.From(ctx).Error("delivery not accepted, queue is full")
	http.Error(w, "not accepted", http.StatusServiceUnavailable)
}

// enqueue offers a delivery to the workers, reporting whether they took it.
//
// Refuses rather than blocks when the queue is full, and refuses rather than
// panics when the queue has already been closed for shutdown.
func (s *server) enqueue(j job) bool {
	s.queueMu.RLock()
	defer s.queueMu.RUnlock()

	if s.queueClosed {
		return false
	}

	select {
	case s.jobs <- j:
		return true

	default:
		return false
	}
}

// closeQueue stops the workers once every in-flight send has finished.
func (s *server) closeQueue() {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()

	if s.queueClosed {
		return
	}

	s.queueClosed = true
	close(s.jobs)
}

// execute runs one delivery, releasing its claim if the work did not take
// effect.
func (s *server) execute(j job) {
	ctx, cancel := context.WithTimeout(logging.Into(s.jobCtx, j.logger), jobTimeout)
	defer cancel()

	s.metrics.DeliveriesInFlight.Inc()
	defer s.metrics.DeliveriesInFlight.Dec()

	started := time.Now()
	executed := true
	var err error
	if s.store != nil {
		executed, err = s.repositoryEnabled(ctx, j.event)
	}
	if err == nil && executed {
		err = s.handleIssueComment(ctx, j.event)
	}
	elapsed := time.Since(started)

	s.metrics.DeliveryDuration.WithLabelValues(j.event.Action).Observe(elapsed.Seconds())

	if err != nil {
		finalize := func(finalizationCtx context.Context) error {
			return s.failDelivery(finalizationCtx, j, err)
		}
		finalizationCtx, cancelFinalization := deliveryFinalizationContext(ctx)
		finishErr := finalize(finalizationCtx)
		cancelFinalization()
		if finishErr != nil {
			logging.From(ctx).Error("delivery failure could not be persisted", "error", finishErr)
			s.retryDeliveryFinalization(j, "failure", finalize)
		}
		s.metrics.Deliveries.WithLabelValues(j.event.Action, metrics.ResultFailure).Inc()
		s.recordFailure(j, err)
		logging.From(ctx).Error("delivery failed", "error", err, "duration", elapsed.String())

		return
	}

	finalize := func(finalizationCtx context.Context) error {
		return s.completeDelivery(finalizationCtx, j)
	}
	finalizationCtx, cancelFinalization := deliveryFinalizationContext(ctx)
	finishErr := finalize(finalizationCtx)
	cancelFinalization()
	if finishErr != nil {
		logging.From(ctx).Error("delivery completion could not be persisted", "error", finishErr)
		s.retryDeliveryFinalization(j, "success", finalize)
	}
	s.metrics.Deliveries.WithLabelValues(j.event.Action, metrics.ResultSuccess).Inc()
	if executed {
		logging.From(ctx).Info("delivery executed", "duration", elapsed.String())
	} else {
		logging.From(ctx).Info("delivery ignored: repository is disabled", "duration", elapsed.String())
	}
}

// recordFailure keeps a failed delivery readable after the log line scrolls
// away.
func (s *server) recordFailure(j job, cause error) {
	s.failures.Record(deliveryFailure{
		Time:        time.Now(),
		DeliveryID:  j.deliveryID,
		Repository:  j.event.Repository.FullName,
		PullRequest: j.event.Issue.Number,
		Action:      j.event.Action,
		Reason:      s.redactor.Error(cause),
	})
}

// handleIssueComment runs the command a comment carries.
//
// Everything past executeComment is the Action's own code, so a comment gets
// the same permission check and the same feedback whichever entry point saw it.
func (s *server) handleIssueComment(ctx context.Context, event *webhook.IssueCommentEvent) error {
	token, err := s.tokens.InstallationToken(event.Installation.ID)
	if err != nil {
		return NewGitHubError(ErrGitHubAppAuth, err)
	}

	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return NewGitHubError(ErrGitHubClient, err)
	}

	rc := runtimeConfigFor(event, s.cfg)
	rc.Token = token

	bc, err := s.serviceConfig(
		ctx,
		client,
		installationStorageID(event.Installation.ID),
		repositoryStorageID(event.Repository.ID),
		event.Repository.Owner.Login,
		event.Repository.Name,
	)
	if err != nil {
		if errors.Is(err, ErrRepoConfigInvalid) {
			return reportInvalidRepoConfig(ctx, client, rc, s.cfg.botConfig, err)
		}

		return err
	}

	// A repository that has rolled back to the Action is left to it, without
	// the service being redeployed to learn that
	if serviceStandsDown(ctx, bc) {
		return nil
	}

	return executeComment(ctx, client, rc, bc)
}

// eventLabel reduces an event name to a value safe to use as a metric label.
//
// The event header is not covered by the signature, so passing it through would
// let any caller that can reach the port mint a new time series per request.
func eventLabel(name string) string {
	switch name {
	case webhook.EventIssueComment, webhook.EventPing:
		return name

	default:
		return eventOther
	}
}

// safeDeliveryID reduces a delivery identifier to what can appear in a log
// line.
//
// The signature covers the body, not the headers, so this one field arrives
// unverified even on a delivery that passed verification. A newline in it would
// let the caller forge log entries; GitHub's own identifiers are UUIDs and
// survive this untouched.
func safeDeliveryID(id string) string {
	if len(id) > maxDeliveryIDLength {
		id = id[:maxDeliveryIDLength]
	}

	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return -1
		}
	}, id)

	if clean == "" {
		return "unknown"
	}

	return clean
}

// count records what became of one webhook request.
func (s *server) count(event, outcome string) {
	s.metrics.WebhookRequests.WithLabelValues(event, outcome).Inc()
}

// runtimeConfigFor translates a delivery into what the executor expects.
//
// The Action fills the same struct from environment variables a workflow set.
func runtimeConfigFor(event *webhook.IssueCommentEvent, cfg *serveConfig) *RuntimeConfig {
	return &RuntimeConfig{
		CommentBody:   event.Comment.Body,
		CommentID:     strconv.FormatInt(event.Comment.ID, 10),
		CommentAction: event.Action,
		PRNumber:      strconv.Itoa(event.Issue.Number),
		RepoOwner:     event.Repository.Owner.Login,
		RepoName:      event.Repository.Name,
		CommentAuthor: event.Comment.User.Login,
		BotUsername:   cfg.botUsername,
		APIBaseURL:    cfg.apiBaseURL,
	}
}
