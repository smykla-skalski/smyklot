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
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/metrics"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

// maxStoredFailureReason is the width of the column a reason is written to.
// Trimming here rather than in the library, which does not know the column
// exists.
const maxStoredFailureReason = 2048

// initDeliveries builds the webhook pipeline and hands it this service's
// answers: what is worth a row, what to run, what to retry, and what to count.
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
			Retry:      retryDelivery,
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

// deliveryInbox adapts the durable store to the webhook library's port.
//
// It is the only translation between GitHub's numeric identifiers and the
// namespaced ones the store keys on, and the only place a stored reason is
// redacted. Both belong here rather than in pkg/webhook: the library does not
// know what this deployment's identifiers look like, and it does not know what
// a secret looks like either.
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

// disposition converts what the store decided into what the pipeline
// understands.
//
// Spelled out rather than converted between two string types that happen to
// agree today: an unknown value has to become something, and the pipeline
// refuses a delivery it cannot place rather than running it twice.
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

// reason makes an error safe to store: redacted, then cut to the column.
func (i deliveryInbox) reason(text string) string {
	if i.redactor != nil {
		text = i.redactor.String(text)
	}
	if len(text) > maxStoredFailureReason {
		text = strings.TrimSpace(text[:maxStoredFailureReason])
	}

	return text
}

// screenDelivery decides whether a delivery is worth a row.
//
// Most of what GitHub sends is noise - a comment the bot itself wrote, a
// comment on an issue that is not a pull request, a check run that produces no
// signal this service acts on - and saying no here costs one parse instead of
// one row and one execution.
func (s *server) screenDelivery(delivery webhook.Delivery) (bool, error) {
	if delivery.Event != webhook.EventIssueComment {
		notification, err := pendingci.ParseNotification(delivery.Event, delivery.Payload)
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

	// Everything about the comment that does not depend on how the process
	// authenticated. The service knows all of it from the payload before it
	// mints a token, so a delivery that could never execute is refused without
	// doing any work - and the token itself is deliberately not checked here,
	// because there is not one yet.
	if err := bot.ValidateCommentInput(runtimeConfigFor(event, s.cfg)); err != nil {
		return false, err
	}

	return true, nil
}

// executeDelivery runs one delivery. Which handler it belongs to is this
// service's business; the library only knows that something has to run.
func (s *server) executeDelivery(ctx context.Context, delivery webhook.Delivery) error {
	ctx = logging.Into(ctx, delivery.Logger)

	s.metrics.DeliveriesInFlight.Inc()
	defer s.metrics.DeliveriesInFlight.Dec()

	if delivery.Event != webhook.EventIssueComment {
		notification, err := pendingci.ParseNotification(delivery.Event, delivery.Payload)
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

// retryDelivery is the default policy plus the one error this service knows
// will not come right.
//
// A repository whose configuration file does not parse will not parse on the
// next attempt either, and the pull request has already been told. Retrying it
// eight times only delays the moment it shows up on the panel.
func retryDelivery(cause error, attempt int) (time.Duration, bool) {
	if errors.Is(cause, bot.ErrRepoConfigInvalid) {
		return 0, false
	}

	return webhook.DefaultRetry(cause, attempt)
}

// deliveryObserver is what the service counts and who it tells.
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
				s.recordFailure(delivery, err)
			}
			s.metrics.Deliveries.WithLabelValues(action, result).Inc()
		},

		// The panel shows what a delivery did, so it is told once the inbox
		// agrees the delivery is done. A retry is not done.
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

// deliveryAction is the histogram's label.
//
// A status event has no top-level action, and this histogram has always been
// labelled with its state instead. Kept here rather than pushed into the
// library, which has no reason to know that one event reports itself
// differently from the others.
func deliveryAction(delivery webhook.Delivery) string {
	if delivery.Event != webhook.EventStatus {
		return delivery.Source.Action
	}

	notification, err := pendingci.ParseNotification(delivery.Event, delivery.Payload)
	if err != nil {
		return ""
	}

	return notification.Action
}

// relevantPendingCISignals drops the signals this service does not act on.
//
// It stays here rather than in internal/pendingci because it reads
// storage.PendingCICheckName, and the storage package imports pendingci - so
// the filter cannot live where the signals do.
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

// serviceEvents are the deliveries this service acts on.
//
// One list rather than two: it decides what is worth reading and it fixes the
// metric label set, so an event outside it is counted as "other" rather than
// minting a time series per made-up header.
func serviceEvents() []string {
	return []string{
		webhook.EventIssueComment,
		webhook.EventCheckRun,
		webhook.EventCheckSuite,
		webhook.EventStatus,
		webhook.EventPullRequest,
	}
}

// deliveryAttrs is what the log lines about a delivery carry beyond what the
// pipeline already knows.
//
// The pull request is a different field in every event, so it is this
// service's to find. Attached here rather than in each handler, because a
// delivery's identifiers belong on every line about it and an attribute added
// in two places prints twice.
func deliveryAttrs(delivery webhook.Delivery) []slog.Attr {
	pullRequest := deliveryPullRequest(delivery)
	if pullRequest == 0 {
		return nil
	}

	return []slog.Attr{slog.Int("pr", pullRequest)}
}

// deliveryPullRequest is the pull request a delivery is about.
//
// Zero when there is not one to name: a check_suite covers a head rather than
// one pull request, and inventing a number would be worse than leaving it out.
func deliveryPullRequest(delivery webhook.Delivery) int {
	if delivery.Event == webhook.EventIssueComment {
		event, err := webhook.ParseIssueComment(delivery.Payload)
		if err != nil {
			return 0
		}

		return event.Issue.Number
	}

	notification, err := pendingci.ParseNotification(delivery.Event, delivery.Payload)
	if err != nil || len(notification.Signals) == 0 {
		return 0
	}

	return notification.Signals[0].PullRequest
}
