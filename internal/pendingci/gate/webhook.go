package gate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

// HandleWebhook applies only durable state transitions. Webhook
// payloads are wake-up hints; the reconciler reads live pull-request and check
// state before it ever decides to merge.
func (g *Gate) HandleWebhook(
	ctx context.Context,
	notification *pendingci.Notification,
	deliveryID string,
) error {
	repositoryID := storage.RepositoryID(notification.Source.Repository.ID)
	return g.coordinator.Exclusive(ctx, repositoryID, func() error {
		return g.applyPendingCINotification(ctx, notification, deliveryID)
	})
}

func (g *Gate) applyPendingCINotification(
	ctx context.Context,
	notification *pendingci.Notification,
	deliveryID string,
) error {
	if notification.Event == webhook.EventPullRequest && notification.Action == "opened" {
		// A repository can be added while reaction polling is disabled. Its first
		// pull request is enough reason to refresh the catalog and provision the
		// required context even when the webhook arrived before the catalog knew it.
		g.notifyGates()
	}
	occurredAt := time.Now().UTC()
	var changed int64

	for _, signal := range notification.Signals {
		count, err := g.applyPendingCISignal(
			ctx, notification.Source, occurredAt, notification.Event, deliveryID, signal,
		)
		if err != nil {
			return fmt.Errorf("apply %s pending CI signal: %w", notification.Event, err)
		}
		changed += count
	}
	if changed > 0 {
		g.Wake()
		logging.From(ctx).Info("pending CI requests notified", "requests", changed)
	} else {
		logging.From(ctx).Debug("pending CI webhook matched no armed request")
	}

	return nil
}

func (g *Gate) applyPendingCISignal(
	ctx context.Context,
	source webhook.Source,
	occurredAt time.Time,
	eventName string,
	deliveryID string,
	signal pendingci.Signal,
) (int64, error) {
	repositoryID := storage.RepositoryID(source.Repository.ID)
	switch signal.Kind {
	case pendingci.SignalWakePullRequest:
		changed, err := g.wakePendingCIPullRequest(
			ctx, repositoryID, occurredAt, eventName, deliveryID, signal,
		)
		if err != nil {
			return 0, err
		}
		if err := g.ensureWebhookPendingCIBaseline(ctx, source, signal.PullRequest); err != nil {
			return 0, err
		}
		if changed {
			return 1, nil
		}

		return 0, nil
	case pendingci.SignalPullRequestDone, pendingci.SignalLabelRemoved:
		changed, err := g.wakePendingCIPullRequest(
			ctx, repositoryID, occurredAt, eventName, deliveryID, signal,
		)
		if changed {
			return 1, err
		}

		return 0, err
	case pendingci.SignalWakeHead:
		return g.store.WakeByHead(ctx, pendingci.WakeHeadRequest{
			RepositoryID: repositoryID, HeadSHA: signal.HeadSHA,
			EventName: eventName, EventKey: signal.EventKey,
			DeliveryID: deliveryID, OccurredAt: occurredAt,
		})
	case pendingci.SignalReauthorize:
		changed, err := g.reauthorizePendingCI(
			ctx,
			repositoryID,
			occurredAt,
			deliveryID,
			signal,
		)
		if changed {
			return 1, err
		}

		return 0, err
	case pendingci.SignalRerequestCheck:
		if g.Checks != nil {
			_, err := g.Checks.RefreshRerequest(
				ctx, repositoryID, signal.HeadSHA, signal.AppID, signal.CheckRunID,
				signal.CheckName, signal.ExternalID, eventName == webhook.EventCheckRun,
			)
			if err != nil {
				return 0, fmt.Errorf("restore rerequested pending CI check: %w", err)
			}
		}

		return g.store.WakeByHead(ctx, pendingci.WakeHeadRequest{
			RepositoryID: repositoryID, HeadSHA: signal.HeadSHA,
			EventName: eventName, EventKey: signal.EventKey,
			DeliveryID: deliveryID, OccurredAt: occurredAt,
		})
	default:
		return 0, fmt.Errorf("unsupported signal kind %q", signal.Kind)
	}
}

func (g *Gate) wakePendingCIPullRequest(
	ctx context.Context,
	repositoryID string,
	occurredAt time.Time,
	eventName, deliveryID string,
	signal pendingci.Signal,
) (bool, error) {
	expectedHead := ""
	if signal.MatchHead {
		expectedHead = signal.HeadSHA
	}
	return g.store.Wake(ctx, pendingci.WakeRequest{
		RepositoryID: repositoryID, PullRequest: signal.PullRequest,
		EventName: eventName, EventKey: signal.EventKey, DeliveryID: deliveryID,
		ExpectedHeadSHA: expectedHead, OccurredAt: occurredAt,
	})
}

