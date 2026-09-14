package main

import (
	"context"
	"errors"
	"slices"

	"github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

var _ panel.DeliveryRecoveryChecker = (*server)(nil)

func (s *server) CheckDeliveryRecovery(ctx context.Context, input storage.DeliveryRecoveryInput) (panel.DeliveryRecoveryCheck, error) {
	source, err := webhook.ParseSource(input.Payload)
	if err != nil || source.InstallationID <= 0 || source.Repository.ID <= 0 ||
		storage.InstallationID(source.InstallationID) != input.TargetID || input.RepositoryID == nil ||
		storage.RepositoryID(source.Repository.ID) != *input.RepositoryID || !slices.Contains(serviceEvents(), input.Event) {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryPayloadInvalid}, nil
	}
	target, repository, err := s.readRepositoryControls(ctx, input.TargetID, *input.RepositoryID)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && (!target.Available || !repository.Available)) {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryRepositoryUnavailable}, nil
	}
	if err != nil {
		return panel.DeliveryRecoveryCheck{}, err
	}
	if input.Event == webhook.EventPush {
		return checkConfigPushRecovery(input, target, repository)
	}
	if !storage.RepositoryEnabled(target, repository) {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryRepositoryDisabled}, nil
	}
	if input.Event == webhook.EventIssueComment {
		return s.checkCommentRecovery(ctx, input)
	}
	notification, err := pendingci.ParseNotification(input.Event, source, input.Payload)
	if err != nil {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryPayloadInvalid}, nil
	}
	return checkNotificationRecovery(ctx, s.gate, *input.RepositoryID, relevantPendingCISignals(notification.Signals))
}

type reauthorizationChecker interface {
	CheckReauthorization(context.Context, string, pendingci.Signal) (bool, error)
}

func checkNotificationRecovery(ctx context.Context, checker reauthorizationChecker, repositoryID string, signals []pendingci.Signal) (panel.DeliveryRecoveryCheck, error) {
	if len(signals) == 0 {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryNoAction}, nil
	}
	effect := "Recheck current pull request and CI state using the original event."
	for _, signal := range signals {
		if signal.Kind != pendingci.SignalReauthorize {
			continue
		}
		allowed, err := checker.CheckReauthorization(ctx, repositoryID, signal)
		if err != nil {
			return panel.DeliveryRecoveryCheck{}, err
		}
		if !allowed {
			return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryReauthorizationUnavailable}, nil
		}
		effect = "Recheck CI authorization and restore the required approval using the original request."
	}
	return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryAvailable, Effect: effect}, nil
}

func checkConfigPushRecovery(input storage.DeliveryRecoveryInput, target storage.Target, repository storage.Repository) (panel.DeliveryRecoveryCheck, error) {
	push, err := parseConfigFilePush(input.Payload)
	if err != nil {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryPayloadInvalid}, nil
	}
	if !push.relevant() || push.Ref != "refs/heads/"+repository.DefaultBranch {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryNoAction}, nil
	}
	connected := repository.ConfigFileSyncEnabled || (repository.Name == ".github" && target.ConfigFileSyncEnabled)
	if !connected {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryConfigurationDisconnected}, nil
	}
	return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryAvailable, Effect: "Reload configuration from the current default branch."}, nil
}
