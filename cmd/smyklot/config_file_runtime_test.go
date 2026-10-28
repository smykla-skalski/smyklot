package main

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/configsync"
	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestConfigFileMaintenanceUsesOptInSeparatelyFromBot(t *testing.T) {
	t.Parallel()
	targetID := storage.InstallationID(1)
	store := maintenanceCatalogStore{
		target: storage.Target{
			ID: targetID, InstallationID: "1", Available: true, ConfigFileSyncEnabled: true,
		},
		repositories: []storage.Repository{
			{ID: storage.RepositoryID(11), FullName: "owner/connected", Available: true, ConfigFileSyncEnabled: true},
			{ID: storage.RepositoryID(12), FullName: "owner/disconnected", Available: true},
			{ID: storage.RepositoryID(13), FullName: "owner/removed", ConfigFileSyncEnabled: true},
		},
	}
	service := &server{store: store}
	jobs, err := service.durableMaintenanceJobs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var scopes []string
	for _, job := range jobs {
		if job.work.kind != workqueue.KindConfigFileSync {
			continue
		}
		if job.work.targetID == nil || *job.work.targetID != targetID {
			t.Fatalf("configuration job has wrong workspace: %#v", job.work)
		}
		scope := "workspace"
		if job.work.repositoryID != nil {
			scope = *job.work.repositoryID
		}
		scopes = append(scopes, scope)
	}
	if len(scopes) != 2 || scopes[0] != "workspace" || scopes[1] != storage.RepositoryID(11) {
		t.Fatalf("configuration scopes = %v, want workspace and connected repository", scopes)
	}

	// A connected repository has one writer, even when commands are enabled.
	store.target.RepositoryDefaultEnabled = true
	service.store = store
	jobs, err = service.durableMaintenanceJobs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if job.work.kind == workqueue.KindConfigMigration && job.work.repositoryID != nil &&
			*job.work.repositoryID == storage.RepositoryID(11) {
			t.Fatal("connected repository also published a competing migration writer")
		}
	}
}

func TestConfigFileWorkRechecksOptInBeforeGitHub(t *testing.T) {
	t.Parallel()
	targetID, repositoryID := storage.InstallationID(1), storage.RepositoryID(11)
	store := maintenanceCatalogStore{
		target:       storage.Target{ID: targetID, InstallationID: "1", Available: true},
		repositories: []storage.Repository{{ID: repositoryID, Available: true}},
	}
	service := &server{store: store}
	for _, scope := range []string{"", repositoryID, "removed-repository"} {
		// No credential provider: touching GitHub would panic instead of silently passing.
		summary, err := service.configurationFileMaintenanceJob(t.Context(), targetID, scope, 1).runWithSummary(workqueue.Item{})
		if err != nil || summary != "Configuration file sync is off" {
			t.Fatalf("disabled %q returned %q, %v", scope, summary, err)
		}
	}

	store.target.Available = false
	store.target.ConfigFileSyncEnabled = true
	store.repositories[0].ConfigFileSyncEnabled = true
	service.store = store
	for _, scope := range []string{"", repositoryID} {
		if _, err := service.reconcileQueuedConfigurationFile(t.Context(), targetID, scope, 1); err != nil {
			t.Fatalf("removed installation returned %v", err)
		}
	}
}

type configurationScopeCoordinator struct {
	keys []string
	stop string
}

func (coordinator *configurationScopeCoordinator) Exclusive(_ context.Context, key string, operation func() error) error {
	coordinator.keys = append(coordinator.keys, key)
	if key == coordinator.stop {
		return context.Canceled
	}
	return operation()
}

