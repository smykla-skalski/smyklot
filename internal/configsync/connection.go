package configsync

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

const (
	ResolutionPanel = "panel"
	ResolutionFile  = "file"
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
	Inputs        *InputRevisions   `json:"inputs,omitempty"`
}

// InputRevisions identifies the saved settings read by the last check. It is
// optional for existing connections, which need a new check before claiming
// that their observation covers the current settings.
type InputRevisions struct {
	Owner int64                  `json:"owner"`
	Sync  map[orgsync.Kind]int64 `json:"sync"`
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
	if connection.Resolution != nil && connection.Resolution.Side != ResolutionPanel && connection.Resolution.Side != ResolutionFile {
		return Connection{}, errors.New("unknown configuration conflict resolution")
	}
	return connection, nil
}

func (connection Connection) change(snapshot PanelSnapshot, stored storage.ConfigFileState, now time.Time) (storage.ConfigFileStateChange, error) {
	connection.Inputs = &InputRevisions{Owner: snapshot.OwnerRevision(), Sync: snapshot.SyncRevisions()}
	document, err := json.Marshal(connection)
	if err != nil {
		return storage.ConfigFileStateChange{}, err
	}
	change := storage.ConfigFileStateChange{
		TargetID: snapshot.Target.ID, RepositoryID: snapshot.RepositoryID(),
		OwnerRevision: snapshot.OwnerRevision(), SyncRevisions: snapshot.SyncRevisions(),
		ExpectedRevision: stored.Revision, Document: document, ChangedAt: now,
		Initialized: connection.Status == StatusReady && connection.Base.Exists,
	}
	return change, change.Validate()
}

func (connection Connection) input(panel []byte, file FileSource) (ReconcileInput, error) {
	input := ReconcileInput{Base: connection.Base, Panel: panel, File: file.Snapshot}
	choice := connection.Resolution
	if choice == nil {
		return input, nil
	}
	if file.Snapshot.Exists {
		same, err := Equivalent(panel, file.Snapshot.Document)
		if err != nil || same {
			// Agreement needs no choice. Merging our proposal changes the
			// comparison without reopening its conflict.
			return input, err
		}
		document, err := resolvedContent(input, choice.Side)
		if err != nil {
			return input, err
		}
		input.Resolution = &Resolution{Comparison: choice.Comparison, Document: document}
		return input, nil
	}
	document := panel
	if choice.Side == ResolutionFile {
		current, err := comparisonKey(input)
		if err != nil {
			return input, err
		}
		if choice.Comparison == current {
			return input, &BlockedError{Code: "file_removed", Message: "The file was deleted and cannot replace panel settings"}
		}
		// Preserve the old binding so Reconcile reports stale_resolution
		// before trying to use the now-absent file.
		document = nil
	}
	input.Resolution = &Resolution{Comparison: choice.Comparison, Document: document}
	return input, nil
}

// A choice decides only overlapping changes. Independent changes from both
// sides survive, so accepting one conflict never silently discards another edit.
func resolvedContent(input ReconcileInput, side string) ([]byte, error) {
	base := input.Base.Document
	if !input.Base.Exists {
		base = []byte("{}")
	}
	preferred, other := input.Panel, input.File.Document
	if side == ResolutionFile {
		preferred, other = other, preferred
	}
	merged, err := Merge(base, preferred, other)
	if err != nil {
		return nil, invalidSettings(err)
	}
	return merged.Document, nil
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
