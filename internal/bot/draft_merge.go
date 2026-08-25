package bot

import (
	"context"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

var errDraftMergeDisabled = errors.New(
	"pull request is a draft and allow_draft_merges is disabled",
)

type draftMergeClient interface {
	GetPRInfo(context.Context, string, string, int) (*github.PRInfo, error)
	MarkPullRequestReadyForReview(context.Context, string, string, int) error
}

func prepareDraftMerge(
	ctx context.Context,
	client draftMergeClient,
	owner, repository string,
	pullRequest int,
	allow bool,
	info *github.PRInfo,
) (*github.PRInfo, error) {
	if !info.Draft {
		return info, nil
	}
	if !allow {
		return nil, errDraftMergeDisabled
	}
	if err := client.MarkPullRequestReadyForReview(
		ctx, owner, repository, pullRequest,
	); err != nil {
		return nil, fmt.Errorf("mark pull request ready for review: %w", err)
	}

	refreshed, err := client.GetPRInfo(ctx, owner, repository, pullRequest)
	if err != nil {
		return nil, fmt.Errorf("refresh pull request after marking it ready for review: %w", err)
	}
	if refreshed.Draft {
		return nil, errors.New("pull request is still a draft after marking it ready for review")
	}

	return refreshed, nil
}
