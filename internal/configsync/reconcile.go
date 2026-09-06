package configsync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Snapshot is one semantic document, not its TOML presentation. A missing file
// is distinct from a present file that contains no settings.
type Snapshot struct {
	Exists   bool   `json:"exists"`
	Document []byte `json:"document,omitempty"`
}

type Problem string

const (
	ProblemConflictingEdits Problem = "conflicting_edits"
	ProblemFileRemoved      Problem = "file_removed"
	ProblemStaleResolution  Problem = "stale_resolution"
)

// Resolution is valid only for the exact comparison that the user inspected.
// A new panel save or file commit must not silently reuse an earlier choice.
type Resolution struct {
	Comparison string `json:"comparison"`
	Document   []byte `json:"document"`
}

type ReconcileInput struct {
	Base       Snapshot
	Panel      []byte
	File       Snapshot
	Resolution *Resolution
}

// Decision contains proposed effects. Callers must atomically compare the panel
// revision before importing and the remote branch head before publishing. The
// baseline advances only when both saved documents already agree.
type Decision struct {
	Document    []byte
	ImportPanel bool
	PublishFile bool
	AdvanceBase bool
	Comparison  string
	Problem     Problem
	Conflicts   []Conflict
}

// Reconcile leaves overlapping edits untouched. On first connection, unrelated
// keys may combine, but no side wins a key configured differently on both sides.
// After connection, deleting the file requires an explicit user choice.
func Reconcile(input ReconcileInput) (Decision, error) {
	comparison, err := comparisonKey(input)
	if err != nil {
		return Decision{}, err
	}
	decision := Decision{Comparison: comparison}
	if input.Resolution != nil {
		if input.Resolution.Comparison != comparison {
			decision.Problem = ProblemStaleResolution
			return decision, nil
		}
		return effects(input, decision, input.Resolution.Document)
	}
	if !input.File.Exists {
		if input.Base.Exists {
			decision.Problem = ProblemFileRemoved
			return decision, nil
		}
		return effects(input, decision, input.Panel)
	}
	base := input.Base.Document
	if !input.Base.Exists {
		base = []byte("{}")
	}
	merged, err := Merge(base, input.Panel, input.File.Document)
	if err != nil {
		return Decision{}, err
	}
	if len(merged.Conflicts) > 0 {
		decision.Problem = ProblemConflictingEdits
		decision.Conflicts = merged.Conflicts
		// Do not expose a partially chosen document as a publishable result.
		return decision, nil
	}
	return effects(input, decision, merged.Document)
}

func effects(input ReconcileInput, decision Decision, document []byte) (Decision, error) {
	panelMatches, err := Equivalent(input.Panel, document)
	if err != nil {
		return Decision{}, err
	}
	fileMatches := false
	if input.File.Exists {
		fileMatches, err = Equivalent(input.File.Document, document)
		if err != nil {
			return Decision{}, err
		}
	}
	decision.Document = append([]byte(nil), document...)
	decision.ImportPanel = !panelMatches
	decision.PublishFile = !fileMatches
	decision.AdvanceBase = panelMatches && fileMatches
	return decision, nil
}

// Include the baseline and file presence in the comparison, and retain numeric
// precision. Canonical comparison avoids invalidating a choice for whitespace or
// key-order changes, while every actual setting change invalidates it.
func comparisonKey(input ReconcileInput) (string, error) {
	documents := []Snapshot{input.Base, {Exists: true, Document: input.Panel}, input.File}
	parts := make([]any, len(documents))
	for index, snapshot := range documents {
		if !snapshot.Exists {
			continue
		}
		value, err := decode(snapshot.Document)
		if err != nil {
			return "", fmt.Errorf("compare configuration documents: %w", err)
		}
		parts[index] = canonicalValue(value)
	}
	encoded, err := json.Marshal(parts)
	if err != nil {
		return "", fmt.Errorf("encode configuration comparison: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// Number tags keep 1 and "1" distinct, without relying on floating point.
func canonicalValue(value any) any {
	switch value := value.(type) {
	case json.Number:
		return []any{"number", canonicalNumber(value)}
	case map[string]any:
		object := make(map[string]any, len(value))
		for key, child := range value {
			object[key] = canonicalValue(child)
		}
		return []any{"object", object}
	case []any:
		list := make([]any, len(value))
		for index, child := range value {
			list[index] = canonicalValue(child)
		}
		return []any{"array", list}
	default:
		return value
	}
}
