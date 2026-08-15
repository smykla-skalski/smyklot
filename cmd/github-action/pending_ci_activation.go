package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type pendingCIArtifacts interface {
	pendingCIApprover
	GetPRInfo(context.Context, string, string, int) (*github.PRInfo, error)
	GetLabels(context.Context, string, string, int) ([]string, error)
	AddLabel(context.Context, string, string, int, string) error
	RemoveLabel(context.Context, string, string, int, string) error
	AddReaction(context.Context, string, string, int, github.ReactionType) error
	RemoveReactionByUser(
		context.Context,
		string,
		string,
		int,
		github.ReactionType,
		string,
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
}

type pendingCIActivationErrors struct {
	approval  error
	label     error
	command   error
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
		ownership, stopped, err := preparePendingCIActivation(
			ctx, command, guard, request, &failures,
		)
		if stopped {
			return err
		}
		info, err := artifacts.GetPRInfo(
			ctx, request.owner, request.repository, request.pullRequest,
		)
		if err != nil {
			failures.approval = err

			return nil
		}
		if pendingCIApprovalRequired(request.runtime, info) {
			failures.approval = artifacts.ApprovePR(
				ctx, request.owner, request.repository, request.pullRequest,
			)
			if failures.approval != nil {
				return nil
			}
		}
		failures.label = artifacts.AddLabel(
			ctx, request.owner, request.repository,
			request.pullRequest, github.LabelPendingCIServiceOwner,
		)
		if failures.label != nil {
			return rollbackPendingCIArtifacts(
				ctx, artifacts, request, ownership, false, !ownership.serviceMarker,
			)
		}
		markerAdded := !ownership.serviceMarker
		_ = artifacts.AddReaction(
			ctx, request.owner, request.repository,
			request.commentID, github.ReactionPendingCI,
		)
		failures.label = artifacts.AddLabel(
			ctx, request.owner, request.repository, request.pullRequest, request.label,
		)
		if failures.label != nil {
			return rollbackPendingCIArtifacts(
				ctx, artifacts, request, ownership, true, markerAdded,
			)
		}

		_, failures.command = command.arm(
			ctx, request.runtime, request.pullRequest, request.commentID,
			request.headSHA, request.baseBranch, request.method,
			request.requiredChecksOnly, request.label,
		)
		if failures.command != nil {
			return handlePendingCIArmFailure(
				ctx, artifacts, command, request, ownership, &failures, markerAdded,
			)
		}
		return removeConflictingPendingCILabels(ctx, artifacts, request)
	})

	return failures, err
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
	if err := revalidatePendingCIActivation(ctx, guard, failures); err != nil ||
		failures.stoodDown {
		return pendingCIArtifactOwnership{}, true, err
	}
	ownership, err := command.armedArtifactOwnership(
		ctx, request.pullRequest, request.label, request.commentID,
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
	failures *pendingCIActivationErrors,
) error {
	allowed, err := guard.AllowsActivation(ctx)
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
	_ = resolveAmbiguousPendingCI(ctx, command, request, failures)
}

func handlePendingCIArmFailure(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	command *pendingCICommand,
	request pendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	failures *pendingCIActivationErrors,
	markerAdded bool,
) error {
	keepMarker := resolveAmbiguousPendingCI(ctx, command, request, failures)
	rollbackErr := rollbackPendingCIArtifacts(
		ctx, artifacts, request, ownership, true, markerAdded && !keepMarker,
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
) bool {
	if !errors.Is(failures.command, pendingci.ErrAmbiguousSourceRevision) {
		return false
	}
	result, err := command.cancelPullRequestLocked(
		ctx,
		request.pullRequest,
		"commands from different comments have an ambiguous source order",
	)
	keepMarker := result.Request != nil || err != nil
	if err != nil {
		failures.command = errors.Join(failures.command, err)

		return keepMarker
	}
	failures.command = nil
	failures.ambiguous = true
	if result.Request != nil {
		command.wake()
	}

	return keepMarker
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
	markerAdded bool,
) error {
	var rollbackErr error
	labelRemoved := true
	if labelAdded && !ownership.label {
		err := cleanupGitHubError(
			"remove method label",
			artifacts.RemoveLabel(
				ctx, request.owner, request.repository, request.pullRequest, request.label,
			),
		)
		labelRemoved = err == nil
		if err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if !ownership.reaction {
		if err := artifacts.RemoveReactionByUser(
			ctx, request.owner, request.repository,
			request.commentID, github.ReactionPendingCI, request.runtime.BotUsername,
		); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove pending reaction: %w", err))
		}
	}
	if markerAdded && labelRemoved {
		if err := cleanupGitHubError(
			"remove ownership marker",
			artifacts.RemoveLabel(
				ctx, request.owner, request.repository,
				request.pullRequest, github.LabelPendingCIServiceOwner,
			),
		); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}

	return rollbackErr
}
