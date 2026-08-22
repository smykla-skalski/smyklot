package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

// handlePendingCIWebhook applies only durable state transitions. Webhook
// payloads are wake-up hints; the reconciler reads live pull-request and check
// state before it ever decides to merge.
func (s *server) handlePendingCIWebhook(
	ctx context.Context,
	notification *webhook.PendingCINotification,
	deliveryID string,
) error {
	repositoryID := repositoryStorageID(notification.Metadata.RepositoryID)
	return s.pendingCICoordinator.Exclusive(ctx, repositoryID, func() error {
		return s.applyPendingCINotification(ctx, notification, deliveryID)
	})
}

func (s *server) applyPendingCINotification(
	ctx context.Context,
	notification *webhook.PendingCINotification,
	deliveryID string,
) error {
	if notification.Event == webhook.EventPullRequest && notification.Action == "opened" {
		// A repository can be added while reaction polling is disabled. Its first
		// pull request is enough reason to refresh the catalog and provision the
		// required context even when the webhook arrived before the catalog knew it.
		s.WakePendingCIGates()
	}
	occurredAt := time.Now().UTC()
	var changed int64

	for _, signal := range notification.Signals {
		count, err := s.applyPendingCISignal(
			ctx, notification.Metadata, occurredAt, notification.Event, deliveryID, signal,
		)
		if err != nil {
			return fmt.Errorf("apply %s pending CI signal: %w", notification.Event, err)
		}
		changed += count
	}
	if changed > 0 {
		s.pendingCI.Wake()
		logging.From(ctx).Info("pending CI requests notified", "requests", changed)
	} else {
		logging.From(ctx).Debug("pending CI webhook matched no armed request")
	}

	return nil
}

