package main

import (
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func (s *server) decodeWebhookJob(eventName string, body []byte) (job, bool, error) {
	if eventName == webhook.EventIssueComment {
		return s.decodeIssueCommentJob(body)
	}
	if !webhook.SupportsPendingCI(eventName) {
		return job{}, false, nil
	}

	notification, err := webhook.ParsePendingCINotification(eventName, body)
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
		metadata:     notification.Metadata,
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
	if err := validateCommentInput(runtimeConfigFor(event, s.cfg)); err != nil {
		return job{}, false, fmt.Errorf("validate issue comment: %w", err)
	}

	return job{
		eventName:   webhook.EventIssueComment,
		action:      event.Action,
		metadata:    issueCommentMetadata(event),
		pullRequest: event.Issue.Number,
		comment:     event,
		key:         event.ContentKey(),
	}, true, nil
}

func issueCommentMetadata(event *webhook.IssueCommentEvent) webhook.Metadata {
	return webhook.Metadata{
		InstallationID:     event.Installation.ID,
		RepositoryID:       event.Repository.ID,
		RepositoryFullName: event.Repository.FullName,
		RepositoryOwner:    event.Repository.Owner.Login,
		RepositoryName:     event.Repository.Name,
	}
}

func relevantPendingCISignals(signals []webhook.PendingCISignal) []webhook.PendingCISignal {
	relevant := make([]webhook.PendingCISignal, 0, len(signals))
	for _, signal := range signals {
		if signal.Kind == webhook.SignalLabelRemoved {
			_, _, label := parsePendingCILabel(signal.Label)
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
		"repo", j.metadata.RepositoryFullName,
		"pr", j.pullRequest,
		"action", j.action,
		"attempt", work.Attempt,
	)

	return j, nil
}
