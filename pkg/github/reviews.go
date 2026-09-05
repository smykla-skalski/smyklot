package github

import (
	"context"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v91/github"
)

// dismissalMessage is what GitHub records against a dismissed review.
const dismissalMessage = "Review dismissed"

// ApprovePR approves a pull request by creating a review with the APPROVE event.
func (c *Client) ApprovePR(ctx context.Context, owner, repo string, prNumber int) error {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, prNumber)

	_, _, err := c.gh.PullRequests.CreateReview(ctx, owner, repo, prNumber, &gogithub.PullRequestReviewRequest{
		Event: new("APPROVE"),
	})

	return wrapError(ErrAPIRequest, http.MethodPost, path, err)
}

// DismissReviewByUsername dismisses all approved reviews by the specified username
//
// The username is passed in rather than looked up: GET /user answers 403
// "Resource not accessible by integration" for an App installation token, which
// is the only kind of token this bot holds in practice.
//
// Every page of reviews is read. The version this replaces read one, so on a
// pull request carrying more than thirty reviews the bot's own approval could
// sit on the second page and survive an unapprove - which is the one outcome
// this function exists to prevent.
func (c *Client) DismissReviewByUsername(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	username string,
) error {
	reviews, err := c.listReviews(ctx, owner, repo, prNumber)
	if err != nil {
		return err
	}

	for _, review := range reviews {
		if review.GetState() != "APPROVED" || review.GetUser().GetLogin() != username {
			continue
		}

		if err := c.dismissReview(ctx, owner, repo, prNumber, review.GetID()); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) dismissReview(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	reviewID int64,
) error {
	path := fmt.Sprintf(
		"/repos/%s/%s/pulls/%d/reviews/%d/dismissals", owner, repo, prNumber, reviewID,
	)

	_, _, err := c.gh.PullRequests.DismissReview(
		ctx, owner, repo, prNumber, reviewID,
		gogithub.PullRequestDismissReviewRequest{Message: dismissalMessage},
	)

	return wrapError(ErrAPIRequest, http.MethodPut, path, err)
}

func (c *Client) listReviews(
	ctx context.Context,
	owner, repo string,
	prNumber int,
) ([]*gogithub.PullRequestReview, error) {
	op := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, prNumber)

	return paginate(ctx, op,
		func(ctx context.Context, opts *gogithub.ListOptions) ([]*gogithub.PullRequestReview, *gogithub.Response, error) {
			return c.gh.PullRequests.ListReviews(ctx, owner, repo, prNumber, opts)
		})
}

// getApprovers lists everyone whose latest review approved the pull request.
//
// A failure is reported as nobody rather than as an error: the caller is
// assembling a description of the pull request, and losing the approver list is
// not a reason to lose the rest of it.
func (c *Client) getApprovers(ctx context.Context, owner, repo string, prNumber int) []string {
	reviews, err := c.listReviews(ctx, owner, repo, prNumber)
	if err != nil {
		return []string{}
	}

	approvers := make([]string, 0, len(reviews))
	seen := make(map[string]bool, len(reviews))

	for _, review := range reviews {
		login := review.GetUser().GetLogin()
		if review.GetState() != "APPROVED" || login == "" || seen[login] {
			continue
		}

		seen[login] = true

		approvers = append(approvers, login)
	}

	return approvers
}
