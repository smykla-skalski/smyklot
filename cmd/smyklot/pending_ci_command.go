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
	CheckArm(context.Context, pendingci.ArmRequest) error
	Arm(context.Context, pendingci.ArmRequest) (pendingci.ArmResult, error)
	GetArmed(context.Context, string, int) (pendingci.Request, error)
	CancelBySource(context.Context, pendingci.CancelRequest) (*pendingci.Request, error)
	CancelByIntent(context.Context, pendingci.CancelIntentRequest) (pendingci.CancelIntentResult, error)
	FinishPR(context.Context, pendingci.FinishPRRequest) (*pendingci.Request, error)
}

type pendingCIArtifactOwnership struct {
	label        bool
	serviceFence bool
}

func (command *pendingCICommand) armedArtifactOwnership(
	ctx context.Context,
	pullRequest int,
	label string,
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
		label: request.Label == label, serviceFence: true,
	}, nil
}

type commandEnvironment struct {
	pendingCI           *pendingCICommand
	pendingCIActivation pendingCIActivationGuard
	pendingCIMode       pendingCIModeResolver
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
	checks             *githubPendingCIChecks
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
	result, err := command.store.Arm(ctx, command.armRequest(
		runtime, pullRequest, commentID, headSHA, baseBranch,
		method, requiredChecksOnly, label,
	))
	if err != nil {
		return nil, fmt.Errorf("persist pending CI command: %w", err)
	}
	command.wake()

	return result.Superseded, nil
}

func (command *pendingCICommand) armCheck(
	ctx context.Context,
	runtime *RuntimeConfig,
	pullRequest, commentID int,
	headSHA, baseBranch string,
	method github.MergeMethod,
	requiredChecksOnly bool,
	checkSlotID int64,
) (*pendingci.Request, error) {
	request := command.armRequest(
		runtime, pullRequest, commentID, headSHA, baseBranch,
		method, requiredChecksOnly, "",
	)
	request.ArtifactKind = pendingci.ArtifactCheck
	request.CheckSlotID = &checkSlotID
	result, err := command.store.Arm(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("persist pending CI command: %w", err)
	}
	command.wake()

	return result.Superseded, nil
}

func (command *pendingCICommand) checkArm(
	ctx context.Context,
	runtime *RuntimeConfig,
	pullRequest, commentID int,
	headSHA, baseBranch string,
	method github.MergeMethod,
	requiredChecksOnly bool,
	label string,
) error {
	return command.store.CheckArm(ctx, command.armRequest(
		runtime, pullRequest, commentID, headSHA, baseBranch,
		method, requiredChecksOnly, label,
	))
}

func (command *pendingCICommand) armRequest(
	runtime *RuntimeConfig,
	pullRequest, commentID int,
	headSHA, baseBranch string,
	method github.MergeMethod,
	requiredChecksOnly bool,
	label string,
) pendingci.ArmRequest {
	requestedAt := command.now()

	return pendingci.ArmRequest{
		TargetID: command.targetID, InstallationID: command.installationID,
		RepositoryID: command.repositoryID, RepositoryFullName: command.repositoryFullName,
		PullRequest: pullRequest, HeadSHA: headSHA, BaseBranch: baseBranch,
		MergeMethod: pendingci.MergeMethod(method), RequiredChecksOnly: requiredChecksOnly,
		Requester: runtime.CommentAuthor, SourceCommentID: int64(commentID),
		SourceRevision: command.sourceRevision, SourceSequence: command.sourceSequence,
		SourceOrder:  command.sourceOrder,
		ArtifactKind: pendingci.ArtifactLabel,
		Label:        label, RequestedAt: requestedAt,
	}
}

