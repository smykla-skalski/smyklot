package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	gogithub "github.com/google/go-github/v91/github"
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

// GetPRComments retrieves every comment on a pull request, oldest first.
//
// Every page of them: GitHub answers thirty, and the comment a caller is
// looking for is usually one of the newest.
//
//nolint:dupl // paginate-and-convert is the idiom every list read here follows
func (c *Client) GetPRComments(
	ctx context.Context,
	owner, repo string,
	prNumber int,
) ([]IssueCommentState, error) {
	op := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, prNumber)

	raw, err := paginate(ctx, op,
		func(ctx context.Context, opts *gogithub.ListOptions) (
			[]*gogithub.IssueComment,
			*gogithub.Response,
			error,
		) {
			return c.gh.Issues.ListComments(
				ctx, owner, repo, prNumber,
				&gogithub.IssueListCommentsOptions{ListOptions: *opts},
			)
		})
	if err != nil {
		return nil, err
	}

	return convertIssueComments(raw), nil
}

func convertIssueComments(raw []*gogithub.IssueComment) []IssueCommentState {
	comments := make([]IssueCommentState, 0, len(raw))

	for _, item := range raw {
		comment := IssueCommentState{ID: item.GetID(), Body: item.GetBody()}
		if item.UpdatedAt != nil {
			comment.UpdatedAt = item.GetUpdatedAt().Format(time.RFC3339)
		}

		comment.User.Login = item.GetUser().GetLogin()
		comment.User.Type = item.GetUser().GetType()

		comments = append(comments, comment)
	}

	return comments
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
