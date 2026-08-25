package bot

import (
	"context"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type pendingCIArtifacts interface {
	pendingCIApprover
	draftMergeClient
	draftMergeHistoryClient
	GetLabels(context.Context, string, string, int) ([]string, error)
	AddLabel(context.Context, string, string, int, string) error
	RemoveLabel(context.Context, string, string, int, string) error
	AddPullRequestReaction(context.Context, string, string, int, github.ReactionType) error
	RemovePullRequestReactionByUser(
		context.Context,
		string,
		string,
		int,
		string,
		github.ReactionType,
	) error
}

type PendingCIActivationRequest struct {
	Runtime            *RuntimeConfig
	Owner              string
	Repository         string
	PullRequest        int
	CommentID          int
	HeadSHA            string
	BaseBranch         string
	Method             github.MergeMethod
	RequiredChecksOnly bool
	Label              string
	ArtifactKind       pendingci.ArtifactKind
	AllowDraftMerges   bool
}

type pendingCIActivationErrors struct {
	Approval  error
	Label     error
	Reaction  error
	Command   error
	Check     error
	Ready     error
	Stale     bool
	Ambiguous bool
	StoodDown bool
}

// activatePendingCI makes external artifacts and durable command replacement
// one repository-owned operation. Each rollback preserves artifacts still
// owned by the prior durable request.
func activatePendingCI(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	command *PendingCICommand,
	guard PendingCIActivationGuard,
	request PendingCIActivationRequest,
) (pendingCIActivationErrors, error) {
	var failures pendingCIActivationErrors
	err := command.exclusive(ctx, func() error {
		return activatePendingCIExclusive(
			ctx, artifacts, command, guard, request, &failures,
		)
	})

	return failures, err
}

func activatePendingCIExclusive(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	command *PendingCICommand,
	guard PendingCIActivationGuard,
	request PendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) error {
	ownership, stopped, err := preparePendingCIActivation(
		ctx, command, guard, request, failures,
	)
	if stopped {
		return err
	}
	info, failed := preparePendingCIDraft(ctx, artifacts, command, request, failures)
	if failed || pendingCIApprovalFailed(ctx, artifacts, request, info, failures) {
		return nil
	}
	stopped, err = addPendingCIServiceReaction(
		ctx, artifacts, request, ownership, failures,
	)
	if stopped {
		return err
	}

	return persistPendingCIActivation(
		ctx, artifacts, command, guard, request, ownership, failures,
	)
}

func preparePendingCIDraft(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	command *PendingCICommand,
	request PendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) (*github.PRInfo, bool) {
	info, err := artifacts.GetPRInfo(
		ctx, request.Owner, request.Repository, request.PullRequest,
	)
	if err == nil && request.AllowDraftMerges {
		err = ValidateDraftMergeAuthorization(
			ctx, artifacts, request.Owner, request.Repository,
			request.PullRequest, command.SourceRevision,
		)
	}
	if err == nil {
		info, err = prepareDraftMerge(
			ctx, artifacts, request.Owner, request.Repository, request.PullRequest,
			request.AllowDraftMerges, info,
		)
	}
	if err != nil {
		failures.Ready = err
		return nil, true
	}

	return info, false
}

func pendingCIApprovalFailed(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	request PendingCIActivationRequest,
	info *github.PRInfo,
	failures *pendingCIActivationErrors,
) bool {
	if !PendingCIApprovalRequired(request.Runtime, info) {
		return false
	}
	failures.Approval = artifacts.ApprovePR(
		ctx, request.Owner, request.Repository, request.PullRequest,
	)

	return failures.Approval != nil
}

func addPendingCIServiceReaction(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	request PendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	failures *pendingCIActivationErrors,
) (bool, error) {
	if ownership.serviceFence {
		return false, nil
	}
	failures.Reaction = artifacts.AddPullRequestReaction(
		ctx, request.Owner, request.Repository,
		request.PullRequest, ReactionPendingCIService,
	)
	if failures.Reaction == nil {
		return false, nil
	}

	// A transport error is ambiguous: GitHub may have accepted the reaction
	// even though the response never reached us. Remove it so the Action runner
	// is not fenced forever.
	return true, rollbackPendingCIArtifacts(ctx, artifacts, request, ownership, false)
}

func persistPendingCIActivation(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	command *PendingCICommand,
	guard PendingCIActivationGuard,
	request PendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	failures *pendingCIActivationErrors,
) error {
	if err := revalidatePendingCIActivation(ctx, guard, request, failures); err != nil ||
		failures.StoodDown {
		rollbackErr := rollbackPendingCIArtifacts(
			ctx, artifacts, request, ownership, false,
		)

		return errors.Join(err, rollbackErr)
	}
	if request.ArtifactKind == pendingci.ArtifactCheck {
		return persistPendingCICheckActivation(
			ctx, artifacts, command, request, ownership, failures,
		)
	}
	failures.Label = artifacts.AddLabel(
		ctx, request.Owner, request.Repository, request.PullRequest, request.Label,
	)
	if failures.Label != nil {
		return rollbackPendingCIArtifacts(ctx, artifacts, request, ownership, true)
	}

	_, failures.Command = command.arm(
		ctx, request.Runtime, request.PullRequest, request.CommentID,
		request.HeadSHA, request.BaseBranch, request.Method,
		request.RequiredChecksOnly, request.Label,
	)
	if failures.Command != nil {
		return handlePendingCIArmFailure(
			ctx, artifacts, command, request, ownership, failures,
		)
	}

	return removeConflictingPendingCILabels(ctx, artifacts, request)
}

func persistPendingCICheckActivation(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	command *PendingCICommand,
	request PendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	failures *pendingCIActivationErrors,
) error {
	if command.Checks == nil {
		failures.Check = errors.New("pending CI Check Run service is unavailable")
		return rollbackPendingCIArtifacts(ctx, artifacts, request, ownership, false)
	}
	target := storage.Target{
		ID: command.TargetID, InstallationID: fmt.Sprint(command.InstallationID),
	}
	repository := storage.Repository{
		ID: command.RepositoryID, FullName: command.RepositoryFullName,
	}
	slot, err := command.Checks.EnsureAuthorized(
		ctx, target, repository, request.PullRequest, request.HeadSHA,
		pendingci.MergeMethod(request.Method), request.Runtime.CommentAuthor,
	)
	if err != nil {
		failures.Check = err
		return rollbackPendingCIArtifacts(ctx, artifacts, request, ownership, false)
	}
	_, failures.Command = command.armCheck(
		ctx, request.Runtime, request.PullRequest, request.CommentID,
		request.HeadSHA, request.BaseBranch, request.Method,
		request.RequiredChecksOnly, slot.ID,
	)
	if failures.Command != nil {
		failures.Check = RestorePendingCICheckAfterArmFailure(
			ctx, command, target, repository, request, slot,
		)
		return handlePendingCIArmFailure(
			ctx, artifacts, command, request, ownership, failures,
		)
	}

	return removeConflictingPendingCILabels(ctx, artifacts, request)
}

func RestorePendingCICheckAfterArmFailure(
	ctx context.Context,
	command *PendingCICommand,
	target storage.Target,
	repository storage.Repository,
	request PendingCIActivationRequest,
	slot pendingci.CheckSlot,
) error {
	current, err := command.Store.GetArmed(ctx, command.RepositoryID, request.PullRequest)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		// The check is already blocking. Leave it that way when ownership cannot
		// be proven instead of risking release of another authorization.
		return fmt.Errorf("verify pending CI check owner after arm failure: %w", err)
	}
	if err == nil && current.ArtifactKind == pendingci.ArtifactCheck &&
		current.CheckSlotID != nil && *current.CheckSlotID == slot.ID {
		if current.AuthorizationState == pendingci.AuthorizationReauthorizationNeeded {
			if current.CandidateHeadSHA == "" {
				return errors.New("prior pending CI reauthorization has no candidate head")
			}
			_, err = command.Checks.EnsureReauthorization(
				ctx, target, repository, current.PullRequest, current.CandidateHeadSHA,
			)
			if err != nil {
				return fmt.Errorf("restore prior pending CI reauthorization: %w", err)
			}

			return nil
		}
		requester := current.AuthorizedBy
		if requester == "" {
			requester = current.Requester
		}
		_, err = command.Checks.EnsureAuthorized(
			ctx, target, repository, current.PullRequest, current.HeadSHA,
			current.MergeMethod, requester,
		)
		if err != nil {
			return fmt.Errorf("restore prior pending CI authorization: %w", err)
		}

		return nil
	}
	_, err = command.Checks.EnsureBaseline(
		ctx, target, repository, request.PullRequest, request.HeadSHA,
	)
	if err != nil {
		return fmt.Errorf("restore pending CI baseline: %w", err)
	}

	return nil
}

