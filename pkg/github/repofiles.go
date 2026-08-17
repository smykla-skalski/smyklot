package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v90/github"
)

// TreeBlob is one file a repository holds, as its tree describes it.
//
// The object rather than the bytes. git names an object by hashing its
// contents, so comparing this id against one computed here answers whether a
// file already says what it should - and one listing answers it for every file
// at once, where the tool this replaces downloaded each of them from every
// repository on every run.
type TreeBlob struct {
	Blob string
	Size int
}

// RepositoryTree is every file a commit holds.
type RepositoryTree struct {
	// Blobs is keyed by path, relative to the repository root. Directories are
	// not in it: git records them, and nothing here writes one.
	Blobs map[string]TreeBlob

	// Truncated is GitHub declining to list the whole thing, which it does past
	// a hundred thousand entries. It is carried rather than hidden because the
	// difference between "this repository does not have that file" and "GitHub
	// did not say" is the difference between creating a file and overwriting
	// one.
	Truncated bool
}

// treeBlobType is what a tree entry holds when it is a file.
const treeBlobType = "blob"

// ListRepositoryTree reads every file a ref points at, in one request.
//
// ref is a branch name, a tag or a commit. A repository with no commits at all
// answers 404, which is an empty tree rather than an error: an empty repository
// is a repository, and every managed file is missing from it.
func (c *Client) ListRepositoryTree(
	ctx context.Context,
	owner, repo, ref string,
) (RepositoryTree, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, ref)

	tree, _, err := c.gh.Git.GetTree(ctx, owner, repo, ref, true)
	if err != nil {
		wrapped := wrapError(ErrAPIRequest, http.MethodGet, path, err)

		var apiErr *APIError
		if errors.As(wrapped, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return RepositoryTree{Blobs: map[string]TreeBlob{}}, nil
		}

		return RepositoryTree{}, wrapped
	}

	blobs := make(map[string]TreeBlob, len(tree.Entries))

	for _, entry := range tree.Entries {
		if entry.GetType() != treeBlobType {
			continue
		}

		blobs[entry.GetPath()] = TreeBlob{Blob: entry.GetSHA(), Size: entry.GetSize()}
	}

	return RepositoryTree{Blobs: blobs, Truncated: tree.GetTruncated()}, nil
}

// maxSyncedFile is the largest file this will read back out of a repository.
//
// The same bound configuration is held to, so a file this cannot read is one
// nothing here could have written.
const maxSyncedFile = 1 << 20

// GetRepositoryFile reads one file at a ref, reporting whether it is there.
//
// Only needed where a tree came back truncated. Every other read goes through
// the tree, which answers for every file at once.
func (c *Client) GetRepositoryFile(
	ctx context.Context,
	owner, repo, ref, filePath string,
) ([]byte, bool, error) {
	content, err := c.getFileContentAtRef(ctx, owner, repo, filePath, ref, maxSyncedFile)
	if err != nil {
		return nil, false, err
	}

	// getFileContentAtRef answers a missing file with no content and no error,
	// which is the shape every caller of it wants and the one this has to
	// translate: a file that exists and is empty is not a file that is absent.
	if content == nil {
		return nil, false, nil
	}

	return content, true, nil
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

// DeleteRef removes a reference.
//
// Used on a branch whose proposal has been merged: the next change starts from
// the default branch rather than from a tip that is already in it.
func (c *Client) DeleteRef(ctx context.Context, owner, repo, ref string) error {
	path := fmt.Sprintf("/repos/%s/%s/git/refs/%s", owner, repo, ref)

	_, err := c.gh.Git.DeleteRef(ctx, owner, repo, "refs/"+ref)

	return wrapError(ErrAPIRequest, http.MethodDelete, path, err)
}
