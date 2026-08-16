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

	// path is the file this was read from, empty when the repository has none.
	path string

	// head is the default branch's head when this was read. The file is looked
	// for at four paths, and re-reading them every time the entry ages out
	// would cost four requests per repository per tick; the head answers all
	// four in one, because none of them can have changed while it has not.
	head string
}

func fetchRepositoryConfig(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
) (repositoryConfigFile, error) {
	// Read before the file, so a commit landing during the read is noticed on
	// the next tick rather than mistaken for the state this answer describes.
	head, err := client.DefaultBranchHead(ctx, owner, repository)
	if err != nil {
		return repositoryConfigFile{}, NewConfigError(ErrConfigLoad, err)
	}

	found, err := client.FindRepoConfig(ctx, owner, repository, "")
	if err != nil {
		return repositoryConfigFile{}, NewConfigError(ErrConfigLoad, err)
	}
	if !found.Found() {
		return repositoryConfigFile{status: storage.RepositoryFileMissing, head: head}, nil
	}
	if len(bytes.TrimSpace(found.Content)) == 0 {
		return repositoryConfigFile{
			status: storage.RepositoryFileValid, path: found.Path, head: head,
		}, nil
	}

	patch, err := parseRepositoryConfig(found)
	if err != nil {
		return repositoryConfigFile{
			status: storage.RepositoryFileInvalid,
			err:    NewConfigError(ErrRepoConfigInvalid, err),
			path:   found.Path,
			head:   head,
		}, nil
	}

	return repositoryConfigFile{
		patch: patch, status: storage.RepositoryFileValid, path: found.Path, head: head,
	}, nil
}

// repositoryConfigUnchanged reports that a cached answer can be kept because
// the default branch has not moved since it was read.
//
// A repository whose head could not be read, or which has none, is re-read:
// keeping an answer requires proof it is still true, and no answer is not proof.
func repositoryConfigUnchanged(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	previous repositoryConfigFile,
) (bool, error) {
	if previous.head == "" {
		return false, nil
	}

	head, err := client.DefaultBranchHead(ctx, owner, repository)
	if err != nil {
		return false, NewConfigError(ErrConfigLoad, err)
	}

	return head != "" && head == previous.head, nil
}

// parseRepositoryConfig reads a found file in whichever format its name says.
func parseRepositoryConfig(found github.RepoConfig) (config.Patch, error) {
	format, err := config.FormatOf(found.Path)
	if err != nil {
		return config.Patch{}, err
	}

	return config.ParsePatch(format, found.Content)
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
	file, err := s.configs.GetByKey(
		ctx, client, repositoryID, owner, repositoryName,
	)
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