func (s *server) applyPendingCISignal(
	ctx context.Context,
	metadata webhook.Metadata,
	occurredAt time.Time,
	eventName string,
	deliveryID string,
	signal webhook.PendingCISignal,
) (int64, error) {
	repositoryID := repositoryStorageID(metadata.RepositoryID)
	switch signal.Kind {
	case webhook.SignalWakePullRequest:
		changed, err := s.wakePendingCIPullRequest(
			ctx, repositoryID, occurredAt, eventName, deliveryID, signal,
		)
		if err != nil {
			return 0, err
		}
		if err := s.ensureWebhookPendingCIBaseline(ctx, metadata, signal.PullRequest); err != nil {
			return 0, err
		}
		if changed {
			return 1, nil
		}

		return 0, nil
	case webhook.SignalPullRequestDone, webhook.SignalLabelRemoved:
		changed, err := s.wakePendingCIPullRequest(
			ctx, repositoryID, occurredAt, eventName, deliveryID, signal,
		)
		if changed {
			return 1, err
		}

		return 0, err
	case webhook.SignalWakeHead:
		return s.store.WakeByHead(ctx, pendingci.WakeHeadRequest{
			RepositoryID: repositoryID, HeadSHA: signal.HeadSHA,
			EventName: eventName, EventKey: signal.EventKey,
			DeliveryID: deliveryID, OccurredAt: occurredAt,
		})
	case webhook.SignalReauthorize:
		changed, err := s.reauthorizePendingCI(
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
	case webhook.SignalRerequestCheck:
		if s.pendingCIChecks != nil {
			_, err := s.pendingCIChecks.RefreshRerequest(
				ctx, repositoryID, signal.HeadSHA, signal.AppID, signal.CheckRunID,
				signal.CheckName, signal.ExternalID, eventName == webhook.EventCheckRun,
			)
			if err != nil {
				return 0, fmt.Errorf("restore rerequested pending CI check: %w", err)
			}
		}

		return s.store.WakeByHead(ctx, pendingci.WakeHeadRequest{
			RepositoryID: repositoryID, HeadSHA: signal.HeadSHA,
			EventName: eventName, EventKey: signal.EventKey,
			DeliveryID: deliveryID, OccurredAt: occurredAt,
		})
	default:
		return 0, fmt.Errorf("unsupported signal kind %q", signal.Kind)
	}
}

func (s *server) wakePendingCIPullRequest(
	ctx context.Context,
	repositoryID string,
	occurredAt time.Time,
	eventName, deliveryID string,
	signal webhook.PendingCISignal,
) (bool, error) {
	expectedHead := ""
	if signal.MatchHead {
		expectedHead = signal.HeadSHA
	}
	return s.store.Wake(ctx, pendingci.WakeRequest{
		RepositoryID: repositoryID, PullRequest: signal.PullRequest,
		EventName: eventName, EventKey: signal.EventKey, DeliveryID: deliveryID,
		ExpectedHeadSHA: expectedHead, OccurredAt: occurredAt,
	})
}

func (s *server) ensureWebhookPendingCIBaseline(
	ctx context.Context,
	metadata webhook.Metadata,
	pullRequest int,
) error {
	if s.panel == nil || s.pendingCIChecks == nil || pullRequest <= 0 {
		return nil
	}
	target, repository, eligible, err := s.webhookPendingCIBaselineControls(
		ctx,
		metadata,
		pullRequest,
	)
	if err != nil || !eligible {
		return err
	}
	installationID, err := parsePositiveInt64(target.InstallationID)
	if err != nil {
		return err
	}
	client, owner, name, err := s.pendingCIClient(installationID, repository.FullName)
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
	if !pendingCIBranchIncluded(state.BaseBranch, repository.DefaultBranch, patterns) {
		return nil
	}
	_, err = s.pendingCIChecks.EnsureBaseline(
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

func (s *server) webhookPendingCIBaselineControls(
	ctx context.Context,
	metadata webhook.Metadata,
	pullRequest int,
) (storage.Target, storage.Repository, bool, error) {
	targetID := installationStorageID(metadata.InstallationID)
	repositoryID := repositoryStorageID(metadata.RepositoryID)
	target, repository, err := s.readRepositoryControls(ctx, targetID, repositoryID)
	if errors.Is(err, storage.ErrNotFound) {
		return storage.Target{}, storage.Repository{}, false, nil
	}
	if err != nil {
		return storage.Target{}, storage.Repository{}, false,
			fmt.Errorf("read pull request baseline controls: %w", err)
	}
	gate, err := s.store.GetPendingCIRepositoryGate(ctx, repositoryID)
	if err != nil {
		return storage.Target{}, storage.Repository{}, false,
			fmt.Errorf("read pull request baseline gate: %w", err)
	}
	if gate.DesiredMode != storage.PendingCIModeChecks &&
		gate.EffectiveMode != storage.PendingCIEffectiveChecks {
		return target, repository, false, nil
	}
	enabled := target.RepositoryDefaultEnabled
	if repository.EnabledOverride != nil {
		enabled = *repository.EnabledOverride
	}
	if !enabled || !target.Available || !repository.Available || !target.Grants("checks") {
		return target, repository, false, nil
	}
	armed, err := s.store.GetArmed(ctx, repositoryID, pullRequest)
	if err == nil && armed.ArtifactKind == pendingci.ArtifactCheck {
		return target, repository, false, nil
	}
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return storage.Target{}, storage.Repository{}, false,
			fmt.Errorf("read pull request baseline owner: %w", err)
	}

	return target, repository, true, nil
}

type pendingCIReauthorizationCandidate struct {
	slot       pendingci.CheckSlot
	request    pendingci.Request
	client     *github.Client
	owner      string
	repository string
}

func (s *server) reauthorizePendingCI(
	ctx context.Context,
	repositoryID string,
	authorizedAt time.Time,
	deliveryID string,
	signal webhook.PendingCISignal,
) (bool, error) {
	candidate, found, err := s.pendingCIReauthorizationCandidate(ctx, repositoryID, signal)
	if err != nil || !found {
		return false, err
	}
	allowed, err := s.preparePendingCIReauthorization(ctx, candidate, signal)
	if err != nil || !allowed {
		return false, err
	}
	updated, err := s.store.Reauthorize(ctx, pendingci.ReauthorizeRequest{
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
	s.pendingCI.Wake()
	target, repositorySettings, err := s.readRepositoryControls(
		ctx,
		updated.TargetID,
		updated.RepositoryID,
	)
	if err != nil {
		return false, err
	}
	if _, err := s.pendingCIChecks.EnsureAuthorized(
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

func (s *server) pendingCIReauthorizationCandidate(
	ctx context.Context,
	repositoryID string,
	signal webhook.PendingCISignal,
) (pendingCIReauthorizationCandidate, bool, error) {
	slot, err := s.store.GetCheckSlotByHead(ctx, repositoryID, signal.HeadSHA)
	if errors.Is(err, storage.ErrNotFound) {
		return pendingCIReauthorizationCandidate{}, false, nil
	}
	if err != nil {
		return pendingCIReauthorizationCandidate{}, false,
			fmt.Errorf("read requested-action check slot: %w", err)
	}
	if signal.PullRequest > 0 && signal.PullRequest != slot.PullRequest {
		return pendingCIReauthorizationCandidate{}, false, nil
	}
	if slot.CheckRunID == nil || *slot.CheckRunID != signal.CheckRunID ||
		slot.AppID != signal.AppID || slot.Name != signal.CheckName ||
		slot.ExternalID != signal.ExternalID {
		return pendingCIReauthorizationCandidate{}, false, nil
	}
	request, err := s.store.GetArmed(ctx, repositoryID, slot.PullRequest)
	if errors.Is(err, storage.ErrNotFound) {
		return pendingCIReauthorizationCandidate{}, false, nil
	}
	if err != nil {
		return pendingCIReauthorizationCandidate{}, false,
			fmt.Errorf("read requested-action pending CI request: %w", err)
	}
	if request.CheckSlotID == nil || *request.CheckSlotID != slot.ID ||
		request.CandidateHeadSHA != signal.HeadSHA {
		return pendingCIReauthorizationCandidate{}, false, nil
	}
	client, owner, repository, err := s.pendingCIRepositoryClient(slot)
	if err != nil {
		return pendingCIReauthorizationCandidate{}, false, err
	}

	return pendingCIReauthorizationCandidate{
		slot: slot, request: request, client: client, owner: owner, repository: repository,
	}, true, nil
}

func (s *server) preparePendingCIReauthorization(
	ctx context.Context,
	candidate pendingCIReauthorizationCandidate,
	signal webhook.PendingCISignal,
) (bool, error) {
	client := candidate.client
	owner := candidate.owner
	repository := candidate.repository
	checker, err := newPermissionChecker(ctx, client, owner, repository)
	if err != nil {
		return false, err
	}
	authorized, err := checkUserPermission(
		ctx,
		client,
		checker,
		signal.Actor,
		owner,
		repository,
	)
	if err != nil {
		return false, NewGitHubError(ErrPermissionCheck, err)
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
	if !pendingCIRequiredContextOwned(required, candidate.slot.Name, candidate.slot.AppID) {
		return false, nil
	}
	botConfig, err := s.serviceConfigWithoutCatalogRefresh(
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
	runtime := &RuntimeConfig{CommentAuthor: signal.Actor, BotUsername: s.cfg.botUsername}
	if pendingCIApprovalAllowed(runtime, botConfig, info) != nil {
		return false, nil
	}
	if pendingCIApprovalRequired(runtime, info) {
		if err := client.ApprovePR(ctx, owner, repository, candidate.slot.PullRequest); err != nil {
			return false, fmt.Errorf("restore requested-action approval: %w", err)
		}
	}

	return true, nil
}

func (s *server) pendingCIRepositoryClient(
	slot pendingci.CheckSlot,
) (*github.Client, string, string, error) {
	return s.pendingCIClient(slot.InstallationID, slot.RepositoryFullName)
}

func (s *server) pendingCIClient(
	installationID int64,
	fullName string,
) (*github.Client, string, string, error) {
	token, err := s.tokens.InstallationToken(installationID)
	if err != nil {
		return nil, "", "", NewGitHubError(ErrGitHubAppAuth, err)
	}
	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return nil, "", "", NewGitHubError(ErrGitHubClient, err)
	}
	owner, repository, found := strings.Cut(fullName, "/")
	if !found || owner == "" || repository == "" || strings.Contains(repository, "/") {
		return nil, "", "", fmt.Errorf("invalid repository name %q", fullName)
	}

	return client, owner, repository, nil
}
