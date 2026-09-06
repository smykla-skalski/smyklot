package configsync

import (
	"context"
	"maps"
	"time"
)

// ConnectionStatus is a read-only, cached observation, never a claim about the
// current GitHub tree. CheckedAt must accompany its result in the panel. A new
// settings revision makes that result pending until another check covers it.
// It deliberately excludes baseline documents, comparison tokens and private
// publication intent; conflict decisions require a separate fresh preview.
type ConnectionStatus struct {
	Enabled   bool             `json:"enabled"`
	Available bool             `json:"available"`
	Status    Status           `json:"status"`
	LastCheck *ConnectionCheck `json:"last_check,omitempty"`
}

type ConnectionCheck struct {
	CheckedAt       time.Time        `json:"checked_at"`
	Status          Status           `json:"status"`
	SettingsCurrent bool             `json:"settings_current"`
	Head            string           `json:"head,omitempty"`
	Path            string           `json:"path,omitempty"`
	Problem         string           `json:"problem,omitempty"`
	Message         string           `json:"message,omitempty"`
	ConflictCount   int              `json:"conflict_count,omitempty"`
	ConflictPaths   [][]string       `json:"conflict_paths,omitempty"`
	Proposal        *CheckedProposal `json:"proposal,omitempty"`
}

type CheckedProposal struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

// ReadStatus performs no remote reads, writes or queue scheduling. Every poll
// compares the observation with all current settings revisions, including Sync
// kinds that did not exist at the last check.
func (engine Engine) ReadStatus(ctx context.Context, targetID, repositoryID string) (ConnectionStatus, error) {
	snapshot, err := engine.Snapshot(ctx, targetID, repositoryID)
	if err != nil {
		return ConnectionStatus{}, err
	}
	answer := ConnectionStatus{Enabled: snapshot.Target.ConfigFileSyncEnabled, Available: snapshot.Target.Available, Status: StatusOff}
	if repository := snapshot.Repository; repository != nil {
		answer.Enabled = repository.ConfigFileSyncEnabled
		answer.Available = answer.Available && repository.Available
	}
	if !answer.Enabled || !answer.Available {
		return answer, nil
	}
	answer.Status = StatusPending
	stored, err := engine.Store.GetConfigFileState(ctx, targetID, repositoryID)
	if err != nil || len(stored.Document) == 0 {
		return answer, err
	}
	connection, err := DecodeConnection(stored, snapshot.Scope())
	if err != nil {
		return answer, err
	}
	current := connection.Inputs != nil && connection.Inputs.Owner == snapshot.OwnerRevision() &&
		maps.Equal(connection.Inputs.Sync, snapshot.SyncRevisions())
	if stored.InitializationRequired && connection.Status == StatusReady {
		current = false
	}
	check := &ConnectionCheck{
		CheckedAt: stored.UpdatedAt, Status: connection.Status, SettingsCurrent: current,
		Head: connection.Head, Path: connection.Path, Problem: connection.Problem, Message: connection.Message,
		ConflictCount: connection.ConflictCount, ConflictPaths: connection.ConflictPaths,
	}
	if proposal := connection.Proposal; proposal != nil && proposal.Number > 0 && connection.Status != StatusReady && connection.Status != StatusOff {
		check.Proposal = &CheckedProposal{Number: proposal.Number, URL: proposal.URL}
	}
	answer.LastCheck = check
	if current {
		answer.Status = connection.Status
	}
	return answer, nil
}
