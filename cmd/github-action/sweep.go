package main

import (
	"context"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/metrics"
)

// startWorkers launches the pool that executes queued deliveries.
func (s *server) startWorkers() *sync.WaitGroup {
	var workers sync.WaitGroup

	for range workerCount {
		workers.Add(1)

		go func() {
			defer workers.Done()

			for j := range s.jobs {
				s.execute(j)
			}
		}()
	}

	return &workers
}

// drain waits for queued deliveries to finish, giving up rather than blocking a
// shutdown forever.
func (s *server) drain(workers *sync.WaitGroup) {
	drained := make(chan struct{})

	go func() {
		workers.Wait()
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(drainTimeout):
		s.logger.Error("gave up waiting for in-flight deliveries", "timeout", drainTimeout.String())
	}
}

// pollLoop sweeps on an interval until ctx is cancelled.
//
// GitHub delivers no webhook when someone adds or removes a reaction, so
// reaction commands can only be found by looking. The same sweep merges pull
// requests that were waiting for CI.
//
// Sweeping in the loop rather than in a goroutine per tick means a sweep that
// outruns the interval delays the next one instead of overlapping with it.
func (s *server) pollLoop(ctx context.Context) {
	if s.cfg.pollInterval <= 0 {
		s.logger.Info("reaction polling disabled")

		return
	}

	s.logger.Info("sweeping reactions", "interval", s.cfg.pollInterval.String())

	ticker := time.NewTicker(s.cfg.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			s.runSweep(ctx)
		}
	}
}

// runSweep sweeps once and records how it went.
//
// The duration matters as much as the outcome: a sweep that grows past the
// interval delays every one after it, and this is where that shows up before
// reactions start arriving late.
func (s *server) runSweep(ctx context.Context) {
	started := time.Now()

	err := s.sweep(ctx)
	elapsed := time.Since(started)

	s.metrics.SweepDuration.Observe(elapsed.Seconds())

	if err != nil {
		s.metrics.Sweeps.WithLabelValues(metrics.ResultFailure).Inc()
		s.logger.Error("sweep failed", "error", err, "duration", elapsed.String())

		return
	}

	s.metrics.Sweeps.WithLabelValues(metrics.ResultSuccess).Inc()
	s.logger.Debug("sweep complete", "duration", elapsed.String())
}

// sweep polls every repository the App is installed on.
//
// The installation list comes from GitHub rather than from configuration, so a
// repository installed while the process runs is swept on the next tick without
// a restart.
func (s *server) sweep(ctx context.Context) error {
	// A sweep is where a chain of per-installation and per-repository
	// attributes starts, so it seeds that chain itself rather than trusting
	// whoever called it to have done so
	ctx = logging.Into(ctx, s.logger)

	appToken, err := s.tokens.AppToken()
	if err != nil {
		return NewGitHubError(ErrGitHubAppAuth, err)
	}

	appClient, err := github.NewAppClient(appToken, s.cfg.apiBaseURL)
	if err != nil {
		return NewGitHubError(ErrGitHubClient, err)
	}

	installations, err := appClient.ListInstallations(ctx)
	if err != nil {
		return NewGitHubError(ErrListInstallations, err)
	}

	for _, installation := range installations {
		installCtx := logging.With(ctx,
			"installation", installation.ID, "account", installation.Account)

		// One installation losing access must not stop the rest of the sweep
		if err := s.sweepInstallation(installCtx, installation); err != nil {
			logging.From(installCtx).Error("installation sweep failed", "error", err)
		}
	}

	return nil
}

// sweepInstallation polls every repository one installation can reach.
func (s *server) sweepInstallation(ctx context.Context, installation github.Installation) error {
	token, err := s.tokens.InstallationToken(installation.ID)
	if err != nil {
		return NewGitHubError(ErrGitHubAppAuth, err)
	}

	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return NewGitHubError(ErrGitHubClient, err)
	}

	repos, err := s.reconcileSweepInstallation(ctx, client, installation)
	if err != nil {
		return err
	}

	for _, repo := range repos {
		// The repository is named here rather than added to the context,
		// because pollAllPRs adds it for the lines below that
		if err := s.sweepRepo(ctx, client, installationStorageID(installation.ID), repo); err != nil {
			logging.From(ctx).Error("repository sweep failed",
				"repo", repoFullName(repo.Owner, repo.Name), "error", err)
		}
	}

	return nil
}

func (s *server) reconcileSweepInstallation(
	ctx context.Context,
	client *github.Client,
	installation github.Installation,
) ([]github.Repository, error) {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()

	repos, err := client.ListInstallationRepos(ctx)
	if err != nil {
		return nil, NewGitHubError(ErrListRepos, err)
	}
	if s.store == nil {
		return repos, nil
	}
	snapshot, err := installationSnapshot(
		s.cfg.apiBaseURL,
		installation,
		repos,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	if err := s.store.ReconcileInstallation(ctx, snapshot); err != nil {
		return nil, err
	}
	if s.panel != nil {
		if err := s.store.GrantOwnerAccess(ctx, snapshot.TargetID, time.Now().UTC()); err != nil {
			return nil, err
		}
		s.panel.Announce(snapshot.TargetID, "")
	}

	return repos, nil
}

// sweepRepo polls one repository, using the same code the poll subcommand runs.
//
// Both files it needs are cached: a sweep would otherwise re-read every
// repository's CODEOWNERS and config on every tick, forever, for content that
// changes far less often than it is looked at.
func (s *server) sweepRepo(
	ctx context.Context,
	client *github.Client,
	targetID string,
	repo github.Repository,
) error {
	if s.store != nil {
		target, repository, err := s.repositoryControls(
			ctx,
			targetID,
			repositoryStorageID(repo.ID),
		)
		if err != nil {
			return err
		}
		enabled := target.RepositoryDefaultEnabled
		if repository.EnabledOverride != nil {
			enabled = *repository.EnabledOverride
		}
		if !enabled {
			return nil
		}
	}

	bc, err := s.serviceConfig(
		ctx,
		client,
		targetID,
		repositoryStorageID(repo.ID),
		repo.Owner,
		repo.Name,
	)
	if err != nil {
		return err
	}

	// Checked before CODEOWNERS is read, so a repository left to the Action
	// costs the sweep one request rather than two
	if serviceStandsDown(logging.With(ctx, "repo", repoFullName(repo.Owner, repo.Name)), bc) {
		return nil
	}

	codeowners, err := s.owners.Get(ctx, client, repo.Owner, repo.Name)
	if err != nil {
		return err
	}

	// The checker is built fresh rather than cached with the content, so it
	// always holds the client carrying the current installation token
	checker, err := checkerFromCodeowners(codeowners, client)
	if err != nil {
		return err
	}

	return pollAllPRs(ctx, client, checker, bc, repo.Owner, repo.Name, s.cfg.botUsername)
}
