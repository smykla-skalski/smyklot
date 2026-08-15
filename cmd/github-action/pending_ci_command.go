package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type pendingCICommandStore interface {
	Arm(context.Context, pendingci.ArmRequest) (pendingci.ArmResult, error)
	GetArmed(context.Context, string, int) (pendingci.Request, error)
	CancelBySource(context.Context, pendingci.CancelRequest) (*pendingci.Request, error)
	CancelByIntent(context.Context, pendingci.CancelIntentRequest) (pendingci.CancelIntentResult, error)
}

type pendingCIArtifactOwnership struct {
	label         bool
	reaction      bool
	serviceMarker bool
}

func (command *pendingCICommand) armedArtifactOwnership(
	ctx context.Context,
	pullRequest int,
	label string,
	commentID int,
) (pendingCIArtifactOwnership, error) {
	request, err := command.store.GetArmed(ctx, command.repositoryID, pullRequest)
	if errors.Is(err, storage.ErrNotFound) {
		return pendingCIArtifactOwnership{}, nil
	}
	if err != nil {
		return pendingCIArtifactOwnership{}, fmt.Errorf(
			"read pending CI artifact owner: %w", err,
		)
	}

	return pendingCIArtifactOwnership{
		label: request.Label == label, reaction: request.SourceCommentID == int64(commentID),
		serviceMarker: true,
	}, nil
}

type commandEnvironment struct {
	pendingCI *pendingCICommand
}

// pendingCICommand translates an already-authorized command into durable
// domain state. It knows neither parsing nor reconciliation policy.
type pendingCICommand struct {
	store              pendingCICommandStore
	wake               func()
	coordinator        pendingCIExclusive
	targetID           string
	installationID     int64
	repositoryID       string
	repositoryFullName string
	sourceCommentID    int64
	sourceRevision     string
	sourceSequence     int
	sourceOrder        int64
	now                func() time.Time
}

func (command *pendingCICommand) arm(
	ctx context.Context,
	runtime *RuntimeConfig,
	pullRequest, commentID int,
	headSHA, baseBranch string,
	method github.MergeMethod,
	requiredChecksOnly bool,
	label string,
) (*pendingci.Request, error) {
	requestedAt := command.now()
	result, err := command.store.Arm(ctx, pendingci.ArmRequest{
		TargetID: command.targetID, InstallationID: command.installationID,
		RepositoryID: command.repositoryID, RepositoryFullName: command.repositoryFullName,
		PullRequest: pullRequest, HeadSHA: headSHA, BaseBranch: baseBranch,
		MergeMethod: pendingci.MergeMethod(method), RequiredChecksOnly: requiredChecksOnly,
		Requester: runtime.CommentAuthor, SourceCommentID: int64(commentID),
		SourceRevision: command.sourceRevision, SourceSequence: command.sourceSequence,
		SourceOrder: command.sourceOrder,
		Label:       label, RequestedAt: requestedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("persist pending CI command: %w", err)
	}
	command.wake()

	return result.Superseded, nil
}

func (s *server) commandEnvironment(
	event *webhook.IssueCommentEvent,
	sourceOrder int64,
) commandEnvironment {
	return commandEnvironment{pendingCI: &pendingCICommand{
		store: s.store, wake: s.pendingCI.Wake,
		coordinator:        s.pendingCICoordinator,
		targetID:           installationStorageID(event.Installation.ID),
		installationID:     event.Installation.ID,
		repositoryID:       repositoryStorageID(event.Repository.ID),
		repositoryFullName: event.Repository.FullName,
		sourceCommentID:    event.Comment.ID,
		sourceRevision:     event.Comment.UpdatedAt,
		sourceSequence:     event.SourceSequence(),
		sourceOrder:        sourceOrder,
		now:                func() time.Time { return time.Now().UTC() },
	}}
}

func (s *server) cancelEditedPendingCI(
	ctx context.Context,
	event *webhook.IssueCommentEvent,
	sourceOrder int64,
) error {
	if event.Action != webhook.ActionEdited && event.Action != webhook.ActionDeleted {
		return nil
	}
	reason := "source comment edited"
	if event.Action == webhook.ActionDeleted {
		reason = "source comment deleted"
	}
	repositoryID := repositoryStorageID(event.Repository.ID)
	var request *pendingci.Request
	err := s.pendingCICoordinator.Exclusive(ctx, repositoryID, func() error {
		var transitionErr error
		request, transitionErr = s.store.CancelBySource(ctx, pendingci.CancelRequest{
			RepositoryID: repositoryID,
			PullRequest:  event.Issue.Number, CommentID: event.Comment.ID,
			SourceRevision: event.Comment.UpdatedAt, SourceSequence: event.SourceSequence(),
			SourceOrder: sourceOrder,
			Reason:      reason, CancelledAt: time.Now().UTC(),
		})

		return transitionErr
	})
	if err != nil {
		return fmt.Errorf("cancel pending CI source: %w", err)
	}
	if request != nil {
		s.pendingCI.Wake()
	}

	return nil
}

func (command *pendingCICommand) cancelPullRequest(
	ctx context.Context,
	pullRequest int,
	reason string,
) (bool, error) {
	var result pendingci.CancelIntentResult
	err := command.coordinator.Exclusive(ctx, command.repositoryID, func() error {
		var transitionErr error
		result, transitionErr = command.cancelPullRequestLocked(ctx, pullRequest, reason)

		return transitionErr
	})
	if err != nil {
		return false, fmt.Errorf("cancel pending CI command: %w", err)
	}
	if result.Request != nil {
		command.wake()
	}

	return result.Accepted, nil
}

func (command *pendingCICommand) cancelPullRequestLocked(
	ctx context.Context,
	pullRequest int,
	reason string,
) (pendingci.CancelIntentResult, error) {
	return command.store.CancelByIntent(ctx, pendingci.CancelIntentRequest{
		RepositoryID: command.repositoryID, PullRequest: pullRequest,
		CommentID: command.sourceCommentID, SourceRevision: command.sourceRevision,
		SourceSequence: command.sourceSequence, SourceOrder: command.sourceOrder,
		Reason: reason, CancelledAt: command.now(),
	})
}

func (command *pendingCICommand) exclusive(
	ctx context.Context,
	operation func() error,
) error {
	return command.coordinator.Exclusive(ctx, command.repositoryID, operation)
}
