// Package gate is the loop that holds a pull request open until CI settles.
//
// A command arms a pull request; from then on this package owns it. It watches
// the checks GitHub reports, keeps the check run the repository sees in step
// with them, renews an authorization the head moved out from under, and merges
// once the policy in internal/pendingci says the evidence is good enough.
//
// It is a subpackage of the domain it serves rather than a sibling because it
// imports that domain on nearly every line, and because internal/storage
// imports the parent - a sibling would have to route around that, and a child
// does not.
//
// What it needs from the service it runs inside is two questions about a
// repository, spelled out in RepositoryConfig. Everything else it holds
// itself.
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

// RepositoryConfig resolves what a repository's own configuration file says,
// which is the one question about a repository that the runtime cannot answer
// for itself: the answer is layered over account settings and a cache the
// service owns.
//
// It does not refresh the catalog. A delivery arriving for a repository nobody
// has enabled must cost nothing, and a runtime that could refresh would be the
// second place doing it - the sweep is the first, and the two would race over
// the same rows on every webhook.
//
// What the panel has switched on is not here: that is two rows, and the runtime
// reads them through its own Store rather than asking the service to.
type RepositoryConfig interface {
	Config(
		ctx context.Context,
		client *github.Client,
		targetID, repositoryID, owner, repository string,
	) (*config.Config, error)
}

// controls reads the account and repository rows the panel writes.
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

// Gate is the pending CI runtime for one process.
//
// Panelled rather than a panel: the runtime asks whether one is configured and
// never anything else, and holding the panel itself would put the whole admin
// surface one dot away from a webhook handler.
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

	// WakeGates asks the sweep to reconcile branch protection sooner than its
	// next tick. The channel behind it belongs to the service, which owns the
	// loop that reads it.
	WakeGates func()

	Logger *slog.Logger
}

// PassingQuiet is how long CI has to stay green before a merge, as the
// reconciler currently has it. Read through the reconciler rather than stored,
// because the panel can retune it while the process runs.
func (g *Gate) PassingQuiet() time.Duration {
	if g.Reconciler == nil {
		return 0
	}

	return g.Reconciler.currentTiming().PassingQuiet
}

// Dependencies are what the service hands the runtime.
//
// One struct rather than a constructor with a dozen positional arguments, and
// the same shape internal/panel already takes: what a subsystem needs is a list
// somebody has to read, and a list reads better with names on it.
type Dependencies struct {
	// Store is the durable state the runtime reads on its own account.
	Store Store

	// Gates is the wider store the branch-protection reconciler needs. It is
	// separate because that reconciler writes repository gate rows nothing
	// else here touches.
	Gates gateStore

	// Checks is where the check run this runtime maintains is persisted.
	Checks pendingci.CheckStore

	// Transitions is the durable pending-CI state machine.
	Transitions transitionStore

	// Leases is the queue the scheduler pulls work from.
	Leases leaseStore

	// Handoffs records a repository handing pending CI back to the Action.
	Handoffs handoffStore

	// Current answers what a pull request is currently armed with.
	Current currentStore

	// Config resolves a repository's own configuration file, layered over the
	// account's settings. The one answer the runtime cannot reach itself.
	Config RepositoryConfig

	// Coordinator serializes work on one repository across every path into it.
	Coordinator bot.Exclusive

	Tokens      *githubapp.TokenStore
	APIBaseURL  string
	BotUsername string

	// QuietPeriod is how long CI must stay green before a merge.
	QuietPeriod time.Duration

	// Panelled says whether a panel is configured. The runtime asks nothing
	// else about it.
	Panelled bool

	// WakeGates asks the sweep to reconcile branch protection sooner than its
	// next tick.
	WakeGates func()

	Logger *slog.Logger

	// Now is the clock. Tests pass a fake one.
	Now func() time.Time
}

// New wires the runtime.
//
// The order matters and is the reason this is one function rather than a
// service assembling nine pieces: the reconciler needs the backend, the backend
// needs the check service, the scheduler needs the reconciler, and the gate
// reconciler and the handoff both need the scheduler's wake. Assembled from
// outside, that order was something a reader had to reconstruct from a hundred
// lines of newServer.
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

// wakeGates is nil-safe, so a deployment that does not run the sweep - which is
// every test that builds only the runtime - does not have to supply one.
func (g *Gate) wakeGates() {
	if g.WakeGates != nil {
		g.WakeGates()
	}
}

// ActivationGuardFor builds the guard one command's activation is checked
// against. The client is the installation's, so it cannot be held on the Gate.
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

// NewControl is the operator's way in, for the panel. Built on demand because
// the panel is optional and this is the only thing it needs from the runtime,
// and it takes its own store because operator transitions are writes the
// runtime itself never makes.
func (g *Gate) NewControl(store ControlStore) *Control {
	return newControl(store, g.Coordinator, g.Scheduler.Wake)
}

// Wake asks the scheduler to look for work now rather than at its next tick.
// Nil-safe, because a process without a runtime still answers webhooks.
func (g *Gate) Wake() {
	if g != nil && g.Scheduler != nil {
		g.Scheduler.Wake()
	}
}

// RetuneQuietPeriod applies a quiet period an operator changed while the
// process was running. It reports whether anything moved.
func (g *Gate) RetuneQuietPeriod(value time.Duration) bool {
	if g == nil || g.Reconciler == nil {
		return false
	}
	if !g.Reconciler.SetPassingQuiet(value) {
		return false
	}
	g.Wake()

	return true
}

// Backend is live GitHub as the reconciler sees it. Exported for the service's
// own specs, which assert on what an installation looks like from here.
func (g *Gate) Backend() *Backend {
	return g.backend
}
