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
	check          bool
	sourceReaction bool
	serviceFence   bool
}

func (backend *githubPendingCIBackend) WakePendingCIGates() {
	backend.server.WakePendingCIGates()
}

var (
	errNoRequiredStatusChecks       = errors.New("base branch has no required status checks")
	errRequiredWorkflowsUnsupported = errors.New(
		"required-only merge-after-CI does not support required workflow rules",
	)
)

const pendingCIRepositoryDisabledReason = "repository disabled in Smyklot"
const pendingCIBranchExcludedReason = "base branch is outside Smyklot merge-after-CI checks"

func (backend *githubPendingCIBackend) Observe(
	ctx context.Context,
	request pendingci.Request,
) (pendingci.Observation, error) {
	observedAt := time.Now().UTC()
	if unavailable, found, err := backend.unavailableObservation(ctx, request, observedAt); err != nil {
		return pendingci.Observation{}, err
	} else if found {
		return unavailable, nil
	}
	client, owner, repository, err := backend.client(ctx, request)
	if err != nil {
		return pendingci.Observation{}, err
	}
	state, err := client.GetPullRequestState(ctx, owner, repository, request.PullRequest)
	if err != nil {
		return pendingci.Observation{}, err
	}
	labelFound, stopped, err := backend.prepareObservation(ctx, request, state, observedAt)
	if err != nil {
		return pendingci.Observation{}, err
	}
	if stopped != nil {
		return *stopped, nil
	}
	sourceReason, err := backend.source.CancellationReason(
		ctx, client, request, owner, repository,
	)
	if err != nil {
		return pendingci.Observation{}, err
	}
	if sourceReason != "" {
		observation := pullRequestObservation(state, labelFound, observedAt)
		observation.CancelReason = sourceReason

		return observation, nil
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
		observation := pullRequestObservation(state, labelFound, observedAt)
		observation.CancelReason = cancelReason

		return observation, nil
	}
	checks, err := backend.checks(ctx, client, request, state, owner, repository)
	if errors.Is(err, errNoRequiredStatusChecks) ||
		errors.Is(err, errRequiredWorkflowsUnsupported) {
		observation := pullRequestObservation(state, labelFound, observedAt)
		observation.CancelReason = err.Error()

		return observation, nil
	}
	if err != nil {
		return pendingci.Observation{}, err
	}
	passingQuiet, err := backend.passingQuiet(ctx, request)
	if err != nil {
		return pendingci.Observation{}, err
	}

	return pendingci.Observation{
		HeadSHA: state.HeadSHA, BaseBranch: state.BaseBranch, PullRequestOpen: state.Open,
		PullRequestMerged: state.Merged, PendingLabelFound: labelFound,
		State: observedCIState(checks.State), Fingerprint: checkFingerprint(checks),
		Summary: checks.Summary, ObservedAt: observedAt, PassingQuiet: passingQuiet,
	}, nil
}

func (backend *githubPendingCIBackend) unavailableObservation(
	ctx context.Context,
	request pendingci.Request,
	observedAt time.Time,
) (pendingci.Observation, bool, error) {
	if backend.server.panel == nil {
		return pendingci.Observation{}, false, nil
	}
	target, repository, err := backend.server.readRepositoryControls(
		ctx, request.TargetID, request.RepositoryID,
	)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return pendingci.Observation{}, false, fmt.Errorf(
			"read pending CI repository availability: %w",
			err,
		)
	}
	if err == nil && target.Available && repository.Available &&
		effectiveRepositoryEnabled(target, repository) {
		return pendingci.Observation{}, false, nil
	}

	return pendingci.Observation{
		HeadSHA: request.HeadSHA, BaseBranch: request.BaseBranch,
		State: pendingci.ObservedIndeterminate, ObservedAt: observedAt,
		CancelReason: pendingCIRepositoryDisabledReason,
	}, true, nil
}

