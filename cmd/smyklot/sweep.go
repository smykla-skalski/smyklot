package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/metrics"
)

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
	var migrationStopped chan struct{}
	if s.panel == nil {
		migrationStopped = make(chan struct{})
		go func() {
			defer close(migrationStopped)
			s.migrationLoop(ctx)
		}()
		defer func() { <-migrationStopped }()
	} else {
		s.dispatchMaintenanceQueue(ctx)
	}

	interval := s.pollInterval()
	s.logPollInterval(interval)
	for {
		delay, err := s.nextMaintenanceDelay(ctx, interval)
		if err != nil {
			s.logger.Error("read maintenance queue schedule", "error", err)
		}
		if delay == nil {
			select {
			case <-ctx.Done():
				return
			case <-s.pollIntervalChanged:
				interval = s.pollInterval()
				s.logPollInterval(interval)
			case <-s.workQueueChanged:
				s.dispatchMaintenanceQueue(ctx)
			}

			continue
		}

		timer := time.NewTimer(*delay)
		select {
		case <-ctx.Done():
			timer.Stop()

			return
		case <-s.pollIntervalChanged:
			timer.Stop()
			interval = s.pollInterval()
			s.logPollInterval(interval)
		case <-s.workQueueChanged:
			timer.Stop()
			s.dispatchMaintenanceQueue(ctx)
			interval = s.pollInterval()
		case <-timer.C:
			s.dispatchMaintenanceQueue(ctx)
			interval = s.pollInterval()
		}
	}
}

func (s *server) nextMaintenanceDelay(
	ctx context.Context,
	fallback time.Duration,
) (*time.Duration, error) {
	now := time.Now().UTC()
	next, err := s.store.NextQueueAvailability(ctx, workqueue.LaneMaintenance, now)
	if err != nil {
		if fallback <= 0 {
			return nil, err
		}

		return &fallback, err
	}
	if next == nil {
		if fallback <= 0 {
			return nil, nil
		}

		return &fallback, nil
	}
	delay := next.Sub(now)
	if delay < 250*time.Millisecond {
		delay = 250 * time.Millisecond
	}
	if fallback > 0 && fallback < delay {
		delay = fallback
	}

	return &delay, nil
}

func (s *server) dispatchMaintenanceQueue(ctx context.Context) {
	if s.panel != nil {
		s.dispatchDurableMaintenance(ctx)

		return
	}
	s.maintainPanel(ctx)
	s.runSweep(ctx)
}

func (s *server) WakeQueue(lane workqueue.Lane) {
	switch lane {
	case workqueue.LaneWebhook:
		if s.deliveries != nil {
			s.deliveries.Wake()
		}
	case workqueue.LanePendingCI:
		if s.gate != nil {
			s.gate.Wake()
		}
	case workqueue.LaneMaintenance:
		select {
		case s.workQueueChanged <- struct{}{}:
		default:
		}
	}
}

func (s *server) migrationLoop(ctx context.Context) {
	for {
		if err := s.migrationSweep(ctx); err != nil {
			s.logger.Error("pending CI migration sweep failed", "error", err)
			timer := time.NewTimer(s.migrationRetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()

				return
			case <-timer.C:
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-s.pendingCIGateChanged:
		}
	}
}

func (s *server) WakePendingCIGates() {
	if s.panel != nil {
		s.WakeQueue(workqueue.LaneMaintenance)

		return
	}
	select {
	case s.pendingCIGateChanged <- struct{}{}:
	default:
	}
}

func (s *server) logPollInterval(interval time.Duration) {
	if interval <= 0 {
		s.logger.Info("reaction polling disabled")

		return
	}
	s.logger.Info("sweeping reactions", "interval", interval.String())
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

// migrationSweep performs state handoff, pre-durable label cleanup, and
// required-check gate reconciliation. It runs at startup and whenever panel or
// pull-request activity says installation policy may have changed, even when
// reaction polling is disabled.
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
		return bot.NewGitHubError(bot.ErrGitHubAppAuth, err)
	}

	appClient, err := github.NewAppClient(appToken, s.cfg.apiBaseURL)
	if err != nil {
		return bot.NewGitHubError(bot.ErrGitHubClient, err)
	}

	installations, err := appClient.ListInstallations(ctx)
	if err != nil {
		return bot.NewGitHubError(bot.ErrListInstallations, err)
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
		return bot.NewGitHubError(bot.ErrGitHubAppAuth, err)
	}

	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return bot.NewGitHubError(bot.ErrGitHubClient, err)
	}

	targetID := storage.InstallationID(installation.ID)
	var repos []github.Repository
	if s.panel == nil {
		repos, err = client.ListInstallationRepos(ctx)
		if err != nil {
			return bot.NewGitHubError(bot.ErrListRepos, err)
		}
	} else {
		repos, err = s.storedSweepRepositories(ctx, targetID)
		if err != nil {
			return err
		}
	}

	var sweepErr error
	for _, repo := range repos {
		// The repository is named here rather than added to the context,
		// because bot.PollAllPRs adds it for the lines below that
		if err := s.sweepRepo(
			ctx, client, targetID, installation.ID, repo,
			pollReactions,
		); err != nil {
			logging.From(ctx).Error("repository sweep failed",
				"repo", bot.RepoFullName(repo.Owner, repo.Name), "error", err)
			sweepErr = errors.Join(sweepErr, err)
		}
	}

	// After the repositories, because planning compares against the catalog.
	// Its failure is reported and does not
	// stop the sweep: org sync is a slower promise than answering a comment,
	// and a planner that could not read GitHub gets another tick.
	if err := s.reconcileInstallationSync(ctx, client, installation); err != nil {
		logging.From(ctx).Error("sync reconcile failed", "error", err)
		sweepErr = errors.Join(sweepErr, err)
	}

	return sweepErr
}

