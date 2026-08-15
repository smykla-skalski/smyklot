package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// GetPullRequestState reads the live fields needed by pending-CI
// reconciliation without fetching reviews.
func (c *Client) GetPullRequestState(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
) (PullRequestState, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, pullRequest)
	data, err := c.makeRequestWithRetry(ctx, http.MethodGet, path, nil)
	if err != nil {
		return PullRequestState{}, err
	}
	var response pullRequestStateResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return PullRequestState{}, NewAPIError(ErrResponseParse, 0, http.MethodGet, path, err)
	}
	if response.Head.SHA == "" {
		return PullRequestState{}, NewAPIError(
			ErrResponseParse, 0, http.MethodGet, path, fmt.Errorf("no head SHA in response"),
		)
	}

	return response.state(pullRequest), nil
}

type pullRequestStateResponse struct {
	State  string `json:"state"`
	Merged bool   `json:"merged"`
	Draft  bool   `json:"draft"`
	Head   struct {
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (response pullRequestStateResponse) state(number int) PullRequestState {
	labels := make([]string, 0, len(response.Labels))
	for _, label := range response.Labels {
		if label.Name != "" {
			labels = append(labels, label.Name)
		}
	}

	return PullRequestState{
		Number: number, Open: response.State == "open" && !response.Merged,
		Merged: response.Merged, Draft: response.Draft, HeadSHA: response.Head.SHA,
		BaseBranch: response.Base.Ref, Labels: labels,
	}
}
