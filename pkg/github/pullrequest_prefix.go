package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gogithub "github.com/google/go-github/v91/github"
)

const pullSortCreated = "created"

// ListPullRequestsByHeadPrefix includes closed proposals so callers can retain
// refusals after updating an open proposal with a new configuration fingerprint.
// Forks are excluded: a matching branch name alone does not make it our branch.
func (c *Client) ListPullRequestsByHeadPrefix(ctx context.Context, owner, repo, prefix, base string) ([]PullRequest, error) {
	options := &gogithub.PullRequestListOptions{
		State: "all", Base: base, Sort: pullSortCreated, Direction: "asc",
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}
	var result []PullRequest
	for {
		pulls, response, err := c.gh.PullRequests.List(ctx, owner, repo, options)
		if err != nil {
			return nil, wrapError(ErrAPIRequest, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls", owner, repo), err)
		}
		for _, pull := range pulls {
			if strings.HasPrefix(pull.GetHead().GetRef(), prefix) &&
				strings.EqualFold(pull.GetHead().GetRepo().GetFullName(), owner+"/"+repo) &&
				pull.GetUser().GetType() == "Bot" {
				result = append(result, asPullRequest(pull))
			}
		}
		if response.NextPage == 0 {
			return result, nil
		}
		options.Page = response.NextPage
	}
}

// ClosePullRequest preserves the branch and its commits.
func (c *Client) ClosePullRequest(ctx context.Context, owner, repo string, number int, body string) error {
	_, _, err := c.gh.PullRequests.Edit(ctx, owner, repo, number, &gogithub.PullRequest{State: new("closed"), Body: new(body)})
	return wrapError(ErrAPIRequest, http.MethodPatch, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number), err)
}
