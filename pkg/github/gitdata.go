package github

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v90/github"
)

// FileMode is the mode a tree entry carries. GitHub accepts a small closed set
// and Smyklot writes ordinary files, so there is one.
const FileMode = "100644"

// treeType is what a tree entry holds. Only blobs are written here; a subtree
// is what GitHub builds from the paths.
const treeType = "blob"

// TreeChange is one path a commit adds, replaces or removes.
type TreeChange struct {
	// Path is where the file sits, relative to the repository root.
	Path string

	// Blob is the object to put there. An empty Blob deletes the path, which
	// is what lets one commit move a file rather than leaving the repository
	// briefly carrying both.
	Blob string
}

// NewPullRequest is a pull request to open.
type NewPullRequest struct {
	Title string
	Body  string

	// Head is the branch carrying the change, Base the branch it is proposed
	// against.
	Head string
	Base string
}

// The states GitHub reports a pull request in. There are two, and Merged is
// what tells apart the two ways of reaching the second one.
const (
	PullRequestOpen   = "open"
	PullRequestClosed = "closed"
)

// PullRequest is an opened pull request, as much of one as Smyklot reads.
type PullRequest struct {
	Number int
	State  string
	Merged bool
	URL    string
}

// GetRef resolves a git reference to the commit it points at.
//
// ref is spelled the way the API spells it, "heads/main" rather than
// "refs/heads/main". A reference that does not exist is an empty string and no
// error: "is this branch there" is a question, and 404 is one of its answers.
func (c *Client) GetRef(ctx context.Context, owner, repo, ref string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/ref/%s", owner, repo, ref)

	reference, _, err := c.gh.Git.GetRef(ctx, owner, repo, ref)
	if err != nil {
		wrapped := wrapError(ErrAPIRequest, http.MethodGet, path, err)

		var apiErr *APIError
		if errors.As(wrapped, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return "", nil
		}

		return "", wrapped
	}

	return reference.GetObject().GetSHA(), nil
}

// CreateRef points a new reference at a commit.
func (c *Client) CreateRef(ctx context.Context, owner, repo, ref, sha string) error {
	path := fmt.Sprintf("/repos/%s/%s/git/refs", owner, repo)

	_, _, err := c.gh.Git.CreateRef(ctx, owner, repo, gogithub.CreateRef{
		Ref: "refs/" + ref,
		SHA: sha,
	})

	return wrapError(ErrAPIRequest, http.MethodPost, path, err)
}

// Commit is a commit object, as much of one as Smyklot reads.
type Commit struct {
	// Tree is the tree the commit records.
	//
	// CreateTree's base is a tree, not a commit - the API documents base_tree
	// as "the SHA1 of an existing Git tree object", while a reference points at
	// a commit. Peeling one to the other is a request rather than an assumption
	// about what GitHub will accept.
	Tree string
}

// GetCommit reads a commit object.
func (c *Client) GetCommit(ctx context.Context, owner, repo, sha string) (Commit, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/commits/%s", owner, repo, sha)

	found, _, err := c.gh.Git.GetCommit(ctx, owner, repo, sha)
	if err != nil {
		return Commit{}, wrapError(ErrAPIRequest, http.MethodGet, path, err)
	}

	return Commit{Tree: found.GetTree().GetSHA()}, nil
}

// UpdateRef moves an existing reference to a commit.
//
// force replaces whatever the reference pointed at rather than requiring the
// new commit to descend from it, which is the only way to reuse a branch whose
// history is being rebuilt rather than added to.
func (c *Client) UpdateRef(ctx context.Context, owner, repo, ref, sha string, force bool) error {
	path := fmt.Sprintf("/repos/%s/%s/git/refs/%s", owner, repo, ref)

	_, _, err := c.gh.Git.UpdateRef(ctx, owner, repo, "refs/"+ref, gogithub.UpdateRef{
		SHA:   sha,
		Force: gogithub.Ptr(force),
	})

	return wrapError(ErrAPIRequest, http.MethodPatch, path, err)
}

