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
}

func pendingCIServiceOwned(
	ctx context.Context,
	reader pendingCIOwnershipReader,
	owner, repository string,
	pullRequest int,
) (bool, error) {
	state, err := reader.GetPullRequestState(
		ctx, owner, repository, pullRequest,
	)
	if err != nil {
		return false, fmt.Errorf("read pending CI ownership: %w", err)
	}

	return hasLabel(state.Labels, github.LabelPendingCIServiceOwner), nil
}

func pendingCIActionOwns(
	ctx context.Context,
	reader pendingCIOwnershipReader,
	owner, repository string,
	pullRequest int,
	label, headSHA string,
) (bool, error) {
	state, err := reader.GetPullRequestState(
		ctx, owner, repository, pullRequest,
	)
	if err != nil {
		return false, fmt.Errorf("revalidate pending CI ownership: %w", err)
	}

	return state.Open && state.HeadSHA == headSHA &&
		hasLabel(state.Labels, label) &&
		!hasLabel(state.Labels, github.LabelPendingCIServiceOwner), nil
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
