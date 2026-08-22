package main

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
	GetPRInfo(context.Context, string, string, int) (*github.PRInfo, error)
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

type pendingCIActivationRequest struct {
	runtime            *RuntimeConfig
	owner              string
	repository         string
	pullRequest        int
	commentID          int
	headSHA            string
	baseBranch         string
	method             github.MergeMethod
	requiredChecksOnly bool
	label              string
	artifactKind       pendingci.ArtifactKind
}

type pendingCIActivationErrors struct {
	approval  error
	label     error
	reaction  error
	command   error
	check     error
	stale     bool
	ambiguous bool
	stoodDown bool
}

// activatePendingCI makes external artifacts and durable command replacement
// one repository-owned operation. Each rollback preserves artifacts still
// owned by the prior durable request.
func activatePendingCI(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	command *pendingCICommand,
	guard pendingCIActivationGuard,
	request pendingCIActivationRequest,
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
	command *pendingCICommand,
	guard pendingCIActivationGuard,
	request pendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) error {
	ownership, stopped, err := preparePendingCIActivation(
		ctx, command, guard, request, failures,
	)
	if stopped {
		return err
	}
	if pendingCIApprovalFailed(ctx, artifacts, request, failures) {
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

func pendingCIApprovalFailed(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	request pendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) bool {
	info, err := artifacts.GetPRInfo(
		ctx, request.owner, request.repository, request.pullRequest,
	)
	if err != nil {
		failures.approval = err

		return true
	}
	if !pendingCIApprovalRequired(request.runtime, info) {
		return false
	}
	failures.approval = artifacts.ApprovePR(
		ctx, request.owner, request.repository, request.pullRequest,
	)

	return failures.approval != nil
}

func addPendingCIServiceReaction(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	request pendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	failures *pendingCIActivationErrors,
) (bool, error) {
	if ownership.serviceFence {
		return false, nil
	}
	failures.reaction = artifacts.AddPullRequestReaction(
		ctx, request.owner, request.repository,
		request.pullRequest, github.ReactionPendingCIService,
	)
	if failures.reaction == nil {
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
	command *pendingCICommand,
	guard pendingCIActivationGuard,
	request pendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	failures *pendingCIActivationErrors,
) error {
	if err := revalidatePendingCIActivation(ctx, guard, request, failures); err != nil ||
		failures.stoodDown {
		rollbackErr := rollbackPendingCIArtifacts(
			ctx, artifacts, request, ownership, false,
		)

		return errors.Join(err, rollbackErr)
	}
	if request.artifactKind == pendingci.ArtifactCheck {
		return persistPendingCICheckActivation(
			ctx, artifacts, command, request, ownership, failures,
		)
	}
	failures.label = artifacts.AddLabel(
		ctx, request.owner, request.repository, request.pullRequest, request.label,
	)
	if failures.label != nil {
		return rollbackPendingCIArtifacts(ctx, artifacts, request, ownership, true)
	}

	_, failures.command = command.arm(
		ctx, request.runtime, request.pullRequest, request.commentID,
		request.headSHA, request.baseBranch, request.method,
		request.requiredChecksOnly, request.label,
	)
	if failures.command != nil {
		return handlePendingCIArmFailure(
			ctx, artifacts, command, request, ownership, failures,
		)
	}

	return removeConflictingPendingCILabels(ctx, artifacts, request)
}

func persistPendingCICheckActivation(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	command *pendingCICommand,
	request pendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	failures *pendingCIActivationErrors,
) error {
	if command.checks == nil {
		failures.check = errors.New("pending CI Check Run service is unavailable")
		return rollbackPendingCIArtifacts(ctx, artifacts, request, ownership, false)
	}
	target := storage.Target{
		ID: command.targetID, InstallationID: fmt.Sprint(command.installationID),
	}
	repository := storage.Repository{
		ID: command.repositoryID, FullName: command.repositoryFullName,
	}
	slot, err := command.checks.EnsureAuthorized(
		ctx, target, repository, request.pullRequest, request.headSHA,
		pendingci.MergeMethod(request.method), request.runtime.CommentAuthor,
	)
	if err != nil {
		failures.check = err
		return rollbackPendingCIArtifacts(ctx, artifacts, request, ownership, false)
	}
	_, failures.command = command.armCheck(
		ctx, request.runtime, request.pullRequest, request.commentID,
		request.headSHA, request.baseBranch, request.method,
		request.requiredChecksOnly, slot.ID,
	)
	if failures.command != nil {
		failures.check = restorePendingCICheckAfterArmFailure(
			ctx, command, target, repository, request, slot,
		)
		return handlePendingCIArmFailure(
			ctx, artifacts, command, request, ownership, failures,
		)
	}

	return removeConflictingPendingCILabels(ctx, artifacts, request)
}

func restorePendingCICheckAfterArmFailure(
	ctx context.Context,
	command *pendingCICommand,
	target storage.Target,
	repository storage.Repository,
	request pendingCIActivationRequest,
	slot pendingci.CheckSlot,
) error {
	current, err := command.store.GetArmed(ctx, command.repositoryID, request.pullRequest)
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
			_, err = command.checks.EnsureReauthorization(
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
		_, err = command.checks.EnsureAuthorized(
			ctx, target, repository, current.PullRequest, current.HeadSHA,
			current.MergeMethod, requester,
		)
		if err != nil {
			return fmt.Errorf("restore prior pending CI authorization: %w", err)
		}

		return nil
	}
	_, err = command.checks.EnsureBaseline(
		ctx, target, repository, request.pullRequest, request.headSHA,
	)
	if err != nil {
		return fmt.Errorf("restore pending CI baseline: %w", err)
	}

	return nil
}

func preparePendingCIActivation(
	ctx context.Context,
	command *pendingCICommand,
	guard pendingCIActivationGuard,
	request pendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) (pendingCIArtifactOwnership, bool, error) {
	if guard == nil {
		return pendingCIArtifactOwnership{}, true,
			errors.New("pending CI activation guard is required")
	}
	if err := revalidatePendingCIActivation(ctx, guard, request, failures); err != nil ||
		failures.stoodDown {
		return pendingCIArtifactOwnership{}, true, err
	}
	ownership, err := command.armedArtifactOwnership(
		ctx, request.pullRequest, request.label,
	)
	if err != nil {
		failures.command = err

		return ownership, true, nil
	}
	failures.command = command.checkArm(
		ctx, request.runtime, request.pullRequest, request.commentID,
		request.headSHA, request.baseBranch, request.method,
		request.requiredChecksOnly, request.label,
	)
	if failures.command != nil {
		classifyPendingCIArmFailure(ctx, command, request, failures)

		return ownership, true, nil
	}

	return ownership, false, nil
}

func revalidatePendingCIActivation(
	ctx context.Context,
	guard pendingCIActivationGuard,
	request pendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) error {
	allowed, err := guard.AllowsActivation(
		ctx,
		request.artifactKind,
		request.baseBranch,
		request.requiredChecksOnly,
	)
	if err != nil {
		return fmt.Errorf("revalidate pending CI runner ownership: %w", err)
	}
	failures.stoodDown = !allowed

	return nil
}

func classifyPendingCIArmFailure(
	ctx context.Context,
	command *pendingCICommand,
	request pendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) {
	if errors.Is(failures.command, pendingci.ErrStaleSourceRevision) {
		failures.command = nil
		failures.stale = true

		return
	}
	resolveAmbiguousPendingCI(ctx, command, request, failures)
}

func handlePendingCIArmFailure(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	command *pendingCICommand,
	request pendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	failures *pendingCIActivationErrors,
) error {
	resolveAmbiguousPendingCI(ctx, command, request, failures)
	rollbackErr := rollbackPendingCIArtifacts(
		ctx, artifacts, request, ownership, true,
	)
	if errors.Is(failures.command, pendingci.ErrStaleSourceRevision) {
		failures.command = nil
		failures.stale = true
	}

	return rollbackErr
}

func resolveAmbiguousPendingCI(
	ctx context.Context,
	command *pendingCICommand,
	request pendingCIActivationRequest,
	failures *pendingCIActivationErrors,
) {
	if !errors.Is(failures.command, pendingci.ErrAmbiguousSourceRevision) {
		return
	}
	result, err := command.cancelPullRequestLocked(
		ctx,
		request.pullRequest,
		"commands from different comments have an ambiguous source order",
	)
	if err != nil {
		failures.command = errors.Join(failures.command, err)

		return
	}
	failures.command = nil
	failures.ambiguous = true
	if result.Request != nil {
		command.wake()
	}
}

func removeConflictingPendingCILabels(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	request pendingCIActivationRequest,
) error {
	labels, err := artifacts.GetLabels(
		ctx, request.owner, request.repository, request.pullRequest,
	)
	if err != nil {
		return fmt.Errorf("list pending CI labels: %w", err)
	}

	return removeConflictingPendingCILabelsFrom(
		labels,
		request.label,
		func(label string) error {
			return artifacts.RemoveLabel(
				ctx, request.owner, request.repository, request.pullRequest, label,
			)
		},
	)
}

func removeConflictingPendingCILabelsFrom(
	labels []string,
	keep string,
	remove func(string) error,
) error {
	var removeErr error
	for _, label := range labels {
		if label == keep || !isPendingCIMethodLabel(label) {
			continue
		}
		removeErr = errors.Join(removeErr, cleanupGitHubError(
			"remove conflicting pending CI label",
			remove(label),
		))
	}

	return removeErr
}

func isPendingCIMethodLabel(label string) bool {
	_, _, parsed := parsePendingCILabel(label)

	return parsed != ""
}

func rollbackPendingCIArtifacts(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	request pendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	labelAdded bool,
) error {
	var rollbackErr error
	if labelAdded && !ownership.label {
		err := cleanupGitHubError(
			"remove method label",
			artifacts.RemoveLabel(
				ctx, request.owner, request.repository, request.pullRequest, request.label,
			),
		)
		if err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if !ownership.serviceFence {
		if err := artifacts.RemovePullRequestReactionByUser(
			ctx, request.owner, request.repository,
			request.pullRequest, request.runtime.BotUsername, github.ReactionPendingCIService,
		); err != nil {
			rollbackErr = errors.Join(
				rollbackErr,
				fmt.Errorf("remove pending CI service fence: %w", err),
			)
		}
	}
	return rollbackErr
}
