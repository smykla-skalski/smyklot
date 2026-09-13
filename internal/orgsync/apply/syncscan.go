package apply

import (
	"context"
	"fmt"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

// syncScanResult preserves the evidence behind a no-action result. One check
// is one repository and sync kind, not one action or one whole repository.
type syncScanResult struct {
	actions      []orgsync.Action
	observations []orgsync.RepositoryState
	cached       int
	unpermitted  int
}

func inactiveSyncSummary(enabled []orgsync.Config) string {
	if len(enabled) == 0 {
		return "Sync is not enabled"
	}
	return "Sync could not run because required GitHub permissions are missing"
}

func (result syncScanResult) summary() string {
	counts := map[orgsync.Observation]int{}
	for _, observation := range result.observations {
		switch observation.Observation {
		case orgsync.ObservationFailed, orgsync.ObservationBlocked, orgsync.ObservationDifferent,
			orgsync.ObservationProposed, orgsync.ObservationDeclined, orgsync.ObservationMatched:
			counts[observation.Observation]++
		default:
			counts[""]++
		}
	}
	var parts []string
	for _, entry := range []struct {
		state       orgsync.Observation
		description string
	}{
		{orgsync.ObservationFailed, "failed"},
		{orgsync.ObservationBlocked, "could not proceed"},
		{orgsync.ObservationDifferent, "found differences"},
		{orgsync.ObservationProposed, "found an open proposal"},
		{orgsync.ObservationDeclined, "found a declined proposal"},
		{orgsync.ObservationMatched, "matched saved settings"},
		{"", "has no confirmed result"},
	} {
		if count := counts[entry.state]; count > 0 {
			noun := "checks"
			if count == 1 {
				noun = "check"
			}
			description := entry.description
			if count > 1 && entry.state == "" {
				description = "have no confirmed result"
			}
			parts = append(parts, fmt.Sprintf("%d %s %s", count, noun, description))
		}
	}
	if result.cached > 0 {
		parts = append(parts, fmt.Sprintf("Recent checks reused: %d", result.cached))
	}
	if result.unpermitted > 0 {
		parts = append(parts, fmt.Sprintf("Sync categories missing GitHub permissions: %d", result.unpermitted))
	}
	if len(parts) == 0 {
		return "No enabled repositories to check"
	}
	return strings.Join(parts, ". ") + ". See Sync status for details."
}

// planSyncActions collects outcomes even where no action can be queued.
func (s *Engine) planSyncActions(ctx context.Context, client *github.Client, active []orgsync.Config, scopes map[orgsync.Kind]syncScope, held syncInventory) (syncScanResult, error) {
	var result syncScanResult
	for _, saved := range active {
		scope := scopes[saved.Kind]
		ask, err := repositoryPlanner(client, saved, scope.overrides, scope.formatting, scope.targetPatch)
		if err != nil {
			logging.From(ctx).Warn("sync configuration cannot be planned", "kind", saved.Kind, "error", err)
			// Invalid saved input is a blocked observation, never silent agreement.
			ask = func(context.Context, storage.Repository) (repositoryAnswer, error) {
				return repositoryAnswer{problem: "Saved sync settings are invalid. Review this category's settings."}, nil
			}
		}
		for _, repository := range held.repositories {
			if !scope.watches(repository) {
				continue
			}
			if err == nil && !scope.covers(repository) {
				result.cached++
				continue
			}
			found, learned := scope.ask(ctx, ask, repository)
			result.actions = append(result.actions, found...)
			result.observations = append(result.observations, learned...)
		}
	}
	if err := s.store.RecordSyncRepositoryState(ctx, result.observations); err != nil {
		return syncScanResult{}, err
	}
	return result, nil
}
