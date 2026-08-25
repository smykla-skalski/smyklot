package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

func (s *server) maintainPanel(ctx context.Context) {
	for _, job := range s.panelMaintenanceJobs(ctx) {
		if _, err := s.runMaintenanceJob(ctx, job); err != nil {
			s.logger.Error(job.failureMessage, "error", err)
		}
	}
}

type maintenanceJob struct {
	work                 recurringWork
	run                  func() error
	runWithSummary       func() (string, error)
	coordinateRepository bool
	failureMessage       string
}

func (s *server) panelMaintenanceJobs(ctx context.Context) []maintenanceJob {
	jobs := []maintenanceJob{{
		work: recurringWork{kind: workqueue.KindDeliveryCleanup, title: "Clean up retained background work"},
		run: func() error {
			now := time.Now().UTC()
			if err := s.store.ExpireSyncPlans(ctx, now); err != nil {
				return err
			}
			policy, err := s.store.GetEffectiveQueuePolicy(
				ctx, workqueue.KindWebhookDelivery, nil,
			)
			if err != nil {
				return err
			}
			if policy.Retention != nil {
				if err := s.store.PruneDeliveries(ctx, now.Add(-*policy.Retention)); err != nil {
					return err
				}
			}
			_, err = s.store.PruneWorkQueue(ctx, now)

			return err
		},
		failureMessage: "delivery cleanup failed",
	}}
	if s.panel == nil {
		return jobs
	}

	return append(jobs,
		maintenanceJob{
			work:           recurringWork{kind: workqueue.KindCatalogRefresh, title: "Refresh installation catalog"},
			run:            func() error { return s.refreshPanelCatalog(ctx) },
			failureMessage: "panel catalog synchronization failed",
		},
		maintenanceJob{
			work:           recurringWork{kind: workqueue.KindAuthCleanup, title: "Clean up expired authentication"},
			run:            func() error { return s.store.DeleteExpiredAuth(ctx, time.Now().UTC()) },
			failureMessage: "panel authentication cleanup failed",
		},
	)
}

// dispatchDurableMaintenance publishes every maintenance candidate before it
// lets the shared dispatcher lease one. It executes at most one lease per wake
// so an overdue backlog yields to panel traffic before the next queue tick.
func (s *server) dispatchDurableMaintenance(ctx context.Context) error {
	jobs, err := s.durableMaintenanceJobs(ctx)
	if err != nil {
		return fmt.Errorf("build maintenance queue: %w", err)
	}
	if err := s.ensureMaintenanceJobs(ctx, jobs); err != nil {
		return fmt.Errorf("publish maintenance queue: %w", err)
	}
	claimed, err := s.runNextMaintenanceJob(ctx, jobs)
	if err != nil {
		return fmt.Errorf("run queued maintenance: %w", err)
	}
	if claimed {
		return nil
	}
	applied, err := s.sync.ApplyOnePlan(ctx)
	if err != nil {
		return fmt.Errorf("apply queued sync plan: %w", err)
	}
	if applied && s.panel != nil {
		s.panel.AnnounceQueue("")
	}

	return nil
}

func (s *server) durableMaintenanceJobs(ctx context.Context) ([]maintenanceJob, error) {
	jobs := s.panelMaintenanceJobs(ctx)
	targets, err := s.store.ListRootTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list maintenance installations: %w", err)
	}
	for _, target := range targets {
		if !target.Available {
			continue
		}
		installationID, err := strconv.ParseInt(target.InstallationID, 10, 64)
		if err != nil || installationID <= 0 {
			return nil, fmt.Errorf("parse maintenance installation id %q", target.InstallationID)
		}
		targetID := target.ID
		jobs = append(jobs, s.targetMaintenanceJobs(ctx, targetID, installationID)...)
		repositories, err := s.storedSweepRepositories(ctx, targetID)
		if err != nil {
			return nil, err
		}
		for _, repository := range repositories {
			repository := repository
			jobs = append(jobs, s.repositoryMaintenanceJobs(
				ctx, targetID, installationID, repository,
			)...)
		}
	}

	return jobs, nil
}

