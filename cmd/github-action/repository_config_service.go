package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type repositoryConfigFile struct {
	patch  config.Patch
	status storage.RepositoryFileStatus
	err    error
}

func fetchRepositoryConfig(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
) (repositoryConfigFile, error) {
	content, err := client.GetRepoConfig(ctx, owner, repository)
	if err != nil {
		return repositoryConfigFile{}, NewConfigError(ErrConfigLoad, err)
	}
	if content == nil {
		return repositoryConfigFile{status: storage.RepositoryFileMissing}, nil
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return repositoryConfigFile{status: storage.RepositoryFileValid}, nil
	}
	patch, err := config.ParsePatch(content)
	if err != nil {
		return repositoryConfigFile{
			status: storage.RepositoryFileInvalid,
			err:    NewConfigError(ErrRepoConfigInvalid, err),
		}, nil
	}

	return repositoryConfigFile{patch: patch, status: storage.RepositoryFileValid}, nil
}

func (s *server) repositoryEnabled(
	ctx context.Context,
	event *webhook.IssueCommentEvent,
) (bool, error) {
	target, repository, err := s.repositoryControls(
		ctx,
		installationStorageID(event.Installation.ID),
		repositoryStorageID(event.Repository.ID),
	)
	if err != nil {
		return false, err
	}
	if !target.Available || !repository.Available {
		return false, nil
	}
	if repository.EnabledOverride != nil {
		return *repository.EnabledOverride, nil
	}

	return target.RepositoryDefaultEnabled, nil
}

func (s *server) repositoryControls(
	ctx context.Context,
	targetID, repositoryID string,
) (storage.Target, storage.Repository, error) {
	target, repository, err := s.readRepositoryControls(ctx, targetID, repositoryID)
	if !repositoryControlsNeedRefresh(target, repository, err) {
		return target, repository, err
	}

	return s.refreshRepositoryControls(ctx, targetID, repositoryID)
}

func (s *server) refreshRepositoryControls(
	ctx context.Context,
	targetID, repositoryID string,
) (storage.Target, storage.Repository, error) {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()

	target, repository, err := s.readRepositoryControls(ctx, targetID, repositoryID)
	if !repositoryControlsNeedRefresh(target, repository, err) {
		return target, repository, err
	}
	if _, syncErr := s.syncPanelCatalogLocked(ctx); syncErr != nil {
		return storage.Target{}, storage.Repository{}, syncErr
	}

	return s.readRepositoryControls(ctx, targetID, repositoryID)
}

func repositoryControlsNeedRefresh(
	target storage.Target,
	repository storage.Repository,
	err error,
) bool {
	return errors.Is(err, storage.ErrNotFound) ||
		(err == nil && (!target.Available || !repository.Available))
}

func (s *server) readRepositoryControls(
	ctx context.Context,
	targetID, repositoryID string,
) (storage.Target, storage.Repository, error) {
	target, err := s.store.GetTarget(ctx, targetID)
	if err != nil {
		return storage.Target{}, storage.Repository{}, err
	}
	repository, err := s.store.GetRepository(ctx, targetID, repositoryID)
	if err != nil {
		return storage.Target{}, storage.Repository{}, err
	}

	return target, repository, nil
}

func (s *server) serviceConfig(
	ctx context.Context,
	client *github.Client,
	targetID, repositoryID, owner, repositoryName string,
) (*config.Config, error) {
	file, err := s.configs.Get(ctx, client, owner, repositoryName)
	if err != nil {
		return nil, err
	}
	if s.panel == nil {
		if file.err != nil {
			return nil, file.err
		}

		return config.ApplyPatch(s.botConfig(), file.patch), nil
	}

	target, repository, err := s.repositoryControls(ctx, targetID, repositoryID)
	if err != nil {
		return nil, err
	}
	var fileError *string
	if file.err != nil {
		reason := s.redactor.Error(file.err)
		fileError = &reason
	}
	fileStateChanged, err := s.store.UpdateRepositoryFileState(ctx, storage.RepositoryFileState{
		TargetID:     targetID,
		RepositoryID: repositoryID,
		Status:       file.status,
		Patch:        file.patch,
		Error:        fileError,
		ObservedAt:   time.Now().UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("persist repository configuration state: %w", err)
	}
	if fileStateChanged && s.panel != nil {
		s.panel.Announce(targetID, repositoryID)
	}
	if file.err != nil && !repository.IgnoreRepositoryFile {
		return nil, file.err
	}

	layers := []config.Layer{{Source: config.SourceTarget, Patch: target.ConfigPatch}}
	if !repository.IgnoreRepositoryFile {
		layers = append(layers, config.Layer{
			Source: config.SourceRepositoryFile,
			Patch:  file.patch,
		})
	}
	layers = append(layers, config.Layer{
		Source: config.SourceRepositoryPanel,
		Patch:  repository.ConfigPatch,
	})
	resolved := config.Resolve(s.botConfig(), layers...)

	return &resolved.Values, nil
}