func TestConfigFileImportUsesPanelSaveExclusionOrder(t *testing.T) {
	t.Parallel()
	store := maintenanceCatalogStore{repositories: []storage.Repository{
		{ID: "repo-b"}, {ID: "repo-a"}, {ID: "repo-a"},
	}}
	coordinator := &configurationScopeCoordinator{}
	service := &server{store: store, pendingCICoordinator: coordinator}
	called := false
	operation := func() error { called = true; return nil }
	if err := service.withConfigurationFileScope(t.Context(), "target", "", operation); err != nil {
		t.Fatal(err)
	}
	if !called || !slices.Equal(coordinator.keys, []string{bot.CatalogCoordinatorKey, "repo-a", "repo-b"}) {
		t.Fatalf("workspace exclusion = %v, operation ran = %t", coordinator.keys, called)
	}
	coordinator.keys = nil
	called = false
	if err := service.withConfigurationFileScope(t.Context(), "target", "repo-b", operation); err != nil {
		t.Fatal(err)
	}
	if !called || !slices.Equal(coordinator.keys, []string{"repo-b"}) {
		t.Fatalf("repository exclusion = %v, operation ran = %t", coordinator.keys, called)
	}
	coordinator.stop = "repo-b"
	called = false
	if err := service.withConfigurationFileScope(t.Context(), "target", "", operation); !errors.Is(err, context.Canceled) || called {
		t.Fatalf("canceled exclusion returned %v, operation ran = %t", err, called)
	}
}

