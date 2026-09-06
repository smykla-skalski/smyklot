package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

type bypassDirectory struct {
	mu                 sync.Mutex
	items              []github.BypassActorIdentity
	complete           bool
	expires            time.Time
	installations      []github.AppInstallationAccess
	installationsKnown bool
	currentAppID       int64
}

// LookupBypassActors resolves display metadata with the selected installation's
// credentials. Cached metadata never participates in ruleset authorization.
func (s *server) LookupBypassActors(ctx context.Context, targetID, kind, query string) ([]github.BypassActorIdentity, bool, error) {
	target, err := s.store.GetTarget(ctx, targetID)
	if err != nil {
		return nil, false, err
	}
	if !target.Available {
		return nil, false, fmt.Errorf("installation is unavailable")
	}
	id, err := strconv.ParseInt(target.InstallationID, 10, 64)
	if err != nil || id <= 0 {
		return nil, false, fmt.Errorf("invalid installation id")
	}
	token, err := s.tokens.InstallationToken(id)
	if err != nil {
		return nil, false, err
	}
	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return nil, false, err
	}
	value, _ := s.bypassDirectories.LoadOrStore(targetID, &bypassDirectory{})
	directory, ok := value.(*bypassDirectory)
	if !ok {
		return nil, false, fmt.Errorf("invalid bypass directory cache")
	}
	directory.mu.Lock()
	defer directory.mu.Unlock()
	if !time.Now().Before(directory.expires) {
		if err := s.refreshBypassDirectory(ctx, client, target, directory); err != nil {
			return nil, false, err
		}
	}
	if query == "" {
		return slices.Clone(directory.items), directory.complete, nil
	}
	actors, err := lookupBypassSuggestions(ctx, client, target.Account.Login, directory, kind, query)
	return actors, directory.complete, err
}

func lookupBypassSuggestions(ctx context.Context, client *github.Client, owner string, directory *bypassDirectory, kind, query string) ([]github.BypassActorIdentity, error) {
	matches := matchingBypassActors(directory.items, kind, query)
	if !bypassSlug.MatchString(query) || slices.ContainsFunc(directory.items, func(actor github.BypassActorIdentity) bool {
		return actor.ActorType == kind && strings.EqualFold(actor.Slug, query)
	}) {
		return matches, nil
	}
	actor, err := client.ResolveBypassActor(ctx, owner, kind, query)
	if err != nil {
		var apiErr *github.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound || errors.Is(err, github.ErrBypassActorNotFound) {
			return matches, nil
		}
		return nil, err
	}
	actor.InstallationStatus = bypassInstallationStatus(actor, directory)
	directory.items = append(directory.items, actor)
	// Exact identities stay visible even when eight partial matches already exist.
	return append([]github.BypassActorIdentity{actor}, matches[:min(7, len(matches))]...), nil
}

var bypassSlug = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,99}$`)

func matchingBypassActors(items []github.BypassActorIdentity, kind, query string) []github.BypassActorIdentity {
	query = strings.ToLower(query)
	matches := []github.BypassActorIdentity{}
	for _, item := range items {
		if item.ActorType == kind && (strings.Contains(strings.ToLower(item.Name), query) || strings.Contains(strings.ToLower(item.Slug), query)) {
			matches = append(matches, item)
		}
	}
	slices.SortFunc(matches, func(a, b github.BypassActorIdentity) int {
		aExact, bExact := strings.EqualFold(a.Slug, query), strings.EqualFold(b.Slug, query)
		if aExact != bExact {
			if aExact {
				return -1
			}
			return 1
		}
		aPrefix := strings.HasPrefix(strings.ToLower(a.Name), query) || strings.HasPrefix(strings.ToLower(a.Slug), query)
		bPrefix := strings.HasPrefix(strings.ToLower(b.Name), query) || strings.HasPrefix(strings.ToLower(b.Slug), query)
		if aPrefix != bPrefix {
			if aPrefix {
				return -1
			}
			return 1
		}
		if order := cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); order != 0 {
			return order
		}
		return cmp.Compare(a.ActorID, b.ActorID)
	})
	return matches[:min(8, len(matches))]
}

func (s *server) currentBypassApp(ctx context.Context, client *github.Client) (github.BypassActorIdentity, error) {
	token, err := s.tokens.AppToken()
	if err != nil {
		return github.BypassActorIdentity{}, err
	}
	appClient, err := github.NewAppClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return github.BypassActorIdentity{}, err
	}
	slug, err := appClient.AppSlug(ctx)
	if err != nil {
		return github.BypassActorIdentity{}, err
	}
	return client.ResolveBypassActor(ctx, "", "Integration", slug)
}
