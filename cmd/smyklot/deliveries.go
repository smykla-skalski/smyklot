package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/pendingci/gate"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/metrics"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

const maxStoredFailureReason = 2048

func (s *server) initDeliveries(
	redactor *logging.Redactor,
	registry prometheus.Registerer,
) error {
	deliveries, err := webhook.New(
		s.cfg.webhookSecret,
		deliveryInbox{store: s.store, redactor: redactor},
		s.executeDelivery,
		webhook.Options{
			Events:     serviceEvents(),
			Screen:     s.screenDelivery,
			Retry:      s.retryDelivery,
			Workers:    workerCount,
			QueueDepth: queueDepth,
			Timeouts: webhook.Timeouts{
				Job:          jobTimeout,
				Finalization: deliveryFinalizationTimeout,
				Drain:        drainTimeout,
			},
			Logger:   s.logger,
			Observer: s.deliveryObserver(),
			Attrs:    deliveryAttrs,
		},
	)
	if err != nil {
		return err
	}

	s.deliveries = deliveries
	metrics.RegisterQueue(
		registry, func() float64 { return float64(deliveries.QueueDepth()) }, queueDepth,
	)

	return nil
}

type deliveryInbox struct {
	store    storage.DeliveryStore
	redactor *logging.Redactor
}

func (i deliveryInbox) Claim(
	ctx context.Context,
	claim webhook.Claim,
) (webhook.ClaimResult, error) {
	repositoryID := storage.RepositoryID(claim.Source.Repository.ID)

	result, err := i.store.ClaimDelivery(ctx, storage.DeliveryClaim{
		ClaimKey:           claim.Key,
		DeliveryID:         claim.DeliveryID,
		TargetID:           storage.InstallationID(claim.Source.InstallationID),
		RepositoryID:       &repositoryID,
		RepositoryFullName: claim.Source.Repository.FullName,
		Event:              claim.Event,
		Payload:            claim.Payload,
		ClaimedAt:          claim.At,
	})
	if err != nil {
		return webhook.ClaimResult{}, err
	}

	return webhook.ClaimResult{ID: result.ID, Disposition: disposition(result.Disposition)}, nil
}

func disposition(stored storage.DeliveryClaimDisposition) webhook.Disposition {
	switch stored {
	case storage.DeliveryClaimAccepted:
		return webhook.Accepted
	case storage.DeliveryClaimInProgress:
		return webhook.InProgress
	case storage.DeliveryClaimRetained:
		return webhook.Retained
	default:
		return webhook.Disposition(stored)
	}
}

func (i deliveryInbox) Lease(
	ctx context.Context,
	now, expires time.Time,
) (webhook.Lease, error) {
	result, err := i.store.LeaseDelivery(ctx, now, expires)
	if err != nil || result.Work == nil {
		return webhook.Lease{AvailableAt: result.AvailableAt}, err
	}

	return webhook.Lease{Work: &webhook.Work{
		ClaimID:    result.Work.ID,
		Key:        result.Work.ClaimKey,
		DeliveryID: result.Work.DeliveryID,
		Event:      result.Work.Event,
		Payload:    result.Work.Payload,
		Attempt:    result.Work.Attempt,
	}}, nil
}

func (i deliveryInbox) Complete(ctx context.Context, claimID int64, at time.Time) error {
	return i.store.CompleteDelivery(ctx, claimID, at)
}

func (i deliveryInbox) Fail(ctx context.Context, failure webhook.Failure) error {
	return i.store.FailDelivery(ctx, storage.DeliveryFailureChange{
		ClaimID:   failure.ClaimID,
		Stage:     failure.Stage,
		Reason:    i.reason(failure.Reason),
		Retryable: failure.Retryable,
		FailedAt:  failure.At,
	})
}

func (i deliveryInbox) Retry(ctx context.Context, retry webhook.Retry) error {
	return i.store.RetryDelivery(ctx, storage.DeliveryRetryChange{
		ClaimID: retry.ClaimID,
		Stage:   retry.Stage,
		Reason:  i.reason(retry.Reason),
		RetryAt: retry.At,
	})
}

func (i deliveryInbox) reason(text string) string {
	if i.redactor != nil {
		text = i.redactor.String(text)
	}
	if len(text) > maxStoredFailureReason {
		text = strings.TrimSpace(text[:maxStoredFailureReason])
	}

	return text
}

func (s *server) screenDelivery(delivery webhook.Delivery) (bool, error) {
	if delivery.Event != webhook.EventIssueComment {
		notification, err := pendingci.ParseNotification(delivery.Event, delivery.Source, delivery.Payload)
		if err != nil {
			return false, err
		}

		return len(relevantPendingCISignals(notification.Signals)) > 0, nil
	}

	event, err := webhook.ParseIssueComment(delivery.Payload)
	if err != nil {
		return false, err
	}
	if !event.Actionable() {
		return false, nil
	}
	if err := bot.ValidateCommentInput(runtimeConfigFor(event, s.cfg)); err != nil {
		return false, err
	}

	return true, nil
}

