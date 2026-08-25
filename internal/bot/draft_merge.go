package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

var errDraftMergeDisabled = errors.New(
	"pull request is a draft and allow_draft_merges is disabled",
)

type draftMergeClient interface {
	GetPRInfo(context.Context, string, string, int) (*github.PRInfo, error)
	MarkPullRequestReadyForReview(context.Context, string, string, int) error
}

type draftMergeHistoryClient interface {
	LatestPullRequestDraftTransition(
		context.Context, string, string, int,
	) (time.Time, bool, error)
}

// ValidateDraftMergeAuthorization fails closed when GitHub records a draft
// transition after the event that authorized a merge. Equal timestamps are
// ambiguous because both REST resources have one-second precision.
func ValidateDraftMergeAuthorization(
	ctx context.Context,
	client draftMergeHistoryClient,
	owner, repository string,
	pullRequest int,
	sourceRevision string,
) error {
	authorizedAt, err := pendingci.ParseSourceRevision(sourceRevision)
	if err != nil {
		return err
	}
	draftedAt, found, err := client.LatestPullRequestDraftTransition(
		ctx, owner, repository, pullRequest,
	)
	if err != nil || !found {
		return err
	}
	if authorizedAt.Before(draftedAt) {
		return fmt.Errorf(
			"%w: pull request was converted back to draft after this merge command; reissue the command",
			pendingci.ErrStaleSourceRevision,
		)
	}
	if authorizedAt.Equal(draftedAt) {
		return fmt.Errorf(
			"%w: GitHub recorded the merge command and draft transition in the same second; reissue the command to confirm the merge",
			pendingci.ErrAmbiguousSourceRevision,
		)
	}

	return nil
}

func draftMergeCommandRevision(
	ctx context.Context,
	client *github.Client,
	runtime *RuntimeConfig,
	commentID int,
	environment CommandEnvironment,
) (string, error) {
	if environment.PendingCI != nil &&
		strings.TrimSpace(environment.PendingCI.SourceRevision) != "" {
		return environment.PendingCI.SourceRevision, nil
	}
	comment, err := client.GetIssueComment(
		ctx, runtime.RepoOwner, runtime.RepoName, int64(commentID),
	)
	if err != nil {
		return "", fmt.Errorf("read merge command revision: %w", err)
	}

	return comment.UpdatedAt, nil
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
