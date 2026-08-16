package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
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
	s.deliveries.Start(s.jobCtx)

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
// reaction commands can only be found by looking. Pending-CI reconciliation
// has its own durable scheduler; this loop only performs its one-time safe
// drain of labels created by older service versions.
//
// Sweeping in the loop rather than in a goroutine per tick means a sweep that
// outruns the interval delays the next one instead of overlapping with it.
func (s *server) pollLoop(ctx context.Context) {
	migrationStopped := make(chan struct{})
	go func() {
		defer close(migrationStopped)
		s.migrationLoop(ctx)
	}()
	defer func() { <-migrationStopped }()

	interval := s.pollInterval()
	s.logPollInterval(interval)
	for {
		if interval <= 0 {
			select {
			case <-ctx.Done():
				return
			case <-s.pollIntervalChanged:
				interval = s.pollInterval()
				s.logPollInterval(interval)
			}

			continue
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			stopTimer(timer)

			return
		case <-s.pollIntervalChanged:
			stopTimer(timer)
			interval = s.pollInterval()
			s.logPollInterval(interval)
		case <-timer.C:
			s.runSweep(ctx)
			interval = s.pollInterval()
		}
	}
}

func (s *server) migrationLoop(ctx context.Context) {
	for {
		if err := s.migrationSweep(ctx); err == nil {
			return
		} else {
			s.logger.Error("pending CI migration sweep failed", "error", err)
		}
		timer := time.NewTimer(s.migrationRetryDelay)
		select {
		case <-ctx.Done():
			stopTimer(timer)

			return
		case <-timer.C:
		}
	}
}

func (s *server) logPollInterval(interval time.Duration) {
	if interval <= 0 {
		s.logger.Info("reaction polling disabled")

		return
	}
	s.logger.Info("sweeping reactions", "interval", interval.String())
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
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
	return s.sweepMode(ctx, true)
}

// migrationSweep performs only state handoff and pre-durable label cleanup.
// It runs once even when reaction polling is disabled.
func (s *server) migrationSweep(ctx context.Context) error {
	return s.sweepMode(ctx, false)
}

func (s *server) sweepMode(ctx context.Context, pollReactions bool) error {
	s.sweepMu.Lock()
	defer s.sweepMu.Unlock()

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

	var sweepErr error

	// Once per tick rather than once per installation. Retiring plans nobody
	// acted on names no installation, so running it inside the loop below issued
	// the same table-wide statement once for every account the App is installed
	// on and changed nothing after the first.
	if s.panel != nil {
		if err := s.store.ExpireSyncPlans(ctx, time.Now().UTC()); err != nil {
			logging.From(ctx).Error("could not retire expired sync plans", "error", err)
			sweepErr = errors.Join(sweepErr, err)
		}
	}

	for _, installation := range installations {
		installCtx := logging.With(ctx,
			"installation", installation.ID, "account", installation.Account)

		// One installation losing access must not stop the rest of the sweep
		if err := s.sweepInstallation(installCtx, installation, pollReactions); err != nil {
			logging.From(installCtx).Error("installation sweep failed", "error", err)
			sweepErr = errors.Join(sweepErr, err)
		}
	}

	return sweepErr
}

// sweepInstallation polls every repository one installation can reach.
func (s *server) sweepInstallation(
	ctx context.Context,
	installation github.Installation,
	pollReactions bool,
) error {
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

	var sweepErr error
	for _, repo := range repos {
		// The repository is named here rather than added to the context,
		// because pollAllPRs adds it for the lines below that
		if err := s.sweepRepo(
			ctx, client, installationStorageID(installation.ID), installation.ID, repo,
			pollReactions,
		); err != nil {
			logging.From(ctx).Error("repository sweep failed",
				"repo", repoFullName(repo.Owner, repo.Name), "error", err)
			sweepErr = errors.Join(sweepErr, err)
		}
	}

	// After the repositories, because planning compares against the catalog the
	// reconcile above has just refreshed. Its failure is reported and does not
	// stop the sweep: org sync is a slower promise than answering a comment,
	// and a planner that could not read GitHub gets another tick.
	if err := s.reconcileInstallationSync(ctx, client, installation); err != nil {
		logging.From(ctx).Error("sync reconcile failed", "error", err)
		sweepErr = errors.Join(sweepErr, err)
	}

	return sweepErr
}

