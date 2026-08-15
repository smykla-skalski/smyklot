package main

import (
	"context"
	"errors"
	"fmt"
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
) error {
	repositoryID := repositoryStorageID(notification.Metadata.RepositoryID)
	return s.pendingCICoordinator.Exclusive(ctx, repositoryID, func() error {
		return s.applyPendingCINotification(ctx, repositoryID, notification)
	})
}

func (s *server) applyPendingCINotification(
	ctx context.Context,
	repositoryID string,
	notification *webhook.PendingCINotification,
) error {
	occurredAt := time.Now().UTC()
	var changed int64

	for _, signal := range notification.Signals {
		if signal.Kind == webhook.SignalLabelRemoved &&
			signal.Label == github.LabelPendingCIServiceOwner {
			if err := s.restorePendingCIServiceOwnership(
				ctx, repositoryID, notification.Metadata, signal.PullRequest,
			); err != nil {
				return fmt.Errorf("restore pending CI service ownership: %w", err)
			}
		}
		count, err := s.applyPendingCISignal(ctx, repositoryID, occurredAt, signal)
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

func (s *server) restorePendingCIServiceOwnership(
	ctx context.Context,
	repositoryID string,
	metadata webhook.Metadata,
	pullRequest int,
) error {
	_, err := s.store.GetArmed(ctx, repositoryID, pullRequest)
	if errors.Is(err, storage.ErrNotFound) {
		cleanupPending, cleanupErr := s.store.HasPendingCleanup(
			ctx,
			pendingci.CleanupFilter{
				RepositoryID:         repositoryID,
				PullRequest:          pullRequest,
				ArtifactsPendingOnly: true,
			},
		)
		if cleanupErr != nil {
			return fmt.Errorf("read pending CI cleanup ownership: %w", cleanupErr)
		}
		if !cleanupPending {
			return nil
		}
		err = nil
	}
	if err != nil {
		return fmt.Errorf("read armed pending CI request: %w", err)
	}
	token, err := s.tokens.InstallationToken(metadata.InstallationID)
	if err != nil {
		return NewGitHubError(ErrGitHubAppAuth, err)
	}
	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return NewGitHubError(ErrGitHubClient, err)
	}

	return client.AddLabel(
		ctx,
		metadata.RepositoryOwner,
		metadata.RepositoryName,
		pullRequest,
		github.LabelPendingCIServiceOwner,
	)
}

func (s *server) applyPendingCISignal(
	ctx context.Context,
	repositoryID string,
	occurredAt time.Time,
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
			EventKey: signal.EventKey, ExpectedHeadSHA: expectedHead, OccurredAt: occurredAt,
		})
		if changed {
			return 1, err
		}

		return 0, err
	case webhook.SignalWakeHead:
		return s.store.WakeByHead(ctx, pendingci.WakeHeadRequest{
			RepositoryID: repositoryID, HeadSHA: signal.HeadSHA,
			EventKey: signal.EventKey, OccurredAt: occurredAt,
		})
	default:
		return 0, fmt.Errorf("unsupported signal kind %q", signal.Kind)
	}
}