func (s *server) executeDelivery(ctx context.Context, delivery webhook.Delivery) error {
	ctx = logging.Into(ctx, delivery.Logger)

	s.metrics.DeliveriesInFlight.Inc()
	defer s.metrics.DeliveriesInFlight.Dec()

	if delivery.Event != webhook.EventIssueComment {
		notification, err := pendingci.ParseNotification(delivery.Event, delivery.Source, delivery.Payload)
		if err != nil {
			return err
		}
		notification.Signals = relevantPendingCISignals(notification.Signals)

		return s.gate.HandleWebhook(ctx, notification, delivery.ID)
	}

	event, err := webhook.ParseIssueComment(delivery.Payload)
	if err != nil {
		return err
	}

	if s.panel != nil {
		enabled, err := s.repositoryEnabled(ctx, event)
		if err != nil {
			return err
		}
		if !enabled {
			logging.From(ctx).Info("delivery ignored: repository is disabled")

			return nil
		}
	}

	return s.handleIssueComment(ctx, event, delivery.Key, delivery.ClaimID)
}

func retryDelivery(cause error, attempt int) (time.Duration, bool) {
	if errors.Is(cause, bot.ErrRepoConfigInvalid) {
		return 0, false
	}

	return webhook.DefaultRetry(cause, attempt)
}

func (s *server) retryDelivery(cause error, attempt int) (time.Duration, bool) {
	if errors.Is(cause, bot.ErrRepoConfigInvalid) {
		return 0, false
	}
	if s.store == nil {
		return retryDelivery(cause, attempt)
	}
	policy, err := s.store.GetEffectiveQueuePolicy(
		context.Background(), workqueue.KindWebhookDelivery, nil,
	)
	if err != nil {
		return retryDelivery(cause, attempt)
	}
	retry, err := workqueue.ParseWebhookRetry(policy.Configuration, workqueue.WebhookRetry{
		MaxDelay: 5 * time.Minute, MaxAttempts: 8,
	})
	if err != nil || attempt >= retry.MaxAttempts {
		return 0, false
	}
	var classified interface{ Retryable() bool }
	if errors.As(cause, &classified) && !classified.Retryable() {
		return 0, false
	}
	delay := policy.RetryDelay
	if delay <= 0 {
		delay = 2 * time.Second
	}
	for index := 1; index < attempt && delay < retry.MaxDelay; index++ {
		delay *= 2
	}
	if delay > retry.MaxDelay {
		delay = retry.MaxDelay
	}

	return delay, true
}

func (s *server) deliveryObserver() webhook.Observer {
	return webhook.Observer{
		Received: func(event, outcome string) {
			s.metrics.WebhookRequests.WithLabelValues(event, outcome).Inc()
		},

		Executed: func(delivery webhook.Delivery, elapsed time.Duration, err error) {
			action := deliveryAction(delivery)
			s.metrics.DeliveryDuration.WithLabelValues(action).Observe(elapsed.Seconds())

			result := metrics.ResultSuccess
			if err != nil {
				result = metrics.ResultFailure
				if _, again := s.retryDelivery(err, delivery.Attempt); !again {
					s.recordFailure(delivery, action, err)
				}
			}
			s.metrics.Deliveries.WithLabelValues(action, result).Inc()
		},

		Finalized: func(delivery webhook.Delivery, outcome webhook.Outcome) {
			if outcome == webhook.OutcomeRetrying || s.panel == nil {
				return
			}
			s.panel.Announce(
				storage.InstallationID(delivery.Source.InstallationID),
				storage.RepositoryID(delivery.Source.Repository.ID),
			)
		},
	}
}

func deliveryAction(delivery webhook.Delivery) string {
	if delivery.Event != webhook.EventStatus {
		return delivery.Source.Action
	}

	notification, err := pendingci.ParseNotification(delivery.Event, delivery.Source, delivery.Payload)
	if err != nil {
		return ""
	}

	return notification.Action
}

func relevantPendingCISignals(signals []pendingci.Signal) []pendingci.Signal {
	relevant := make([]pendingci.Signal, 0, len(signals))
	for _, signal := range signals {
		if signal.Kind == pendingci.SignalReauthorize &&
			(signal.ActionID != gate.ReauthorizeAction ||
				signal.CheckName != storage.PendingCICheckName) {
			continue
		}
		if signal.Kind == pendingci.SignalLabelRemoved {
			_, _, label := bot.ParsePendingCILabel(signal.Label)
			if label == "" {
				continue
			}
		}
		relevant = append(relevant, signal)
	}

	return relevant
}

func serviceEvents() []string {
	return []string{
		webhook.EventIssueComment,
		webhook.EventCheckRun,
		webhook.EventCheckSuite,
		webhook.EventStatus,
		webhook.EventPullRequest,
	}
}

func deliveryAttrs(delivery webhook.Delivery) []slog.Attr {
	pullRequest := deliveryPullRequest(delivery)
	if pullRequest == 0 {
		return nil
	}

	return []slog.Attr{slog.Int("pr", pullRequest)}
}

func deliveryPullRequest(delivery webhook.Delivery) int {
	if delivery.Event == webhook.EventIssueComment {
		event, err := webhook.ParseIssueComment(delivery.Payload)
		if err != nil {
			return 0
		}

		return event.Issue.Number
	}

	notification, err := pendingci.ParseNotification(delivery.Event, delivery.Source, delivery.Payload)
	if err != nil {
		return 0
	}

	signals := relevantPendingCISignals(notification.Signals)
	if len(signals) != 1 {
		return 0
	}

	return signals[0].PullRequest
}