// reconcileInstallationSync computes and applies whatever org sync is due.
//
// Planning and applying on the same tick, rather than waiting for the next one,
// so that a repository somebody switched sync on for is not left waiting an
// interval for work that was already decided.
func (s *server) reconcileInstallationSync(
	ctx context.Context,
	client *github.Client,
	installation github.Installation,
) error {
	if s.panel == nil {
		// No database means nowhere to keep a plan, and a sync with no plan is
		// the shape this work exists to replace.
		return nil
	}

	targetID := installationStorageID(installation.ID)

	if err := s.planInstallationSync(ctx, client, targetID, orgsync.TriggerReconcile); err != nil {
		return err
	}

	return s.applySyncPlans(ctx)
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
	if s.panel == nil {
		return repos, nil
	}
	snapshot, err := completeInstallationSnapshot(
		ctx,
		s.cfg.apiBaseURL,
		client,
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
	installationID int64,
	repo github.Repository,
	pollReactions bool,
) error {
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

	// Offered before the stand-down check, deliberately. Standing down is about
	// who answers comments; the file's format is not that question, and the
	// service is the only entry point with a database to remember a refusal in.
	// Left after the check, every repository that had pinned itself to the
	// Action would keep its legacy file for ever and nothing could migrate it.
	//
	// A failure is logged rather than returned: the sweep's job is to answer
	// reactions, and an unsolicited pull request failing to open must not stop
	// that.
	if err := s.migrateRepositoryConfig(ctx, client, targetID, repo); err != nil {
		logging.From(ctx).Warn("could not propose the configuration migration",
			"repo", repoFullName(repo.Owner, repo.Name), "error", err)
	}

	// Checked before CODEOWNERS is read, so a repository left to the Action
	// costs the sweep one request rather than two
	if serviceStandsDown(logging.With(ctx, "repo", repoFullName(repo.Owner, repo.Name)), bc) {
		return s.handoffPendingCIToAction(ctx, client, repo)
	}

	ctx = logging.With(ctx, "repo", repoFullName(repo.Owner, repo.Name))
	prs, err := client.GetOpenPRs(ctx, repo.Owner, repo.Name)
	if err != nil {
		return NewGitHubError(ErrGetPRs, err)
	}
	cleaned, err := s.reconcilePendingCIServiceArtifacts(
		ctx, client, repo, prs, !pollReactions,
	)
	if err != nil {
		return err
	}

	if s.panel != nil {
		target, repository, controlsErr := s.repositoryControls(
			ctx,
			targetID,
			repositoryStorageID(repo.ID),
		)
		if controlsErr != nil {
			return controlsErr
		}
		enabled := target.RepositoryDefaultEnabled
		if repository.EnabledOverride != nil {
			enabled = *repository.EnabledOverride
		}
		if !enabled {
			return nil
		}
	}

	if err := s.drainLegacyPendingCILabels(
		ctx, client, targetID, installationID, repo, prs, cleaned,
	); err != nil {
		return err
	}
	if !pollReactions {
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
	logging.From(ctx).Info("polling PR reactions")

	return processAllPRs(
		ctx, client, checker, bc, repo.Owner, repo.Name, s.cfg.botUsername, prs,
		s.reactionCommandEnvironment(repositoryStorageID(repo.ID)), false,
	)
}

// migrateRepositoryConfig reads the repository's configuration back out of the
// cache serviceConfig filled, and offers the move to TOML.
func (s *server) migrateRepositoryConfig(
	ctx context.Context,
	client *github.Client,
	targetID string,
	repo github.Repository,
) error {
	// The read is the same one serviceConfig has already made for this
	// repository, so it costs a map lookup whether or not there is a panel to
	// remember an answer in. proposeConfigMigration is where that is decided.
	file, err := s.configs.GetByKey(
		ctx, client, repositoryStorageID(repo.ID), repo.Owner, repo.Name,
	)
	if err != nil {
		return err
	}

	return s.proposeConfigMigration(ctx, client, targetID, repo, file)
}

func (s *server) handoffPendingCIToAction(
	ctx context.Context,
	client *github.Client,
	repo github.Repository,
) error {
	const reason = "repository switched to the GitHub Action runner"
	_, err := s.pendingCIHandoff.CancelRepository(
		ctx, repositoryStorageID(repo.ID), reason, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("cancel pending CI during runner handoff: %w", err)
	}
	prs, err := client.GetOpenPRs(ctx, repo.Owner, repo.Name)
	if err != nil {
		return NewGitHubError(ErrGetPRs, err)
	}

	_, err = s.reconcilePendingCIServiceArtifacts(ctx, client, repo, prs, true)

	return err
}
