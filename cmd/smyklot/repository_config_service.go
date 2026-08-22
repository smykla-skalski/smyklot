package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
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

	// superseded are the other paths that also hold a configuration file. They
	// change nothing about how a comment is answered and are reported to the
	// repository, which is the point of reading them at all.
	superseded []string

	// fingerprint identifies everything the file could have been read from when
	// it was read. The file is looked for at four paths, and re-probing them
	// every time the entry ages out would cost four requests per repository per
	// tick; comparing this answers all four in one.
	fingerprint string
}

// fetchRepositoryConfig reads a repository's own configuration file.
//
// previous is what the cache already holds, and the fingerprint is what decides
// whether it can be handed straight back. That question costs one request and
// answers all four candidate paths at once, which is what keeps looking in four
// places from costing four times as much as looking in one.
//
// The fingerprint is read before the file, so a commit landing during the read
// is noticed on the next tick rather than mistaken for the state this answer
// describes.
func fetchRepositoryConfig(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	previous *repositoryConfigFile,
) (repositoryConfigFile, error) {
	// The same preferred path goes into both, so what is watched cannot drift
	// from what is searched
	const preferred = ""

	fingerprint, err := repoConfigFingerprint(ctx, client, owner, repository, preferred)
	if err != nil {
		return repositoryConfigFile{}, bot.NewConfigError(bot.ErrConfigLoad, err)
	}

	// An empty fingerprint is "could not tell", and never compares equal, so a
	// repository Smyklot could not read is re-read rather than assumed.
	if previous != nil && fingerprint != "" && previous.fingerprint == fingerprint {
		return *previous, nil
	}

	found, err := findRepoConfig(ctx, client, owner, repository, preferred)
	if err != nil {
		return repositoryConfigFile{}, bot.NewConfigError(bot.ErrConfigLoad, err)
	}
	if !found.Found() {
		return repositoryConfigFile{
			status: storage.RepositoryFileMissing, fingerprint: fingerprint,
		}, nil
	}

	return repositoryConfigFileFrom(found, fingerprint), nil
}

func repositoryConfigFileFrom(found foundRepoConfig, fingerprint string) repositoryConfigFile {
	if len(bytes.TrimSpace(found.Content)) == 0 {
		return repositoryConfigFile{
			status:      storage.RepositoryFileValid,
			path:        found.Path,
			superseded:  found.Superseded,
			fingerprint: fingerprint,
		}
	}

	patch, err := parseRepositoryConfig(found)
	if err != nil {
		return repositoryConfigFile{
			status:      storage.RepositoryFileInvalid,
			err:         bot.NewConfigError(bot.ErrRepoConfigInvalid, err),
			path:        found.Path,
			superseded:  found.Superseded,
			fingerprint: fingerprint,
		}
	}

	return repositoryConfigFile{
		patch:       patch,
		status:      storage.RepositoryFileValid,
		path:        found.Path,
		superseded:  found.Superseded,
		fingerprint: fingerprint,
	}
}

// parseRepositoryConfig reads a found file in whichever format its name says.
func parseRepositoryConfig(found foundRepoConfig) (config.Patch, error) {
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
		storage.InstallationID(event.Installation.ID),
		storage.RepositoryID(event.Repository.ID),
	)
	if err != nil {
		return false, err
	}
	if !target.Available || !repository.Available {
		return false, nil
	}

	return storage.RepositoryEnabled(target, repository), nil
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
	return s.serviceConfigWithControls(
		ctx, client, targetID, repositoryID, owner, repositoryName,
		s.repositoryControls,
	)
}

func (s *server) serviceConfigWithoutCatalogRefresh(
	ctx context.Context,
	client *github.Client,
	targetID, repositoryID, owner, repositoryName string,
) (*config.Config, error) {
	return s.serviceConfigWithControls(
		ctx, client, targetID, repositoryID, owner, repositoryName,
		s.readRepositoryControls,
	)
}

func (s *server) serviceConfigWithControls(
	ctx context.Context,
	client *github.Client,
	targetID, repositoryID, owner, repositoryName string,
	controls func(context.Context, string, string) (storage.Target, storage.Repository, error),
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

	target, repository, err := controls(ctx, targetID, repositoryID)
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
		Path:         file.path,
		Superseded:   file.superseded,
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
