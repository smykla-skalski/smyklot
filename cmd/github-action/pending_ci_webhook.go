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
) error {
	repositoryID := repositoryStorageID(notification.Metadata.RepositoryID)
	occurredAt := time.Now().UTC()
	var changed int64

	for _, signal := range notification.Signals {
		count, err := s.applyPendingCISignal(ctx, repositoryID, occurredAt, signal)
		if err != nil {
			return fmt.Errorf("apply %s pending CI signal: %w", notification.Event, err)
		}
		changed += count
	}
	if changed > 0 {
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
	signal webhook.PendingCISignal,
) (int64, error) {
	switch signal.Kind {
	case webhook.SignalWakePullRequest:
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
	case webhook.SignalPullRequestDone:
		lifecycle := pendingci.LifecycleCancelled
		reason := "pull request closed before pending CI merge"
		if signal.Merged {
			lifecycle = pendingci.LifecycleMerged
			reason = "pull request merged"
		}

		return s.finishPendingCIPR(ctx, repositoryID, signal.PullRequest, lifecycle, reason, occurredAt)
	case webhook.SignalLabelRemoved:
		return s.finishPendingCIPR(
			ctx,
			repositoryID,
			signal.PullRequest,
			pendingci.LifecycleCancelled,
			"pending CI label removed",
			occurredAt,
		)
	default:
		return 0, fmt.Errorf("unsupported signal kind %q", signal.Kind)
	}
}

func (s *server) finishPendingCIPR(
	ctx context.Context,
	repositoryID string,
	pullRequest int,
	lifecycle pendingci.Lifecycle,
	reason string,
	finishedAt time.Time,
) (int64, error) {
	request, err := s.store.FinishPR(ctx, pendingci.FinishPRRequest{
		RepositoryID: repositoryID,
		PullRequest:  pullRequest,
		Lifecycle:    lifecycle,
		Reason:       reason,
		FinishedAt:   finishedAt,
	})
	if request == nil {
		return 0, err
	}

	return 1, err
}
