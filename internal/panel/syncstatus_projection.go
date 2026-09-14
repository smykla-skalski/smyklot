package panel

import (
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const syncStateOutdated = "outdated"

// observedTime represents absence without manufacturing a fresh timestamp.
func observedTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func syncStatusRow(repository storage.Repository, facts syncStatusFacts) syncRepositoryStatusDTO {
	row := syncRepositoryStatusDTO{Repository: repository.Name, Cells: make(map[string]syncCellDTO, len(orgsync.Kinds()))}
	formatting := storage.RepositoryFormattingPolicy(facts.formatting, facts.target.ConfigPatch, repository)
	for _, kind := range orgsync.Kinds() {
		enabled := facts.enabled[kind] && facts.target.Available && repository.Available && storage.RepositoryEnabled(facts.target, repository)
		override := facts.overrides[repository.ID][kind]
		if override != nil && override.Enabled != nil {
			enabled = enabled && *override.Enabled
		}
		digest := orgsync.DigestRepositoryConfiguration(kind, facts.configs[kind], override, formatting)
		cell := repositorySyncCell(repository.ID, kind, enabled, digest, facts)
		row.Cells[string(kind)] = cell
		if row.Reason == "" && cell.State == syncStateRefused {
			row.Reason = cell.Reason
		}
	}
	row.Removals = facts.removals[repository.ID]
	return row
}

func repositorySyncCell(repositoryID string, kind orgsync.Kind, enabled bool, digest string, facts syncStatusFacts) syncCellDTO {
	if !enabled {
		return syncCellDTO{State: "off"}
	}
	if reason := facts.unavailable[kind]; reason != "" {
		return syncCellDTO{State: syncStateRefused, Reason: reason}
	}
	if reason := facts.invalid[kind]; reason != "" {
		return syncCellDTO{State: syncStateRefused, Reason: reason}
	}
	state := facts.observations[repositoryID][kind]
	cell := syncCellDTO{State: "unknown"}
	if state.Observation != "" {
		cell.ObservedAt = observedTime(state.AppliedAt)
		cell.ObservedOutcome = state.Observation
		cell.ProposalURL = state.ProposalURL
	}
	if cell, active := pendingRepositorySyncCell(repositoryID, kind, digest, facts, cell); active {
		return cell
	}
	return observedRepositorySyncCell(state, digest, facts.now, cell)
}

func pendingRepositorySyncCell(repositoryID string, kind orgsync.Kind, digest string, facts syncStatusFacts, cell syncCellDTO) (syncCellDTO, bool) {
	pending, problem := facts.pending[repositoryID][kind], facts.problems[repositoryID][kind]
	if pending > 0 || problem != "" {
		if input := facts.actionInputs[repositoryID][kind]; input != "" && input != digest {
			cell.State, cell.Reason = syncStateOutdated, "Pending changes use earlier settings. A fresh check is needed."
			return cell, true
		}
		if problem != "" {
			cell.State, cell.Reason = "check_failed", problem
		} else {
			cell.State, cell.Changes = "pending", pending
		}
		return cell, true
	}
	return cell, false
}

func observedRepositorySyncCell(state orgsync.RepositoryState, digest string, now time.Time, cell syncCellDTO) syncCellDTO {
	if state.ObservedDigest == "" || state.Observation == "" {
		cell.Reason = "These settings have no confirmed repository check."
		if state.Problem != "" {
			cell.Reason = "The last attempt failed: " + state.Problem + ". Its original settings are unknown."
		}
		return cell
	}
	if state.ObservedDigest != digest {
		cell.State, cell.Reason = syncStateOutdated, "Settings changed since the last check."
		return cell
	}
	if state.AppliedAt.IsZero() || state.AppliedAt.After(now) || now.Sub(state.AppliedAt) >= orgsync.RecheckInterval {
		cell.State, cell.Reason = syncStateOutdated, "The last observation needs to be checked again."
		return cell
	}
	switch state.Observation {
	case orgsync.ObservationMatched:
		cell.State = "in_step"
	case orgsync.ObservationApplied:
		cell.State = "applied"
	case orgsync.ObservationProposed:
		cell.State, cell.Reason = "proposed", "A pull request was opened. Its changes still need to be merged."
	case orgsync.ObservationDeclined:
		cell.State, cell.Reason = "declined", "The pull request was closed without merging. This change will not be proposed again automatically."
	case orgsync.ObservationDifferent:
		cell.State, cell.Reason = "needs_sync", "The repository differs from the saved settings."
	case orgsync.ObservationFailed:
		cell.State, cell.Reason = "check_failed", state.Problem
	case orgsync.ObservationBlocked:
		cell.State, cell.Reason = syncStateRefused, state.Problem
	}
	return cell
}