func (s *server) targetMaintenanceJobs(
	ctx context.Context,
	targetID string,
	installationID int64,
) []maintenanceJob {
	return []maintenanceJob{
		{
			work: recurringWork{kind: workqueue.KindSyncScan, targetID: &targetID, title: "Scan organization sync drift"},
			runWithSummary: func() (string, error) {
				client, err := s.queuedInstallationClient(installationID)
				if err != nil {
					return "", err
				}

				return s.sync.PlanInstallationWithSummary(
					ctx, client, targetID, orgsync.TriggerReconcile,
				)
			},
			failureMessage: "organization sync scan failed",
		},
		{
			work: recurringWork{kind: workqueue.KindPathRefresh, targetID: &targetID, title: "Refresh repository paths"},
			run: func() error {
				client, err := s.queuedInstallationClient(installationID)
				if err != nil {
					return err
				}
				s.sync.RefreshPaths(ctx, client, targetID, 0)

				return nil
			},
			failureMessage: "path refresh failed",
		},
	}
}

func (s *server) repositoryMaintenanceJobs(
	ctx context.Context,
	targetID string,
	installationID int64,
	repository github.Repository,
) []maintenanceJob {
	repositoryID := storage.RepositoryID(repository.ID)

	return []maintenanceJob{
		{
			work: recurringWork{
				kind: workqueue.KindPendingCIGate, targetID: &targetID,
				repositoryID: &repositoryID, title: "Reconcile pending CI protection",
			},
			run: func() error {
				return s.reconcileQueuedPendingCIGate(
					ctx, targetID, installationID, repository,
				)
			},
			failureMessage: "pending CI protection reconciliation failed",
		},
		{
			work: recurringWork{
				kind: workqueue.KindConfigMigration, targetID: &targetID,
				repositoryID: &repositoryID, title: "Check configuration migration",
			},
			run: func() error {
				client, err := s.queuedInstallationClient(installationID)
				if err != nil {
					return err
				}

				return s.migrateRepositoryConfig(ctx, client, targetID, repository)
			},
			coordinateRepository: true,
			failureMessage:       "configuration migration check failed",
		},
		{
			work: recurringWork{
				kind: workqueue.KindReactionScan, targetID: &targetID,
				repositoryID: &repositoryID, title: "Discover pull request reactions",
			},
			run: func() error {
				return s.scanQueuedReactions(ctx, targetID, installationID, repository)
			},
			coordinateRepository: true,
			failureMessage:       "reaction discovery failed",
		},
	}
}

func (s *server) ensureMaintenanceJobs(ctx context.Context, jobs []maintenanceJob) error {
	announced := map[string]bool{}
	now := time.Now().UTC()
	claims := make([]workqueue.RecurringClaim, 0, len(jobs))
	for _, job := range jobs {
		claim := workqueue.RecurringClaim{
			Kind: job.work.kind, TargetID: job.work.targetID,
			RepositoryID: job.work.repositoryID, Title: job.work.title,
			Now: now, LeaseDuration: recurringWorkLease,
		}
		claims = append(claims, claim)
		item, err := s.store.EnsureRecurringWork(ctx, claim)
		if err != nil {
			return err
		}
		if item.ID == "" || s.panel == nil {
			continue
		}
		targetID := ""
		if item.TargetID != nil {
			targetID = *item.TargetID
		}
		announced[targetID] = true
	}
	superseded, err := s.store.SupersedeMissingRecurringWork(ctx, claims, now)
	if err != nil {
		return err
	}
	for _, item := range superseded {
		targetID := ""
		if item.TargetID != nil {
			targetID = *item.TargetID
		}
		announced[targetID] = true
	}
	if s.panel != nil {
		for targetID := range announced {
			s.panel.AnnounceQueue(targetID)
		}
	}

	return nil
}

func (s *server) runNextMaintenanceJob(
	ctx context.Context,
	jobs []maintenanceJob,
) (bool, error) {
	for _, job := range jobs {
		claimed, err := s.runMaintenanceJob(ctx, job)
		if err != nil {
			return claimed, fmt.Errorf("%s: %w", job.failureMessage, err)
		}
		if claimed {
			return true, nil
		}
	}

	return false, nil
}

