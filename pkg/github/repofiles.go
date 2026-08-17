package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
// A tree that is not there answers 404, which is an empty one rather than an
// error - an empty repository is a repository, and every managed file is
// missing from it.
func (c *Client) readTree(
	ctx context.Context,
	owner, repo, at string,
	whole bool,
) (RepositoryTree, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/trees/%s", owner, repo, at)
	if whole {
		path += "?recursive=1"
	}

	tree, _, err := c.gh.Git.GetTree(ctx, owner, repo, at, whole)
	if err != nil {
		wrapped := wrapError(ErrAPIRequest, http.MethodGet, path, err)

		var apiErr *APIError
		if errors.As(wrapped, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return RepositoryTree{Entries: map[string]TreeEntry{}}, nil
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

// ResolveTreePath reads what a ref records at one path, a level at a time.
//
// Needed where a whole-tree listing came back truncated, and exact where that
// listing cannot be: one request per path segment, each answering what git
// holds there rather than whether a file can be downloaded from it. Asking the
// contents API instead would answer 404 for a path whose parent is a file,
// which reads as "nothing is there" - and nothing-is-there is what turns a
// write into a create that takes the parent out.
func (c *Client) ResolveTreePath(
	ctx context.Context,
	owner, repo, ref, filePath string,
) (TreePath, error) {
	segments := strings.Split(filePath, "/")
	at := ref

	for index, segment := range segments {
		listing, err := c.readTree(ctx, owner, repo, at, false)
		if err != nil {
			return TreePath{}, err
		}

		entry, found := listing.Entries[segment]
		if !found {
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

// DeleteRef removes a reference, and reports success where there was none.
//
// Used on a branch whose proposal has been merged: the next change starts from
// the default branch rather than from a tip that is already in it. A branch
// already gone is that outcome rather than a failure - a repository with
// delete_branch_on_merge removed it the moment the pull request landed, and so
// did anybody who tidied up by hand. GetRef reads a 404 the same way, for the
// same reason: the question is about the end state.
func (c *Client) DeleteRef(ctx context.Context, owner, repo, ref string) error {
	path := fmt.Sprintf("/repos/%s/%s/git/refs/%s", owner, repo, ref)

	_, err := c.gh.Git.DeleteRef(ctx, owner, repo, "refs/"+ref)
	if err == nil {
		return nil
	}

	wrapped := wrapError(ErrAPIRequest, http.MethodDelete, path, err)

	var apiErr *APIError
	if errors.As(wrapped, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return nil
	}

	return wrapped
}