func (backend *githubPendingCIBackend) prepareObservation(
	ctx context.Context,
	request pendingci.Request,
	state github.PullRequestState,
	observedAt time.Time,
) (bool, *pendingci.Observation, error) {
	artifact := request.ArtifactKind
	if artifact == "" {
		artifact = pendingci.ArtifactLabel
	}
	labelFound := artifact == pendingci.ArtifactCheck || hasLabel(state.Labels, request.Label)
	if !state.Open || !labelFound {
		observation := pullRequestObservation(state, labelFound, observedAt)

		return labelFound, &observation, nil
	}
	if err := backend.requireCurrent(ctx, request); err != nil {
		return false, nil, err
	}
	if artifact != pendingci.ArtifactCheck {
		return labelFound, nil, nil
	}
	included, err := backend.checkBranchIncluded(ctx, request, state.BaseBranch)
	if err != nil {
		return false, nil, err
	}
	if !included {
		observation := pullRequestObservation(state, true, observedAt)
		observation.CancelReason = pendingCIBranchExcludedReason

		return true, &observation, nil
	}
	ready, reason, err := backend.checkGateReady(ctx, request)
	if err != nil {
		return false, nil, err
	}
	if !ready {
		observation := pullRequestObservation(state, true, observedAt)
		observation.Summary = "Pending CI readiness paused: " + reason

		return true, &observation, nil
	}
	if request.AuthorizationState == pendingci.AuthorizationAuthorized {
		if err := backend.ensureAuthorizedCheck(ctx, request); err != nil {
			return false, nil, err
		}
	} else if err := backend.ensureReauthorizationCheck(ctx, request); err != nil {
		return false, nil, err
	}

	return true, nil, nil
}

func (backend *githubPendingCIBackend) ensureReauthorizationCheck(
	ctx context.Context,
	request pendingci.Request,
) error {
	if request.CandidateHeadSHA == "" || request.CheckSlotID == nil {
		return errors.New("pending CI reauthorization has no candidate check")
	}
	target, repository, err := backend.server.readRepositoryControls(
		ctx, request.TargetID, request.RepositoryID,
	)
	if err != nil {
		return fmt.Errorf("read reauthorization check settings: %w", err)
	}
	slot, err := backend.server.pendingCIChecks.EnsureReauthorization(
		ctx, target, repository, request.PullRequest, request.CandidateHeadSHA,
	)
	if err != nil {
		return fmt.Errorf("restore pending CI reauthorization check: %w", err)
	}
	if slot.ID != *request.CheckSlotID {
		return errors.New("pending CI reauthorization does not own its candidate check")
	}

	return nil
}

func (backend *githubPendingCIBackend) checkBranchIncluded(
	ctx context.Context,
	request pendingci.Request,
	baseBranch string,
) (bool, error) {
	if backend.server.panel == nil {
		return true, nil
	}
	target, repository, err := backend.server.readRepositoryControls(
		ctx, request.TargetID, request.RepositoryID,
	)
	if err != nil {
		return false, fmt.Errorf("read pending CI branch policy: %w", err)
	}
	_, patterns, _ := storage.EffectivePendingCISettings(target, repository, 0)

	return pendingCIBranchIncluded(baseBranch, repository.DefaultBranch, patterns), nil
}

func pullRequestObservation(
	state github.PullRequestState,
	labelFound bool,
	observedAt time.Time,
) pendingci.Observation {
	return pendingci.Observation{
		HeadSHA: state.HeadSHA, BaseBranch: state.BaseBranch, PullRequestOpen: state.Open,
		PullRequestMerged: state.Merged, PendingLabelFound: labelFound,
		State: pendingci.ObservedIndeterminate, ObservedAt: observedAt,
	}
}

