package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/orgsync/apply"
	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/pendingci/gate"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/config"
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
)

// server executes PR commands from GitHub webhook deliveries.
type server struct {
	cfg    *serveConfig
	tokens *githubapp.TokenStore
	store  storage.Store
	panel  *adminpanel.Server
	listen func(context.Context, string, string) (net.Listener, error)

	logger   *slog.Logger
	logLevel *slog.LevelVar
	redactor *logging.Redactor

	// sampledWaits is the pool's lifetime wait count as of the last
	// measurement, which is what makes the stored series a level.
	sampledWaits int64

	// unstoredStats holds query counters a failed write could not store, to be
	// folded into the next measurement rather than lost.
	unstoredStats []storage.QueryStats

	runtimeMu                   sync.RWMutex
	runtimeBackgroundWorkPaused bool
	runtimeBotConfig            *config.Config
	runtimePollInterval         time.Duration

	// runtimePathIndexInterval is how often a repository's file list is checked
	// for changes, for every installation that does not say otherwise. Held
	// here rather than read from the panel each sweep, like the poll interval
	// beside it.
	runtimePathIndexInterval time.Duration
	pollIntervalChanged      chan struct{}
	workQueueChanged         chan struct{}
	migrationRetryDelay      time.Duration
	sweepMu                  sync.Mutex

	registry *prometheus.Registry
	metrics  *metrics.Metrics

	// readiness answers whether GitHub is reachable, kept current by probeLoop
	readiness *readiness

	// failures holds the deliveries that were accepted and then failed, which
	// GitHub's own delivery log records as successes
	failures *failureLog

	sync *apply.Engine

	// configs and owners hold the two files every repository is read for. The
	// sweep touches both for every repository on every tick
	configs *repoCache[repositoryConfigFile]
	owners  *repoCache[string]

	deliveries *webhook.Pipeline

	gate                 *gate.Gate
	pendingCICoordinator bot.Exclusive
	pendingCIGateChanged chan struct{}

	// catalogMu orders complete GitHub catalog snapshots and the per-installation
	// snapshots discovered by the sweep. Network reads are covered too, so an
	// older read can never commit after a newer one.
	catalogMu sync.Mutex

	// candidates holds each installation's organization roster, which the panel
	// completes logins against. Keyed by target id; see ListTargetCandidates for
	// why it is cached rather than read per keystroke.
	candidatesMu sync.Mutex
	candidates   map[string]candidateRoster
}

