package main

import (
	"context"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/feedback"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type pendingCIApprover interface {
	ApprovePR(context.Context, string, string, int) error
}

func preparePendingCIApproval(
	ctx context.Context,
	approver pendingCIApprover,
	runtime *RuntimeConfig,
	botConfig *config.Config,
	pullRequest int,
	info *github.PRInfo,
) *feedback.Feedback {
	if !botConfig.AllowSelfApproval && info.Author == runtime.CommentAuthor {
		return feedback.NewUnauthorized(
			runtime.CommentAuthor,
			[]string{selfApprovalNotAllowed},
		)
	}
	botApproved := isBotAlreadyApproved(info, runtime.BotUsername)
	userApproved := false
	for _, login := range info.ApprovedBy {
		if login == runtime.CommentAuthor {
			userApproved = true

			break
		}
	}
	if botApproved || userApproved {
		return nil
	}
	if err := approver.ApprovePR(
		ctx, runtime.RepoOwner, runtime.RepoName, pullRequest,
	); err != nil {
		return feedback.NewApprovalFailed(err.Error())
	}

	return nil
}