func preparePendingCIActivation(
	ctx context.Context,
	command *PendingCICommand,
	guard PendingCIActivationGuard,
	request PendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) (pendingCIArtifactOwnership, bool, error) {
	if guard == nil {
		return pendingCIArtifactOwnership{}, true,
			errors.New("pending CI activation guard is required")
	}
	if err := revalidatePendingCIActivation(ctx, guard, request, failures); err != nil ||
		failures.StoodDown {
		return pendingCIArtifactOwnership{}, true, err
	}
	ownership, err := command.armedArtifactOwnership(
		ctx, request.PullRequest, request.Label,
	)
	if err != nil {
		failures.Command = err

		return ownership, true, nil
	}
	failures.Command = command.checkArm(
		ctx, request.Runtime, request.PullRequest, request.CommentID,
		request.HeadSHA, request.BaseBranch, request.Method,
		request.RequiredChecksOnly, request.Label,
	)
	if failures.Command != nil {
		classifyPendingCIArmFailure(ctx, command, request, failures)

		return ownership, true, nil
	}

	return ownership, false, nil
}

func revalidatePendingCIActivation(
	ctx context.Context,
	guard PendingCIActivationGuard,
	request PendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) error {
	allowed, err := guard.AllowsActivation(
		ctx,
		request.ArtifactKind,
		request.BaseBranch,
		request.RequiredChecksOnly,
	)
	if err != nil {
		return fmt.Errorf("revalidate pending CI runner ownership: %w", err)
	}
	failures.StoodDown = !allowed

	return nil
}

