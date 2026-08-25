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
	AddReactionState(
		context.Context, string, string, int, github.ReactionType,
	) (github.Reaction, error)
	GetPRComments(context.Context, string, string, int) ([]github.IssueCommentState, error)
	GetIssueComment(context.Context, string, string, int64) (github.IssueCommentState, error)
	GetCommentReactions(context.Context, string, string, int) ([]github.Reaction, error)
	PullRequestDraftedAfterLabel(context.Context, string, string, int, string) (bool, error)
	RemoveCommentReaction(context.Context, string, string, int, int64) error
	RemoveReactionByUser(
		context.Context, string, string, int, github.ReactionType, string,
	) error
}

type actionPendingCIArtifact struct {
	commentID int
	revision  string
	marker    github.Reaction
	bound     bool
	legacy    bool
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
	revision, found, legacy, err := latestActionPendingCIRevision(
		ctx, client, botConfig, owner, repository, pullRequest,
		method, requiredOnly, botUsername,
	)
	if err != nil {
		return false, err
	}
	if !found {
		if legacy || !botConfig.AllowDraftMerges {
			return client.PullRequestDraftedAfterLabel(
				ctx, owner, repository, pullRequest, label,
			)
		}

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
) (string, bool, bool, error) {
	artifacts, legacy, err := actionPendingCIArtifacts(
		ctx, client, botConfig, owner, repository, pullRequest,
		method, requiredOnly, botUsername,
	)
	if err != nil {
		return "", false, false, err
	}
	var latest time.Time
	for _, artifact := range artifacts {
		if !artifact.bound {
			continue
		}
		revision, revisionErr := pendingci.ParseSourceRevision(artifact.revision)
		if revisionErr != nil {
			return "", false, false, revisionErr
		}
		if revision.After(latest) {
			latest = revision
		}
	}
	if latest.IsZero() {
		return "", false, legacy, nil
	}

	return latest.UTC().Format(time.RFC3339Nano), true, legacy, nil
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
) ([]actionPendingCIArtifact, bool, error) {
	comments, err := client.GetPRComments(ctx, owner, repository, pullRequest)
	if err != nil {
		return nil, false, fmt.Errorf("read pending CI command comments: %w", err)
	}
	artifacts := make([]actionPendingCIArtifact, 0)
	legacy := false
	for _, comment := range comments {
		parsed, parseErr := commands.ParseCommand(comment.Body, botConfig)
		if parseErr != nil || !actionPendingCICommandMatches(parsed, method, requiredOnly) {
			continue
		}
		marker, marked, eyes, markerErr := actionPendingCICommentMarkers(
			ctx, client, owner, repository, int(comment.ID), botUsername,
		)
		if markerErr != nil {
			return nil, false, markerErr
		}
		if !marked {
			if eyes {
				legacy = true
				artifacts = append(artifacts, actionPendingCIArtifact{
					commentID: int(comment.ID),
					revision:  comment.UpdatedAt,
					legacy:    true,
				})
			}

			continue
		}
		revision, revisionErr := pendingci.ParseSourceRevision(comment.UpdatedAt)
		if revisionErr != nil {
			return nil, false, fmt.Errorf(
				"parse pending CI command revision on comment %d: %w", comment.ID, revisionErr,
			)
		}
		artifacts = append(artifacts, actionPendingCIArtifact{
			commentID: int(comment.ID),
			revision:  revision.UTC().Format(time.RFC3339Nano),
			marker:    marker,
			bound:     marker.CreatedAt.After(revision),
		})
	}

	return artifacts, legacy, nil
}

func migrateLegacyActionPendingCIArtifacts(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	botConfig *config.Config,
	owner, repository string,
	pullRequest int,
	method github.MergeMethod,
	requiredOnly bool,
	botUsername string,
) error {
	artifacts, _, err := actionPendingCIArtifacts(
		ctx, client, botConfig, owner, repository, pullRequest,
		method, requiredOnly, botUsername,
	)
	if err != nil {
		return err
	}
	for _, artifact := range artifacts {
		if !artifact.legacy {
			continue
		}
		marker, markerErr := client.AddReactionState(
			ctx, owner, repository, artifact.commentID, ReactionPendingCIAction,
		)
		if markerErr == nil {
			markerErr = validateActionPendingCIMarker(marker, artifact.revision)
		}
		if markerErr == nil {
			continue
		}
		if marker.ID != 0 {
			markerErr = errors.Join(markerErr, client.RemoveCommentReaction(
				ctx, owner, repository, artifact.commentID, marker.ID,
			))
		}

		return fmt.Errorf("migrate legacy pending CI command: %w", markerErr)
	}

	return nil
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

func actionPendingCICommentMarkers(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	owner, repository string,
	commentID int,
	botUsername string,
) (github.Reaction, bool, bool, error) {
	reactions, err := client.GetCommentReactions(ctx, owner, repository, commentID)
	if err != nil {
		return github.Reaction{}, false, false, fmt.Errorf(
			"read pending CI command marker on comment %d: %w", commentID, err,
		)
	}
	var marker github.Reaction
	eyes := false
	for _, reaction := range reactions {
		if reaction.User != botUsername {
			continue
		}
		if reaction.Type == ReactionPendingCI {
			eyes = true
		}
		if reaction.Type == ReactionPendingCIAction && newerActionPendingCIMarker(reaction, marker) {
			marker = reaction
		}
	}

	return marker, marker.ID != 0, eyes, nil
}

func newerActionPendingCIMarker(candidate, current github.Reaction) bool {
	return current.ID == 0 || candidate.CreatedAt.After(current.CreatedAt) ||
		candidate.CreatedAt.Equal(current.CreatedAt) && candidate.ID > current.ID
}

func validateActionPendingCIMarker(marker github.Reaction, sourceRevision string) error {
	source, err := pendingci.ParseSourceRevision(sourceRevision)
	if err != nil {
		return err
	}
	if marker.ID == 0 || marker.CreatedAt.IsZero() {
		return errors.New("GitHub returned an incomplete pending CI command marker; reissue the command")
	}
	if marker.CreatedAt.Before(source) {
		return fmt.Errorf(
			"%w: GitHub recorded the pending CI marker before the command revision; reissue the command",
			pendingci.ErrStaleSourceRevision,
		)
	}
	if marker.CreatedAt.Equal(source) {
		return fmt.Errorf(
			"%w: GitHub recorded the pending CI marker and command revision in the same second; reissue the command",
			pendingci.ErrAmbiguousSourceRevision,
		)
	}

	return nil
}

func removeActionPendingCIArtifactIfOwned(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	owner, repository string,
	commentID int,
	markerID int64,
	botUsername, sourceRevision string,
) (bool, error) {
	comment, err := client.GetIssueComment(ctx, owner, repository, int64(commentID))
	if err != nil {
		return false, fmt.Errorf("read pending CI command before rollback: %w", err)
	}
	currentRevision, err := pendingci.ParseSourceRevision(comment.UpdatedAt)
	if err != nil {
		return false, err
	}
	expectedRevision, err := pendingci.ParseSourceRevision(sourceRevision)
	if err != nil {
		return false, err
	}
	if !currentRevision.Equal(expectedRevision) {
		return false, nil
	}
	if err := client.RemoveCommentReaction(
		ctx, owner, repository, commentID, markerID,
	); err != nil {
		return false, err
	}
	_ = client.RemoveReactionByUser(
		ctx, owner, repository, commentID, ReactionPendingCI, botUsername,
	)

	return true, nil
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
) (bool, error) {
	artifacts, legacy, err := actionPendingCIArtifacts(
		ctx, client, botConfig, owner, repository, pullRequest,
		method, requiredOnly, botUsername,
	)
	if err != nil {
		return true, err
	}
	surviving := legacy
	var cleanupErrors []error
	for _, artifact := range artifacts {
		if artifact.legacy {
			continue
		}
		var authorizationErr error
		if artifact.bound {
			authorizationErr = ValidateDraftMergeAuthorization(
				ctx, client, owner, repository, pullRequest, artifact.revision,
			)
		} else {
			authorizationErr = pendingci.ErrAmbiguousSourceRevision
		}
		if authorizationErr == nil {
			surviving = true

			continue
		}
		if !errors.Is(authorizationErr, pendingci.ErrStaleSourceRevision) &&
			!errors.Is(authorizationErr, pendingci.ErrAmbiguousSourceRevision) {
			cleanupErrors = append(cleanupErrors, authorizationErr)
			surviving = true

			continue
		}
		removed, err := removeActionPendingCIArtifactIfOwned(
			ctx, client, owner, repository, artifact.commentID,
			artifact.marker.ID, botUsername, artifact.revision,
		)
		if err != nil {
			cleanupErrors = append(cleanupErrors, err)
			surviving = true

			continue
		}
		if !removed {
			surviving = true

			continue
		}
		_ = client.AddReaction(
			ctx, owner, repository, artifact.commentID, ReactionWarning,
		)
	}

	return surviving, errors.Join(cleanupErrors...)
}
