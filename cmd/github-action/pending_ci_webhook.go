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
		return s.applyPendingCINotification(ctx, repositoryID, notification, deliveryID)
	})
}

func (s *server) applyPendingCINotification(
	ctx context.Context,
	repositoryID string,
	notification *webhook.PendingCINotification,
	deliveryID string,
) error {
	occurredAt := time.Now().UTC()
	var changed int64

	for _, signal := range notification.Signals {
		count, err := s.applyPendingCISignal(
			ctx, repositoryID, occurredAt, notification.Event, deliveryID, signal,
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
	repositoryID string,
	occurredAt time.Time,
	eventName string,
	deliveryID string,
	signal webhook.PendingCISignal,
) (int64, error) {
	switch signal.Kind {
	case webhook.SignalWakePullRequest, webhook.SignalPullRequestDone, webhook.SignalLabelRemoved:
		expectedHead := ""
		if signal.MatchHead {
			expectedHead = signal.HeadSHA
		}
		changed, err := s.store.Wake(ctx, pendingci.WakeRequest{
			RepositoryID: repositoryID, PullRequest: signal.PullRequest,
			EventName: eventName, EventKey: signal.EventKey, DeliveryID: deliveryID,
			ExpectedHeadSHA: expectedHead, OccurredAt: occurredAt,
		})
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
	default:
		return 0, fmt.Errorf("unsupported signal kind %q", signal.Kind)
	}
}

func (s *server) reauthorizePendingCI(
	ctx context.Context,
	repositoryID string,
	authorizedAt time.Time,
	deliveryID string,
	signal webhook.PendingCISignal,
) (bool, error) {
	slot, err := s.store.GetCheckSlotByHead(ctx, repositoryID, signal.HeadSHA)
	if errors.Is(err, storage.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read requested-action check slot: %w", err)
	}
	if signal.PullRequest > 0 && signal.PullRequest != slot.PullRequest {
		return false, nil
	}
	if slot.CheckRunID == nil || *slot.CheckRunID != signal.CheckRunID ||
		slot.AppID != signal.AppID || slot.Name != signal.CheckName ||
		slot.ExternalID != signal.ExternalID {
		return false, nil
	}
	request, err := s.store.GetArmed(ctx, repositoryID, slot.PullRequest)
	if errors.Is(err, storage.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read requested-action pending CI request: %w", err)
	}
	if request.CheckSlotID == nil || *request.CheckSlotID != slot.ID ||
		request.CandidateHeadSHA != signal.HeadSHA {
		return false, nil
	}
	client, owner, repository, err := s.pendingCIRepositoryClient(slot)
	if err != nil {
		return false, err
	}
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
	updated, err := s.store.Reauthorize(ctx, pendingci.ReauthorizeRequest{
		RepositoryID: repositoryID, PullRequest: slot.PullRequest,
		HeadSHA: signal.HeadSHA, BaseBranch: request.CandidateBaseBranch,
		CheckSlotID: slot.ID, Actor: signal.Actor, EventKey: signal.EventKey,
		DeliveryID: deliveryID, AuthorizedAt: authorizedAt,
	})
	if err != nil || updated == nil {
		return false, err
	}
	target, repositorySettings, err := s.repositoryControls(
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

func (s *server) pendingCIRepositoryClient(
	slot pendingci.CheckSlot,
) (*github.Client, string, string, error) {
	token, err := s.tokens.InstallationToken(slot.InstallationID)
	if err != nil {
		return nil, "", "", NewGitHubError(ErrGitHubAppAuth, err)
	}
	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return nil, "", "", NewGitHubError(ErrGitHubClient, err)
	}
	owner, repository, found := strings.Cut(slot.RepositoryFullName, "/")
	if !found || owner == "" || repository == "" || strings.Contains(repository, "/") {
		return nil, "", "", fmt.Errorf("invalid repository name %q", slot.RepositoryFullName)
	}

	return client, owner, repository, nil
}
