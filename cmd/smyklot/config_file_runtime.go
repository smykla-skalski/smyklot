package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/configsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func (s *server) configurationFileMaintenanceJob(
	ctx context.Context, targetID, repositoryID string, installationID int64,
) maintenanceJob {
	work := recurringWork{
		kind: workqueue.KindConfigFileSync, targetID: &targetID,
		title: "Sync the workspace configuration file",
	}
	if repositoryID != "" {
		work.repositoryID = &repositoryID
		work.title = "Sync the repository configuration file"
	}
	return maintenanceJob{
		work: work,
		runWithSummary: func() (string, error) {
			return s.reconcileQueuedConfigurationFile(ctx, targetID, repositoryID, installationID)
		},
		failureMessage: "configuration file synchronization failed",
	}
}

func (s *server) reconcileQueuedConfigurationFile(
	ctx context.Context, targetID, repositoryID string, installationID int64,
) (string, error) {
	connection := configsync.Connection{Status: configsync.StatusOff}
	err := s.withConfigurationFileScope(ctx, targetID, repositoryID, func() error {
		// Candidates can outlive a settings save or a catalog refresh. Recheck
		// access and opt-in under exclusion before obtaining a GitHub client.
		enabled, err := s.configurationFileEnabled(ctx, targetID, repositoryID)
		if err != nil || !enabled {
			return err
		}
		client, err := s.queuedInstallationClient(installationID)
		if err != nil {
			return err
		}
		engine := configsync.Engine{Store: s.store, QuietPeriod: s.cfg.pendingCIQuietPeriod}
		connection, err = engine.Run(ctx, client, targetID, repositoryID)
		return err
	})
	if err != nil {
		return "", err
	}
	if s.panel != nil {
		s.panel.Announce(targetID, repositoryID)
	}
	if s.gate != nil {
		s.gate.Wake()
	}
	// A persisted conflict needs attention, but this check completed. Keep its
	// successor so a correction on GitHub is still found without a manual retry.
	switch connection.Status {
	case configsync.StatusOff:
		return "Configuration file sync is off", nil
	case configsync.StatusReady:
		return "Panel and file settings are in sync", nil
	case configsync.StatusProposed:
		return "Configuration changes are in a pull request", nil
	case configsync.StatusBlocked:
		return "Configuration file needs attention", nil
	default:
		return "Configuration file check is pending", nil
	}
}

func (s *server) configurationFileEnabled(ctx context.Context, targetID, repositoryID string) (bool, error) {
	target, err := s.store.GetTarget(ctx, targetID)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && !target.Available) {
		return false, nil
	}
	if err != nil || repositoryID == "" {
		return target.ConfigFileSyncEnabled, err
	}
	repository, err := s.store.GetRepository(ctx, targetID, repositoryID)
	if errors.Is(err, storage.ErrNotFound) {
		return false, nil
	}
	return repository.Available && repository.ConfigFileSyncEnabled, err
}

func (s *server) requireConfigurationFileBaseline(ctx context.Context, targetID, repositoryID string) error {
	stored, err := s.store.GetConfigFileState(ctx, targetID, repositoryID)
	if err != nil {
		return bot.NewConfigError(bot.ErrConfigLoad, err)
	}
	connection, err := configsync.DecodeConnection(stored, config.PanelFileRepository)
	if err != nil {
		return bot.NewConfigError(bot.ErrConfigLoad, err)
	}
	if stored.InitializationRequired || !connection.Base.Exists {
		// Opting in must not drop file-owned restrictions before those settings
		// have been imported or their initial conflict has been resolved.
		return bot.NewConfigError(bot.ErrConfigLoad,
			errors.New("configuration file sync is waiting for successful reconciliation after being enabled"))
	}
	return nil
}

// Workspace imports can change settings for every repository. Acquire the same
// catalog-then-repositories order as panel saves and catalog transfers. Repository
// imports need only their own exclusion and never refresh the catalog inside it.
func (s *server) withConfigurationFileScope(
	ctx context.Context, targetID, repositoryID string, operation func() error,
) error {
	if repositoryID != "" {
		return bot.ExclusiveRepositories(ctx, s.pendingCICoordinator, []string{repositoryID}, operation)
	}
	return bot.ExclusiveRepositories(ctx, s.pendingCICoordinator, []string{bot.CatalogCoordinatorKey}, func() error {
		repositories, err := s.store.ListRepositories(ctx, targetID)
		if err != nil {
			return fmt.Errorf("list repositories for configuration file synchronization: %w", err)
		}
		ids := make([]string, 0, len(repositories))
		for _, repository := range repositories {
			ids = append(ids, repository.ID)
		}
		return bot.ExclusiveRepositories(ctx, s.pendingCICoordinator, ids, operation)
	})
}
