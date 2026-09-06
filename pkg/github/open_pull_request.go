package github

import (
	"context"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v91/github"
)

// FindOpenPullRequestByHead finds an outstanding proposal even if it was
// retargeted or a more recent pull request from the same branch was closed.
func (c *Client) FindOpenPullRequestByHead(ctx context.Context, owner, repository, branch string) (*PullRequest, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls?head=%s:%s&state=open", owner, repository, owner, branch)
	pulls, _, err := c.gh.PullRequests.List(ctx, owner, repository, &gogithub.PullRequestListOptions{
		State: PullRequestOpen, Head: owner + ":" + branch, Sort: "created", Direction: "desc",
		ListOptions: gogithub.ListOptions{PerPage: 1},
	})
	if err != nil {
		return nil, wrapError(ErrAPIRequest, http.MethodGet, path, err)
	}
	if len(pulls) == 0 {
		return nil, nil
	}
	pull := asPullRequest(pulls[0])
	return &pull, nil
}
