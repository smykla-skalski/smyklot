package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	gogithub "github.com/google/go-github/v90/github"
)

// TreeEntry is one path a repository's tree records.
//
// The object rather than the bytes. git names an object by hashing its
// contents, so comparing this id against one computed here answers whether a
// file already says what it should - and one listing answers it for every file
// at once, where the tool this replaces downloaded each of them from every
// repository on every run.
type TreeEntry struct {
	// Mode is git's own: 100644 for an ordinary file, 100755 for an executable
	// one, 120000 for a symbolic link, 160000 for a submodule, 040000 for a
	// directory.
	//
	// Carried rather than dropped, because a path holding anything but an
	// ordinary file cannot be written to without destroying what is there and
	// git will let that happen without a word: a tree entry naming a directory
	// as a blob replaces the whole directory, and one naming a submodule
	// replaces the pointer.
	Mode string

	Blob string
	Size int
}

// directoryMode is the mode git records a directory under.
const directoryMode = "040000"

// OrdinaryFile reports the one thing this writes: a regular, non-executable
// file.
func (e TreeEntry) OrdinaryFile() bool { return e.Mode == FileMode }

// Directory reports an entry that has things under it, which is the one kind
// that can be descended into.
func (e TreeEntry) Directory() bool { return e.Mode == directoryMode }

func asTreeEntry(entry *gogithub.TreeEntry) TreeEntry {
	return TreeEntry{Mode: entry.GetMode(), Blob: entry.GetSHA(), Size: entry.GetSize()}
}

// RepositoryTree is what a tree records, whole or one level of it.
type RepositoryTree struct {
	// Entries is keyed by the name each one carries in what was read: a path
	// from the repository root for a whole listing, a plain name for one level.
	// Directories are in it either way, because a directory is exactly what
	// must not be written over.
	Entries map[string]TreeEntry

	// Truncated is GitHub declining to list the whole thing, which it does past
	// a hundred thousand entries. It is carried rather than hidden because the
	// difference between "this repository does not have that file" and "GitHub
	// did not say" is the difference between creating a file and overwriting
	// one.
	Truncated bool

	// Missing is there being no such tree: at the root of a ref, a repository
	// with no commits, or a branch nothing has pushed to yet.
	//
	// Carried for the same reason Truncated is. An empty tree and no tree read
	// the same way to a caller asking which of its files are there - all of
	// them absent - and they are not the same thing to a caller deciding what
	// to do about it. There is nothing to propose against a branch that does
	// not exist.
	Missing bool
}

// ListRepositoryTree reads every file a ref points at, in one request.
//
// ref is a branch name, a tag or a commit. A repository with no commits at all
// answers 404, which is an empty tree rather than an error: an empty repository
// is a repository, and every managed file is missing from it.
func (c *Client) ListRepositoryTree(
	ctx context.Context,
	owner, repo, ref string,
) (RepositoryTree, error) {
	return c.readTree(ctx, owner, repo, ref, true)
}

// readTree reads a tree object: everything under it, or the one level it names.
//
// A tree that is not there answers 404, which is a tree the caller is told is
// missing rather than an error - an empty repository is a repository, and every
// managed file is missing from it.
//
// The ref is escaped, because this endpoint takes exactly one path segment
// where a branch name may hold several: a repository whose default branch is
// `release/main` asked GitHub for a route it does not have and was answered 404
// - read, before this, as a repository with no commits.
func (c *Client) readTree(
	ctx context.Context,
	owner, repo, at string,
	whole bool,
) (RepositoryTree, error) {
	at = url.PathEscape(at)

	path := fmt.Sprintf("/repos/%s/%s/git/trees/%s", owner, repo, at)
	if whole {
		path += "?recursive=1"
	}

	tree, _, err := c.gh.Git.GetTree(ctx, owner, repo, at, whole)
	if err != nil {
		wrapped := wrapError(ErrAPIRequest, http.MethodGet, path, err)

		var apiErr *APIError
		if errors.As(wrapped, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return RepositoryTree{Entries: map[string]TreeEntry{}, Missing: true}, nil
		}

		return RepositoryTree{}, wrapped
	}

	entries := make(map[string]TreeEntry, len(tree.Entries))
	for _, entry := range tree.Entries {
		entries[entry.GetPath()] = asTreeEntry(entry)
	}

	return RepositoryTree{Entries: entries, Truncated: tree.GetTruncated()}, nil
}

// TreePath is what a ref records on the way to one path.
type TreePath struct {
	// Entry is what sits at the path, when anything does.
	Entry TreeEntry
	Found bool

	// Blocked names a directory on the way there that is not a directory at
	// all, so nothing can sit at the path without replacing it.
	Blocked string
}

