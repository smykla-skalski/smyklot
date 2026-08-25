package apply

import (
	"context"
	"fmt"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/smykla-skalski/smyklot/internal/bot"
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

// RefreshPaths keeps the panel's path finder answering with what exists.
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
// pathIndexConcurrency is how many repositories are read at once.
//
// Eight because the work is waiting rather than computing, and because the
// ceiling that matters is GitHub's: an installation has 15,000 requests an
// hour and a repository whose branch has moved costs a whole tree read, so the
// aim is to overlap the latency of a handful, not to spend the budget of an
// installation in one tick.
const pathIndexConcurrency = 8

// RefreshPaths is never fatal. It feeds a control that helps somebody type; a
// reconcile that failed because a tree could not be read would stop the sync it
// is beside for the sake of an autocomplete.
func (s *Engine) RefreshPaths(
	ctx context.Context,
	client *github.Client,
	targetID string,
	processInterval time.Duration,
) {
	// An installation that has never configured sync gets no index at all.
	//
	// This is the majority of them, and the cost of indexing one is not small:
	// a ref read per repository per interval, and a whole tree wherever a
	// branch has moved - up to 500 requests for one repository, against an
	// installation's 15,000 an hour. `PlanInstallation` already returns
	// after a single table read for these; this is the same door, and reading
	// the same table is what opens it.
	//
	// Configured rather than switched ON: the finder is what somebody types a
	// path into while setting file sync up, so an index that arrived only once
	// the thing was already running would be empty exactly when it is needed.
	configs, err := s.store.ListSyncConfigs(ctx, targetID)
	if err != nil {
		logging.From(ctx).Warn("could not read the sync configuration for the path index",
			"error", err)

		return
	}
	if len(configs) == 0 {
		return
	}

	repositories, err := s.store.ListRepositories(ctx, targetID)
	if err != nil {
		logging.From(ctx).Warn("could not read repositories for the path index", "error", err)

		return
	}

	// Described rather than read: what this needs from a stored row is when it
	// was taken and at which commit, and reading the lists to learn that
	// decoded every path in the installation on every tick and kept none of it.
	stored, err := s.store.ListSyncRepositoryPathScans(ctx, targetID)
	if err != nil {
		logging.From(ctx).Warn("could not read the path index", "error", err)

		return
	}

	known := make(map[string]orgsync.RepositoryPathScan, len(stored))
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

	// A few at a time, because each due repository is at least one round trip to
	// GitHub and they do not depend on each other. In turn, two hundred
	// repositories was two hundred sequential reads - about thirty seconds of
	// the sweep goroutine's tick, spent before any plan is applied, for an
	// index that feeds an autocomplete.
	//
	// Bounded rather than unbounded: the point is to overlap the waiting, not to
	// open an installation's whole catalog against GitHub's rate limit at once.
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(pathIndexConcurrency)

	for _, repository := range repositories {
		// Only repositories Smyklot is allowed to operate on. A disabled
		// repository contributes paths no active sync may apply.
		if !repository.Available || !storage.RepositoryEnabled(target, repository) {
			continue
		}

		interval := pathIndexInterval(
			processInterval, target.PathIndexIntervalOverride,
			repository.PathIndexIntervalOverride,
		)

		// Nil is a repository nothing has scanned yet, which is what the two
		// `seen &&` guards below used to spell with a second parameter.
		var was *orgsync.RepositoryPathScan
		if scan, seen := known[repository.ID]; seen {
			if now.Sub(scan.ObservedAt) < interval {
				continue
			}
			was = &scan
		}

		group.Go(func() error {
			// Never an error: every failure inside is already a warning and a
			// return, and one repository that cannot be read must not cancel
			// the rest through the group's context.
			s.refreshRepositoryPaths(groupCtx, client, targetID, repository, was, now)

			return nil
		})
	}

	// The only error a group with no failing member returns is its context's,
	// and a cancelled sweep has nothing left to prune.
	if err := group.Wait(); err != nil {
		return
	}

	// And what the installation no longer synchronizes. A repository that left,
	// was disabled or archived, or had its access withdrawn is skipped above,
	// so nothing would replace its list and the finder would keep offering its
	// paths. Last, because it reads the catalog the loop has just walked.
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
func (s *Engine) refreshRepositoryPaths(
	ctx context.Context,
	client *github.Client,
	targetID string,
	repository storage.Repository,
	was *orgsync.RepositoryPathScan,
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
	if was != nil && head != "" && head == was.HeadSHA {
		// The timestamp alone. The list this row holds is still the list that
		// branch points at, and rewriting it to move one column re-encoded
		// every path in the repository.
		if err := s.store.TouchSyncRepositoryPaths(ctx, repository.ID, now); err != nil {
			logging.From(ctx).Warn("could not record a repository's paths as current",
				"repo", repository.FullName, "error", err)
		}

		return
	}

	paths, partial, missing, err := repositoryPaths(
		ctx, client, repository.FullName, repository.DefaultBranch)
	if err != nil {
		logging.From(ctx).Warn("could not read a repository's paths",
			"repo", repository.FullName, "error", err)

		return
	}

	// A tree that is not there, under a branch that IS. Those two together are
	// not a repository holding no files - a repository with no commits has no
	// head either, and answers both reads empty, which is the case just below.
	// This is a branch renamed between the two reads, or access withdrawn
	// (GitHub answers 404, not 403, for a repository a token cannot see).
	//
	// So the stored list is kept rather than replaced with nothing. Overwriting
	// took every path the repository contributes out of the finder, recorded
	// the empty list as complete, and stuck: the row carried the new head, so
	// every later tick took the unchanged-head path and only stamped the time.
	if missing && head != "" {
		logging.From(ctx).Warn("no tree under a branch that has one, keeping the stored paths",
			"repo", repository.FullName, "branch", repository.DefaultBranch)

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
	owner, repo, err := namedRepository(fullName)
	if err != nil {
		return "", err
	}

	return client.GetRef(ctx, owner, repo, "heads/"+branch)
}

// namedRepository is `owner/repo` split, and refused where there is no owner.
//
// `splitFullName` answers an empty owner for a name carrying no slash, which is
// what its own callers want - they are naming a repository in a sentence. These
// two are building an API path, where an empty owner asks GitHub for a route
// that does not exist and is answered 404, which reads as a repository holding
// nothing. So they refuse it instead, in one place rather than two.
func namedRepository(fullName string) (string, string, error) {
	owner, repo := splitFullName(fullName)
	if owner == "" {
		return "", "", fmt.Errorf(
			"%w: repository name %q has no owner", bot.ErrInvalidInput, fullName)
	}

	return owner, repo, nil
}

// repositoryPaths reads every ordinary file one repository holds, reports
// whether GitHub listed them all, and whether there was a tree to read at all.
//
// Ordinary files only: a directory is not something a template can be written
// at, and a symbolic link or a submodule is a path that cannot be written to
// without destroying what is there.
//
// The third answer is the one that costs something to get wrong. A tree that is
// not there answers 404, which `RepositoryTree` reports as Missing rather than
// as an error - and read as "this repository holds no files" it replaced a good
// list with an empty one and recorded the empty one as complete. GitHub answers
// 404 for a branch renamed between reading the ref and reading the tree, and
// for a repository the token has lost access to, so this is not a rare shape.
// Worse, it stuck: the row kept the new head, so every later tick took the
// unchanged-head path and only stamped the time.
func repositoryPaths(
	ctx context.Context,
	client *github.Client,
	fullName, branch string,
) (paths []string, partial, missing bool, err error) {
	owner, repo, err := namedRepository(fullName)
	if err != nil {
		return nil, false, false, err
	}

	tree, err := client.ListWholeRepositoryTree(ctx, owner, repo, branch)
	if err != nil {
		return nil, false, false, err
	}
	if tree.Missing {
		return nil, false, true, nil
	}

	// Ordinary files only - mode 100644. An executable, a symlink and a submodule
	// are all things sync itself refuses to write (`notAnOrdinaryFile`), so
	// offering one would be offering a path that cannot be a template. The
	// finder says so where a reader would otherwise wonder why their script is
	// missing; this is the only place the rule is applied.
	paths = make([]string, 0, len(tree.Entries))
	for path, entry := range tree.Entries {
		if entry.OrdinaryFile() {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	return paths, tree.Truncated, false, nil
}