func (backend *githubPendingCIBackend) ensureAuthorizedCheck(
	ctx context.Context,
	request pendingci.Request,
) error {
	target, repository, err := backend.server.readRepositoryControls(
		ctx,
		request.TargetID,
		request.RepositoryID,
	)
	if err != nil {
		return fmt.Errorf("read authorized check settings: %w", err)
	}
	slot, err := backend.server.pendingCIChecks.EnsureAuthorized(
		ctx,
		target,
		repository,
		request.PullRequest,
		request.HeadSHA,
		request.MergeMethod,
		request.AuthorizedBy,
	)
	if err != nil {
		return fmt.Errorf("restore authorized pending CI check: %w", err)
	}
	if request.CheckSlotID == nil || slot.ID != *request.CheckSlotID {
		return errors.New("authorized pending CI request does not own its head check")
	}

	return nil
}

func (backend *githubPendingCIBackend) passingQuiet(
	ctx context.Context,
	request pendingci.Request,
) (*time.Duration, error) {
	if backend.server.panel == nil {
		return nil, nil
	}
	target, repository, err := backend.server.readRepositoryControls(
		ctx,
		request.TargetID,
		request.RepositoryID,
	)
	if err != nil {
		return nil, fmt.Errorf("read pending CI quiet-period settings: %w", err)
	}
	_, _, quiet := storage.EffectivePendingCISettings(
		target,
		repository,
		backend.server.pendingCIReconciler.currentTiming().PassingQuiet,
	)

	return &quiet, nil
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

	method := github.MergeMethod(request.MergeMethod)
	if request.ArtifactKind == pendingci.ArtifactCheck {
		return mergePendingPRAtHeadWithoutQueue(
			ctx, client, owner, repository, request.PullRequest,
			method, request.BaseBranch, headSHA,
		)
	}

	return mergePendingPRAtHead(
		ctx, client, owner, repository, request.PullRequest, method, headSHA,
	)
}

func mergePendingPRAtHeadWithoutQueue(
	ctx context.Context,
	client *github.Client,
	owner string,
	repository string,
	pullRequest int,
	method github.MergeMethod,
	baseBranch string,
	headSHA string,
) error {
	state, err := client.GetPullRequestState(ctx, owner, repository, pullRequest)
	if err != nil {
		return fmt.Errorf("read final pending CI merge revision: %w", err)
	}
	if !state.Open || state.HeadSHA != headSHA || state.BaseBranch != baseBranch {
		return errors.New("pending CI merge revision changed after check authorization")
	}
	mergeQueue, err := client.IsMergeQueueEnabled(
		ctx, owner, repository, state.BaseBranch,
	)
	if err != nil {
		return fmt.Errorf("read pending CI merge queue policy: %w", err)
	}
	if mergeQueue {
		return fmt.Errorf(
			"merge-after-CI checks do not support the merge queue on base branch %s",
			state.BaseBranch,
		)
	}

	// The method is part of the exact authorization. Action mode retains its
	// legacy merge-to-squash-to-rebase fallback, but a durable check request
	// must not issue a second merge attempt after the final base/queue read.
	return client.MergePRAtHead(ctx, owner, repository, pullRequest, method, headSHA)
}

func (backend *githubPendingCIBackend) SatisfyCheck(
	ctx context.Context,
	request pendingci.Request,
) error {
	client, owner, repository, err := backend.client(ctx, request)
	if err != nil {
		return err
	}
	if err := preflightPendingCICheckMerge(
		ctx, client, owner, repository, request,
	); err != nil {
		return err
	}
	if request.CheckSlotID == nil {
		return errors.New("pending CI check request has no durable check slot")
	}
	slot, err := backend.server.store.GetCheckSlot(ctx, *request.CheckSlotID)
	if err != nil {
		return fmt.Errorf("read pending CI check slot: %w", err)
	}
	_, err = backend.server.pendingCIChecks.EnsureMergeReady(ctx, slot)

	return err
}