func newServer(cfg *serveConfig) (*server, error) {
	if cfg.pendingCIQuietPeriod < pendingci.MinPassingQuiet ||
		cfg.pendingCIQuietPeriod > pendingci.MaxPassingQuiet ||
		(cfg.pendingCIQuietPeriod > 0 && cfg.pendingCIQuietPeriod < time.Second) {
		return nil, ErrInvalidPendingCIQuietPeriod
	}
	tokens, err := githubapp.NewTokenStore(
		cfg.appClientID, cfg.appPrivateKey, cfg.apiBaseURL, githubapp.DefaultMintTimeout)
	if err != nil {
		return nil, bot.NewGitHubError(bot.ErrGitHubAppAuth, err)
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

	level := &slog.LevelVar{}
	level.Set(cfg.logLevel)
	resolvedConfig := config.Resolve(cfg.botConfig)
	srv := &server{
		cfg:                      cfg,
		tokens:                   tokens,
		listen:                   (&net.ListenConfig{}).Listen,
		logger:                   logging.New(out, cfg.logFormat, level, redactor),
		logLevel:                 level,
		redactor:                 redactor,
		runtimeBotConfig:         &resolvedConfig.Values,
		runtimePollInterval:      cfg.pollInterval,
		runtimePathIndexInterval: cfg.pathIndexInterval,
		pollIntervalChanged:      make(chan struct{}, 1),
		workQueueChanged:         make(chan struct{}, 1),
		pendingCIGateChanged:     make(chan struct{}, 1),
		migrationRetryDelay:      gate.RetryDelay,
		registry:                 registry,
		metrics:                  metrics.New(registry),
		configs:                  newRepoConfigCache(),
		owners:                   newRepoCache(codeownersTTL, bot.FetchCodeowners),
		readiness:                newReadiness(),
		failures:                 newFailureLog(maxRecordedFailures),
	}

	metrics.RegisterReadiness(registry, func() bool { return srv.readiness.state().Ready })
	metrics.RegisterWorkQueue(registry, func() (workqueue.MetricsSnapshot, error) {
		return srv.store.WorkQueueMetrics(context.Background(), time.Now().UTC())
	})
	if err := srv.initStorage(context.Background()); err != nil {
		return nil, err
	}
	srv.sync = apply.New(srv.store, tokens, cfg.apiBaseURL)
	srv.sync.SetFormattingPolicy(resolvedConfig.Values.Formatting)
	srv.sync.SetBeginWork(srv.beginBackgroundWork)
	pendingCICoordinator := bot.NewCoordinator()
	srv.pendingCICoordinator = pendingCICoordinator
	srv.sync.SetCoordinator(pendingCICoordinator)
	srv.gate = gate.New(gate.Dependencies{
		Store:       srv.store,
		Gates:       srv.store,
		Checks:      srv.store,
		Transitions: srv.store,
		Leases:      srv.store,
		Handoffs:    srv.store,
		Current:     srv.store,
		Config:      srv.serviceConfigWithoutCatalogRefresh,
		Coordinator: pendingCICoordinator,
		Tokens:      srv.tokens,
		APIBaseURL:  cfg.apiBaseURL,
		BotUsername: cfg.botUsername,
		QuietPeriod: cfg.pendingCIQuietPeriod,
		Panelled:    cfg.panel != nil,
		WakeGates:   srv.WakePendingCIGates,
		Logger:      srv.logger,
		BeginWork:   srv.beginBackgroundWork,
	})
	if err := srv.initPanel(); err != nil {
		_ = srv.store.Close()

		return nil, err
	}

	if err := srv.initDeliveries(redactor, registry); err != nil {
		_ = srv.store.Close()

		return nil, err
	}

	return srv, nil
}

// Close releases durable service resources.
func (s *server) Close() error {
	return s.store.Close()
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

	mux.Handle("POST "+s.cfg.webhookPath, s.deliveries.Receiver())

	// At the root rather than under the panel, and served whether or not a
	// panel is configured: a schema is published documentation, and its address
	// is written into repositories that will still be reading it long after
	// whoever set the deployment up has moved on.
	mux.HandleFunc("GET "+schemaRoot+"/{schema}", serveSchema)
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

// Run serves until ctx is cancelled, then drains what is already in flight.
func (s *server) Run(ctx context.Context) error {
	webhookListener, err := s.listen(ctx, "tcp", s.cfg.listenAddress)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
			return nil
		}

		return fmt.Errorf("listen for webhooks: %w", err)
	}

	adminListener, err := s.listen(ctx, "tcp", s.cfg.adminAddress)
	if err != nil {
		listenErr := fmt.Errorf("listen for admin: %w", err)
		if closeErr := webhookListener.Close(); closeErr != nil {
			return errors.Join(listenErr, fmt.Errorf("close webhook listener: %w", closeErr))
		}
		if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
			return nil
		}

		return listenErr
	}

	return s.runWithListeners(ctx, webhookListener, adminListener)
}

func (s *server) runWithListeners(
	ctx context.Context,
	webhookListener net.Listener,
	adminListener net.Listener,
) error {
	// A listener that dies must stop the sweep and the probe too, not leave
	// them running on behalf of a service that is no longer serving
	runCtx, stopBackground := context.WithCancel(ctx)
	defer stopBackground()

	s.deliveries.Start(runCtx)
	background := s.startBackground(runCtx)

	webhooks := s.newHTTPServer(webhookListener.Addr().String(), s.handler())
	admin := s.newHTTPServer(adminListener.Addr().String(), s.adminHandler())

	s.logger.Info("listening",
		"address", webhooks.Addr,
		"webhook_path", s.cfg.webhookPath,
		"admin_address", admin.Addr)

	shutdownErr := s.serveUntilDone(ctx,
		httpEndpoint{server: webhooks, listener: webhookListener},
		httpEndpoint{server: admin, listener: adminListener},
	)

	stopBackground()
	if err := s.deliveries.Shutdown(context.Background()); err != nil {
		s.logger.Error("gave up waiting for in-flight deliveries", "error", err)
	}
	<-background

	return shutdownErr
}

