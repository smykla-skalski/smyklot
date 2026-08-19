package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

// There is no cap on what one repository contributes.
//
// There was one, at five thousand paths, and it threw away 84% of a repository
// the size of kubernetes without saying so - a finder that quietly does not
// know about a file is worse than a slow one, because somebody types the path
// it will not offer and has no way to tell whether they got it wrong.
//
// The reason it was affordable to drop is gone: the list is read only where the
// branch has moved, and the browser matches 50,000 paths in 33-64ms.
//
// GitHub's own refusal to list a very large tree in one answer is divided
// around rather than accepted - see ListWholeRepositoryTree. What survives that
// is recorded on the row and said in the panel rather than left to look like a
// repository holding fewer files than it does.

// refreshSyncPaths keeps the panel's path finder answering with what exists.
//
// Typing a path into an empty box is guessing: somebody is asked for a string
// that has to match, character for character, something they cannot see. This
// is where "what exists" comes from.
//
// A tick costs one small read per repository and nothing else. The tree is
// read only where the default branch has moved since the last scan, which is
// what makes the interval a choice rather than a budget: measured on a
// repository holding 8,229 files, the tree is 2.65 MB and 1.2s and its head
// commit is 342 bytes.
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

	known := make(map[string]orgsync.RepositoryPaths, len(stored))
	for _, row := range stored {
		known[row.RepositoryID] = row
	}

	// The installation's answer, read once: it is the same for every repository
	// under it, and the sweep is already holding the target.
	target, err := s.store.GetTarget(ctx, targetID)
	if err != nil {
		logging.From(ctx).Warn("could not read the installation for the path index",
			"error", err)

		return
	}

	now := time.Now().UTC()
	for _, repository := range repositories {
		// Only what sync watches. A repository the installation does not
		// synchronize contributes paths nobody can configure a file at.
		if !repository.Available {
			continue
		}

		interval := pathIndexInterval(
			s.pathIndexInterval(), target.PathIndexIntervalOverride,
			repository.PathIndexIntervalOverride,
		)

		was, seen := known[repository.ID]
		if seen && now.Sub(was.ObservedAt) < interval {
			continue
		}

		s.refreshRepositoryPaths(ctx, client, targetID, repository, was, seen, now)
	}

	// And what the installation no longer holds. A repository that left it, or
	// was archived, or whose access was withdrawn, is skipped by the loop above
	// - so nothing was ever going to replace its list, and the finder went on
	// offering paths from a repository nobody can configure a file at. Last,
	// because it reads the catalog the loop has just been walking.
	dropped, err := s.store.PruneSyncRepositoryPaths(ctx, targetID)
	if err != nil {
		logging.From(ctx).Warn("could not prune the path index", "error", err)

		return
	}

	if dropped > 0 {
		logging.From(ctx).Info("dropped path lists for repositories no longer synchronized",
			"repositories", dropped)
	}
}

// pathIndexInterval is how often this repository's file list is checked.
//
// Nearest wins: the repository if it says, then its installation, then the
// process. Three levels because the right answer is not the same everywhere -
// an installation whose repositories are edited all day wants a fresher finder
// than one holding archived services, and inside either there is usually one
// repository that is the exception.
func pathIndexInterval(process time.Duration, target, repository *time.Duration) time.Duration {
	if repository != nil {
		return *repository
	}
	if target != nil {
		return *target
	}

	return process
}

// refreshRepositoryPaths brings one repository's list up to date.
//
// Every failure here is a warning and a return: this feeds a control that helps
// somebody type a path, and one repository that cannot be read is a finder with
// less to offer rather than a sweep that stopped.
func (s *server) refreshRepositoryPaths(
	ctx context.Context,
	client *github.Client,
	targetID string,
	repository storage.Repository,
	was orgsync.RepositoryPaths,
	seen bool,
	now time.Time,
) {
	head, err := repositoryHead(ctx, client, repository.FullName, repository.DefaultBranch)
	if err != nil {
		logging.From(ctx).Warn("could not read a repository's head commit",
			"repo", repository.FullName, "error", err)

		return
	}

	// The whole reason a rescan is affordable. The list is megabytes and a
	// second's work; this is the 342 bytes that say whether reading it again
	// would produce anything different. Both sides have to be a real commit -
	// an empty one is a repository with no commits at all on one side and a
	// list written before this was recorded on the other, and neither is
	// evidence that nothing has changed.
	if seen && head != "" && head == was.HeadSHA {
		was.ObservedAt = now
		if err := s.store.SetSyncRepositoryPaths(ctx, was); err != nil {
			logging.From(ctx).Warn("could not record a repository's paths as current",
				"repo", repository.FullName, "error", err)
		}

		return
	}

	paths, partial, err := repositoryPaths(
		ctx, client, repository.FullName, repository.DefaultBranch)
	if err != nil {
		logging.From(ctx).Warn("could not read a repository's paths",
			"repo", repository.FullName, "error", err)

		return
	}

	if err := s.store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
		RepositoryID: repository.ID,
		TargetID:     targetID,
		Paths:        paths,
		ObservedAt:   now,
		HeadSHA:      head,
		Partial:      partial,
	}); err != nil {
		logging.From(ctx).Warn("could not store a repository's paths",
			"repo", repository.FullName, "error", err)
	}
}

// repositoryHead is the commit one repository's default branch points at.
//
// A branch that is not there answers an empty string rather than an error: a
// repository with no commits is a repository, and it holds no paths.
func repositoryHead(
	ctx context.Context,
	client *github.Client,
	fullName, branch string,
) (string, error) {
	owner, repo, ok := strings.Cut(fullName, "/")
	if !ok {
		return "", fmt.Errorf("%w: repository name %q has no owner", ErrInvalidInput, fullName)
	}

	return client.GetRef(ctx, owner, repo, "heads/"+branch)
}

// repositoryPaths reads every ordinary file one repository holds, and reports
// whether GitHub listed them all.
//
// Ordinary files only: a directory is not something a template can be written
// at, and a symbolic link or a submodule is a path that cannot be written to
// without destroying what is there.
func repositoryPaths(
	ctx context.Context,
	client *github.Client,
	fullName, branch string,
) ([]string, bool, error) {
	owner, repo, ok := strings.Cut(fullName, "/")
	if !ok {
		return nil, false, fmt.Errorf(
			"%w: repository name %q has no owner", ErrInvalidInput, fullName)
	}

	tree, err := client.ListWholeRepositoryTree(ctx, owner, repo, branch)
	if err != nil {
		return nil, false, err
	}

	paths := make([]string, 0, len(tree.Entries))
	for path, entry := range tree.Entries {
		if entry.OrdinaryFile() {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	return paths, tree.Truncated, nil
}
