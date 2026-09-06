package configsync

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

type ResolutionChoice struct {
	Comparison string `json:"comparison"`
	Side       string `json:"side"`
}

type Status string

const (
	StatusOff      Status = "off"
	StatusPending  Status = "pending"
	StatusReady    Status = "ready"
	StatusProposed Status = "proposed"
	StatusBlocked  Status = "blocked"
)

// Connection retains one baseline, never duplicate full documents for previews
// or resolutions. Conflict previews are rebuilt from the current observation;
// a choice is bound to the exact comparison that the user reviewed.
type Connection struct {
	Version       int               `json:"version"`
	Base          Snapshot          `json:"base"`
	Status        Status            `json:"status"`
	Head          string            `json:"head,omitempty"`
	Path          string            `json:"path,omitempty"`
	Problem       string            `json:"problem,omitempty"`
	Message       string            `json:"message,omitempty"`
	Comparison    string            `json:"comparison,omitempty"`
	ConflictCount int               `json:"conflict_count,omitempty"`
	ConflictPaths [][]string        `json:"conflict_paths,omitempty"`
	Resolution    *ResolutionChoice `json:"resolution,omitempty"`
	Proposal      *Proposal         `json:"proposal,omitempty"`
}

func DecodeConnection(stored storage.ConfigFileState, scope config.PanelFileScope) (Connection, error) {
	if len(stored.Document) == 0 {
		return Connection{Version: 1, Status: StatusPending}, nil
	}
	var connection Connection
	if err := config.DecodeExactJSONWithin(stored.Document, &connection, storage.MaxConfigFileStateBytes); err != nil {
		return Connection{}, err
	}
	if connection.Version != 1 {
		return Connection{}, errors.New("unsupported configuration connection version")
	}
	if connection.Base.Exists {
		if _, err := DecodeDocument(connection.Base.Document, scope); err != nil {
			return Connection{}, err
		}
	}
	if connection.Resolution != nil && connection.Resolution.Side != "panel" && connection.Resolution.Side != "file" {
		return Connection{}, errors.New("unknown configuration conflict resolution")
	}
	return connection, nil
}

func (connection Connection) change(snapshot PanelSnapshot, stored storage.ConfigFileState, now time.Time) (storage.ConfigFileStateChange, error) {
	document, err := json.Marshal(connection)
	if err != nil {
		return storage.ConfigFileStateChange{}, err
	}
	change := storage.ConfigFileStateChange{
		TargetID: snapshot.Target.ID, RepositoryID: snapshot.RepositoryID(),
		OwnerRevision: snapshot.OwnerRevision(), SyncRevisions: snapshot.SyncRevisions(),
		ExpectedRevision: stored.Revision, Document: document, ChangedAt: now,
	}
	return change, change.Validate()
}

func (connection Connection) input(panel []byte, file FileSource) (ReconcileInput, error) {
	input := ReconcileInput{Base: connection.Base, Panel: panel, File: file.Snapshot}
	if choice := connection.Resolution; choice != nil {
		if file.Snapshot.Exists {
			same, err := Equivalent(panel, file.Snapshot.Document)
			if err != nil || same {
				// Agreement needs no choice. In particular, merging our proposal
				// changes the remote comparison without reopening its conflict.
				return input, err
			}
		}
		document := panel
		if choice.Side == "file" {
			if !file.Snapshot.Exists {
				current, err := comparisonKey(input)
				if err != nil {
					return input, err
				}
				if choice.Comparison == current {
					return input, &BlockedError{Code: "file_removed", Message: "The file was deleted and cannot replace panel settings"}
				}
				// Keep the old binding so Reconcile surfaces stale_resolution
				// before attempting to use this now-absent document.
			}
			document = file.Snapshot.Document
		}
		input.Resolution = &Resolution{Comparison: choice.Comparison, Document: document}
	}
	return input, nil
}

// A repeated long parent key must not turn a bounded file into an oversized
// persisted summary. The count remains complete; previews read the source again.
func conflictSummary(conflicts []Conflict) [][]string {
	var paths [][]string
	pathBytes := 0
	for _, conflict := range conflicts[:min(100, len(conflicts))] {
		for _, part := range conflict.Path {
			pathBytes += len(part)
		}
		if pathBytes > 16<<10 {
			break
		}
		paths = append(paths, conflict.Path)
	}
	return paths
}