func classifyPendingCIArmFailure(
	ctx context.Context,
	command *PendingCICommand,
	request PendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) {
	if errors.Is(failures.Command, pendingci.ErrStaleSourceRevision) {
		failures.Command = nil
		failures.Stale = true

		return
	}
	resolveAmbiguousPendingCI(ctx, command, request, failures)
}

func handlePendingCIArmFailure(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	command *PendingCICommand,
	request PendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	failures *pendingCIActivationErrors,
) error {
	resolveAmbiguousPendingCI(ctx, command, request, failures)
	rollbackErr := rollbackPendingCIArtifacts(
		ctx, artifacts, request, ownership, true,
	)
	if errors.Is(failures.Command, pendingci.ErrStaleSourceRevision) {
		failures.Command = nil
		failures.Stale = true
	}

	return rollbackErr
}

func resolveAmbiguousPendingCI(
	ctx context.Context,
	command *PendingCICommand,
	request PendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) {
	if !errors.Is(failures.Command, pendingci.ErrAmbiguousSourceRevision) {
		return
	}
	result, err := command.cancelPullRequestLocked(
		ctx,
		request.PullRequest,
		"commands from different comments have an ambiguous source order",
	)
	if err != nil {
		if errors.Is(err, pendingci.ErrAmbiguousSourceRevision) {
			failures.Command = nil
			failures.Ambiguous = true

			return
		}
		failures.Command = errors.Join(failures.Command, err)

		return
	}
	failures.Command = nil
	failures.Ambiguous = true
	if result.Request != nil {
		command.Wake()
	}
}

func removeConflictingPendingCILabels(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	request PendingCIActivationRequest,
) error {
	labels, err := artifacts.GetLabels(
		ctx, request.Owner, request.Repository, request.PullRequest,
	)
	if err != nil {
		return fmt.Errorf("list pending CI labels: %w", err)
	}

	return RemoveConflictingPendingCILabelsFrom(
		labels,
		request.Label,
		func(label string) error {
			return artifacts.RemoveLabel(
				ctx, request.Owner, request.Repository, request.PullRequest, label,
			)
		},
	)
}

func RemoveConflictingPendingCILabelsFrom(
	labels []string,
	keep string,
	remove func(string) error,
) error {
	var removeErr error
	for _, label := range labels {
		if label == keep || !isPendingCIMethodLabel(label) {
			continue
		}
		removeErr = errors.Join(removeErr, CleanupGitHubError(
			"remove conflicting pending CI label",
			remove(label),
		))
	}

	return removeErr
}

func isPendingCIMethodLabel(label string) bool {
	_, _, parsed := ParsePendingCILabel(label)

	return parsed != ""
}

func rollbackPendingCIArtifacts(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	request PendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	labelAdded bool,
) error {
	var rollbackErr error
	if labelAdded && !ownership.label {
		err := CleanupGitHubError(
			"remove method label",
			artifacts.RemoveLabel(
				ctx, request.Owner, request.Repository, request.PullRequest, request.Label,
			),
		)
		if err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if !ownership.serviceFence {
		if err := artifacts.RemovePullRequestReactionByUser(
			ctx, request.Owner, request.Repository,
			request.PullRequest, request.Runtime.BotUsername, ReactionPendingCIService,
		); err != nil {
			rollbackErr = errors.Join(
				rollbackErr,
				fmt.Errorf("remove pending CI service fence: %w", err),
			)
		}
	}
	return rollbackErr
}