type httpEndpoint struct {
	server   *http.Server
	listener net.Listener
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
func (s *server) serveUntilDone(ctx context.Context, endpoints ...httpEndpoint) error {
	stopped := make(chan error, len(endpoints))

	for _, endpoint := range endpoints {
		go func() {
			err := endpoint.server.Serve(endpoint.listener)
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

	for _, endpoint := range endpoints {
		shutdownErr = errors.Join(shutdownErr, endpoint.server.Shutdown(shutdownCtx))
	}

	return shutdownErr
}

// startBackground runs the sweep, the readiness probe and the measurement loop
// until ctx is cancelled, and reports when they have all stopped.
func (s *server) startBackground(ctx context.Context) <-chan struct{} {
	var running sync.WaitGroup

	running.Add(4)

	go func() {
		defer running.Done()

		s.pollLoop(ctx)
	}()

	go func() {
		defer running.Done()

		s.sampleLoop(ctx)
	}()

	go func() {
		defer running.Done()

		s.probeLoop(ctx)
	}()

	go func() {
		defer running.Done()

		s.gate.Scheduler.Run(ctx)
	}()

	stopped := make(chan struct{})

	go func() {
		running.Wait()
		close(stopped)
	}()

	return stopped
}

// recordFailure keeps a failed delivery readable after the log line scrolls
// away.
func (s *server) recordFailure(delivery webhook.Delivery, action string, cause error) {
	s.failures.Record(deliveryFailure{
		Time:        time.Now(),
		DeliveryID:  delivery.ID,
		Repository:  delivery.Source.Repository.FullName,
		PullRequest: deliveryPullRequest(delivery),
		Action:      action,
		Reason:      s.redactor.Error(cause),
	})
}

// handleIssueComment runs the command a comment carries.
//
// Everything past bot.ExecuteComment is the Action's own code, so a comment gets
// the same permission check and the same feedback whichever entry point saw it.
func (s *server) handleIssueComment(
	ctx context.Context,
	event *webhook.IssueCommentEvent,
	eventKey string,
	sourceOrder int64,
) error {
	repositoryID := storage.RepositoryID(event.Repository.ID)

	return s.pendingCICoordinator.Exclusive(ctx, repositoryID, func() error {
		return s.handleIssueCommentCoordinated(ctx, event, eventKey, sourceOrder)
	})
}

func (s *server) handleIssueCommentCoordinated(
	ctx context.Context,
	event *webhook.IssueCommentEvent,
	eventKey string,
	sourceOrder int64,
) error {
	token, err := s.tokens.InstallationToken(event.Installation.ID)
	if err != nil {
		return bot.NewGitHubError(bot.ErrGitHubAppAuth, err)
	}

	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return bot.NewGitHubError(bot.ErrGitHubClient, err)
	}
	current, err := issueCommentIsCurrent(ctx, client, event)
	if err != nil {
		return err
	}
	if !current {
		logging.From(ctx).Info("ignored stale issue comment delivery")

		return nil
	}

	source := pendingci.SourceRevisionRequest{
		RepositoryID: storage.RepositoryID(event.Repository.ID),
		PullRequest:  event.Issue.Number, CommentID: event.Comment.ID,
		Revision: event.Comment.UpdatedAt, Sequence: pendingci.CommentSequence(event.Action),
		SourceOrder: sourceOrder, EventKey: eventKey, ObservedAt: time.Now().UTC(),
	}
	claim, err := gate.ClaimSource(
		ctx, s.store, inlineExclusive{}, source,
		gate.SourceCancellation(event, source.RepositoryID),
	)
	if err != nil {
		return fmt.Errorf("claim issue comment revision: %w", err)
	}
	if !claim.Source.Accepted {
		logging.From(ctx).Info("ignored stale issue comment revision")

		return nil
	}
	if claim.Cancelled != nil {
		s.gate.Wake()
	}

	rc := runtimeConfigFor(event, s.cfg)
	rc.Token = token

	bc, err := s.serviceConfig(
		ctx,
		client,
		storage.InstallationID(event.Installation.ID),
		storage.RepositoryID(event.Repository.ID),
		event.Repository.Owner.Login,
		event.Repository.Name,
	)
	if err != nil {
		if errors.Is(err, bot.ErrRepoConfigInvalid) {
			return reportInvalidRepoConfig(ctx, client, rc, s.botConfig(), err)
		}

		return err
	}

	// A repository that has rolled back to the Action is left to it, without
	// the service being redeployed to learn that
	if bot.ServiceStandsDown(ctx, bc) {
		return nil
	}

	return bot.ExecuteCommentWithEnvironment(
		ctx, client, rc, bc, s.commandEnvironment(client, event, claim.Source.SourceOrder),
	)
}

// runtimeConfigFor translates a delivery into what the executor expects.
//
// The Action fills the same struct from environment variables a workflow set.
func runtimeConfigFor(event *webhook.IssueCommentEvent, cfg *serveConfig) *bot.RuntimeConfig {
	return &bot.RuntimeConfig{
		CommentBody:     event.Comment.Body,
		CommentID:       strconv.FormatInt(event.Comment.ID, 10),
		CommentRevision: event.Comment.UpdatedAt,
		CommentAction:   event.Action,
		PRNumber:        strconv.Itoa(event.Issue.Number),
		RepoOwner:       event.Repository.Owner.Login,
		RepoName:        event.Repository.Name,
		CommentAuthor:   event.Comment.User.Login,
		BotUsername:     cfg.botUsername,
		APIBaseURL:      cfg.apiBaseURL,
	}
}