func (s *server) commandEnvironment(
	client *github.Client,
	event *webhook.IssueCommentEvent,
	sourceOrder int64,
) commandEnvironment {
	guard := githubPendingCIActivationGuard{
		server: s, client: client,
		targetID:     installationStorageID(event.Installation.ID),
		repositoryID: repositoryStorageID(event.Repository.ID),
		owner:        event.Repository.Owner.Login,
		repository:   event.Repository.Name,
	}
	return commandEnvironment{
		pendingCIActivation: guard,
		pendingCIMode:       guard,
		pendingCI: &pendingCICommand{
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
			checks:             s.pendingCIChecks,
		},
	}
}

// cancelAndRun keeps durable cancellation and its external cleanup in one
// repository-owned operation. A newer command cannot arm between the two.
func (command *pendingCICommand) cancelAndRun(
	ctx context.Context,
	pullRequest int,
	reason string,
	operation func() error,
) (bool, error) {
	if operation == nil {
		return false, errors.New("pending CI cleanup operation is required")
	}
	var result pendingci.CancelIntentResult
	err := command.coordinator.Exclusive(ctx, command.repositoryID, func() error {
		var transitionErr error
		result, transitionErr = command.cancelPullRequestLocked(ctx, pullRequest, reason)
		if transitionErr != nil || !result.Accepted {
			return transitionErr
		}
		if result.Request != nil && result.Request.ArtifactKind == pendingci.ArtifactCheck {
			if err := command.releaseBlockingCheck(ctx, *result.Request); err != nil {
				return err
			}
		}

		return operation()
	})
	if result.Request != nil {
		command.wake()
	}
	if err != nil {
		return false, fmt.Errorf("cancel pending CI command: %w", err)
	}

	return result.Accepted, nil
}

func (command *pendingCICommand) releaseBlockingCheck(
	ctx context.Context,
	request pendingci.Request,
) error {
	if command.checks == nil || request.CheckSlotID == nil {
		return errors.New("pending CI check cleanup is unavailable")
	}
	slot, err := command.checks.store.GetCheckSlot(ctx, *request.CheckSlotID)
	if err != nil {
		return fmt.Errorf("read pending CI check cleanup slot: %w", err)
	}
	target := storage.Target{
		ID: slot.TargetID, InstallationID: fmt.Sprint(slot.InstallationID),
	}
	repository := storage.Repository{ID: slot.RepositoryID, FullName: slot.RepositoryFullName}
	if _, err := command.checks.EnsureBaseline(
		ctx,
		target,
		repository,
		slot.PullRequest,
		slot.HeadSHA,
	); err != nil {
		return fmt.Errorf("release pending CI required check: %w", err)
	}

	return nil
}

func (command *pendingCICommand) cancelPullRequestLocked(
	ctx context.Context,
	pullRequest int,
	reason string,
) (pendingci.CancelIntentResult, error) {
	if command.sourceCommentID == 0 {
		request, err := command.store.FinishPR(ctx, pendingci.FinishPRRequest{
			RepositoryID: command.repositoryID, PullRequest: pullRequest,
			Lifecycle: pendingci.LifecycleCancelled, Trigger: pendingci.TriggerFallback,
			Reason: reason, FinishedAt: command.now(),
		})

		return pendingci.CancelIntentResult{Accepted: true, Request: request}, err
	}

	return command.store.CancelByIntent(ctx, pendingci.CancelIntentRequest{
		RepositoryID: command.repositoryID, PullRequest: pullRequest,
		CommentID: command.sourceCommentID, SourceRevision: command.sourceRevision,
		SourceSequence: command.sourceSequence, SourceOrder: command.sourceOrder,
		Reason: reason, CancelledAt: command.now(),
	})
}

func (s *server) reactionCommandEnvironment(repositoryID string) commandEnvironment {
	return commandEnvironment{pendingCI: &pendingCICommand{
		store: s.store, wake: s.pendingCI.Wake,
		coordinator: s.pendingCICoordinator, repositoryID: repositoryID,
		now: func() time.Time { return time.Now().UTC() },
	}}
}

func (command *pendingCICommand) exclusive(
	ctx context.Context,
	operation func() error,
) error {
	return command.coordinator.Exclusive(ctx, command.repositoryID, operation)
}