func (s *server) storedSweepRepositories(
	ctx context.Context,
	targetID string,
) ([]github.Repository, error) {
	stored, err := s.store.ListRepositories(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("list catalog repositories for sweep: %w", err)
	}
	repositories := make([]github.Repository, 0, len(stored))
	for _, repository := range stored {
		id, parseErr := strconv.ParseInt(repository.ID, 10, 64)
		parts := strings.SplitN(repository.FullName, "/", 2)
		if parseErr != nil || len(parts) != 2 {
			return nil, fmt.Errorf("read catalog repository identity %q", repository.FullName)
		}
		repositories = append(repositories, github.Repository{
			ID: id, Owner: parts[0], Name: parts[1], FullName: repository.FullName,
			Private: repository.Private, DefaultBranch: repository.DefaultBranch,
		})
	}

	return repositories, nil
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
		// The panel is where a plan is read and approved, and nothing else
		// approves one. Without it every reconcile would compute work nobody
		// can accept, hold the installation's one live slot until it expired,
		// and start again - so a deployment with no panel does not sync.
		//
		// It is also where the configuration is written, so there would be
		// nothing to enforce; the catalog reconcile above stands down for the
		// same reason and never records what an installation granted.
		return nil
	}

	targetID := storage.InstallationID(installation.ID)

	_, err := s.runRecurringWorkWithSummary(ctx, recurringWork{
		kind: workqueue.KindSyncScan, targetID: &targetID, title: "Scan organization sync drift",
	}, func() (string, error) {
		return s.sync.PlanInstallationWithSummary(
			ctx, client, targetID, orgsync.TriggerReconcile,
		)
	})
	if err != nil {
		return err
	}

	// After the plan rather than before it: this feeds a control that helps
	// somebody type a path, and nothing here is planned from it.
	if _, err := s.runRecurringWork(ctx, recurringWork{
		kind: workqueue.KindPathRefresh, targetID: &targetID, title: "Refresh repository paths",
	}, func() error {
		s.sync.RefreshPaths(ctx, client, targetID, 0)
		return nil
	}); err != nil {
		return err
	}

	return s.sync.ApplyPlans(ctx)
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
		storage.RepositoryID(repo.ID),
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
	repositoryID := storage.RepositoryID(repo.ID)
	_, migrationErr := s.runRecurringWork(ctx, recurringWork{
		kind: workqueue.KindConfigMigration, targetID: &targetID,
		repositoryID: &repositoryID, title: "Check configuration migration",
	}, func() error { return s.migrateRepositoryConfig(ctx, client, targetID, repo) })
	if migrationErr != nil {
		logging.From(ctx).Warn("could not propose the configuration migration",
			"repo", bot.RepoFullName(repo.Owner, repo.Name), "error", migrationErr)
	}
	var target storage.Target
	var repository storage.Repository
	if s.panel != nil {
		target, repository, err = s.repositoryControls(
			ctx, targetID, storage.RepositoryID(repo.ID),
		)
		if err != nil {
			return err
		}
	}

	// Checked before CODEOWNERS is read, so a repository left to the Action
	// costs the sweep one request rather than two
	if bot.ServiceStandsDown(logging.With(ctx, "repo", bot.RepoFullName(repo.Owner, repo.Name)), bc) {
		prs, err := s.handoffPendingCIToAction(ctx, client, repo)
		if err != nil {
			return err
		}
		return s.reconcileInactivePendingCIGate(ctx, client, target, repository, prs)
	}

	ctx = logging.With(ctx, "repo", bot.RepoFullName(repo.Owner, repo.Name))
	prs, err := client.GetOpenPRs(ctx, repo.Owner, repo.Name)
	if err != nil {
		return bot.NewGitHubError(bot.ErrGetPRs, err)
	}
	cleaned, err := s.gate.ReconcileServiceArtifacts(
		ctx, client, repo, prs, !pollReactions,
	)
	if err != nil {
		return err
	}

	enabled, err := s.reconcileActivePendingCIGate(ctx, client, target, repository, prs)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	if err := s.gate.DrainLegacyLabels(
		ctx, client, targetID, installationID, repo, prs, cleaned,
	); err != nil {
		return err
	}
	if !pollReactions {
		return nil
	}

	_, err = s.runRecurringWork(ctx, recurringWork{
		kind: workqueue.KindReactionScan, targetID: &targetID,
		repositoryID: &repositoryID, title: "Discover pull request reactions",
	}, func() error { return s.processRepositoryReactions(ctx, client, repo, bc, prs) })

	return err
}

