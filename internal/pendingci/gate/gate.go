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

type RepositoryConfig interface {
	Config(
		ctx context.Context,
		client *github.Client,
		targetID, repositoryID, owner, repository string,
	) (*config.Config, error)
}

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
	Store       Store
	Checks      *Checks
	Coordinator bot.Exclusive
	Scheduler   *Scheduler
	Reconciler  *Reconciler
	Handoff     *Handoff
	Gates       *GateReconciler
	backend     *Backend
	Config      RepositoryConfig
	Tokens      *githubapp.TokenStore
	APIBaseURL  string
	BotUsername string
	Panelled    bool
	WakeGates   func()
	Logger      *slog.Logger
}

func (g *Gate) PassingQuiet() time.Duration {
	if g.Reconciler == nil {
		return 0
	}

	return g.Reconciler.currentTiming().PassingQuiet
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
	Now         func() time.Time
}

func New(deps Dependencies) *Gate {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	gate := &Gate{
		Store:       deps.Store,
		Coordinator: deps.Coordinator,
		Config:      deps.Config,
		Tokens:      deps.Tokens,
		APIBaseURL:  deps.APIBaseURL,
		BotUsername: deps.BotUsername,
		Panelled:    deps.Panelled,
		WakeGates:   deps.WakeGates,
		Logger:      deps.Logger,
	}

	gate.Checks = &Checks{
		store: deps.Checks, tokens: deps.Tokens, apiBaseURL: deps.APIBaseURL,
		now: now, syncer: bot.NewCoordinator(),
	}
	gate.Gates = &GateReconciler{store: deps.Gates, checks: gate.Checks, now: now}

	backend := &Backend{
		current:     deps.Current,
		source:      SourceValidator{config: deps.Config},
		config:      deps.Config,
		checkRuns:   gate.Checks,
		store:       deps.Store,
		tokens:      deps.Tokens,
		apiBaseURL:  deps.APIBaseURL,
		botUsername: deps.BotUsername,
		panelled:    deps.Panelled,
		wakeGates:   gate.wakeGates,
		quietPeriod: gate.PassingQuiet,
	}

	timing := defaultTiming()
	timing.PassingQuiet = deps.QuietPeriod
	gate.backend = backend
	gate.Reconciler = newReconciler(
		deps.Transitions, backend, backend, deps.Coordinator, timing,
	)
	gate.Scheduler = newScheduler(deps.Leases, gate.Reconciler, deps.Logger)
	gate.Scheduler.RetunePassingQuiet(deps.QuietPeriod)
	gate.Gates.wake = gate.Scheduler.Wake
	gate.Handoff = &Handoff{
		store: deps.Handoffs, coordinator: deps.Coordinator, wake: gate.Scheduler.Wake,
	}

	return gate
}

func (g *Gate) wakeGates() {
	if g.WakeGates != nil {
		g.WakeGates()
	}
}

func (g *Gate) ActivationGuardFor(
	client *github.Client,
	targetID, repositoryID, owner, repository string,
) ActivationGuard {
	return ActivationGuard{
		config: g.Config, store: g.Store, panelled: g.Panelled,
		client: client, targetID: targetID, repositoryID: repositoryID,
		owner: owner, repository: repository,
	}
}

func (g *Gate) NewControl(store ControlStore) *Control {
	return newControl(store, g.Coordinator, g.Scheduler.Wake)
}

func (g *Gate) Wake() {
	if g != nil && g.Scheduler != nil {
		g.Scheduler.Wake()
	}
}

func (g *Gate) RetuneQuietPeriod(value time.Duration) bool {
	if g == nil || g.Reconciler == nil {
		return false
	}
	if !g.Reconciler.SetPassingQuiet(value) {
		return false
	}
	if g.Scheduler == nil {
		g.Wake()

		return true
	}
	g.Scheduler.RetunePassingQuiet(value)

	return true
}

func (g *Gate) Backend() *Backend {
	return g.backend
}
