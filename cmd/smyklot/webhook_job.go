package main

import (
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/pendingci/gate"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func (s *server) decodeWebhookJob(eventName string, body []byte) (job, bool, error) {
	if eventName == webhook.EventIssueComment {
		return s.decodeIssueCommentJob(body)
	}
	if !pendingci.Supports(eventName) {
		return job{}, false, nil
	}

	notification, err := pendingci.ParseNotification(eventName, body)
	if err != nil {
		return job{}, false, err
	}
	notification.Signals = relevantPendingCISignals(notification.Signals)
	if len(notification.Signals) == 0 {
		return job{}, false, nil
	}
	pullRequest := 0
	if len(notification.Signals) == 1 {
		pullRequest = notification.Signals[0].PullRequest
	}

	return job{
		eventName:    eventName,
		action:       notification.Action,
		source:       notification.Source,
		pullRequest:  pullRequest,
		notification: notification,
		key:          notification.Key,
	}, true, nil
}

func (s *server) decodeIssueCommentJob(body []byte) (job, bool, error) {
	event, err := webhook.ParseIssueComment(body)
	if err != nil {
		return job{}, false, err
	}
	if !event.Actionable() {
		return job{}, false, nil
	}
	if err := bot.ValidateCommentInput(runtimeConfigFor(event, s.cfg)); err != nil {
		return job{}, false, fmt.Errorf("validate issue comment: %w", err)
	}

	return job{
		eventName:   webhook.EventIssueComment,
		action:      event.Action,
		source:      issueCommentSource(event),
		pullRequest: event.Issue.Number,
		comment:     event,
		key:         event.ContentKey(),
	}, true, nil
}

func issueCommentSource(event *webhook.IssueCommentEvent) webhook.Source {
	return webhook.Source{
		InstallationID: event.Installation.ID,
		Repository: webhook.Repository{
			ID:       event.Repository.ID,
			Owner:    event.Repository.Owner.Login,
			Name:     event.Repository.Name,
			FullName: event.Repository.FullName,
		},
		Action: event.Action,
	}
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

func (s *server) deliveryJob(work storage.DeliveryWork) (job, error) {
	j, actionable, err := s.decodeWebhookJob(work.Event, work.Payload)
	if err != nil {
		return job{}, fmt.Errorf("parse persisted %s delivery: %w", work.Event, err)
	}
	if !actionable {
		return job{}, errors.New("persisted delivery is not actionable")
	}

	j.key = work.ClaimKey
	j.deliveryID = work.DeliveryID
	j.claimID = work.ID
	j.attempt = work.Attempt
	j.logger = s.logger.With(
		"delivery_id", work.DeliveryID,
		"event", work.Event,
		"repo", j.source.Repository.FullName,
		"pr", j.pullRequest,
		"action", j.action,
		"attempt", work.Attempt,
	)

	return j, nil
}