func (s *server) processRepositoryReactions(
	ctx context.Context,
	client *github.Client,
	repo github.Repository,
	bc *config.Config,
	prs []map[string]interface{},
) error {
	codeowners, err := s.owners.Get(ctx, client, repo.Owner, repo.Name)
	if err != nil {
		return err
	}

	// The checker is built fresh rather than cached with the content, so it
	// always holds the client carrying the current installation token
	checker, err := bot.CheckerFromCodeowners(codeowners, client)
	if err != nil {
		return err
	}
	logging.From(ctx).Info("polling PR reactions")

	return bot.ProcessAllPRs(
		ctx, client, checker, bc, repo.Owner, repo.Name, s.cfg.botUsername, prs,
		s.reactionCommandEnvironment(storage.RepositoryID(repo.ID)), false,
	)
}

func (s *server) reconcileInactivePendingCIGate(
	ctx context.Context,
	client *github.Client,
	target storage.Target,
	repository storage.Repository,
	prs []map[string]interface{},
) error {
	if s.panel == nil {
		return nil
	}
	err := s.pendingCICoordinator.Exclusive(ctx, repository.ID, func() error {
		freshTarget, freshRepository, readErr := s.readRepositoryControls(
			ctx, target.ID, repository.ID,
		)
		if readErr != nil {
			return readErr
		}

		return s.gate.Gates.Reconcile(
			ctx, client, freshTarget, freshRepository, prs, false,
		)
	})
	if err != nil {
		return fmt.Errorf("reconcile inactive pending CI gate: %w", err)
	}

	return nil
}

func (s *server) reconcileActivePendingCIGate(
	ctx context.Context,
	client *github.Client,
	target storage.Target,
	repository storage.Repository,
	prs []map[string]interface{},
) (bool, error) {
	if s.panel == nil {
		return true, nil
	}
	enabled := false
	err := s.pendingCICoordinator.Exclusive(ctx, repository.ID, func() error {
		freshTarget, freshRepository, readErr := s.readRepositoryControls(
			ctx, target.ID, repository.ID,
		)
		if readErr != nil {
			return readErr
		}
		enabled = storage.RepositoryEnabled(freshTarget, freshRepository)

		return s.gate.Gates.Reconcile(
			ctx, client, freshTarget, freshRepository, prs, enabled,
		)
	})
	if err != nil {
		return enabled, fmt.Errorf("reconcile active pending CI gate: %w", err)
	}

	return enabled, nil
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
		ctx, client, storage.RepositoryID(repo.ID), repo.Owner, repo.Name,
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
) ([]map[string]interface{}, error) {
	const reason = "repository switched to the GitHub Action runner"
	_, err := s.gate.Handoff.CancelRepository(
		ctx, storage.RepositoryID(repo.ID), reason, time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("cancel pending CI during runner handoff: %w", err)
	}
	prs, err := client.GetOpenPRs(ctx, repo.Owner, repo.Name)
	if err != nil {
		return nil, bot.NewGitHubError(bot.ErrGetPRs, err)
	}

	_, err = s.gate.ReconcileServiceArtifacts(ctx, client, repo, prs, true)

	return prs, err
}
