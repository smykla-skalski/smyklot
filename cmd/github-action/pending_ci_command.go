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
	FinishPR(context.Context, pendingci.FinishPRRequest) (*pendingci.Request, error)
}

type pendingCIArtifactOwnership struct {
	label    bool
	reaction bool
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
	sourceRevision     string
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
		SourceRevision: command.sourceRevision, Label: label, RequestedAt: requestedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("persist pending CI command: %w", err)
	}
	command.wake()

	return result.Superseded, nil
}

func (s *server) commandEnvironment(event *webhook.IssueCommentEvent) commandEnvironment {
	return commandEnvironment{pendingCI: &pendingCICommand{
		store: s.store, wake: s.pendingCI.Wake,
		coordinator:        s.pendingCICoordinator,
		targetID:           installationStorageID(event.Installation.ID),
		installationID:     event.Installation.ID,
		repositoryID:       repositoryStorageID(event.Repository.ID),
		repositoryFullName: event.Repository.FullName,
		sourceRevision:     event.Comment.UpdatedAt,
		now:                func() time.Time { return time.Now().UTC() },
	}}
}

func (s *server) cancelEditedPendingCI(
	ctx context.Context,
	event *webhook.IssueCommentEvent,
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
			Reason: reason, CancelledAt: time.Now().UTC(),
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
) error {
	var request *pendingci.Request
	err := command.coordinator.Exclusive(ctx, command.repositoryID, func() error {
		var transitionErr error
		request, transitionErr = command.store.FinishPR(ctx, pendingci.FinishPRRequest{
			RepositoryID: command.repositoryID, PullRequest: pullRequest,
			Lifecycle: pendingci.LifecycleCancelled, Reason: reason, FinishedAt: command.now(),
		})

		return transitionErr
	})
	if err != nil {
		return fmt.Errorf("cancel pending CI command: %w", err)
	}
	if request != nil {
		command.wake()
	}

	return nil
}

func (command *pendingCICommand) exclusive(
	ctx context.Context,
	operation func() error,
) error {
	return command.coordinator.Exclusive(ctx, command.repositoryID, operation)
}
