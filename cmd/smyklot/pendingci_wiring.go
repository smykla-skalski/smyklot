package main

import (
	"context"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type inlineExclusive struct{}

func (inlineExclusive) Exclusive(_ context.Context, _ string, operation func() error) error {
	return operation()
}

func (s *server) commandEnvironment(
	client *github.Client,
	event *webhook.IssueCommentEvent,
	sourceOrder int64,
) bot.CommandEnvironment {
	guard := s.gate.ActivationGuardFor(
		client,
		storage.InstallationID(event.Installation.ID),
		storage.RepositoryID(event.Repository.ID),
		event.Repository.Owner.Login,
		event.Repository.Name,
	)
	return bot.CommandEnvironment{
		PendingCIActivation: guard,
		PendingCIMode:       guard,
		PendingCI: &bot.PendingCICommand{
			Store: s.store, Wake: s.gate.Wake,
			Coordinator:        inlineExclusive{},
			TargetID:           storage.InstallationID(event.Installation.ID),
			InstallationID:     event.Installation.ID,
			RepositoryID:       storage.RepositoryID(event.Repository.ID),
			RepositoryFullName: event.Repository.FullName,
			SourceCommentID:    event.Comment.ID,
			SourceRevision:     event.Comment.UpdatedAt,
			SourceSequence:     pendingci.CommentSequence(event.Action),
			SourceOrder:        sourceOrder,
			Now:                func() time.Time { return time.Now().UTC() },
			Checks:             s.gate.Checks,
		},
	}
}

func (s *server) reactionCommandEnvironment(repositoryID string) bot.CommandEnvironment {
	return bot.CommandEnvironment{PendingCI: &bot.PendingCICommand{
		Store: s.store, Wake: s.gate.Wake,
		Coordinator: inlineExclusive{}, RepositoryID: repositoryID,
		Now: func() time.Time { return time.Now().UTC() },
	}}
}
