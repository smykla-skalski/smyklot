package main

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
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
	default:
		return 0, fmt.Errorf("unsupported signal kind %q", signal.Kind)
	}
}
