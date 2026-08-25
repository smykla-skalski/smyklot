package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

func (c *Client) pullRequestNodeID(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, pullRequest)
	pull, _, err := c.gh.PullRequests.Get(ctx, owner, repo, pullRequest)
	if err != nil {
		return "", wrapError(ErrAPIRequest, http.MethodGet, path, err)
	}
	if pull.GetNodeID() == "" {
		return "", NewAPIError(
			ErrResponseParse, 0, http.MethodGet, path,
			errors.New("no node_id in response"),
		)
	}

	return pull.GetNodeID(), nil
}