func preflightPendingCICheckMerge(
	ctx context.Context,
	client *github.Client,
	owner string,
	repository string,
	request pendingci.Request,
) error {
	state, err := client.GetPullRequestState(
		ctx, owner, repository, request.PullRequest,
	)
	if err != nil {
		return fmt.Errorf("read pending CI check merge revision: %w", err)
	}
	if !state.Open || state.HeadSHA != request.HeadSHA || state.BaseBranch != request.BaseBranch {
		return errors.New("pending CI check merge revision changed before authorization")
	}
	mergeQueue, err := client.IsMergeQueueEnabled(
		ctx, owner, repository, state.BaseBranch,
	)
	if err != nil {
		return fmt.Errorf("read pending CI check merge queue policy: %w", err)
	}
	if mergeQueue {
		return fmt.Errorf(
			"merge-after-CI checks do not support the merge queue on base branch %s",
			state.BaseBranch,
		)
	}

	return nil
}

func (backend *githubPendingCIBackend) RestoreBlockingCheck(
	ctx context.Context,
	request pendingci.Request,
) error {
	target := storage.Target{
		ID: request.TargetID, InstallationID: fmt.Sprint(request.InstallationID),
	}
	repository := storage.Repository{
		ID: request.RepositoryID, FullName: request.RepositoryFullName,
	}
	_, err := backend.server.pendingCIChecks.EnsureAuthorized(
		ctx, target, repository, request.PullRequest, request.HeadSHA,
		request.MergeMethod, request.AuthorizedBy,
	)

	return err
}

func (backend *githubPendingCIBackend) RequireReauthorizationCheck(
	ctx context.Context,
	request pendingci.Request,
	headSHA string,
) (pendingci.CheckSlot, error) {
	target, repository, err := backend.server.readRepositoryControls(
		ctx,
		request.TargetID,
		request.RepositoryID,
	)
	if err != nil {
		return pendingci.CheckSlot{}, fmt.Errorf("read reauthorization settings: %w", err)
	}

	return backend.server.pendingCIChecks.EnsureReauthorization(
		ctx,
		target,
		repository,
		request.PullRequest,
		headSHA,
	)
}

func (backend *githubPendingCIBackend) GetPendingCICheckSlot(
	ctx context.Context,
	id int64,
) (pendingci.CheckSlot, error) {
	return backend.server.store.GetCheckSlot(ctx, id)
}

func (backend *githubPendingCIBackend) PendingCICheckSlotIsCurrent(
	ctx context.Context,
	request pendingci.Request,
	slot pendingci.CheckSlot,
) (bool, error) {
	current, err := backend.current.GetArmed(ctx, request.RepositoryID, request.PullRequest)
	if errors.Is(err, storage.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return current.CheckSlotID != nil && *current.CheckSlotID == slot.ID, nil
}

func (backend *githubPendingCIBackend) RestoreRetiredPendingCICheck(
	ctx context.Context,
	slot pendingci.CheckSlot,
) error {
	target := storage.Target{
		ID: slot.TargetID, InstallationID: fmt.Sprint(slot.InstallationID),
	}
	repository := storage.Repository{
		ID: slot.RepositoryID, FullName: slot.RepositoryFullName,
	}
	_, err := backend.server.pendingCIChecks.EnsureBaseline(
		ctx,
		target,
		repository,
		slot.PullRequest,
		slot.HeadSHA,
	)

	return err
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
	if scope.label && request.ArtifactKind != pendingci.ArtifactCheck {
		labelErr := cleanupGitHubError(
			"remove pending CI label",
			client.RemoveLabel(ctx, owner, repository, request.PullRequest, request.Label),
		)
		cleanupErr = errors.Join(cleanupErr, labelErr)
	}
	if scope.check && request.ArtifactKind == pendingci.ArtifactCheck {
		if request.CheckSlotID == nil {
			cleanupErr = errors.Join(cleanupErr, errors.New("pending CI check cleanup has no slot"))
		} else {
			slot, slotErr := backend.server.store.GetCheckSlot(ctx, *request.CheckSlotID)
			if slotErr == nil {
				target := storage.Target{
					ID: slot.TargetID, InstallationID: fmt.Sprint(slot.InstallationID),
				}
				repositorySettings := storage.Repository{
					ID: slot.RepositoryID, FullName: slot.RepositoryFullName,
				}
				_, slotErr = backend.server.pendingCIChecks.EnsureBaseline(
					ctx, target, repositorySettings, slot.PullRequest, slot.HeadSHA,
				)
			}
			cleanupErr = errors.Join(cleanupErr, slotErr)
		}
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
			label:          request.ArtifactKind != pendingci.ArtifactCheck,
			check:          request.ArtifactKind == pendingci.ArtifactCheck,
			sourceReaction: request.SourceCommentID > 0, serviceFence: true,
		}, nil
	}
	if err != nil {
		return pendingCICleanupScope{}, fmt.Errorf(
			"read replacement pending CI request: %w", err,
		)
	}

	return pendingCICleanupScope{
		label: request.ArtifactKind != pendingci.ArtifactCheck && current.Label != request.Label,
		check: request.ArtifactKind == pendingci.ArtifactCheck &&
			!sameOptionalInt64(current.CheckSlotID, request.CheckSlotID),
		sourceReaction: request.SourceCommentID > 0 &&
			current.SourceCommentID != request.SourceCommentID,
	}, nil
}

