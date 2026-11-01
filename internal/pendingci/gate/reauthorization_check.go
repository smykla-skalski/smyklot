package gate

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// The preview observes policy and permissions without approving a pull request.
// Execution checks again before applying the approval. This is not a grant.
type reauthorizationCheck struct {
	allowed          bool
	approvalRequired bool
}

func (backend *Backend) preparePendingCIReauthorization(ctx context.Context, candidate reauthorizationCandidate, signal pendingci.Signal) (bool, error) {
	check, err := backend.checkPendingCIReauthorization(ctx, candidate, signal)
	if err != nil || !check.allowed {
		return false, err
	}
	if check.approvalRequired {
		if err := candidate.client.ApprovePR(ctx, candidate.owner, candidate.repository, candidate.slot.PullRequest); err != nil {
			return false, fmt.Errorf("restore requested-action approval: %w", err)
		}
	}
	return true, nil
}

func (backend *Backend) checkPendingCIReauthorization(
	ctx context.Context,
	candidate reauthorizationCandidate,
	signal pendingci.Signal,
) (reauthorizationCheck, error) {
	client := candidate.client
	owner := candidate.owner
	repository := candidate.repository
	checker, err := bot.NewPermissionChecker(ctx, client, owner, repository)
	if err != nil {
		return reauthorizationCheck{}, err
	}
	authorized, err := bot.CheckUserPermission(
		ctx,
		client,
		checker,
		signal.Actor,
		owner,
		repository,
	)
	if err != nil {
		return reauthorizationCheck{}, bot.NewGitHubError(bot.ErrPermissionCheck, err)
	}
	if !authorized {
		return reauthorizationCheck{}, nil
	}
	state, err := client.GetPullRequestState(ctx, owner, repository, candidate.slot.PullRequest)
	if err != nil {
		return reauthorizationCheck{}, fmt.Errorf("read requested-action pull request state: %w", err)
	}
	if !state.Open || state.HeadSHA != signal.HeadSHA ||
		state.BaseBranch != candidate.request.CandidateBaseBranch {
		return reauthorizationCheck{}, nil
	}
	mergeQueue, err := client.IsMergeQueueEnabled(ctx, owner, repository, state.BaseBranch)
	if err != nil {
		return reauthorizationCheck{}, fmt.Errorf("read requested-action merge queue policy: %w", err)
	}
	if mergeQueue {
		return reauthorizationCheck{}, nil
	}
	required, err := client.GetRequiredStatusChecks(ctx, owner, repository, state.BaseBranch)
	if err != nil {
		return reauthorizationCheck{}, fmt.Errorf("read requested-action base protection: %w", err)
	}
	if !requiredContextOwned(required, candidate.slot.Name, candidate.slot.AppID) {
		return reauthorizationCheck{}, nil
	}
	botConfig, err := backend.config(
		ctx,
		client,
		candidate.request.TargetID,
		candidate.request.RepositoryID,
		owner,
		repository,
	)
	if err != nil {
		return reauthorizationCheck{}, fmt.Errorf("read requested-action configuration: %w", err)
	}
	info, err := client.GetPRInfo(ctx, owner, repository, candidate.slot.PullRequest)
	if err != nil {
		return reauthorizationCheck{}, fmt.Errorf("read requested-action pull request approvals: %w", err)
	}
	runtime := &bot.RuntimeConfig{CommentAuthor: signal.Actor, BotUsername: backend.botUsername}
	if bot.PendingCIApprovalAllowed(runtime, botConfig, info) != nil {
		return reauthorizationCheck{}, nil
	}
	return reauthorizationCheck{allowed: true, approvalRequired: bot.PendingCIApprovalRequired(runtime, info)}, nil
}

func (backend *Backend) CarryAuthorizationCheck(
	ctx context.Context,
	request pendingci.Request,
	headSHA string,
) (pendingci.CheckSlot, bool, error) {
	client, owner, repository, err := backend.client(ctx, request)
	if err != nil {
		return pendingci.CheckSlot{}, false, err
	}
	appID, err := backend.checkRuns.AppID(ctx)
	if err != nil {
		return pendingci.CheckSlot{}, false, err
	}
	candidate := reauthorizationCandidate{
		slot: pendingci.CheckSlot{
			PullRequest: request.PullRequest, Name: storage.PendingCICheckName, AppID: appID,
		},
		request: request, client: client, owner: owner, repository: repository,
	}
	candidate.request.CandidateBaseBranch = request.BaseBranch
	allowed, err := backend.preparePendingCIReauthorization(
		ctx, candidate, pendingci.Signal{Actor: request.AuthorizedBy, HeadSHA: headSHA},
	)
	if err != nil || !allowed {
		return pendingci.CheckSlot{}, false, err
	}
	target, repositorySettings, err := readControls(
		ctx, backend.store, request.TargetID, request.RepositoryID,
	)
	if err != nil {
		return pendingci.CheckSlot{}, false, fmt.Errorf("read carried authorization settings: %w", err)
	}
	slot, err := backend.checkRuns.EnsureAuthorized(
		ctx, target, repositorySettings, request.PullRequest, headSHA,
		request.MergeMethod, request.AuthorizedBy,
	)
	if err != nil {
		return pendingci.CheckSlot{}, false, fmt.Errorf("publish carried authorization check: %w", err)
	}

	return slot, true, nil
}

// CheckReauthorization observes current eligibility without approving or changing
// a pending request. Execution must recheck it under the repository coordinator.
func (g *Gate) CheckReauthorization(ctx context.Context, repositoryID string, signal pendingci.Signal) (bool, error) {
	candidate, found, err := g.reauthorizationCandidate(ctx, repositoryID, signal)
	if err != nil || !found {
		return false, err
	}
	check, err := g.backend.checkPendingCIReauthorization(ctx, candidate, signal)
	return check.allowed, err
}
