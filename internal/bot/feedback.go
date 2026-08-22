package bot

import (
	"context"

	"github.com/smykla-skalski/smyklot/pkg/feedback"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/permissions"
)

// PostFeedback posts a comment and adds a reaction to a PR.
func PostFeedback(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	prNum, commentID int,
	message string,
	reaction github.ReactionType,
) error {
	// Only post-comment if the message is not empty
	if message != "" {
		if err := client.PostComment(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			message,
		); err != nil {
			return NewGitHubError(ErrPostComment, err)
		}
	}

	// Remove eyes reaction after the operation completes
	_ = client.RemoveReactionByUser(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
		github.ReactionEyes,
		rc.BotUsername,
	)

	// Add final status reaction
	if err := client.AddReaction(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
		reaction,
	); err != nil {
		return NewGitHubError(ErrAddReaction, err)
	}

	return nil
}

// addEyesReaction adds an eyes reaction to a comment to acknowledge the command.
func addEyesReaction(ctx context.Context, client *github.Client, rc *RuntimeConfig, commentID int) error {
	if err := client.AddReaction(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
		github.ReactionEyes,
	); err != nil {
		return NewGitHubError(ErrAddReaction, err)
	}

	return nil
}

// postOperationFailure posts failure feedback for a failed operation.
func postOperationFailure(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	prNum, commentID int,
	operationErr error,
	feedbackFunc func(string) *feedback.Feedback,
	sentinelErr error,
) error {
	fb := feedbackFunc(operationErr.Error())

	if err := PostFeedback(
		ctx,
		client,
		rc,
		prNum,
		commentID,
		fb.Message,
		github.ReactionError,
	); err != nil {
		return err
	}

	return NewGitHubError(sentinelErr, operationErr)
}

// handleUnauthorized posts feedback for unauthorized users.
func handleUnauthorized(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	checker *permissions.Checker,
	prNum, commentID int,
) error {
	fb := feedback.NewUnauthorized(rc.CommentAuthor, checker.GetApprovers())

	return PostFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionError)
}

// postCombinedFeedback posts combined feedback with appropriate reaction
func postCombinedFeedback(ctx context.Context, client *github.Client, rc *RuntimeConfig, prNum, commentID int, fb *feedback.Feedback) error {
	// Map feedback type to reaction
	var reaction github.ReactionType
	switch fb.Type {
	case feedback.Success:
		reaction = github.ReactionSuccess
	case feedback.Error:
		reaction = github.ReactionError
	case feedback.Warning:
		reaction = github.ReactionWarning
	case feedback.Pending:
		reaction = github.ReactionPendingCI
	default:
		reaction = github.ReactionSuccess
	}

	// Post comment if there's a message
	if fb.RequiresComment() {
		if err := client.PostComment(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			fb.Message,
		); err != nil {
			return NewGitHubError(ErrPostComment, err)
		}
	}

	// Remove eyes reaction before adding final status reaction
	_ = client.RemoveReactionByUser(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
		github.ReactionEyes,
		rc.BotUsername,
	)

	// Add reaction
	if err := client.AddReaction(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
		reaction,
	); err != nil {
		return NewGitHubError(ErrAddReaction, err)
	}

	return nil
}

// postNotMergeable posts feedback when PR is not mergeable.
func postNotMergeable(ctx context.Context, client *github.Client, rc *RuntimeConfig, prNum, commentID int) error {
	fb := feedback.NewNotMergeable()

	return PostFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionWarning)
}
