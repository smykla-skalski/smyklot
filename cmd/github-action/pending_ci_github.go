package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type githubPendingCIBackend struct {
	server  *server
	current pendingCICurrentStore
	source  pendingCISourceValidator
}

type pendingCICurrentStore interface {
	GetArmed(context.Context, string, int) (pendingci.Request, error)
}

type pendingCICleanupScope struct {
	label          bool
	sourceReaction bool
	serviceFence   bool
}

var errNoRequiredStatusChecks = errors.New("base branch has no required status checks")

func (backend *githubPendingCIBackend) Observe(
	ctx context.Context,
	request pendingci.Request,
) (pendingci.Observation, error) {
	observedAt := time.Now().UTC()
	client, owner, repository, err := backend.client(ctx, request)
	if err != nil {
		return pendingci.Observation{}, err
	}
	state, err := client.GetPullRequestState(ctx, owner, repository, request.PullRequest)
	if err != nil {
		return pendingci.Observation{}, err
	}
	labelFound := hasLabel(state.Labels, request.Label)
	if !state.Open || !labelFound {
		return pendingci.Observation{
			HeadSHA: state.HeadSHA, BaseBranch: state.BaseBranch, PullRequestOpen: state.Open,
			PullRequestMerged: state.Merged, PendingLabelFound: labelFound,
			State: pendingci.ObservedIndeterminate, ObservedAt: observedAt,
		}, nil
	}
	if err := backend.requireCurrent(ctx, request); err != nil {
		return pendingci.Observation{}, err
	}
	sourceReason, err := backend.source.CancellationReason(
		ctx, client, request, owner, repository,
	)
	if err != nil {
		return pendingci.Observation{}, err
	}
	if sourceReason != "" {
		return pendingci.Observation{
			HeadSHA: state.HeadSHA, BaseBranch: state.BaseBranch, PullRequestOpen: state.Open,
			PullRequestMerged: state.Merged, PendingLabelFound: labelFound,
			CancelReason: sourceReason,
			State:        pendingci.ObservedIndeterminate, ObservedAt: observedAt,
		}, nil
	}
	if err := client.AddPullRequestReaction(
		ctx, owner, repository, request.PullRequest,
		github.ReactionPendingCIService,
	); err != nil {
		return pendingci.Observation{}, fmt.Errorf(
			"restore pending CI service handoff fence: %w", err,
		)
	}
	if err := removeConflictingPendingCILabelsFrom(
		state.Labels,
		request.Label,
		func(label string) error {
			return client.RemoveLabel(
				ctx, owner, repository, request.PullRequest, label,
			)
		},
	); err != nil {
		return pendingci.Observation{}, err
	}
	cancelReason, err := backend.cancelReason(ctx, client, request, owner, repository)
	if err != nil {
		return pendingci.Observation{}, err
	}
	if cancelReason != "" {
		return pendingci.Observation{
			HeadSHA: state.HeadSHA, BaseBranch: state.BaseBranch, PullRequestOpen: state.Open,
			PullRequestMerged: state.Merged, PendingLabelFound: labelFound,
			CancelReason: cancelReason,
			State:        pendingci.ObservedIndeterminate, ObservedAt: observedAt,
		}, nil
	}
	checks, err := backend.checks(ctx, client, request, state, owner, repository)
	if errors.Is(err, errNoRequiredStatusChecks) {
		return pendingci.Observation{
			HeadSHA: state.HeadSHA, BaseBranch: state.BaseBranch, PullRequestOpen: state.Open,
			PullRequestMerged: state.Merged, PendingLabelFound: labelFound,
			CancelReason: errNoRequiredStatusChecks.Error(),
			State:        pendingci.ObservedIndeterminate, ObservedAt: observedAt,
		}, nil
	}
	if err != nil {
		return pendingci.Observation{}, err
	}

	return pendingci.Observation{
		HeadSHA: state.HeadSHA, BaseBranch: state.BaseBranch, PullRequestOpen: state.Open,
		PullRequestMerged: state.Merged, PendingLabelFound: labelFound,
		State: observedCIState(checks.State), Fingerprint: checkFingerprint(checks),
		Summary: checks.Summary, ObservedAt: observedAt,
	}, nil
}

func (backend *githubPendingCIBackend) requireCurrent(
	ctx context.Context,
	request pendingci.Request,
) error {
	current, err := backend.current.GetArmed(ctx, request.RepositoryID, request.PullRequest)
	if err != nil {
		return fmt.Errorf("verify current pending CI request: %w", err)
	}
	if current.ID != request.ID || current.Revision != request.Revision {
		return fmt.Errorf("verify current pending CI request: %w", storage.ErrConflict)
	}

	return nil
}

func (backend *githubPendingCIBackend) MergeAtHead(
	ctx context.Context,
	request pendingci.Request,
	headSHA string,
) error {
	client, owner, repository, err := backend.client(ctx, request)
	if err != nil {
		return err
	}

	return mergePendingPRAtHead(
		ctx, client, owner, repository, request.PullRequest,
		github.MergeMethod(request.MergeMethod), headSHA,
	)
}

func (backend *githubPendingCIBackend) CleanupArtifacts(
	ctx context.Context,
	request pendingci.Request,
	lifecycle pendingci.Lifecycle,
) error {
	return backend.cleanupArtifactsExclusive(ctx, request, lifecycle)
}

