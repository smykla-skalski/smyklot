package bot

import (
	"context"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/feedback"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

const selfApprovalNotAllowed = "(self-approval not allowed)"

type pendingCIApprover interface {
	ApprovePR(context.Context, string, string, int) error
}

func PendingCIApprovalAllowed(
	runtime *RuntimeConfig,
	botConfig *config.Config,
	info *github.PRInfo,
) *feedback.Feedback {
	if !botConfig.AllowSelfApproval && info.Author == runtime.CommentAuthor {
		return feedback.NewUnauthorized(
			runtime.CommentAuthor,
			[]string{selfApprovalNotAllowed},
		)
	}

	return nil
}

func PendingCIApprovalRequired(
	runtime *RuntimeConfig,
	info *github.PRInfo,
) bool {
	botApproved := isBotAlreadyApproved(info, runtime.BotUsername)
	for _, login := range info.ApprovedBy {
		if login == runtime.CommentAuthor {
			return false
		}
	}

	return !botApproved
}

func approvePendingCI(
	ctx context.Context,
	approver pendingCIApprover,
	runtime *RuntimeConfig,
	pullRequest int,
	required bool,
) *feedback.Feedback {
	if !required {
		return nil
	}
	if err := approver.ApprovePR(
		ctx, runtime.RepoOwner, runtime.RepoName, pullRequest,
	); err != nil {
		return feedback.NewApprovalFailed(err.Error())
	}

	return nil
}
