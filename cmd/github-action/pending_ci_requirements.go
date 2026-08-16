package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

type pendingCIRequirementReader interface {
	GetRequiredStatusChecks(context.Context, string, string, string) ([]github.RequiredCheck, error)
}

type pendingCIOwnershipReader interface {
	GetPullRequestState(context.Context, string, string, int) (github.PullRequestState, error)
	HasPullRequestReaction(
		context.Context, string, string, int, string, github.ReactionType,
	) (bool, error)
}

func pendingCIServiceOwned(
	ctx context.Context,
	reader pendingCIOwnershipReader,
	owner, repository string,
	pullRequest int,
	botUsername string,
) (bool, error) {
	state, err := reader.GetPullRequestState(ctx, owner, repository, pullRequest)
	if err != nil {
		return false, fmt.Errorf("read pending CI ownership: %w", err)
	}

	return pendingCIServiceOwnedForState(
		ctx, reader, owner, repository, pullRequest, botUsername, state,
	)
}

func pendingCIServiceOwnedForState(
	ctx context.Context,
	reader pendingCIOwnershipReader,
	owner, repository string,
	pullRequest int,
	botUsername string,
	state github.PullRequestState,
) (bool, error) {
	if hasLabel(state.Labels, github.LegacyLabelPendingCIServiceOwner) {
		return true, nil
	}
	owned, err := reader.HasPullRequestReaction(
		ctx, owner, repository, pullRequest, botUsername,
		github.ReactionPendingCIService,
	)
	if err != nil {
		return false, fmt.Errorf("read pending CI service ownership: %w", err)
	}

	return owned, nil
}

func pendingCIActionOwns(
	ctx context.Context,
	reader pendingCIOwnershipReader,
	owner, repository string,
	pullRequest int,
	label, headSHA, botUsername string,
) (bool, error) {
	state, err := reader.GetPullRequestState(
		ctx, owner, repository, pullRequest,
	)
	if err != nil {
		return false, fmt.Errorf("revalidate pending CI ownership: %w", err)
	}

	if !state.Open || state.HeadSHA != headSHA || !hasLabel(state.Labels, label) {
		return false, nil
	}
	serviceOwned, err := pendingCIServiceOwnedForState(
		ctx, reader, owner, repository, pullRequest, botUsername, state,
	)
	if err != nil {
		return false, err
	}

	return !serviceOwned, nil
}

func pendingCIRequiredChecks(
	ctx context.Context,
	reader pendingCIRequirementReader,
	owner, repository, baseBranch string,
	requiredChecksOnly bool,
) ([]github.RequiredCheck, error) {
	if !requiredChecksOnly {
		return nil, nil
	}
	if baseBranch == "" {
		return nil, errors.New("cannot resolve the base branch for required checks")
	}
	required, err := reader.GetRequiredStatusChecks(ctx, owner, repository, baseBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get required checks: %w", err)
	}
	if len(required) == 0 {
		return nil, errors.New("the base branch has no required status checks")
	}

	return required, nil
}
