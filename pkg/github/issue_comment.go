package github

import (
	"context"
	"errors"
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

	comment, err := doJSON[IssueCommentState](ctx, c, http.MethodGet, path, nil)
	if err != nil {
		return IssueCommentState{}, err
	}

	if comment.ID == 0 || comment.UpdatedAt == "" {
		return IssueCommentState{}, NewAPIError(
			ErrResponseParse, 0, http.MethodGet, path,
			errors.New("incomplete issue comment response"),
		)
	}

	return comment, nil
}