func (s *server) runMaintenanceJob(ctx context.Context, job maintenanceJob) (bool, error) {
	run := job.run
	runWithSummary := job.runWithSummary
	if job.coordinateRepository && job.work.repositoryID != nil {
		if runWithSummary != nil {
			original := runWithSummary
			runWithSummary = func() (string, error) {
				var summary string
				err := s.pendingCICoordinator.Exclusive(ctx, *job.work.repositoryID, func() error {
					var runErr error
					summary, runErr = original()

					return runErr
				})

				return summary, err
			}
		} else {
			original := run
			run = func() error {
				return s.pendingCICoordinator.Exclusive(ctx, *job.work.repositoryID, original)
			}
		}
	}
	if runWithSummary != nil {
		return s.runRecurringWorkWithSummary(ctx, job.work, runWithSummary)
	}

	return s.runRecurringWork(ctx, job.work, run)
}

func (s *server) queuedInstallationClient(installationID int64) (*github.Client, error) {
	token, err := s.tokens.InstallationToken(installationID)
	if err != nil {
		return nil, bot.NewGitHubError(bot.ErrGitHubAppAuth, err)
	}
	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return nil, bot.NewGitHubError(bot.ErrGitHubClient, err)
	}

	return client, nil
}

func (s *server) reconcileQueuedPendingCIGate(
	ctx context.Context,
	targetID string,
	installationID int64,
	repository github.Repository,
) error {
	client, err := s.queuedInstallationClient(installationID)
	if err != nil {
		return err
	}
	botConfig, err := s.serviceConfig(
		ctx, client, targetID, storage.RepositoryID(repository.ID),
		repository.Owner, repository.Name,
	)
	if err != nil {
		return err
	}
	target, stored, err := s.repositoryControls(
		ctx, targetID, storage.RepositoryID(repository.ID),
	)
	if err != nil {
		return err
	}
	if bot.ServiceStandsDown(
		logging.With(ctx, "repo", bot.RepoFullName(repository.Owner, repository.Name)),
		botConfig,
	) {
		prs, err := s.handoffPendingCIToAction(ctx, client, repository)
		if err != nil {
			return err
		}

		return s.reconcileInactivePendingCIGate(ctx, client, target, stored, prs)
	}
	prs, err := client.GetOpenPRs(ctx, repository.Owner, repository.Name)
	if err != nil {
		return bot.NewGitHubError(bot.ErrGetPRs, err)
	}
	cleaned, err := s.gate.ReconcileServiceArtifacts(ctx, client, repository, prs, true)
	if err != nil {
		return err
	}
	enabled, err := s.reconcileActivePendingCIGate(ctx, client, target, stored, prs)
	if err != nil || !enabled {
		return err
	}

	return s.gate.DrainLegacyLabels(
		ctx, client, targetID, installationID, repository, prs, cleaned,
	)
}

func (s *server) scanQueuedReactions(
	ctx context.Context,
	targetID string,
	installationID int64,
	repository github.Repository,
) error {
	client, err := s.queuedInstallationClient(installationID)
	if err != nil {
		return err
	}
	botConfig, err := s.serviceConfig(
		ctx, client, targetID, storage.RepositoryID(repository.ID),
		repository.Owner, repository.Name,
	)
	if err != nil {
		return err
	}
	if bot.ServiceStandsDown(
		logging.With(ctx, "repo", bot.RepoFullName(repository.Owner, repository.Name)),
		botConfig,
	) {
		return nil
	}
	prs, err := client.GetOpenPRs(ctx, repository.Owner, repository.Name)
	if err != nil {
		return bot.NewGitHubError(bot.ErrGetPRs, err)
	}

	return s.processRepositoryReactions(ctx, client, repository, botConfig, prs)
}

func (s *server) refreshPanelCatalog(ctx context.Context) error {
	_, err := s.SyncCatalog(ctx)
	if err == nil {
		if s.gate != nil {
			s.gate.Wake()
		}
		s.WakePendingCIGates()
	}

	return err
}
