package gate

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type sourceClaimStore interface {
	ClaimSourceRevision(
		context.Context,
		pendingci.SourceRevisionRequest,
	) (pendingci.SourceRevisionResult, error)
	CancelBySource(context.Context, pendingci.CancelRequest) (*pendingci.Request, error)
}

type sourceClaimResult struct {
	Source    pendingci.SourceRevisionResult
	Cancelled *pendingci.Request
}

// ClaimSource serializes mutable comment ordering with activation.
// An edit therefore wins before activation preflight or waits until the
// request is armed and can be terminalized normally.
func ClaimSource(
	ctx context.Context,
	store sourceClaimStore,
	exclusive bot.Exclusive,
	source pendingci.SourceRevisionRequest,
	cancellation *pendingci.CancelRequest,
) (sourceClaimResult, error) {
	var result sourceClaimResult
	err := exclusive.Exclusive(ctx, source.RepositoryID, func() error {
		var err error
		result.Source, err = store.ClaimSourceRevision(ctx, source)
		if err != nil || !result.Source.Accepted || cancellation == nil {
			return err
		}
		change := *cancellation
		change.SourceOrder = result.Source.SourceOrder
		result.Cancelled, err = store.CancelBySource(ctx, change)

		return err
	})
	if err != nil {
		return sourceClaimResult{}, fmt.Errorf("coordinate pending CI source: %w", err)
	}

	return result, nil
}

func SourceCancellation(
	event *webhook.IssueCommentEvent,
	repositoryID string,
) *pendingci.CancelRequest {
	if event.Action != webhook.ActionEdited && event.Action != webhook.ActionDeleted {
		return nil
	}
	reason := "source comment edited"
	if event.Action == webhook.ActionDeleted {
		reason = "source comment deleted"
	}

	return &pendingci.CancelRequest{
		RepositoryID: repositoryID,
		PullRequest:  event.Issue.Number, CommentID: event.Comment.ID,
		SourceRevision: event.Comment.UpdatedAt,
		SourceSequence: pendingci.CommentSequence(event.Action),
		Trigger:        pendingci.TriggerWebhook,
		Reason:         reason,
		CancelledAt:    time.Now().UTC(),
	}
}