func (backend *githubPendingCIBackend) cleanupArtifactsExclusive(
	ctx context.Context,
	request pendingci.Request,
	lifecycle pendingci.Lifecycle,
) error {
	client, owner, repository, err := backend.client(ctx, request)
	if err != nil {
		return fmt.Errorf("authenticate pending CI cleanup: %w", err)
	}
	scope, err := backend.cleanupScope(ctx, request)
	if err != nil {
		return err
	}
	var cleanupErr error
	if scope.label {
		labelErr := cleanupGitHubError(
			"remove pending CI label",
			client.RemoveLabel(ctx, owner, repository, request.PullRequest, request.Label),
		)
		cleanupErr = errors.Join(cleanupErr, labelErr)
	}
	if scope.serviceFence {
		cleanupErr = errors.Join(cleanupErr, cleanupGitHubError(
			"remove pending CI service fence",
			client.RemovePullRequestReactionByUser(
				ctx, owner, repository, request.PullRequest,
				backend.server.cfg.botUsername, github.ReactionPendingCIService,
			),
		))
	}
	commentID := int(request.SourceCommentID)
	if scope.sourceReaction {
		cleanupErr = errors.Join(cleanupErr, cleanupGitHubError(
			"remove pending CI reaction",
			client.RemoveReactionByUser(
				ctx, owner, repository, commentID,
				github.ReactionPendingCI, backend.server.cfg.botUsername,
			),
		))
	}
	if lifecycle == pendingci.LifecycleMerged && request.SourceCommentID > 0 {
		cleanupErr = errors.Join(cleanupErr, cleanupGitHubError(
			"add pending CI success reaction",
			client.AddReaction(ctx, owner, repository, commentID, github.ReactionSuccess),
		))
	}

	return cleanupErr
}

func (backend *githubPendingCIBackend) cleanupScope(
	ctx context.Context,
	request pendingci.Request,
) (pendingCICleanupScope, error) {
	current, err := backend.current.GetArmed(ctx, request.RepositoryID, request.PullRequest)
	if errors.Is(err, storage.ErrNotFound) {
		return pendingCICleanupScope{
			label: true, sourceReaction: request.SourceCommentID > 0, serviceFence: true,
		}, nil
	}
	if err != nil {
		return pendingCICleanupScope{}, fmt.Errorf(
			"read replacement pending CI request: %w", err,
		)
	}

	return pendingCICleanupScope{
		label: current.Label != request.Label,
		sourceReaction: request.SourceCommentID > 0 &&
			current.SourceCommentID != request.SourceCommentID,
	}, nil
}

func cleanupGitHubError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var apiErr *github.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return nil
	}

	return fmt.Errorf("%s: %w", operation, err)
}

func (backend *githubPendingCIBackend) client(
	_ context.Context,
	request pendingci.Request,
) (*github.Client, string, string, error) {
	owner, repository, found := strings.Cut(request.RepositoryFullName, "/")
	if !found || owner == "" || repository == "" || strings.Contains(repository, "/") {
		return nil, "", "", fmt.Errorf("invalid repository name %q", request.RepositoryFullName)
	}
	token, err := backend.server.tokens.InstallationToken(request.InstallationID)
	if err != nil {
		return nil, "", "", NewGitHubError(ErrGitHubAppAuth, err)
	}
	client, err := github.NewClient(token, backend.server.cfg.apiBaseURL)
	if err != nil {
		return nil, "", "", NewGitHubError(ErrGitHubClient, err)
	}

	return client, owner, repository, nil
}

func (backend *githubPendingCIBackend) cancelReason(
	ctx context.Context,
	client *github.Client,
	request pendingci.Request,
	owner, repository string,
) (string, error) {
	if backend.server.panel != nil {
		target, repo, err := backend.server.repositoryControls(ctx, request.TargetID, request.RepositoryID)
		if err != nil {
			return "", err
		}
		enabled := target.RepositoryDefaultEnabled
		if repo.EnabledOverride != nil {
			enabled = *repo.EnabledOverride
		}
		if !target.Available || !repo.Available || !enabled {
			return "repository disabled in Smyklot", nil
		}
	}
	botConfig, err := backend.server.serviceConfig(
		ctx, client, request.TargetID, request.RepositoryID, owner, repository,
	)
	if err != nil {
		return "", err
	}
	if botConfig.Runner == config.RunnerAction {
		return "repository switched to the GitHub Action runner", nil
	}

	return "", nil
}

func (backend *githubPendingCIBackend) checks(
	ctx context.Context,
	client *github.Client,
	request pendingci.Request,
	state github.PullRequestState,
	owner, repository string,
) (*github.CheckStatus, error) {
	var required []github.RequiredCheck
	if request.RequiredChecksOnly {
		var err error
		required, err = client.GetRequiredStatusChecks(ctx, owner, repository, state.BaseBranch)
		if err != nil {
			return nil, err
		}
		if required == nil {
			required = []github.RequiredCheck{}
		}
		if len(required) == 0 {
			return nil, errNoRequiredStatusChecks
		}
	}

	return client.GetCheckStatus(ctx, owner, repository, state.HeadSHA, required)
}

func observedCIState(state github.CIState) pendingci.ObservedState {
	switch state {
	case github.CIStatePassing:
		return pendingci.ObservedPassing
	case github.CIStatePending:
		return pendingci.ObservedPending
	case github.CIStateFailing:
		return pendingci.ObservedFailing
	case github.CIStateNoChecks:
		return pendingci.ObservedNoChecks
	default:
		return pendingci.ObservedIndeterminate
	}
}

func checkFingerprint(status *github.CheckStatus) string {
	return fmt.Sprintf(
		"%s:%d:%d:%d:%d:%d:%d",
		status.State, status.Total, status.Passed, status.Failed,
		status.InProgress, status.Unknown, status.Missing,
	)
}

func hasLabel(labels []string, wanted string) bool {
	for _, label := range labels {
		if label == wanted {
			return true
		}
	}

	return false
}