func (g *Gate) ensureWebhookPendingCIBaseline(
	ctx context.Context,
	source webhook.Source,
	pullRequest int,
) error {
	if !g.panelled || g.Checks == nil || pullRequest <= 0 {
		return nil
	}
	target, repository, eligible, err := g.webhookPendingCIBaselineControls(
		ctx,
		source,
		pullRequest,
	)
	if err != nil || !eligible {
		return err
	}
	installationID, err := parsePositiveInt64(target.InstallationID)
	if err != nil {
		return err
	}
	client, owner, name, err := g.pendingCIClient(installationID, repository.FullName)
	if err != nil {
		return err
	}
	state, err := client.GetPullRequestState(ctx, owner, name, pullRequest)
	if err != nil {
		return fmt.Errorf("read pull request baseline state: %w", err)
	}
	if !state.Open {
		return nil
	}
	patterns := target.PendingCIBranchPatternsDefault
	if repository.PendingCIBranchPatternsOverride != nil {
		patterns = *repository.PendingCIBranchPatternsOverride
	}
	if !branchIncluded(state.BaseBranch, repository.DefaultBranch, patterns) {
		return nil
	}
	_, err = g.Checks.EnsureBaseline(
		ctx,
		target,
		repository,
		pullRequest,
		state.HeadSHA,
	)
	if err != nil {
		return fmt.Errorf("ensure pull request baseline check: %w", err)
	}

	return nil
}

func (g *Gate) webhookPendingCIBaselineControls(
	ctx context.Context,
	source webhook.Source,
	pullRequest int,
) (storage.Target, storage.Repository, bool, error) {
	targetID := storage.InstallationID(source.InstallationID)
	repositoryID := storage.RepositoryID(source.Repository.ID)
	target, repository, err := readControls(ctx, g.store, targetID, repositoryID)
	if errors.Is(err, storage.ErrNotFound) {
		return storage.Target{}, storage.Repository{}, false, nil
	}
	if err != nil {
		return storage.Target{}, storage.Repository{}, false,
			fmt.Errorf("read pull request baseline controls: %w", err)
	}
	gate, err := g.store.GetPendingCIRepositoryGate(ctx, repositoryID)
	if err != nil {
		return storage.Target{}, storage.Repository{}, false,
			fmt.Errorf("read pull request baseline gate: %w", err)
	}
	if gate.DesiredMode != storage.PendingCIModeChecks &&
		gate.EffectiveMode != storage.PendingCIEffectiveChecks {
		return target, repository, false, nil
	}
	enabled := storage.RepositoryEnabled(target, repository)
	if !enabled || !target.Available || !repository.Available || !target.Grants("checks") {
		return target, repository, false, nil
	}
	armed, err := g.store.GetArmed(ctx, repositoryID, pullRequest)
	if err == nil && armed.ArtifactKind == pendingci.ArtifactCheck {
		return target, repository, false, nil
	}
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return storage.Target{}, storage.Repository{}, false,
			fmt.Errorf("read pull request baseline owner: %w", err)
	}

	return target, repository, true, nil
}

type reauthorizationCandidate struct {
	slot       pendingci.CheckSlot
	request    pendingci.Request
	client     *github.Client
	owner      string
	repository string
}

func (g *Gate) reauthorizePendingCI(
	ctx context.Context,
	repositoryID string,
	authorizedAt time.Time,
	deliveryID string,
	signal pendingci.Signal,
) (bool, error) {
	candidate, found, err := g.reauthorizationCandidate(ctx, repositoryID, signal)
	if err != nil || !found {
		return false, err
	}
	allowed, err := g.preparePendingCIReauthorization(ctx, candidate, signal)
	if err != nil || !allowed {
		return false, err
	}
	updated, err := g.store.Reauthorize(ctx, pendingci.ReauthorizeRequest{
		RepositoryID: repositoryID, PullRequest: candidate.slot.PullRequest,
		HeadSHA: signal.HeadSHA, BaseBranch: candidate.request.CandidateBaseBranch,
		CheckSlotID: candidate.slot.ID, Actor: signal.Actor, EventKey: signal.EventKey,
		DeliveryID: deliveryID, AuthorizedAt: authorizedAt,
	})
	if err != nil || updated == nil {
		return false, err
	}
	// Reauthorization made the durable request due. Wake the scheduler before
	// repairing the external check so a transient GitHub failure cannot leave
	// the request asleep until an unrelated event arrives.
	g.Wake()
	target, repositorySettings, err := readControls(
		ctx, g.store, updated.TargetID, updated.RepositoryID,
	)
	if err != nil {
		return false, err
	}
	if _, err := g.Checks.EnsureAuthorized(
		ctx,
		target,
		repositorySettings,
		updated.PullRequest,
		updated.HeadSHA,
		updated.MergeMethod,
		updated.AuthorizedBy,
	); err != nil {
		return false, fmt.Errorf("restore authorized pending CI check: %w", err)
	}

	return true, nil
}

