package main

import (
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func (s *server) commandEnvironment(
	client *github.Client,
	event *webhook.IssueCommentEvent,
	sourceOrder int64,
) bot.CommandEnvironment {
	guard := githubPendingCIActivationGuard{
		server: s, client: client,
		targetID:     storage.InstallationID(event.Installation.ID),
		repositoryID: storage.RepositoryID(event.Repository.ID),
		owner:        event.Repository.Owner.Login,
		repository:   event.Repository.Name,
	}
	return bot.CommandEnvironment{
		PendingCIActivation: guard,
		PendingCIMode:       guard,
		PendingCI: &bot.PendingCICommand{
			Store: s.store, Wake: s.pendingCI.Wake,
			Coordinator:        s.pendingCICoordinator,
			TargetID:           storage.InstallationID(event.Installation.ID),
			InstallationID:     event.Installation.ID,
			RepositoryID:       storage.RepositoryID(event.Repository.ID),
			RepositoryFullName: event.Repository.FullName,
			SourceCommentID:    event.Comment.ID,
			SourceRevision:     event.Comment.UpdatedAt,
			SourceSequence:     event.SourceSequence(),
			SourceOrder:        sourceOrder,
			Now:                func() time.Time { return time.Now().UTC() },
			Checks:             s.pendingCIChecks,
		},
	}
}

func (s *server) reactionCommandEnvironment(repositoryID string) bot.CommandEnvironment {
	return bot.CommandEnvironment{PendingCI: &bot.PendingCICommand{
		Store: s.store, Wake: s.pendingCI.Wake,
		Coordinator: s.pendingCICoordinator, RepositoryID: repositoryID,
		Now: func() time.Time { return time.Now().UTC() },
	}}
}
