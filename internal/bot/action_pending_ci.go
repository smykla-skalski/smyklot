package bot

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/commands"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type actionPendingCIArtifactClient interface {
	draftMergeHistoryClient
	AddReaction(context.Context, string, string, int, github.ReactionType) error
	GetPRComments(context.Context, string, string, int) ([]github.IssueCommentState, error)
	GetIssueComment(context.Context, string, string, int64) (github.IssueCommentState, error)
	GetCommentReactions(context.Context, string, string, int) ([]github.Reaction, error)
	PullRequestDraftedAfterLabel(context.Context, string, string, int, string) (bool, error)
	RemoveReactionByUser(
		context.Context, string, string, int, github.ReactionType, string,
	) error
}

type actionPendingCIArtifact struct {
	commentID int
	revision  string
}

func actionPendingCIDraftCancelled(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	botConfig *config.Config,
	owner, repository string,
	pullRequest int,
	method github.MergeMethod,
	requiredOnly bool,
	label string,
	botUsername string,
	currentlyDraft bool,
) (bool, error) {
	if currentlyDraft {
		return true, nil
	}
	if !botConfig.AllowDraftMerges {
		return client.PullRequestDraftedAfterLabel(
			ctx, owner, repository, pullRequest, label,
		)
	}
	revision, found, err := latestActionPendingCIRevision(
		ctx, client, botConfig, owner, repository, pullRequest,
		method, requiredOnly, botUsername,
	)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	err = ValidateDraftMergeAuthorization(
		ctx, client, owner, repository, pullRequest, revision,
	)
	if errors.Is(err, pendingci.ErrStaleSourceRevision) ||
		errors.Is(err, pendingci.ErrAmbiguousSourceRevision) {
		return true, nil
	}

	return false, err
}

func latestActionPendingCIRevision(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	botConfig *config.Config,
	owner, repository string,
	pullRequest int,
	method github.MergeMethod,
	requiredOnly bool,
	botUsername string,
) (string, bool, error) {
	artifacts, err := actionPendingCIArtifacts(
		ctx, client, botConfig, owner, repository, pullRequest,
		method, requiredOnly, botUsername,
	)
	if err != nil {
		return "", false, err
	}
	var latest time.Time
	for _, artifact := range artifacts {
		revision, revisionErr := pendingci.ParseSourceRevision(artifact.revision)
		if revisionErr != nil {
			return "", false, revisionErr
		}
		if revision.After(latest) {
			latest = revision
		}
	}
	if latest.IsZero() {
		return "", false, nil
	}

	return latest.UTC().Format(time.RFC3339Nano), true, nil
}

func actionPendingCIArtifacts(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	botConfig *config.Config,
	owner, repository string,
	pullRequest int,
	method github.MergeMethod,
	requiredOnly bool,
	botUsername string,
) ([]actionPendingCIArtifact, error) {
	comments, err := client.GetPRComments(ctx, owner, repository, pullRequest)
	if err != nil {
		return nil, fmt.Errorf("read pending CI command comments: %w", err)
	}
	artifacts := make([]actionPendingCIArtifact, 0)
	for _, comment := range comments {
		parsed, parseErr := commands.ParseCommand(comment.Body, botConfig)
		if parseErr != nil || !actionPendingCICommandMatches(parsed, method, requiredOnly) {
			continue
		}
		marked, markerErr := commentHasActionPendingCIMarker(
			ctx, client, owner, repository, int(comment.ID), botUsername,
		)
		if markerErr != nil {
			return nil, markerErr
		}
		if !marked {
			continue
		}
		revision, revisionErr := pendingci.ParseSourceRevision(comment.UpdatedAt)
		if revisionErr != nil {
			return nil, fmt.Errorf(
				"parse pending CI command revision on comment %d: %w", comment.ID, revisionErr,
			)
		}
		artifacts = append(artifacts, actionPendingCIArtifact{
			commentID: int(comment.ID),
			revision:  revision.UTC().Format(time.RFC3339Nano),
		})
	}

	return artifacts, nil
}

func actionPendingCICommandMatches(
	command commands.Command,
	method github.MergeMethod,
	requiredOnly bool,
) bool {
	if !command.IsValid || !command.WaitForCI || command.RequiredChecksOnly != requiredOnly {
		return false
	}
	wanted := commands.CommandMerge
	switch method {
	case github.MergeMethodSquash:
		wanted = commands.CommandSquash
	case github.MergeMethodRebase:
		wanted = commands.CommandRebase
	}

	return slices.Contains(command.Commands, wanted)
}

func commentHasActionPendingCIMarker(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	owner, repository string,
	commentID int,
	botUsername string,
) (bool, error) {
	reactions, err := client.GetCommentReactions(ctx, owner, repository, commentID)
	if err != nil {
		return false, fmt.Errorf("read pending CI command marker on comment %d: %w", commentID, err)
	}
	for _, reaction := range reactions {
		if reaction.User == botUsername && reaction.Type == ReactionPendingCIAction {
			return true, nil
		}
	}

	return false, nil
}

func removeActionPendingCIArtifactIfOwned(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	owner, repository string,
	commentID int,
	botUsername, sourceRevision string,
) error {
	comment, err := client.GetIssueComment(ctx, owner, repository, int64(commentID))
	if err != nil {
		return fmt.Errorf("read pending CI command before rollback: %w", err)
	}
	currentRevision, err := pendingci.ParseSourceRevision(comment.UpdatedAt)
	if err != nil {
		return err
	}
	expectedRevision, err := pendingci.ParseSourceRevision(sourceRevision)
	if err != nil {
		return err
	}
	if !currentRevision.Equal(expectedRevision) {
		return nil
	}

	return errors.Join(
		client.RemoveReactionByUser(
			ctx, owner, repository, commentID, ReactionPendingCIAction, botUsername,
		),
		client.RemoveReactionByUser(
			ctx, owner, repository, commentID, ReactionPendingCI, botUsername,
		),
	)
}

func settleCancelledActionPendingCI(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	botConfig *config.Config,
	owner, repository string,
	pullRequest int,
	method github.MergeMethod,
	requiredOnly bool,
	botUsername string,
) error {
	artifacts, err := actionPendingCIArtifacts(
		ctx, client, botConfig, owner, repository, pullRequest,
		method, requiredOnly, botUsername,
	)
	if err != nil {
		return err
	}
	var cleanupErrors []error
	for _, artifact := range artifacts {
		err := ValidateDraftMergeAuthorization(
			ctx, client, owner, repository, pullRequest, artifact.revision,
		)
		if err == nil {
			continue
		}
		if !errors.Is(err, pendingci.ErrStaleSourceRevision) &&
			!errors.Is(err, pendingci.ErrAmbiguousSourceRevision) {
			cleanupErrors = append(cleanupErrors, err)

			continue
		}
		if err := removeActionPendingCIArtifactIfOwned(
			ctx, client, owner, repository, artifact.commentID,
			botUsername, artifact.revision,
		); err != nil {
			cleanupErrors = append(cleanupErrors, err)

			continue
		}
		if err := client.AddReaction(
			ctx, owner, repository, artifact.commentID, ReactionWarning,
		); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}

	return errors.Join(cleanupErrors...)
}
