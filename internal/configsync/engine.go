package configsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type ConnectionStore interface {
	GetTarget(context.Context, string) (storage.Target, error)
	GetRepository(context.Context, string, string) (storage.Repository, error)
	ListRepositories(context.Context, string) ([]storage.Repository, error)
	ListSyncConfigs(context.Context, string) ([]orgsync.Config, error)
	GetSyncRepositoryOverride(context.Context, string, string, orgsync.Kind) (orgsync.RepositoryOverride, error)
	GetConfigFileState(context.Context, string, string) (storage.ConfigFileState, error)
	SaveConfigFileState(context.Context, storage.ConfigFileStateChange) (storage.ConfigFileState, error)
	SaveInstallationSettings(context.Context, storage.SaveInstallationSettingsRequest) (storage.SaveInstallationSettingsResult, error)
}

type Engine struct {
	Store       ConnectionStore
	QuietPeriod time.Duration
}

// Run must execute inside the same catalog/repository exclusion used by panel
// saves. Storage still verifies every revision, including all Sync kinds. Each
// imported change is followed by a fresh read before any GitHub publication.
func (engine Engine) Run(ctx context.Context, client *github.Client, targetID, repositoryID string) (Connection, error) {
	for attempt := 0; attempt < 3; attempt++ {
		connection, imported, err := engine.runOnce(ctx, client, targetID, repositoryID)
		if err != nil || !imported {
			return connection, err
		}
	}
	return Connection{}, storage.ErrConflict
}

func (engine Engine) Snapshot(ctx context.Context, targetID, repositoryID string) (PanelSnapshot, error) {
	target, err := engine.Store.GetTarget(ctx, targetID)
	if err != nil {
		return PanelSnapshot{}, err
	}
	snapshot := PanelSnapshot{Target: target}
	if repositoryID == "" {
		snapshot.SyncConfigs, err = engine.Store.ListSyncConfigs(ctx, targetID)
		return snapshot, err
	}
	repository, err := engine.Store.GetRepository(ctx, targetID, repositoryID)
	if err != nil {
		return snapshot, err
	}
	snapshot.Repository = &repository
	for _, kind := range orgsync.Kinds() {
		item, err := engine.Store.GetSyncRepositoryOverride(ctx, targetID, repositoryID, kind)
		if errors.Is(err, storage.ErrNotFound) {
			continue
		}
		if err != nil {
			return snapshot, err
		}
		snapshot.SyncOverrides = append(snapshot.SyncOverrides, item)
	}
	return snapshot, nil
}

func (engine Engine) location(ctx context.Context, client *github.Client, snapshot PanelSnapshot) (RemoteLocation, error) {
	repository := snapshot.Repository
	if repository == nil {
		repositories, err := engine.Store.ListRepositories(ctx, snapshot.Target.ID)
		if err != nil {
			return RemoteLocation{}, err
		}
		for _, candidate := range repositories {
			if candidate.Name == ".github" && candidate.Available {
				repository = &candidate
				break
			}
		}
		if repository == nil {
			return RemoteLocation{}, &BlockedError{Code: "workspace_repository_missing", Message: "Give Smyklot access to the .github repository to sync workspace settings"}
		}
	}
	id, err := storage.ParseRepositoryID(repository.ID)
	if err != nil {
		return RemoteLocation{}, err
	}
	live, err := client.GetRepositoryByID(ctx, id)
	if err != nil {
		return RemoteLocation{}, err
	}
	if live.Owner == "" || live.Name == "" || strings.ContainsAny(live.Owner+live.Name, "/\\") || live.FullName != live.Owner+"/"+live.Name {
		return RemoteLocation{}, errors.New("GitHub returned an invalid repository name")
	}
	if snapshot.Repository == nil && live.Name != ".github" {
		return RemoteLocation{}, &BlockedError{Code: "workspace_repository_renamed", Message: "Refresh repository access to find the current .github repository before syncing workspace settings"}
	}
	if live.DefaultBranch == "" {
		return RemoteLocation{}, &BlockedError{Code: "missing_branch", Message: "The default branch is unavailable"}
	}
	return RemoteLocation{RepositoryID: id, Owner: live.Owner, Repository: live.Name, DefaultBranch: live.DefaultBranch, Scope: snapshot.Scope()}, nil
}

func connectionEnabled(snapshot PanelSnapshot) bool {
	if !snapshot.Target.Available {
		return false
	}
	if snapshot.Repository == nil {
		return snapshot.Target.ConfigFileSyncEnabled
	}
	return snapshot.Repository.Available && snapshot.Repository.ConfigFileSyncEnabled
}

