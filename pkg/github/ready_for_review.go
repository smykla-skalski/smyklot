package github

import (
	"context"
	"errors"
	"fmt"
)

// MarkPullRequestReadyForReview publishes a draft pull request.
//
// The mutation is sent once because a lost response is ambiguous. On failure,
// live state is read once to recognize a mutation GitHub applied before the
// response was interrupted.
func (c *Client) MarkPullRequestReadyForReview(
	ctx context.Context,
	owner, repo string,
	pullRequest int,
) error {
	nodeID, err := c.pullRequestNodeID(ctx, owner, repo, pullRequest)
	if err != nil {
		return err
	}

	const mutation = `mutation($pullRequestId: ID!) {
		markPullRequestReadyForReview(input: {pullRequestId: $pullRequestId}) {
			clientMutationId
		}
	}`

	err = c.graphql(withoutRetry(ctx), mutation, map[string]any{
		"pullRequestId": nodeID,
	}, nil)
	if err == nil {
		return nil
	}

	state, verificationErr := c.GetPullRequestState(withoutRetry(ctx), owner, repo, pullRequest)
	if verificationErr == nil && !state.Draft {
		return nil
	}
	if verificationErr != nil {
		return errors.Join(err, fmt.Errorf("verify ready-for-review mutation: %w", verificationErr))
	}

	return err
}
