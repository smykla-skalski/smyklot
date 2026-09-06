package main

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type bypassIdentitySource func(context.Context) ([]github.BypassActorIdentity, error)

const bypassInstallationUnknown = "unknown"

func (s *server) refreshBypassDirectory(ctx context.Context, client *github.Client, target storage.Target, directory *bypassDirectory) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	items := []github.BypassActorIdentity{}
	directory.currentAppID = 0
	directory.installations = nil
	directory.installationsKnown = false
	complete := true
	if app, err := s.currentBypassApp(ctx, client); err == nil {
		items = append(items, app)
		directory.currentAppID = app.ActorID
	} else {
		complete = false
	}
	var sources []bypassIdentitySource
	if target.Kind == storage.TargetOrganization {
		installations, err := client.ListOrganizationAppInstallations(ctx, target.Account.Login)
		directory.installations, directory.installationsKnown = installations, err == nil
		if err != nil {
			complete = false
		}
		sources = append(sources, installedBypassSources(client, installations, directory.currentAppID)...)
		sources = append(sources, organizationBypassSources(client, target.Account.Login)...)
	}
	repositories, err := s.store.ListRepositories(ctx, target.ID)
	if err != nil {
		return err
	}
	for _, repository := range repositories {
		if !repository.Available {
			continue
		}
		owner, name, ok := strings.Cut(repository.FullName, "/")
		if !ok {
			continue
		}
		sources = append(sources, func(ctx context.Context) ([]github.BypassActorIdentity, error) {
			return client.ListRepositoryBypassIdentities(ctx, owner, name)
		})
	}
	observed, sourcesComplete := collectBypassIdentities(ctx, sources)
	for _, actor := range observed {
		if !slices.ContainsFunc(items, func(existing github.BypassActorIdentity) bool {
			return existing.ActorID == actor.ActorID && existing.ActorType == actor.ActorType
		}) {
			items = append(items, actor)
		}
	}
	for index := range items {
		items[index].InstallationStatus = bypassInstallationStatus(items[index], directory)
	}
	directory.items, directory.complete = items, complete && sourcesComplete
	ttl := candidateTTL
	if !directory.complete {
		ttl = time.Minute
	}
	directory.expires = time.Now().Add(ttl)
	return nil
}

func installedBypassSources(client *github.Client, installations []github.AppInstallationAccess, currentAppID int64) []bypassIdentitySource {
	var sources []bypassIdentitySource
	for _, installation := range installations {
		if installation.AppID == currentAppID || installation.Slug == "" {
			continue
		}
		sources = append(sources, func(ctx context.Context) ([]github.BypassActorIdentity, error) {
			actor, err := client.ResolveBypassActor(ctx, "", "Integration", installation.Slug)
			if err != nil {
				return nil, err
			}
			return []github.BypassActorIdentity{actor}, nil
		})
	}
	return sources
}

func organizationBypassSources(client *github.Client, organization string) []bypassIdentitySource {
	return []bypassIdentitySource{
		func(ctx context.Context) ([]github.BypassActorIdentity, error) {
			return client.ListBypassTeams(ctx, organization)
		},
		func(ctx context.Context) ([]github.BypassActorIdentity, error) {
			users, err := client.ListOrganizationMembers(ctx, organization)
			if err != nil {
				return nil, err
			}
			items := make([]github.BypassActorIdentity, 0, len(users))
			for _, user := range users {
				name := user.Login
				if user.Name != nil && *user.Name != "" {
					name = *user.Name
				}
				items = append(items, github.BypassActorIdentity{ActorID: user.ID, ActorType: "User", Name: name, Slug: user.Login, AvatarURL: user.AvatarURL})
			}
			return items, nil
		},
	}
}

func bypassInstallationStatus(actor github.BypassActorIdentity, directory *bypassDirectory) string {
	if actor.ActorType != "Integration" {
		return ""
	}
	if !directory.installationsKnown {
		return bypassInstallationUnknown
	}
	for _, installation := range directory.installations {
		if installation.AppID != actor.ActorID {
			continue
		}
		if installation.SuspendedAt != nil {
			return "suspended"
		}
		if installation.RepositorySelection == "all" {
			return "all_repositories"
		}
		if installation.RepositorySelection == "selected" {
			return "selected_repositories"
		}
		return bypassInstallationUnknown
	}
	if actor.ActorID == directory.currentAppID {
		return bypassInstallationUnknown
	}
	return "not_installed"
}

// Four requests at a time keeps the initial roster responsive without sending
// one concurrent request per repository in a large workspace.
func collectBypassIdentities(ctx context.Context, sources []bypassIdentitySource) ([]github.BypassActorIdentity, bool) {
	type result struct {
		items []github.BypassActorIdentity
		err   error
	}
	jobs := make(chan bypassIdentitySource)
	results := make(chan result, len(sources))
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for job := range jobs {
				items, err := job(ctx)
				results <- result{items, err}
			}
		})
	}
	complete := true
dispatch:
	for _, source := range sources {
		select {
		case jobs <- source:
		case <-ctx.Done():
			complete = false
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	close(results)
	var items []github.BypassActorIdentity
	for result := range results {
		if result.err != nil {
			complete = false
			continue
		}
		items = append(items, result.items...)
	}
	return items, complete
}