func (g *Gate) reauthorizationCandidate(
	ctx context.Context,
	repositoryID string,
	signal pendingci.Signal,
) (reauthorizationCandidate, bool, error) {
	slot, err := g.store.GetCheckSlotByHead(ctx, repositoryID, signal.HeadSHA)
	if errors.Is(err, storage.ErrNotFound) {
		return reauthorizationCandidate{}, false, nil
	}
	if err != nil {
		return reauthorizationCandidate{}, false,
			fmt.Errorf("read requested-action check slot: %w", err)
	}
	if signal.PullRequest > 0 && signal.PullRequest != slot.PullRequest {
		return reauthorizationCandidate{}, false, nil
	}
	if slot.CheckRunID == nil || *slot.CheckRunID != signal.CheckRunID ||
		slot.AppID != signal.AppID || slot.Name != signal.CheckName ||
		slot.ExternalID != signal.ExternalID {
		return reauthorizationCandidate{}, false, nil
	}
	request, err := g.store.GetArmed(ctx, repositoryID, slot.PullRequest)
	if errors.Is(err, storage.ErrNotFound) {
		return reauthorizationCandidate{}, false, nil
	}
	if err != nil {
		return reauthorizationCandidate{}, false,
			fmt.Errorf("read requested-action pending CI request: %w", err)
	}
	if request.CheckSlotID == nil || *request.CheckSlotID != slot.ID ||
		request.CandidateHeadSHA != signal.HeadSHA {
		return reauthorizationCandidate{}, false, nil
	}
	client, owner, repository, err := g.pendingCIRepositoryClient(slot)
	if err != nil {
		return reauthorizationCandidate{}, false, err
	}

	return reauthorizationCandidate{
		slot: slot, request: request, client: client, owner: owner, repository: repository,
	}, true, nil
}

func (g *Gate) preparePendingCIReauthorization(
	ctx context.Context,
	candidate reauthorizationCandidate,
	signal pendingci.Signal,
) (bool, error) {
	client := candidate.client
	owner := candidate.owner
	repository := candidate.repository
	checker, err := bot.NewPermissionChecker(ctx, client, owner, repository)
	if err != nil {
		return false, err
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
		return false, bot.NewGitHubError(bot.ErrPermissionCheck, err)
	}
	if !authorized {
		return false, nil
	}
	state, err := client.GetPullRequestState(ctx, owner, repository, candidate.slot.PullRequest)
	if err != nil {
		return false, fmt.Errorf("read requested-action pull request state: %w", err)
	}
	if !state.Open || state.HeadSHA != signal.HeadSHA ||
		state.BaseBranch != candidate.request.CandidateBaseBranch {
		return false, nil
	}
	mergeQueue, err := client.IsMergeQueueEnabled(ctx, owner, repository, state.BaseBranch)
	if err != nil {
		return false, fmt.Errorf("read requested-action merge queue policy: %w", err)
	}
	if mergeQueue {
		return false, nil
	}
	required, err := client.GetRequiredStatusChecks(ctx, owner, repository, state.BaseBranch)
	if err != nil {
		return false, fmt.Errorf("read requested-action base protection: %w", err)
	}
	if !requiredContextOwned(required, candidate.slot.Name, candidate.slot.AppID) {
		return false, nil
	}
	botConfig, err := g.config(
		ctx,
		client,
		candidate.request.TargetID,
		candidate.request.RepositoryID,
		owner,
		repository,
	)
	if err != nil {
		return false, fmt.Errorf("read requested-action configuration: %w", err)
	}
	info, err := client.GetPRInfo(ctx, owner, repository, candidate.slot.PullRequest)
	if err != nil {
		return false, fmt.Errorf("read requested-action pull request approvals: %w", err)
	}
	runtime := &bot.RuntimeConfig{CommentAuthor: signal.Actor, BotUsername: g.botUsername}
	if bot.PendingCIApprovalAllowed(runtime, botConfig, info) != nil {
		return false, nil
	}
	if bot.PendingCIApprovalRequired(runtime, info) {
		if err := client.ApprovePR(ctx, owner, repository, candidate.slot.PullRequest); err != nil {
			return false, fmt.Errorf("restore requested-action approval: %w", err)
		}
	}

	return true, nil
}

func (g *Gate) pendingCIRepositoryClient(
	slot pendingci.CheckSlot,
) (*github.Client, string, string, error) {
	return g.pendingCIClient(slot.InstallationID, slot.RepositoryFullName)
}

func (g *Gate) pendingCIClient(
	installationID int64,
	fullName string,
) (*github.Client, string, string, error) {
	token, err := g.tokens.InstallationToken(installationID)
	if err != nil {
		return nil, "", "", bot.NewGitHubError(bot.ErrGitHubAppAuth, err)
	}
	client, err := github.NewClient(token, g.apiBaseURL)
	if err != nil {
		return nil, "", "", bot.NewGitHubError(bot.ErrGitHubClient, err)
	}
	owner, repository, found := strings.Cut(fullName, "/")
	if !found || owner == "" || repository == "" || strings.Contains(repository, "/") {
		return nil, "", "", fmt.Errorf("invalid repository name %q", fullName)
	}

	return client, owner, repository, nil
}
