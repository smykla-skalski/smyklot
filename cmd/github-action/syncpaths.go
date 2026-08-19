package main

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

// pathIndexTTL is how long a repository's path list is believed.
//
// A day, because what it answers is "which paths exist to configure", and the
// cost of being a day out of date is a finder that does not yet offer a file
// somebody added this morning - which it says out loud, since a path it does
// not know is still a path it will accept. The cost of being fresher is one
// GitHub request per repository per tick, forever, for a list nobody reads
// between visits to the panel.
const pathIndexTTL = 24 * time.Hour

// pathIndexCap bounds what one repository contributes.
//
// A repository with a hundred thousand files is one whose paths nobody scans
// for a template anyway, and the whole list is shipped to a browser. The cap is
// said in the answer rather than hidden: the finder reports how many paths it
// knows, so a reader can see it is not everything.
const pathIndexCap = 5000

// refreshSyncPaths keeps the panel's path finder answering with what exists.
//
// Typing a path into an empty box is guessing: somebody is asked for a string
// that has to match, character for character, something they cannot see. This
// is where "what exists" comes from - one tree listing per repository, at most
// once a day.
//
// Never fatal. It feeds a control that helps somebody type; a reconcile that
// failed because a tree could not be read would stop the sync it is beside for
// the sake of an autocomplete.
func (s *server) refreshSyncPaths(
	ctx context.Context,
	client *github.Client,
	targetID string,
) {
	repositories, err := s.store.ListRepositories(ctx, targetID)
	if err != nil {
		logging.From(ctx).Warn("could not read repositories for the path index", "error", err)

		return
	}

	stored, err := s.store.ListSyncRepositoryPaths(ctx, targetID)
	if err != nil {
		logging.From(ctx).Warn("could not read the path index", "error", err)

		return
	}

	observed := make(map[string]time.Time, len(stored))
	for _, row := range stored {
		observed[row.RepositoryID] = row.ObservedAt
	}

	now := time.Now().UTC()
	for _, repository := range repositories {
		// Only what sync watches. A repository the installation does not
		// synchronize contributes paths nobody can configure a file at.
		if !repository.Available {
			continue
		}
		if at, known := observed[repository.ID]; known && now.Sub(at) < pathIndexTTL {
			continue
		}

		paths, err := repositoryPaths(ctx, client, repository.FullName, repository.DefaultBranch)
		if err != nil {
			logging.From(ctx).Warn("could not read a repository's paths",
				"repo", repository.FullName, "error", err)

			continue
		}

		if err := s.store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
			RepositoryID: repository.ID,
			TargetID:     targetID,
			Paths:        paths,
			ObservedAt:   now,
		}); err != nil {
			logging.From(ctx).Warn("could not store a repository's paths",
				"repo", repository.FullName, "error", err)
		}
	}
}

// repositoryPaths reads every ordinary file one repository holds.
//
// Ordinary files only: a directory is not something a template can be written
// at, and a symbolic link or a submodule is a path that cannot be written to
// without destroying what is there.
func repositoryPaths(
	ctx context.Context,
	client *github.Client,
	fullName, branch string,
) ([]string, error) {
	owner, repo, ok := strings.Cut(fullName, "/")
	if !ok {
		return nil, fmt.Errorf("%w: repository name %q has no owner", ErrInvalidInput, fullName)
	}

	tree, err := client.ListRepositoryTree(ctx, owner, repo, branch)
	if err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(tree.Entries))
	for path, entry := range tree.Entries {
		if entry.OrdinaryFile() {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	if len(paths) > pathIndexCap {
		paths = slices.Clip(paths[:pathIndexCap])
	}

	return paths, nil
}
