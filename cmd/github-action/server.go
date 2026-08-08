package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/githubapp"
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

	// repoFileTTL is how long a repository's CODEOWNERS and .github/smyklot.yaml
	// are trusted before they are read again.
	//
	// Comfortably longer than the sweep interval on purpose: at the same
	// cadence every tick would land on a just-expired entry and the cache would
	// buy nothing for the caller that reads these most
	repoFileTTL = time.Hour

	// maxDeliveryIDLength caps the unverified delivery identifier before it
	// reaches a log line. GitHub's are UUIDs, well under this
	maxDeliveryIDLength = 64
)

// server executes PR commands from GitHub webhook deliveries.
type server struct {
	cfg    *serveConfig
	tokens *githubapp.TokenStore

	// configs and owners hold the two files every repository is read for. The
	// sweep touches both for every repository on every tick
	configs *repoCache[*config.Config]
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

	// jobCtx outlives the request that enqueued a job and survives shutdown
	// being signalled, so a delivery already in the queue still completes
	jobCtx context.Context
}

// job is one delivery waiting to be executed.
type job struct {
	event      *webhook.IssueCommentEvent
	key        string
	deliveryID string
}

func newServer(cfg *serveConfig) (*server, error) {
	tokens, err := githubapp.NewTokenStore(
		cfg.appClientID, cfg.appPrivateKey, cfg.apiBaseURL, githubapp.DefaultMintTimeout)
	if err != nil {
		return nil, NewGitHubError(ErrGitHubAppAuth, err)
	}

	return &server{
		cfg:    cfg,
		tokens: tokens,
		configs: newRepoCache(repoFileTTL,
			func(ctx context.Context, client *github.Client, owner, repo string) (*config.Config, error) {
				return effectiveConfig(ctx, client, owner, repo, cfg.botConfig)
			}),
		owners:  newRepoCache(repoFileTTL, fetchCodeowners),
		deduper: webhook.NewDeduper(webhook.DefaultTTL, webhook.DefaultMaxEntries, nil),
		jobs:    make(chan job, queueDepth),
		jobCtx:  context.Background(),
	}, nil
}

// handler builds the service's routes.
//
// Only the webhook route sits behind signature verification; a probe has no
// signature to offer and reveals nothing.
func (s *server) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET "+healthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})

	verify := webhook.Middleware(s.cfg.webhookSecret)
	mux.Handle("POST "+s.cfg.webhookPath, verify(http.HandlerFunc(s.handleDelivery)))

	return mux
}

// Run serves until ctx is cancelled, then drains what is already in flight.
func (s *server) Run(ctx context.Context) error {
	// Work already accepted must survive the shutdown signal, or a rolling
	// update leaves a pull request approved but never merged
	s.jobCtx = context.WithoutCancel(ctx)

	// A listener that dies must stop the sweep too, not leave it polling on
	// behalf of a service that is no longer serving
	runCtx, stopSweep := context.WithCancel(ctx)
	defer stopSweep()

	workers := s.startWorkers()

	sweepStopped := make(chan struct{})

	go func() {
		defer close(sweepStopped)

		s.pollLoop(runCtx)
	}()

	srv := &http.Server{
		Addr:              s.cfg.listenAddress,
		Handler:           s.handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
	}

	listenErr := make(chan error, 1)

	go func() {
		log.Printf("listening on %s, webhooks at %s", s.cfg.listenAddress, s.cfg.webhookPath)
		listenErr <- srv.ListenAndServe()
	}()

	var shutdownErr error

	select {
	case err := <-listenErr:
		if !errors.Is(err, http.ErrServerClosed) {
			shutdownErr = err
		}

	case <-ctx.Done():
		log.Print("shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		shutdownErr = srv.Shutdown(shutdownCtx)
	}

	// Stop accepting first, then drain. Shutdown abandons a handler that is
	// still running once its deadline passes, so closeQueue rather than a bare
	// close: a late handler must be refused, not met with a closed channel
	stopSweep()
	s.closeQueue()
	s.drain(workers)
	<-sweepStopped

	return shutdownErr
}

// handleDelivery decides what a verified delivery deserves and answers GitHub.
//
// It answers before the work runs. GitHub gives a delivery ten seconds and does
// not retry one that times out, so executing inline would lose commands
// whenever a merge took longer than that.
func (s *server) handleDelivery(w http.ResponseWriter, r *http.Request) {
	deliveryID := safeDeliveryID(r.Header.Get(webhook.DeliveryHeader))

	eventName := r.Header.Get(webhook.EventHeader)

	// GitHub sends a ping when the webhook is first configured, and expects an
	// answer before it will send anything else
	if eventName == webhook.EventPing {
		w.WriteHeader(http.StatusOK)

		return
	}

	// A command only ever arrives on a comment. Subscribing to more events than
	// that is the operator's business, but nothing else has anything to execute
	if eventName != webhook.EventIssueComment {
		w.WriteHeader(http.StatusNoContent)

		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)

		return
	}

	event, err := webhook.ParseIssueComment(body)
	if err != nil {
		//nolint:gosec // G706: deliveryID passed through safeDeliveryID
		log.Printf("delivery %s: %v", deliveryID, err)
		http.Error(w, "malformed payload", http.StatusBadRequest)

		return
	}

	if !event.Actionable() {
		w.WriteHeader(http.StatusNoContent)

		return
	}

	if err := validateCommentInput(runtimeConfigFor(event, s.cfg)); err != nil {
		//nolint:gosec // G706: deliveryID passed through safeDeliveryID
		log.Printf("delivery %s from %s: %v", deliveryID, event.Repository.FullName, err)
		http.Error(w, "unusable payload", http.StatusBadRequest)

		return
	}

	s.dispatch(w, job{event: event, key: event.IdempotencyKey(), deliveryID: deliveryID})
}

// dispatch claims a delivery and queues it, or explains why it did neither.
func (s *server) dispatch(w http.ResponseWriter, j job) {
	// Claiming before queueing is what makes a redelivery harmless: the second
	// copy never reaches a worker
	if !s.deduper.Begin(j.key) {
		log.Printf("delivery %s from %s: already handled", j.deliveryID, j.event.Repository.FullName)
		w.WriteHeader(http.StatusOK)

		return
	}

	if s.enqueue(j) {
		w.WriteHeader(http.StatusAccepted)

		return
	}

	// Releasing the claim keeps the delivery retryable rather than dropping it
	// silently: GitHub records the refusal and a redelivery gets a fresh try
	s.deduper.Abandon(j.key)
	log.Printf("delivery %s from %s: not accepted", j.deliveryID, j.event.Repository.FullName)
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
	ctx, cancel := context.WithTimeout(s.jobCtx, jobTimeout)
	defer cancel()

	if err := s.handleIssueComment(ctx, j.event); err != nil {
		s.deduper.Abandon(j.key)
		log.Printf("delivery %s from %s: %v", j.deliveryID, j.event.Repository.FullName, err)
	}
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

	bc, err := s.configs.Get(ctx, client, event.Repository.Owner.Login, event.Repository.Name)
	if err != nil {
		if errors.Is(err, ErrRepoConfigInvalid) {
			return reportInvalidRepoConfig(ctx, client, rc, s.cfg.botConfig, err)
		}

		return err
	}

	return executeComment(ctx, client, rc, bc)
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
