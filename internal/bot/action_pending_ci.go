package bot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	PullRequestDraftedAfterLabel(context.Context, string, string, int, string, string) (bool, error)
	RemoveCommentReaction(context.Context, string, string, int, int64) error
	RemoveReactionByUser(
		context.Context, string, string, int, github.ReactionType, string,
	) error
}

type actionPendingCIArtifact struct {
	commentID int
	revision  string
	marker    github.Reaction
	pending   github.Reaction
	bound     bool
	legacy    bool
}

type actionPendingCIArtifactExclusion struct {
	commentID int
	fenceID   int64
}

type actionPendingCIArtifactMatch struct {
	artifact actionPendingCIArtifact
	found    bool
	legacy   bool
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
		if legacy {
			return client.PullRequestDraftedAfterLabel(
				ctx, owner, repository, pullRequest, label, botUsername,
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
		method, requiredOnly, botUsername, actionPendingCIArtifactExclusion{},
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
	exclusion actionPendingCIArtifactExclusion,
) ([]actionPendingCIArtifact, bool, error) {
	comments, err := client.GetPRComments(ctx, owner, repository, pullRequest)
	if err != nil {
		return nil, false, fmt.Errorf("read pending CI command comments: %w", err)
	}
	artifacts := make([]actionPendingCIArtifact, 0)
	legacy := false
	for _, comment := range comments {
		match, artifactErr := actionPendingCIArtifactForComment(
			ctx, client, botConfig, owner, repository, method, requiredOnly,
			botUsername, comment, exclusion,
		)
		if artifactErr != nil {
			return nil, false, artifactErr
		}
		legacy = legacy || match.legacy
		if match.found {
			artifacts = append(artifacts, match.artifact)
		}
	}

	return artifacts, legacy, nil
}

func actionPendingCIArtifactForComment(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	botConfig *config.Config,
	owner, repository string,
	method github.MergeMethod,
	requiredOnly bool,
	botUsername string,
	comment github.IssueCommentState,
	exclusion actionPendingCIArtifactExclusion,
) (actionPendingCIArtifactMatch, error) {
	parsed, err := commands.ParseCommand(comment.Body, botConfig)
	if err != nil || !actionPendingCICommandMatches(parsed, method, requiredOnly) {
		return actionPendingCIArtifactMatch{}, nil
	}
	marker, pending, rejected, err := actionPendingCICommentMarkers(
		ctx, client, owner, repository, int(comment.ID), botUsername,
	)
	if err != nil {
		return actionPendingCIArtifactMatch{}, err
	}
	if int(comment.ID) == exclusion.commentID &&
		exclusion.fenceID != 0 && pending.ID == exclusion.fenceID {
		return actionPendingCIArtifactMatch{}, nil
	}
	if marker.ID == 0 {
		if rejected.ID != 0 {
			return actionPendingCIArtifactMatch{}, nil
		}
		artifact := actionPendingCIArtifact{
			commentID: int(comment.ID), revision: comment.UpdatedAt,
			pending: pending, legacy: true,
		}

		found := pending.ID != 0

		return actionPendingCIArtifactMatch{
			artifact: artifact, found: found, legacy: found,
		}, nil
	}
	if pending.ID == 0 || rejected.ID != 0 ||
		!newerActionPendingCIMarker(marker, pending) {
		return actionPendingCIArtifactMatch{}, nil
	}
	revision, err := pendingci.ParseSourceRevision(comment.UpdatedAt)
	if err != nil {
		return actionPendingCIArtifactMatch{}, fmt.Errorf(
			"parse pending CI command revision on comment %d: %w", comment.ID, err,
		)
	}
	artifact := actionPendingCIArtifact{
		commentID: int(comment.ID), revision: revision.UTC().Format(time.RFC3339Nano),
		marker: marker, pending: pending, bound: pending.CreatedAt.After(revision),
	}

	return actionPendingCIArtifactMatch{artifact: artifact, found: true}, nil
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
) (github.Reaction, github.Reaction, github.Reaction, error) {
	reactions, err := client.GetCommentReactions(ctx, owner, repository, commentID)
	if err != nil {
		return github.Reaction{}, github.Reaction{}, github.Reaction{}, fmt.Errorf(
			"read pending CI command marker on comment %d: %w", commentID, err,
		)
	}
	var marker github.Reaction
	var pending github.Reaction
	var rejected github.Reaction
	for _, reaction := range reactions {
		if reaction.User != botUsername {
			continue
		}
		if reaction.Type == ReactionPendingCI && newerActionPendingCIMarker(reaction, pending) {
			pending = reaction
		}
		if reaction.Type == ReactionPendingCIAction &&
			newerActionPendingCIMarker(reaction, marker) {
			marker = reaction
		}
		if reaction.Type == ReactionPendingCIRejected &&
			newerActionPendingCIMarker(reaction, rejected) {
			rejected = reaction
		}
	}
	return marker, pending, rejected, nil
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
	if err := removeActionPendingCIReaction(
		ctx, client, owner, repository, commentID, markerID,
	); err != nil {
		return false, err
	}
	_ = client.RemoveReactionByUser(
		ctx, owner, repository, commentID, ReactionPendingCI, botUsername,
	)

	return true, nil
}

func removeActionPendingCIReaction(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	owner, repository string,
	commentID int,
	reactionID int64,
) error {
	err := client.RemoveCommentReaction(ctx, owner, repository, commentID, reactionID)
	var apiErr *github.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return nil
	}

	return err
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
	label string,
	currentlyDraft bool,
) (bool, error) {
	artifacts, _, err := actionPendingCIArtifacts(
		ctx, client, botConfig, owner, repository, pullRequest,
		method, requiredOnly, botUsername, actionPendingCIArtifactExclusion{},
	)
	if err != nil {
		return true, err
	}
	surviving := false
	var cleanupErrors []error
	for _, artifact := range artifacts {
		if artifact.legacy {
			legacySurvives, legacyErr := settleLegacyActionPendingCIArtifact(
				ctx, client, owner, repository, pullRequest,
				label, botUsername, artifact, currentlyDraft,
			)
			surviving = surviving || legacySurvives
			if legacyErr != nil {
				cleanupErrors = append(cleanupErrors, legacyErr)
			}

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

func settleLegacyActionPendingCIArtifact(
	ctx context.Context,
	client actionPendingCIArtifactClient,
	owner, repository string,
	pullRequest int,
	label string,
	botUsername string,
	artifact actionPendingCIArtifact,
	currentlyDraft bool,
) (bool, error) {
	cancelled := currentlyDraft
	var err error
	if !cancelled {
		cancelled, err = client.PullRequestDraftedAfterLabel(
			ctx, owner, repository, pullRequest, label, botUsername,
		)
	}
	if err != nil || !cancelled {
		return true, err
	}
	if err = removeActionPendingCIReaction(
		ctx, client, owner, repository, artifact.commentID, artifact.pending.ID,
	); err != nil {
		return true, err
	}
	_ = client.AddReaction(ctx, owner, repository, artifact.commentID, ReactionWarning)

	return false, nil
}