func (engine Engine) runOnce(ctx context.Context, client *github.Client, targetID, repositoryID string) (Connection, bool, error) {
	snapshot, err := engine.Snapshot(ctx, targetID, repositoryID)
	if err != nil {
		return Connection{}, false, err
	}
	if !connectionEnabled(snapshot) {
		return Connection{Version: 1, Status: StatusOff}, false, nil
	}
	stored, err := engine.Store.GetConfigFileState(ctx, targetID, repositoryID)
	if err != nil {
		return Connection{}, false, err
	}
	connection, err := DecodeConnection(stored, snapshot.Scope())
	if err != nil {
		return connection, false, err
	}
	if snapshot.Repository != nil && snapshot.Repository.IgnoreRepositoryFile {
		return engine.block(ctx, snapshot, stored, connection,
			&BlockedError{Code: "file_disabled", Message: "Turn on file settings before syncing changes in both directions"})
	}
	file, err := engine.readForReconciliation(ctx, client, snapshot)
	if errors.Is(err, storage.ErrConflict) {
		return connection, true, nil
	}
	if validObjectID(file.Head) {
		connection.Head, connection.Path = file.Head, file.Path
	}
	if err != nil {
		return engine.block(ctx, snapshot, stored, connection, err)
	}
	panel, err := snapshot.JSON()
	if err != nil {
		return connection, false, err
	}
	input, err := connection.input(panel, file.Source)
	if err != nil {
		return engine.block(ctx, snapshot, stored, connection, err)
	}
	decision, err := Reconcile(input)
	if err != nil {
		return engine.block(ctx, snapshot, stored, connection, invalidSettings(err))
	}
	if decision.Problem == ProblemStaleResolution && input.File.Exists {
		// A stale choice has no authority over new edits. If the conflict itself
		// disappeared, ordinary reconciliation can proceed without that choice.
		// Missing files still require an explicit review before recreation.
		fresh := input
		fresh.Resolution = nil
		merged, err := Reconcile(fresh)
		if err != nil {
			return engine.block(ctx, snapshot, stored, connection, invalidSettings(err))
		}
		if merged.Problem == "" {
			input, decision, connection.Resolution = fresh, merged, nil
		}
	}
	connection.Head, connection.Path, connection.Comparison = file.Head, file.Path, decision.Comparison
	connection.Problem, connection.Message, connection.ConflictCount, connection.ConflictPaths = "", "", 0, nil
	if decision.Problem != "" {
		connection.Status, connection.Problem = StatusBlocked, string(decision.Problem)
		connection.ConflictCount = len(decision.Conflicts)
		connection.ConflictPaths = conflictSummary(decision.Conflicts)
		return engine.save(ctx, snapshot, stored, connection)
	}
	return engine.apply(ctx, client, snapshot, stored, connection, file, input, decision)
}

func (engine Engine) readForReconciliation(ctx context.Context, client *github.Client, snapshot PanelSnapshot) (RemoteFile, error) {
	location, err := engine.location(ctx, client, snapshot)
	if err != nil {
		return RemoteFile{}, err
	}
	file, err := ReadRemoteFile(ctx, client, location)
	if err != nil {
		return file, err
	}
	current, err := engine.location(ctx, client, snapshot)
	if err != nil {
		return file, err
	}
	// Names can be reused by another repository during the immutable file read.
	// Discard that observation before importing settings or advancing state.
	if current != location {
		return file, storage.ErrConflict
	}
	return file, nil
}

func (engine Engine) save(ctx context.Context, snapshot PanelSnapshot, stored storage.ConfigFileState, connection Connection) (Connection, bool, error) {
	change, err := connection.change(snapshot, stored, time.Now().UTC())
	if err == nil {
		_, err = engine.Store.SaveConfigFileState(ctx, change)
	}
	return connection, false, err
}

func (engine Engine) block(ctx context.Context, snapshot PanelSnapshot, stored storage.ConfigFileState, connection Connection, err error) (Connection, bool, error) {
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		return connection, false, fmt.Errorf("reconcile configuration file: %w", err)
	}
	connection.Status, connection.Problem, connection.Message = StatusBlocked, blocked.Code, blocked.Message
	connection.Comparison, connection.ConflictCount, connection.ConflictPaths = "", 0, nil
	return engine.save(ctx, snapshot, stored, connection)
}
