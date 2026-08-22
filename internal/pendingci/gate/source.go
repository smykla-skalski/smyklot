package gate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/commands"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type sourceValidator struct {
	config RepositoryConfig
}

// CancellationReason re-reads the mutable command during fallback polling.
// Webhooks stay the fast path; this closes missed edit and delete deliveries.
func (validator sourceValidator) CancellationReason(
	ctx context.Context,
	client *github.Client,
	request pendingci.Request,
	owner, repository string,
) (string, error) {
	comment, err := client.GetIssueComment(
		ctx, owner, repository, request.SourceCommentID,
	)
	if err != nil {
		var apiErr *github.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return "source comment deleted", nil
		}

		return "", fmt.Errorf("read pending CI source comment: %w", err)
	}
	botConfig, err := validator.config(
		ctx, client, request.TargetID, request.RepositoryID, owner, repository,
	)
	if err != nil {
		return "", fmt.Errorf("read pending CI source configuration: %w", err)
	}
	if !sourceMatches(comment, request, botConfig) {
		return "source comment edited", nil
	}

	return "", nil
}

func sourceMatches(
	comment github.IssueCommentState,
	request pendingci.Request,
	botConfig *config.Config,
) bool {
	if comment.ID != request.SourceCommentID ||
		!strings.EqualFold(comment.User.Login, request.Requester) {
		return false
	}
	parsed, err := commands.ParseCommand(comment.Body, botConfig)
	if err != nil || !parsed.IsValid || !parsed.WaitForCI ||
		parsed.RequiredChecksOnly != request.RequiredChecksOnly {
		return false
	}
	for _, command := range parsed.Commands {
		if command == commands.CommandHelp || command == commands.CommandUnapprove {
			return false
		}
	}
	expected := commands.CommandMerge
	switch request.MergeMethod {
	case pendingci.MergeMethodSquash:
		expected = commands.CommandSquash
	case pendingci.MergeMethodRebase:
		expected = commands.CommandRebase
	}
	for _, command := range parsed.Commands {
		if command == expected {
			return comment.UpdatedAt == request.SourceRevision
		}
	}

	return false
}
