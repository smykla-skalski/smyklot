package bot

import (
	"context"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/feedback"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/permissions"
)

// HandleReactions processes reaction-based approvals and merges.
func HandleReactions(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	checker *permissions.Checker,
	prNum, commentID int,
	environment CommandEnvironment,
) error {
	// Fetch reactions - use PR reactions if commentID equals prNum (PR description),
	// otherwise get comment reactions
	var reactions []github.Reaction
	var err error

	if commentID == prNum {
		// Get reactions on the PR description
		reactions, err = client.GetPRReactions(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
		)
	} else {
		// Get reactions on a comment
		reactions, err = client.GetCommentReactions(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			commentID,
		)
	}

	if err != nil {
		// Don't fail if we can't fetch reactions, just skip
		return nil
	}

	// Fetch current PR labels
	labels, err := client.GetLabels(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		prNum,
	)
	if err != nil {
		// Don't fail if we can't fetch labels, just skip
		return nil
	}

	// Build maps for quick lookup
	reactionMap := make(map[github.ReactionType]bool)
	labelMap := make(map[string]bool)

	for _, reaction := range reactions {
		// Check if user has permission
		canApprove, err := CheckUserPermission(
			ctx,
			client,
			checker,
			reaction.User,
			rc.RepoOwner,
			rc.RepoName,
		)
		if err != nil || !canApprove {
			continue
		}

		reactionMap[reaction.Type] = true
	}

	for _, label := range labels {
		labelMap[label] = true
	}

	// Handle removed reactions (reconciliation)
	if err := handleRemovedReactions(
		ctx,
		client,
		rc,
		bc,
		prNum,
		reactionMap,
		labelMap,
	); err != nil {
		return err
	}

	// Process each reaction
	for _, reaction := range reactions {
		// Check if user has permission
		canApprove, err := CheckUserPermission(
			ctx,
			client,
			checker,
			reaction.User,
			rc.RepoOwner,
			rc.RepoName,
		)
		if err != nil || !canApprove {
			continue
		}

		// Handle approve reaction
		if reaction.Type == github.ReactionApprove {
			if err := handleReactionApprove(ctx, client, rc, bc, prNum, commentID, reaction.User); err != nil {
				return err
			}
		}

		// Handle merge reaction
		if reaction.Type == github.ReactionMerge {
			if err := handleReactionMerge(ctx, client, rc, bc, prNum, commentID, reaction.User); err != nil {
				return err
			}
		}

		// Handle cleanup reaction
		if reaction.Type == github.ReactionCleanup {
			if err := handleReactionCleanup(
				ctx, client, rc, bc, prNum, commentID, environment,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

// handleRemovedReactions handles reactions that were removed.
func handleRemovedReactions(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum int,
	reactionMap map[github.ReactionType]bool,
	labelMap map[string]bool,
) error {
	// Check if approve reaction was removed
	if labelMap[github.LabelReactionApprove] && !reactionMap[github.ReactionApprove] {
		// Approve reaction was removed, unapprove the PR
		if err := client.DismissReviewByUsername(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			rc.BotUsername,
		); err != nil {
			// Don't fail, just log
			logging.From(ctx).Warn("failed to dismiss review after reaction removal", "error", err)
		}

		// Remove the label
		_ = client.RemoveLabel(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			github.LabelReactionApprove,
		)
	}

	// Check if merge reaction was removed
	if labelMap[github.LabelReactionMerge] && !reactionMap[github.ReactionMerge] {
		// Get PR info to check if it's already merged
		info, err := client.GetPRInfo(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
		)
		if err != nil {
			// Don't fail, just log
			logging.From(ctx).Warn("failed to get PR info after reaction removal", "error", err)

			return nil
		}

		// If PR is already merged, post warning (unless disabled)
		if info.State == github.PullRequestClosed {
			if !bc.QuietReactions {
				fb := feedback.NewReactionMergeRemoved()
				_ = client.PostComment(
					ctx,
					rc.RepoOwner,
					rc.RepoName,
					prNum,
					fb.Message,
				)
			}
		}

		// Remove the label
		_ = client.RemoveLabel(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			github.LabelReactionMerge,
		)
	}

	// Check if cleanup reaction was removed
	if labelMap[github.LabelReactionCleanup] && !reactionMap[github.ReactionCleanup] {
		// Cleanup reaction was removed, just remove the label
		// (no action needed since cleanup is one-time operation)
		_ = client.RemoveLabel(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			github.LabelReactionCleanup,
		)
	}

	return nil
}

// handleReactionApprove handles approval via 👍 reaction.
func handleReactionApprove(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	approver string,
) error {
	// Get PR info to check existing approvals and prevent self-approval
	info, err := client.GetPRInfo(ctx, rc.RepoOwner, rc.RepoName, prNum)
	if err != nil {
		return postOperationFailure(
			ctx,
			client,
			rc,
			prNum,
			commentID,
			err,
			feedback.NewApprovalFailed,
			ErrApprovePR,
		)
	}

	// Prevent self-approval unless explicitly allowed
	if !bc.AllowSelfApproval && info.Author == approver {
		fb := feedback.NewUnauthorized(approver, []string{SelfApprovalNotAllowed})
		return PostFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionError)
	}

	// Check if bot already approved the PR (prevents duplicate approvals)
	if IsBotAlreadyApproved(info, rc.BotUsername) {
		// Bot already approved - skip approval but still add label
		_ = client.AddLabel(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			github.LabelReactionApprove,
		)
		return nil
	}

	// Approve the PR
	if err := client.ApprovePR(ctx, rc.RepoOwner, rc.RepoName, prNum); err != nil {
		return postOperationFailure(
			ctx,
			client,
			rc,
			prNum,
			commentID,
			err,
			feedback.NewApprovalFailed,
			ErrApprovePR,
		)
	}

	// Add label to track reaction-based approval
	_ = client.AddLabel(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		prNum,
		github.LabelReactionApprove,
	)

	// Post success feedback
	fb := feedback.NewReactionApprovalSuccess(approver, bc.QuietReactions)

	return PostFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionSuccess)
}

// handleReactionMerge handles merge via 🚀 reaction.
func handleReactionMerge(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	author string,
) error {
	// Get PR info to check if it's mergeable and prevent self-approval
	info, err := client.GetPRInfo(ctx, rc.RepoOwner, rc.RepoName, prNum)
	if err != nil {
		return postOperationFailure(
			ctx,
			client,
			rc,
			prNum,
			commentID,
			err,
			feedback.NewMergeFailed,
			ErrMergePR,
		)
	}

	// Prevent self-approval unless explicitly allowed (merge also approves)
	if !bc.AllowSelfApproval && info.Author == author {
		fb := feedback.NewUnauthorized(author, []string{SelfApprovalNotAllowed})
		return PostFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionError)
	}

	// Check if PR is mergeable
	if !info.Mergeable {
		return postNotMergeable(ctx, client, rc, prNum, commentID)
	}

	// Check if bot already approved the PR (prevents duplicate approvals from edits/reactions)
	botAlreadyApproved := IsBotAlreadyApproved(info, rc.BotUsername)

	// Check if user already approved the PR (avoid redundant bot approval)
	userAlreadyApproved := false
	for _, approver := range info.ApprovedBy {
		if approver == author {
			userAlreadyApproved = true
			break
		}
	}

	// Approve the PR if neither bot nor user has already approved
	if !botAlreadyApproved && !userAlreadyApproved {
		if err := client.ApprovePR(ctx, rc.RepoOwner, rc.RepoName, prNum); err != nil {
			return postOperationFailure(
				ctx,
				client,
				rc,
				prNum,
				commentID,
				err,
				feedback.NewApprovalFailed,
				ErrApprovePR,
			)
		}
	}

	// Merge the PR (using default merge method)
	if err := client.MergePR(ctx, rc.RepoOwner, rc.RepoName, prNum, github.MergeMethodMerge); err != nil {
		// Check if we should enable auto-merge instead
		if shouldEnableAutoMerge(err) {
			if err := client.EnableAutoMerge(
				ctx,
				rc.RepoOwner,
				rc.RepoName,
				prNum,
				github.MergeMethodMerge,
			); err != nil {
				return postOperationFailure(
					ctx,
					client,
					rc,
					prNum,
					commentID,
					err,
					feedback.NewAutoMergeFailed,
					ErrMergePR,
				)
			}

			// Add label to track reaction-based auto-merge
			_ = client.AddLabel(
				ctx,
				rc.RepoOwner,
				rc.RepoName,
				prNum,
				github.LabelReactionMerge,
			)

			// Post auto-merge enabled feedback
			fb := feedback.NewAutoMergeEnabled(author, bc.QuietReactions)
			return PostFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionSuccess)
		}

		return postOperationFailure(
			ctx,
			client,
			rc,
			prNum,
			commentID,
			err,
			feedback.NewMergeFailed,
			ErrMergePR,
		)
	}

	// Add label to track reaction-based merge
	_ = client.AddLabel(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		prNum,
		github.LabelReactionMerge,
	)

	// Post success feedback
	fb := feedback.NewReactionMergeSuccess(author, bc.QuietReactions)

	return PostFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionSuccess)
}

// handleReactionCleanup handles cleanup via ❤️ reaction.
func handleReactionCleanup(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	environment CommandEnvironment,
) error {
	fb, accepted, err := executeCoordinatedCleanup(
		ctx, client, rc, bc, prNum, commentID,
		"cleanup reaction", environment,
	)
	if err != nil {
		return err
	}
	if !accepted {
		return nil
	}

	// If cleanup failed, post error feedback
	if fb.Type == feedback.Error {
		return PostFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionError)
	}

	// Cleanup succeeded - the comment and reactions are already deleted by executeCleanup
	// Remove the label to track that cleanup completed
	_ = client.RemoveLabel(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		prNum,
		github.LabelReactionCleanup,
	)

	return nil
}