func (backend *githubPendingCIBackend) checkGateReady(
	ctx context.Context,
	request pendingci.Request,
) (bool, string, error) {
	gate, err := backend.server.store.GetPendingCIRepositoryGate(ctx, request.RepositoryID)
	if err != nil {
		return false, "", fmt.Errorf("read pending CI check readiness: %w", err)
	}
	if gate.EffectiveMode == storage.PendingCIEffectiveChecks &&
		gate.Readiness == storage.PendingCIDraining &&
		gate.DesiredMode == storage.PendingCIModeLabels {
		return true, "", nil
	}
	if gate.Readiness != storage.PendingCIReady {
		return false, gate.Reason, nil
	}

	return true, "", nil
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
		target, repo, err := backend.server.readRepositoryControls(
			ctx, request.TargetID, request.RepositoryID,
		)
		if err != nil {
			return "", err
		}
		if !target.Available || !repo.Available ||
			!effectiveRepositoryEnabled(target, repo) {
			return pendingCIRepositoryDisabledReason, nil
		}
	}
	botConfig, err := backend.server.serviceConfigWithoutCatalogRefresh(
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
		requirements, err := client.GetRequiredCIRequirements(
			ctx, owner, repository, state.BaseBranch,
		)
		if err != nil {
			return nil, err
		}
		if requirements.RequiredWorkflow {
			return nil, errRequiredWorkflowsUnsupported
		}
		required = requirements.StatusChecks
		if required == nil {
			required = []github.RequiredCheck{}
		}
	}

	if request.ArtifactKind != pendingci.ArtifactCheck {
		if request.RequiredChecksOnly && len(required) == 0 {
			return nil, errNoRequiredStatusChecks
		}
		return client.GetCheckStatus(ctx, owner, repository, state.HeadSHA, required)
	}
	if request.CheckSlotID == nil {
		return nil, errors.New("pending CI check request has no durable check slot")
	}
	slot, err := backend.server.store.GetCheckSlot(ctx, *request.CheckSlotID)
	if err != nil {
		return nil, fmt.Errorf("read pending CI check slot: %w", err)
	}
	appID := slot.AppID
	if request.RequiredChecksOnly {
		required = pendingCIExternalRequiredChecks(required, slot.Name, appID)
	}
	if request.RequiredChecksOnly && len(required) == 0 {
		return nil, errNoRequiredStatusChecks
	}

	return client.GetCheckStatusExcludingCheck(
		ctx, owner, repository, state.HeadSHA, required,
		github.OwnedCheck{
			Name: storage.PendingCICheckName, AppID: appID, ExternalID: slot.ExternalID,
		},
	)
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

func sameOptionalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}
