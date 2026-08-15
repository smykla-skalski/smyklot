package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// IssueCommentState is the live mutable state used to reject stale webhook
// deliveries before they can change command state.
type IssueCommentState struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	UpdatedAt string `json:"updated_at"`
	User      struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"user"`
}

// GetIssueComment reads one comment directly rather than trusting the mutable
// copy carried by a webhook delivery.
func (c *Client) GetIssueComment(
	ctx context.Context,
	owner, repository string,
	commentID int64,
) (IssueCommentState, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d", owner, repository, commentID)
	data, err := c.makeRequestWithRetry(ctx, http.MethodGet, path, nil)
	if err != nil {
		return IssueCommentState{}, err
	}
	var comment IssueCommentState
	if err := json.Unmarshal(data, &comment); err != nil {
		return IssueCommentState{}, NewAPIError(
			ErrResponseParse, 0, http.MethodGet, path, err,
		)
	}
	if comment.ID == 0 || comment.UpdatedAt == "" {
		return IssueCommentState{}, NewAPIError(
			ErrResponseParse, 0, http.MethodGet, path,
			fmt.Errorf("incomplete issue comment response"),
		)
	}

	return comment, nil
}