func TestConnectedFileSupersedesLegacyMigrationBeforeReads(t *testing.T) {
	t.Parallel()
	targetID, repositoryID := storage.InstallationID(1), storage.RepositoryID(11)
	store := maintenanceCatalogStore{
		target: storage.Target{
			ID: targetID, InstallationID: "1", Available: true, RepositoryDefaultEnabled: true,
		},
		repositories: []storage.Repository{{ID: repositoryID, Available: true, ConfigFileSyncEnabled: true}},
	}
	service := &server{store: store, panel: &adminpanel.Server{}}
	repository := github.Repository{ID: 11, Owner: "owner", Name: "connected"}
	for _, job := range service.repositoryMaintenanceJobs(t.Context(), targetID, 1, repository, true, nil) {
		if job.work.kind == workqueue.KindConfigMigration {
			if err := job.run(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := service.migrateRepositoryConfig(t.Context(), nil, targetID, repository); err != nil {
		t.Fatal(err)
	}
	if err := service.proposeConfigMigration(t.Context(), nil, targetID, repository, repositoryConfigFile{}); err != nil {
		t.Fatal(err)
	}
}

type configurationRuntimeStore struct {
	maintenanceCatalogStore
	connection storage.ConfigFileState
}

func (store configurationRuntimeStore) GetConfigFileState(context.Context, string, string) (storage.ConfigFileState, error) {
	return store.connection, nil
}

func (configurationRuntimeStore) UpdateRepositoryFileState(
	context.Context, storage.RepositoryFileState,
) (bool, error) {
	return false, nil
}

func TestConnectedFileUsesReconciledSettingsAndPreservesRunner(t *testing.T) {
	t.Parallel()
	for _, connected := range []bool{false, true} {
		t.Run(map[bool]string{false: "file precedence", true: "reconciled precedence"}[connected], func(t *testing.T) {
			targetID, repositoryID := storage.InstallationID(1), storage.RepositoryID(11)
			prefix, runner := "/unreconciled ", config.RunnerAction
			file := repositoryConfigFile{
				status: storage.RepositoryFileValid,
				patch:  config.Patch{CommandPrefix: &prefix, Runner: &runner},
			}
			store := configurationRuntimeStore{maintenanceCatalogStore: maintenanceCatalogStore{
				target: storage.Target{ID: targetID, Available: true},
				repositories: []storage.Repository{{
					ID: repositoryID, TargetID: targetID, Available: true, ConfigFileSyncEnabled: connected,
				}},
			}}
			baseline, err := (configsync.PanelSnapshot{Target: store.target, Repository: &store.repositories[0]}).JSON()
			if err != nil {
				t.Fatal(err)
			}
			store.connection.Document, err = json.Marshal(configsync.Connection{
				Version: 1, Status: configsync.StatusReady,
				Base: configsync.Snapshot{Exists: true, Document: baseline},
			})
			if err != nil {
				t.Fatal(err)
			}
			service := &server{
				store: store, panel: &adminpanel.Server{}, runtimeBotConfig: config.Default(),
				configs: newRepoCache(time.Minute, func(context.Context, *github.Client, string, string, *repositoryConfigFile) (repositoryConfigFile, error) {
					return file, nil
				}),
			}
			resolved, err := service.serviceConfigWithoutCatalogRefresh(t.Context(), nil, targetID, repositoryID, "owner", "repo")
			if err != nil {
				t.Fatal(err)
			}
			wantPrefix := prefix
			if connected {
				wantPrefix = config.Default().CommandPrefix
			}
			if resolved.CommandPrefix != wantPrefix || resolved.Runner != config.RunnerAction {
				t.Fatalf("resolved prefix/runner = %q/%q, want %q/%q", resolved.CommandPrefix, resolved.Runner, wantPrefix, config.RunnerAction)
			}
		})
	}
}

func TestConfigFileInitializationCannotRelaxCommands(t *testing.T) {
	t.Parallel()
	for _, reconnect := range []bool{false, true} {
		t.Run(map[bool]string{false: "first activation", true: "reconnection"}[reconnect], func(t *testing.T) {
			testConfigFileInitialization(t, reconnect)
		})
	}
}

func TestConfigFileInitializationPreservesPriorPrecedence(t *testing.T) {
	t.Parallel()
	fileCommands := []string{"approve"}
	panelPrefix := "/panel "
	for _, connected := range []bool{false, true} {
		t.Run(map[bool]string{false: "before activation", true: "activation pending"}[connected], func(t *testing.T) {
			store := configurationRuntimeStore{maintenanceCatalogStore: maintenanceCatalogStore{
				target: storage.Target{ID: "target", Available: true},
				repositories: []storage.Repository{{
					ID: "repo", TargetID: "target", Available: true, ConfigFileSyncEnabled: connected,
					ConfigPatch: config.Patch{CommandPrefix: &panelPrefix},
				}},
			}}
			store.connection.InitializationRequired = true
			service := &server{
				store: store, panel: &adminpanel.Server{}, runtimeBotConfig: config.Default(),
				configs: newRepoCache(time.Minute, func(context.Context, *github.Client, string, string, *repositoryConfigFile) (repositoryConfigFile, error) {
					return repositoryConfigFile{status: storage.RepositoryFileValid, patch: config.Patch{AllowedCommands: &fileCommands}}, nil
				}),
			}
			resolved, err := service.serviceConfigWithoutCatalogRefresh(t.Context(), nil, "target", "repo", "owner", "repo")
			if err != nil || resolved == nil || resolved.CommandPrefix != panelPrefix || !slices.Equal(resolved.AllowedCommands, fileCommands) {
				t.Fatalf("configuration changed on activation: %#v, %v", resolved, err)
			}
		})
	}
}

type missingConfigCase struct {
	name       string
	refreshed  repositoryConfigFile
	loadErr    error
	priorBase  bool
	wantActive bool
}

func TestConfigFileInitializationWithMissingFile(t *testing.T) {
	t.Parallel()
	for _, test := range []missingConfigCase{
		{name: "missing file keeps commands available", refreshed: repositoryConfigFile{status: storage.RepositoryFileMissing}, wantActive: true},
		{name: "new file keeps commands available", refreshed: repositoryConfigFile{status: storage.RepositoryFileValid}, wantActive: true},
		{name: "unreadable file fails closed", loadErr: errors.New("unavailable")},
		{name: "invalid file fails closed", refreshed: repositoryConfigFile{status: storage.RepositoryFileInvalid, err: errors.New("invalid")}},
		{name: "removed file after reconnect keeps commands available", refreshed: repositoryConfigFile{status: storage.RepositoryFileMissing}, priorBase: true, wantActive: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			testMissingConfigInitialization(t, test)
		})
	}
}

func testMissingConfigInitialization(t *testing.T, test missingConfigCase) {
	t.Helper()
	store := configurationRuntimeStore{maintenanceCatalogStore: maintenanceCatalogStore{
		target:       storage.Target{ID: "target", Available: true},
		repositories: []storage.Repository{{ID: "repo", TargetID: "target", Available: true, ConfigFileSyncEnabled: true}},
	}}
	store.connection.InitializationRequired = true
	if test.priorBase {
		allowed := []string{"approve"}
		store.repositories[0].ConfigPatch.AllowedCommands = &allowed
		baseline, err := (configsync.PanelSnapshot{Target: store.target, Repository: &store.repositories[0]}).JSON()
		if err != nil {
			t.Fatal(err)
		}
		store.connection.Document, err = json.Marshal(configsync.Connection{
			Version: 1, Status: configsync.StatusReady,
			Base: configsync.Snapshot{Exists: true, Document: baseline},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	reads := 0
	service := &server{
		store: store, panel: &adminpanel.Server{}, runtimeBotConfig: config.Default(),
		configs: newRepoCache(time.Minute, func(_ context.Context, _ *github.Client, _, _ string, _ *repositoryConfigFile) (repositoryConfigFile, error) {
			reads++
			if reads == 1 {
				return repositoryConfigFile{status: storage.RepositoryFileMissing}, nil
			}
			return test.refreshed, test.loadErr
		}),
	}
	resolved, err := service.serviceConfigWithoutCatalogRefresh(t.Context(), nil, "target", "repo", "owner", "repo")
	if test.wantActive {
		if err != nil || resolved == nil || resolved.CommandPrefix != config.Default().CommandPrefix {
			t.Fatalf("missing file blocked commands: %#v, %v", resolved, err)
		}
		if test.priorBase && !slices.Equal(resolved.AllowedCommands, []string{"approve"}) {
			t.Fatalf("reconnection lost saved command restrictions: %v", resolved.AllowedCommands)
		}
	} else if err == nil || resolved != nil {
		t.Fatalf("unsafe initialization activated settings: %#v, %v", resolved, err)
	}
	if reads != 2 {
		t.Fatalf("configuration reads = %d, want fresh verification of missing file", reads)
	}
}

func testConfigFileInitialization(t *testing.T, reconnect bool) {
	t.Helper()
	allowed := []string{"approve"}
	store := configurationRuntimeStore{maintenanceCatalogStore: maintenanceCatalogStore{
		target:       storage.Target{ID: "target", Available: true},
		repositories: []storage.Repository{{ID: "repo", TargetID: "target", Available: true, ConfigFileSyncEnabled: true}},
	}}
	store.connection.InitializationRequired = true
	if reconnect {
		baseline, err := (configsync.PanelSnapshot{Target: store.target, Repository: &store.repositories[0]}).JSON()
		if err != nil {
			t.Fatal(err)
		}
		store.connection.Document, err = json.Marshal(configsync.Connection{
			Version: 1, Status: configsync.StatusReady,
			Base: configsync.Snapshot{Exists: true, Document: baseline},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	service := &server{
		store: store, panel: &adminpanel.Server{}, runtimeBotConfig: config.Default(),
		configs: newRepoCache(time.Minute, func(context.Context, *github.Client, string, string, *repositoryConfigFile) (repositoryConfigFile, error) {
			return repositoryConfigFile{status: storage.RepositoryFileValid, patch: config.Patch{AllowedCommands: &allowed}}, nil
		}),
	}
	resolved, err := service.serviceConfigWithoutCatalogRefresh(t.Context(), nil, "target", "repo", "owner", "repo")
	if err != nil || resolved == nil || !slices.Equal(resolved.AllowedCommands, allowed) {
		t.Fatalf("uninitialized connection lost file restrictions: %#v, %v", resolved, err)
	}
}