// CreateBlob stores a file's contents and returns the object.
func (c *Client) CreateBlob(ctx context.Context, owner, repo string, content []byte) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/blobs", owner, repo)

	// base64 rather than utf-8, because a configuration file is bytes: a
	// byte-order mark or a stray latin-1 character would otherwise be refused
	// or silently rewritten, and Smyklot reads files that have both.
	blob, _, err := c.gh.Git.CreateBlob(ctx, owner, repo, gogithub.Blob{
		Content:  gogithub.Ptr(base64.StdEncoding.EncodeToString(content)),
		Encoding: gogithub.Ptr("base64"),
	})
	if err != nil {
		return "", wrapError(ErrAPIRequest, http.MethodPost, path, err)
	}

	return blob.GetSHA(), nil
}

// CreateTree builds a tree from base with the given paths changed.
//
// base is a tree, which GetCommit resolves from a commit. Every path not
// named is inherited from it, so this describes a change rather than a whole
// repository.
func (c *Client) CreateTree(
	ctx context.Context,
	owner, repo, base string,
	changes []TreeChange,
) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/trees", owner, repo)

	entries := make([]*gogithub.TreeEntry, 0, len(changes))

	for _, change := range changes {
		entry := &gogithub.TreeEntry{
			Path: gogithub.Ptr(change.Path),
			Mode: gogithub.Ptr(FileMode),
			Type: gogithub.Ptr(treeType),
		}

		// A nil SHA is how the API spells a deletion, and go-github renders
		// exactly that entry as an explicit null rather than omitting the key.
		// Setting it to the empty string instead would ask GitHub for an object
		// named "", which is a 422.
		if change.Blob != "" {
			entry.SHA = gogithub.Ptr(change.Blob)
		}

		entries = append(entries, entry)
	}

	tree, _, err := c.gh.Git.CreateTree(ctx, owner, repo, base, entries)
	if err != nil {
		return "", wrapError(ErrAPIRequest, http.MethodPost, path, err)
	}

	return tree.GetSHA(), nil
}

// CreateCommit records a tree on top of one parent.
func (c *Client) CreateCommit(
	ctx context.Context,
	owner, repo, message, tree, parent string,
) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/commits", owner, repo)

	commit, _, err := c.gh.Git.CreateCommit(ctx, owner, repo, gogithub.Commit{
		Message: gogithub.Ptr(message),
		Tree:    &gogithub.Tree{SHA: gogithub.Ptr(tree)},
		Parents: []*gogithub.Commit{{SHA: gogithub.Ptr(parent)}},
	}, nil)
	if err != nil {
		return "", wrapError(ErrAPIRequest, http.MethodPost, path, err)
	}

	return commit.GetSHA(), nil
}

// CreatePullRequest opens a pull request.
func (c *Client) CreatePullRequest(
	ctx context.Context,
	owner, repo string,
	request NewPullRequest,
) (PullRequest, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls", owner, repo)

	pull, _, err := c.gh.PullRequests.Create(ctx, owner, repo, gogithub.CreatePullRequest{
		Title: gogithub.Ptr(request.Title),
		Body:  gogithub.Ptr(request.Body),
		Head:  request.Head,
		Base:  request.Base,
	})
	if err != nil {
		return PullRequest{}, wrapError(ErrAPIRequest, http.MethodPost, path, err)
	}

	return asPullRequest(pull), nil
}

// FindPullRequestByHead returns the most recent pull request opened from a
// branch, whatever state it is in, or nil when there is none.
//
// Whatever state, because the answer decides whether to propose something
// again: an open one is still being considered, a merged one is done, and a
// closed one was refused. Listing only the open ones would read the refusal as
// "nobody has asked yet" and ask forever.
func (c *Client) FindPullRequestByHead(
	ctx context.Context,
	owner, repo, branch string,
) (*PullRequest, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls?head=%s:%s&state=all", owner, repo, owner, branch)

	pulls, _, err := c.gh.PullRequests.List(ctx, owner, repo, &gogithub.PullRequestListOptions{
		State:       "all",
		Head:        owner + ":" + branch,
		Sort:        "created",
		Direction:   "desc",
		ListOptions: gogithub.ListOptions{PerPage: 1},
	})
	if err != nil {
		return nil, wrapError(ErrAPIRequest, http.MethodGet, path, err)
	}

	if len(pulls) == 0 {
		return nil, nil
	}

	found := asPullRequest(pulls[0])

	return &found, nil
}

func asPullRequest(pull *gogithub.PullRequest) PullRequest {
	return PullRequest{
		Number: pull.GetNumber(),
		State:  pull.GetState(),
		Merged: pull.GetMerged() || !pull.GetMergedAt().IsZero(),
		URL:    pull.GetHTMLURL(),
	}
}
