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

// dispatchDurableMaintenance leases existing work before reconciling the full
// candidate set. It executes at most one lease per wake so an overdue backlog
// yields to panel traffic before the next queue tick.
func (s *server) dispatchDurableMaintenance(ctx context.Context) error {
	jobs, err := s.durableMaintenanceJobs(ctx)
	if err != nil {
		return fmt.Errorf("build maintenance queue: %w", err)
	}
	claimed, err := s.runNextMaintenanceJob(ctx, jobs)
	if err != nil {
		return fmt.Errorf("run queued maintenance: %w", err)
	}
	if claimed {
		return nil
	}
	// Existing occurrences schedule their successors transactionally when
	// they finish. Reconcile the full candidate set only after that durable
	// backlog is empty, rather than republishing every candidate per lease.
	if err := s.ensureMaintenanceJobs(ctx, jobs); err != nil {
		return fmt.Errorf("publish maintenance queue: %w", err)
	}
	claimed, err = s.runNextMaintenanceJob(ctx, jobs)
	if err != nil {
		return fmt.Errorf("run published maintenance: %w", err)
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
	claims := maintenanceClaims(jobs, now)
	for _, claim := range claims {
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

func maintenanceClaims(jobs []maintenanceJob, now time.Time) []workqueue.RecurringClaim {
	claims := make([]workqueue.RecurringClaim, 0, len(jobs))
	for _, job := range jobs {
		claims = append(claims, workqueue.RecurringClaim{
			Kind: job.work.kind, TargetID: job.work.targetID,
			RepositoryID: job.work.repositoryID, Title: job.work.title,
			Now: now, LeaseDuration: recurringWorkLease,
		})
	}

	return claims
}

func (s *server) runNextMaintenanceJob(
	ctx context.Context,
	jobs []maintenanceJob,
) (bool, error) {
	item, claimed, err := s.store.ClaimNextRecurringWork(ctx, workqueue.RecurringLease{
		Now: time.Now().UTC(), LeaseDuration: recurringWorkLease,
	})
	if err != nil || !claimed {
		return false, err
	}
	for _, job := range jobs {
		if recurringWorkMatchesItem(job.work, item) {
			if err := s.runClaimedMaintenanceJob(ctx, item, job); err != nil {
				return true, fmt.Errorf("%s: %w", job.failureMessage, err)
			}

			return true, nil
		}
	}
	failure := fmt.Sprintf("no maintenance job matches queue item %q", item.ID)
	work := recurringWork{
		kind: item.Kind, targetID: item.TargetID, repositoryID: item.RepositoryID,
		title: item.Title,
	}
	s.announceRecurringWork(work)
	now := time.Now().UTC()
	_, finishErr := s.store.FinishRecurringWork(
		ctx, item.ID, failure, "", now,
	)
	s.announceRecurringWork(work)
	if finishErr != nil {
		return true, finishErr
	}
	retired, reconcileErr := s.store.SupersedeMissingRecurringWork(
		ctx, maintenanceClaims(jobs, now), now,
	)
	for _, retiredItem := range retired {
		s.announceRecurringWork(recurringWork{
			kind: retiredItem.Kind, targetID: retiredItem.TargetID,
			repositoryID: retiredItem.RepositoryID, title: retiredItem.Title,
		})
	}

	return true, reconcileErr
}

func recurringWorkMatchesItem(work recurringWork, item workqueue.Item) bool {
	return work.kind == item.Kind && optionalStringEqual(work.targetID, item.TargetID) &&
		optionalStringEqual(work.repositoryID, item.RepositoryID)
}

func optionalStringEqual(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func (s *server) runMaintenanceJob(ctx context.Context, job maintenanceJob) (bool, error) {
	run, runWithSummary := s.coordinatedMaintenanceRun(ctx, job)
	if runWithSummary != nil {
		return s.runRecurringWorkWithSummary(ctx, job.work, runWithSummary)
	}

	return s.runRecurringWork(ctx, job.work, run)
}

func (s *server) runClaimedMaintenanceJob(
	ctx context.Context,
	item workqueue.Item,
	job maintenanceJob,
) error {
	run, runWithSummary := s.coordinatedMaintenanceRun(ctx, job)
	if runWithSummary == nil {
		runWithSummary = func() (string, error) { return "", run() }
	}

	return s.runClaimedRecurringWorkWithSummary(ctx, item, job.work, runWithSummary)
}

func (s *server) coordinatedMaintenanceRun(
	ctx context.Context,
	job maintenanceJob,
) (func() error, func() (string, error)) {
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
	return run, runWithSummary
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
