package main

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/commands"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func (s *server) checkCommentRecovery(ctx context.Context, input storage.DeliveryRecoveryInput) (panel.DeliveryRecoveryCheck, error) {
	event, err := webhook.ParseIssueComment(input.Payload)
	if err != nil {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryPayloadInvalid}, nil
	}
	if err := bot.ValidateCommentInput(runtimeConfigFor(event, s.cfg)); err != nil {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryPayloadInvalid}, nil
	}
	if !event.Actionable() {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryNoAction}, nil
	}
	token, err := s.tokens.InstallationToken(event.Installation.ID)
	if err != nil {
		return panel.DeliveryRecoveryCheck{}, err
	}
	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return panel.DeliveryRecoveryCheck{}, err
	}
	return s.checkCommentRecoveryWithClient(ctx, input, event, client)
}

func (s *server) checkCommentRecoveryWithClient(ctx context.Context, input storage.DeliveryRecoveryInput, event *webhook.IssueCommentEvent, client *github.Client) (panel.DeliveryRecoveryCheck, error) {
	current, err := issueCommentIsCurrent(ctx, client, event)
	if err != nil {
		return panel.DeliveryRecoveryCheck{}, err
	}
	if !current {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoverySourceChanged}, nil
	}
	source, err := s.store.CheckSourceRevision(ctx, pendingci.SourceRevisionRequest{
		RepositoryID: *input.RepositoryID, PullRequest: event.Issue.Number, CommentID: event.Comment.ID,
		Revision: event.Comment.UpdatedAt, Sequence: pendingci.CommentSequence(event.Action),
		SourceOrder: input.SourceOrder, EventKey: input.ClaimKey, ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		return panel.DeliveryRecoveryCheck{}, err
	}
	if !source.Accepted {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoverySourceChanged}, nil
	}
	bc, err := s.serviceConfigWithoutCatalogRefresh(ctx, client, input.TargetID, *input.RepositoryID, event.Repository.Owner.Login, event.Repository.Name)
	if errors.Is(err, bot.ErrRepoConfigInvalid) {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryConfigurationInvalid}, nil
	}
	if err != nil {
		return panel.DeliveryRecoveryCheck{}, err
	}
	if bot.ServiceStandsDown(ctx, bc) {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryServiceDisabled}, nil
	}
	if event.Action == webhook.ActionDeleted {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryAvailable, Effect: "Recheck cancellation and cleanup for the deleted comment."}, nil
	}
	parsed, err := commands.ParseCommand(event.Comment.Body, bc)
	if err != nil || (!parsed.IsValid && bc.DisableReactions) {
		return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryNoAction}, nil
	}
	// Help needs no command permission; reaction handling checks each reactor at execution.
	if parsed.IsValid && !slices.Contains(parsed.Commands, commands.CommandHelp) {
		checker, err := bot.NewPermissionChecker(ctx, client, event.Repository.Owner.Login, event.Repository.Name)
		if err != nil {
			return panel.DeliveryRecoveryCheck{}, err
		}
		allowed, err := bot.CheckUserPermission(ctx, client, checker, event.Comment.User.Login, event.Repository.Owner.Login, event.Repository.Name)
		if err != nil {
			return panel.DeliveryRecoveryCheck{}, err
		}
		if !allowed {
			return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryPermissionChanged}, nil
		}
	}
	return panel.DeliveryRecoveryCheck{Reason: panel.RecoveryAvailable, Effect: "Process the original comment using current configuration and permissions."}, nil
}
