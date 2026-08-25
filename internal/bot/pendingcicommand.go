package bot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type PendingCICommandStore interface {
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

func (command *PendingCICommand) armedArtifactOwnership(
	ctx context.Context,
	pullRequest int,
	label string,
) (pendingCIArtifactOwnership, error) {
	request, err := command.Store.GetArmed(ctx, command.RepositoryID, pullRequest)
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

type CommandEnvironment struct {
	PendingCI           *PendingCICommand
	PendingCIActivation PendingCIActivationGuard
	PendingCIMode       PendingCIModeResolver
	DraftMergeRevision  string
}

// PendingCICommand translates an already-authorized command into durable
// domain state. It knows neither parsing nor reconciliation policy.
type PendingCICommand struct {
	Store              PendingCICommandStore
	Wake               func()
	Coordinator        Exclusive
	TargetID           string
	InstallationID     int64
	RepositoryID       string
	RepositoryFullName string
	SourceCommentID    int64
	SourceRevision     string
	SourceSequence     int
	SourceOrder        int64
	Now                func() time.Time
	Checks             PendingCIChecks
}

func (command *PendingCICommand) arm(
	ctx context.Context,
	runtime *RuntimeConfig,
	pullRequest, commentID int,
	headSHA, baseBranch string,
	method github.MergeMethod,
	requiredChecksOnly bool,
	label string,
) (*pendingci.Request, error) {
	result, err := command.Store.Arm(ctx, command.armRequest(
		runtime, pullRequest, commentID, headSHA, baseBranch,
		method, requiredChecksOnly, label,
	))
	if err != nil {
		return nil, fmt.Errorf("persist pending CI command: %w", err)
	}
	command.Wake()

	return result.Superseded, nil
}

func (command *PendingCICommand) armCheck(
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
	result, err := command.Store.Arm(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("persist pending CI command: %w", err)
	}
	command.Wake()

	return result.Superseded, nil
}

func (command *PendingCICommand) checkArm(
	ctx context.Context,
	runtime *RuntimeConfig,
	pullRequest, commentID int,
	headSHA, baseBranch string,
	method github.MergeMethod,
	requiredChecksOnly bool,
	label string,
) error {
	return command.Store.CheckArm(ctx, command.armRequest(
		runtime, pullRequest, commentID, headSHA, baseBranch,
		method, requiredChecksOnly, label,
	))
}

func (command *PendingCICommand) armRequest(
	runtime *RuntimeConfig,
	pullRequest, commentID int,
	headSHA, baseBranch string,
	method github.MergeMethod,
	requiredChecksOnly bool,
	label string,
) pendingci.ArmRequest {
	requestedAt := command.Now()

	return pendingci.ArmRequest{
		TargetID: command.TargetID, InstallationID: command.InstallationID,
		RepositoryID: command.RepositoryID, RepositoryFullName: command.RepositoryFullName,
		PullRequest: pullRequest, HeadSHA: headSHA, BaseBranch: baseBranch,
		MergeMethod: pendingci.MergeMethod(method), RequiredChecksOnly: requiredChecksOnly,
		Requester: runtime.CommentAuthor, SourceCommentID: int64(commentID),
		SourceRevision: command.SourceRevision, SourceSequence: command.SourceSequence,
		SourceOrder:  command.SourceOrder,
		ArtifactKind: pendingci.ArtifactLabel,
		Label:        label, RequestedAt: requestedAt,
	}
}

// cancelAndRun keeps durable cancellation and its external cleanup in one
// repository-owned operation. A newer command cannot arm between the two.
func (command *PendingCICommand) cancelAndRun(
	ctx context.Context,
	pullRequest int,
	reason string,
	operation func() error,
) (bool, error) {
	if operation == nil {
		return false, errors.New("pending CI cleanup operation is required")
	}
	var result pendingci.CancelIntentResult
	err := command.Coordinator.Exclusive(ctx, command.RepositoryID, func() error {
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
		command.Wake()
	}
	if err != nil {
		return false, fmt.Errorf("cancel pending CI command: %w", err)
	}

	return result.Accepted, nil
}

func (command *PendingCICommand) releaseBlockingCheck(
	ctx context.Context,
	request pendingci.Request,
) error {
	if command.Checks == nil || request.CheckSlotID == nil {
		return errors.New("pending CI check cleanup is unavailable")
	}
	slot, err := command.Checks.CheckSlot(ctx, *request.CheckSlotID)
	if err != nil {
		return fmt.Errorf("read pending CI check cleanup slot: %w", err)
	}
	target := storage.Target{
		ID: slot.TargetID, InstallationID: fmt.Sprint(slot.InstallationID),
	}
	repository := storage.Repository{ID: slot.RepositoryID, FullName: slot.RepositoryFullName}
	if _, err := command.Checks.EnsureBaseline(
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

func (command *PendingCICommand) cancelPullRequestLocked(
	ctx context.Context,
	pullRequest int,
	reason string,
) (pendingci.CancelIntentResult, error) {
	if command.SourceCommentID == 0 {
		request, err := command.Store.FinishPR(ctx, pendingci.FinishPRRequest{
			RepositoryID: command.RepositoryID, PullRequest: pullRequest,
			Lifecycle: pendingci.LifecycleCancelled, Trigger: pendingci.TriggerFallback,
			Reason: reason, FinishedAt: command.Now(),
		})

		return pendingci.CancelIntentResult{Accepted: true, Request: request}, err
	}

	return command.Store.CancelByIntent(ctx, pendingci.CancelIntentRequest{
		RepositoryID: command.RepositoryID, PullRequest: pullRequest,
		CommentID: command.SourceCommentID, SourceRevision: command.SourceRevision,
		SourceSequence: command.SourceSequence, SourceOrder: command.SourceOrder,
		Reason: reason, CancelledAt: command.Now(),
	})
}

func (command *PendingCICommand) exclusive(
	ctx context.Context,
	operation func() error,
) error {
	return command.Coordinator.Exclusive(ctx, command.RepositoryID, operation)
}
