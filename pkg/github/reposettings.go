package github

import (
	"context"
	"fmt"
	"net/http"
)

// RepositorySettings is a repository's settings, as much of them as sync reads.
//
// Values rather than pointers, because GitHub answers with every one of these.
// The optionality lives in configuration, which may decline to say; an
// observation cannot.
type RepositorySettings struct {
	AllowMergeCommit    bool `json:"allow_merge_commit"`
	AllowSquashMerge    bool `json:"allow_squash_merge"`
	AllowRebaseMerge    bool `json:"allow_rebase_merge"`
	AllowAutoMerge      bool `json:"allow_auto_merge"`
	DeleteBranchOnMerge bool `json:"delete_branch_on_merge"`
	AllowUpdateBranch   bool `json:"allow_update_branch"`

	SquashMergeCommitTitle   string `json:"squash_merge_commit_title"`
	SquashMergeCommitMessage string `json:"squash_merge_commit_message"`
	MergeCommitTitle         string `json:"merge_commit_title"`
	MergeCommitMessage       string `json:"merge_commit_message"`

	HasIssues      bool `json:"has_issues"`
	HasProjects    bool `json:"has_projects"`
	HasWiki        bool `json:"has_wiki"`
	HasDiscussions bool `json:"has_discussions"`
}

// GetRepositorySettings reads what a repository is set to.
func (c *Client) GetRepositorySettings(
	ctx context.Context,
	owner, repo string,
) (RepositorySettings, error) {
	path := fmt.Sprintf("/repos/%s/%s", owner, repo)

	return doJSON[RepositorySettings](ctx, c, http.MethodGet, path, nil)
}

// UpdateRepositorySettings changes the settings a body names, and only those.
//
// The body is sent exactly as given rather than translated through a struct.
// This endpoint replaces what it is sent, so which keys are present is the
// whole of the instruction - and it was decided when the plan was computed and
// read by whoever approved it. Rebuilding it here would put a second author
// between the two, and the failure that produces is silent: a field that stops
// being mapped is a field that stops being applied, with nothing to say so.
func (c *Client) UpdateRepositorySettings(
	ctx context.Context,
	owner, repo string,
	body map[string]any,
) error {
	if len(body) == 0 {
		// Nothing to change. An empty PATCH is a request that can only fail or
		// do nothing, and the caller has a bug worth naming rather than a
		// request worth making.
		return fmt.Errorf("%w: no settings to change", ErrAPIRequest)
	}

	path := fmt.Sprintf("/repos/%s/%s", owner, repo)
	_, err := doJSON[RepositorySettings](ctx, c, http.MethodPatch, path, body)

	return err
}