// At reads what this listing records at a path.
//
// The same answer a level walk gives, in the same type, so a caller that has a
// whole listing and a caller that had to walk are not two callers. The two used
// to work the question out separately and had to be kept agreeing about what a
// link, a submodule and a file-shaped parent mean.
//
// Only meaningful on a listing GitHub finished. A path missing from a truncated
// one is a path that was not reached, not a path a repository does not have,
// which is what ResolveTreePaths exists for.
func (t RepositoryTree) At(filePath string) TreePath {
	// Ancestors first, descending, which is the order the walk meets them in:
	// nothing can sit under a path that is not a directory, so what is at the
	// path itself is not the question any more.
	for parent := parentPath(filePath); parent != ""; parent = parentPath(parent) {
		if entry, held := t.Entries[parent]; held && !entry.Directory() {
			return TreePath{Blocked: parent}
		}
	}

	if entry, held := t.Entries[filePath]; held {
		return TreePath{Entry: entry, Found: true}
	}

	return TreePath{}
}

// parentPath is the directory a path sits in, empty at the repository root.
//
// The same answer orgsync.parentPath gives, written twice because the client
// must not import what decides what to sync. The two have to stay in step:
// RepositoryTree.At uses this to decide a path is blocked by its parent, and
// FileConfig.validateNesting uses the other to decide two configured paths
// contradict each other, so a divergence is the planner and the client
// disagreeing about the same pair of paths.
func parentPath(filePath string) string {
	parent := path.Dir(filePath)
	if parent == "." || parent == "/" || parent == filePath {
		return ""
	}

	return parent
}

// ResolveTreePaths reads what a ref records at each of several paths, walking
// the tree a level at a time.
//
// Needed where a whole-tree listing came back truncated, and exact where that
// listing cannot be: each level answers what git holds there rather than
// whether a file can be downloaded from it. Asking the contents API instead
// would answer 404 for a path whose parent is a file, which reads as "nothing
// is there" - and nothing-is-there is what turns a write into a create that
// takes the parent out.
//
// Several at once because they share their levels. Every managed path passes
// through the root, and most of them through the same two or three directories
// after it, so reading one path at a time reads the root once per path.
func (c *Client) ResolveTreePaths(
	ctx context.Context,
	owner, repo, ref string,
	paths []string,
) (map[string]TreePath, error) {
	levels := map[string]RepositoryTree{}
	found := make(map[string]TreePath, len(paths))

	for _, filePath := range paths {
		resolved, err := c.resolveTreePath(ctx, owner, repo, ref, filePath, levels)
		if err != nil {
			return nil, err
		}

		found[filePath] = resolved
	}

	return found, nil
}

func (c *Client) resolveTreePath(
	ctx context.Context,
	owner, repo, ref, filePath string,
	levels map[string]RepositoryTree,
) (TreePath, error) {
	segments := strings.Split(filePath, "/")
	at := ref

	for index, segment := range segments {
		listing, read := levels[at]
		if !read {
			var err error
			if listing, err = c.readTree(ctx, owner, repo, at, false); err != nil {
				return TreePath{}, err
			}

			levels[at] = listing
		}

		entry, held := listing.Entries[segment]
		if !held {
			if listing.Truncated {
				return TreePath{}, fmt.Errorf(
					"%w: GitHub would not list all of %s", ErrResponseParse, at)
			}

			return TreePath{}, nil
		}

		if index == len(segments)-1 {
			return TreePath{Entry: entry, Found: true}, nil
		}

		if !entry.Directory() {
			return TreePath{Blocked: strings.Join(segments[:index+1], "/")}, nil
		}

		at = entry.Blob
	}

	return TreePath{}, nil
}

// EditPullRequest rewrites a pull request's title and body.
//
// Used to keep an open proposal describing what it would currently do. A
// proposal is long-lived - it sits until somebody merges it - and the changes
// under it move as the repository does.
func (c *Client) EditPullRequest(
	ctx context.Context,
	owner, repo string,
	number int,
	title, body string,
) error {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number)

	_, _, err := c.gh.PullRequests.Edit(ctx, owner, repo, number, &gogithub.PullRequest{
		Title: gogithub.Ptr(title),
		Body:  gogithub.Ptr(body),
	})

	return wrapError(ErrAPIRequest, http.MethodPatch, path, err)
}

// There is deliberately no way to remove a reference here.
//
// GitHub's delete has no compare-and-swap, where its move refuses anything that
// is not a fast-forward. A branch read and then removed is a branch somebody
// could have pushed to in between, and their commit would be gone with no error
// and no trace - which is the failure this whole area exists to stop. Nothing
// smyklot proposes needs a branch taken away: a merged one is built on, and
// what merged is already in the default branch, so the next pull request from
// it carries only what is new.
