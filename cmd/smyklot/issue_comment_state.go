package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type issueCommentStateReader interface {
	GetIssueComment(context.Context, string, string, int64) (github.IssueCommentState, error)
}

// issueCommentIsCurrent verifies mutable webhook content against GitHub before
// it can cancel or replace a command. Delivery order is not source order.
func issueCommentIsCurrent(
	ctx context.Context,
	reader issueCommentStateReader,
	event *webhook.IssueCommentEvent,
) (bool, error) {
	comment, err := reader.GetIssueComment(
		ctx,
		event.Repository.Owner.Login,
		event.Repository.Name,
		event.Comment.ID,
	)
	if err != nil {
		var apiErr *github.APIError
		missing := errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
		if missing {
			return event.Action == webhook.ActionDeleted, nil
		}

		return false, fmt.Errorf("read current issue comment: %w", err)
	}
	if event.Action == webhook.ActionDeleted {
		return false, nil
	}

	return comment.ID == event.Comment.ID &&
		comment.Body == event.Comment.Body &&
		comment.UpdatedAt == event.Comment.UpdatedAt &&
		comment.User.Login == event.Comment.User.Login &&
		comment.User.Type == event.Comment.User.Type, nil
}
