// Package gate is the loop that holds a pull request open until CI settles.
package gate

import (
	"context"
	"log/slog"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/githubapp"
)

type RepositoryConfig func(
	ctx context.Context,
	client *github.Client,
	targetID, repositoryID, owner, repository string,
) (*config.Config, error)

func readControls(
	ctx context.Context,
	store Store,
	targetID, repositoryID string,
) (storage.Target, storage.Repository, error) {
	target, err := store.GetTarget(ctx, targetID)
	if err != nil {
		return storage.Target{}, storage.Repository{}, err
	}
	repository, err := store.GetRepository(ctx, targetID, repositoryID)
	if err != nil {
		return storage.Target{}, storage.Repository{}, err
	}

	return target, repository, nil
}

type Gate struct {
	Checks    *Checks
	Scheduler *Scheduler
	Handoff   *Handoff
	Gates     *GateReconciler

	store       Store
	coordinator bot.Exclusive
	reconciler  *Reconciler
	backend     *Backend
	config      RepositoryConfig
	tokens      *githubapp.TokenStore
	apiBaseURL  string
	botUsername string
	panelled    bool
	wakeGates   func()
}

func (g *Gate) PassingQuiet() time.Duration {
	if g.reconciler == nil {
		return 0
	}

	return g.reconciler.currentTiming().PassingQuiet
}

type Dependencies struct {
	Store       Store
	Gates       gateStore
	Checks      pendingci.CheckStore
	Transitions transitionStore
	Leases      leaseStore
	Handoffs    handoffStore
	Current     currentStore
	Config      RepositoryConfig
	Coordinator bot.Exclusive
	Tokens      *githubapp.TokenStore
	APIBaseURL  string
	BotUsername string
	QuietPeriod time.Duration
	Panelled    bool
	WakeGates   func()
	Logger      *slog.Logger
}

func New(deps Dependencies) *Gate {
	now := func() time.Time { return time.Now().UTC() }

	gate := &Gate{
		store:       deps.Store,
		coordinator: deps.Coordinator,
		config:      deps.Config,
		tokens:      deps.Tokens,
		apiBaseURL:  deps.APIBaseURL,
		botUsername: deps.BotUsername,
		panelled:    deps.Panelled,
		wakeGates:   deps.WakeGates,
	}

	gate.Checks = &Checks{
		store: deps.Checks, tokens: deps.Tokens, apiBaseURL: deps.APIBaseURL,
		now: now, syncer: bot.NewCoordinator(),
	}
	gate.Gates = &GateReconciler{store: deps.Gates, checks: gate.Checks, now: now}

	backend := &Backend{
		current:     deps.Current,
		source:      sourceValidator{config: deps.Config},
		config:      deps.Config,
		checkRuns:   gate.Checks,
		store:       deps.Store,
		tokens:      deps.Tokens,
		apiBaseURL:  deps.APIBaseURL,
		botUsername: deps.BotUsername,
		panelled:    deps.Panelled,
		wakeGates:   gate.notifyGates,
		quietPeriod: gate.PassingQuiet,
	}

	timing := defaultTiming()
	timing.PassingQuiet = deps.QuietPeriod
	gate.backend = backend
	gate.reconciler = newReconciler(
		deps.Transitions, backend, backend, deps.Coordinator, timing,
	)
	gate.Scheduler = newScheduler(deps.Leases, gate.reconciler, deps.Logger)
	gate.Scheduler.RetunePassingQuiet(deps.QuietPeriod)
	gate.Gates.wake = gate.Scheduler.Wake
	gate.Handoff = &Handoff{
		store: deps.Handoffs, coordinator: deps.Coordinator, wake: gate.Scheduler.Wake,
	}

	return gate
}

func (g *Gate) notifyGates() {
	if g.wakeGates != nil {
		g.wakeGates()
	}
}

func (g *Gate) ActivationGuardFor(
	client *github.Client,
	targetID, repositoryID, owner, repository string,
) ActivationGuard {
	return ActivationGuard{
		config: g.config, store: g.store, panelled: g.panelled,
		client: client, targetID: targetID, repositoryID: repositoryID,
		owner: owner, repository: repository,
	}
}

func (g *Gate) NewControl(store ControlStore) *Control {
	return newControl(store, g.coordinator, g.Wake)
}

func (g *Gate) Wake() {
	if g != nil && g.Scheduler != nil {
		g.Scheduler.Wake()
	}
}

func (g *Gate) RetuneQuietPeriod(value time.Duration) bool {
	if g == nil || g.reconciler == nil {
		return false
	}
	if !g.reconciler.SetPassingQuiet(value) {
		return false
	}
	if g.Scheduler != nil {
		g.Scheduler.RetunePassingQuiet(value)
	}

	return true
}

func (g *Gate) Backend() *Backend {
	return g.backend
}
