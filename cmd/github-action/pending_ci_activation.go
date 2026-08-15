package main

import (
	"context"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type pendingCIArtifacts interface {
	AddLabel(context.Context, string, string, int, string) error
	RemoveLabel(context.Context, string, string, int, string) error
	AddReaction(context.Context, string, string, int, github.ReactionType) error
	RemoveReaction(context.Context, string, string, int, github.ReactionType) error
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
	label   error
	command error
}

// activatePendingCI makes external artifacts and durable command replacement
// one repository-owned operation. Each rollback preserves artifacts still
// owned by the prior durable request.
func activatePendingCI(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	command *pendingCICommand,
	request pendingCIActivationRequest,
) (pendingCIActivationErrors, error) {
	var failures pendingCIActivationErrors
	err := command.exclusive(ctx, func() error {
		ownership, err := command.armedArtifactOwnership(
			ctx, request.pullRequest, request.label, request.commentID,
		)
		if err != nil {
			failures.command = err

			return nil
		}
		_ = artifacts.AddReaction(
			ctx, request.owner, request.repository,
			request.commentID, github.ReactionPendingCI,
		)
		failures.label = artifacts.AddLabel(
			ctx, request.owner, request.repository, request.pullRequest, request.label,
		)
		if failures.label != nil {
			rollbackPendingCIArtifacts(ctx, artifacts, request, ownership, false)

			return nil
		}

		var superseded *pendingci.Request
		superseded, failures.command = command.arm(
			ctx, request.runtime, request.pullRequest, request.commentID,
			request.headSHA, request.baseBranch, request.method,
			request.requiredChecksOnly, request.label,
		)
		if failures.command != nil {
			rollbackPendingCIArtifacts(ctx, artifacts, request, ownership, true)

			return nil
		}
		if superseded != nil && superseded.Label != request.label {
			_ = artifacts.RemoveLabel(
				ctx, request.owner, request.repository,
				request.pullRequest, superseded.Label,
			)
		}

		return nil
	})

	return failures, err
}

func rollbackPendingCIArtifacts(
	ctx context.Context,
	artifacts pendingCIArtifacts,
	request pendingCIActivationRequest,
	ownership pendingCIArtifactOwnership,
	labelAdded bool,
) {
	if labelAdded && !ownership.label {
		_ = artifacts.RemoveLabel(
			ctx, request.owner, request.repository, request.pullRequest, request.label,
		)
	}
	if !ownership.reaction {
		_ = artifacts.RemoveReaction(
			ctx, request.owner, request.repository,
			request.commentID, github.ReactionPendingCI,
		)
	}
}
