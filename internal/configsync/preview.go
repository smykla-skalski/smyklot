package configsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// ConnectionPreview is a fresh, read-only review. Each choice is the complete
// result of resolving overlapping edits, with independent edits preserved. The
// private baseline and publication intent are never part of this response.
type ConnectionPreview struct {
	Status        Status          `json:"status"`
	CheckedAt     time.Time       `json:"checked_at"`
	Path          string          `json:"path,omitempty"`
	Head          string          `json:"head,omitempty"`
	Problem       string          `json:"problem,omitempty"`
	Message       string          `json:"message,omitempty"`
	ConflictCount int             `json:"conflict_count,omitempty"`
	ConflictPaths [][]string      `json:"conflict_paths,omitempty"`
	ReviewToken   string          `json:"review_token,omitempty"`
	Choices       []PreviewChoice `json:"choices,omitempty"`
}

type PreviewChoice struct {
	Side        string          `json:"side"`
	Available   bool            `json:"available"`
	Document    json.RawMessage `json:"document,omitempty"`
	ImportPanel bool            `json:"import_panel"`
	PublishFile bool            `json:"publish_file"`
	Message     string          `json:"message,omitempty"`
}

type reviewObservation struct {
	snapshot   PanelSnapshot
	stored     storage.ConfigFileState
	connection Connection
	file       RemoteFile
	input      ReconcileInput
	decision   Decision
}

// Preview must share the catalog/repository exclusion used by Run and panel
// saves. The second snapshot also rejects concurrent installation discovery or
// out-of-process changes while the immutable GitHub observation is loading.
func (engine Engine) Preview(ctx context.Context, client *github.Client, targetID, repositoryID string) (ConnectionPreview, error) {
	observation, err := engine.observeReview(ctx, client, targetID, repositoryID)
	if err != nil {
		var blocked *BlockedError
		if errors.As(err, &blocked) {
			status := StatusBlocked
			if blocked.Code == "sync_off" {
				status = StatusOff
			}
			return ConnectionPreview{
				Status: status, CheckedAt: time.Now().UTC(), Problem: blocked.Code, Message: blocked.Message,
				Path: observation.file.Path, Head: observation.file.Head,
			}, nil
		}
		return ConnectionPreview{}, err
	}
	return engine.review(observation)
}

// PrepareResolution only prepares a CAS-protected state change. The caller must
// persist it with actor attribution, elevation checks and a durable notification
// in one transaction. No settings, queue item or GitHub object is changed here.
// A client supplies only a reviewed token and side, never a replacement document.
func (engine Engine) PrepareResolution(
	ctx context.Context, client *github.Client, targetID, repositoryID, token, side string,
) (storage.ConfigFileStateChange, error) {
	if token == "" || (side != ResolutionPanel && side != ResolutionFile) {
		return storage.ConfigFileStateChange{}, storage.ErrConflict
	}
	observation, err := engine.observeReview(ctx, client, targetID, repositoryID)
	if err != nil {
		return storage.ConfigFileStateChange{}, err
	}
	preview, err := engine.review(observation)
	if err != nil {
		return storage.ConfigFileStateChange{}, err
	}
	if preview.ReviewToken == "" || token != preview.ReviewToken {
		return storage.ConfigFileStateChange{}, storage.ErrConflict
	}
	for _, choice := range preview.Choices {
		if choice.Side != side || !choice.Available {
			continue
		}
		connection := observation.connection
		connection.Resolution = &ResolutionChoice{Side: side, Comparison: observation.decision.Comparison}
		connection.Status, connection.Problem, connection.Message = StatusPending, "", ""
		connection.Head, connection.Path = observation.file.Head, preview.Path
		connection.ConflictCount, connection.ConflictPaths = 0, nil
		return connection.change(observation.snapshot, observation.stored, time.Now().UTC())
	}
	return storage.ConfigFileStateChange{}, &BlockedError{Code: "resolution_unavailable", Message: "This choice cannot produce valid settings"}
}

func (engine Engine) observeReview(ctx context.Context, client *github.Client, targetID, repositoryID string) (reviewObservation, error) {
	var observation reviewObservation
	snapshot, err := engine.Snapshot(ctx, targetID, repositoryID)
	if err != nil {
		return observation, err
	}
	if !connectionEnabled(snapshot) {
		return observation, &BlockedError{Code: "sync_off", Message: "Enable configuration file sync to review changes"}
	}
	if snapshot.Repository != nil && snapshot.Repository.IgnoreRepositoryFile {
		return observation, &BlockedError{Code: "file_disabled", Message: "Turn on file settings before syncing changes in both directions"}
	}
	stored, err := engine.Store.GetConfigFileState(ctx, targetID, repositoryID)
	if err != nil {
		return observation, err
	}
	connection, err := DecodeConnection(stored, snapshot.Scope())
	if err != nil {
		return observation, err
	}
	location, err := engine.location(ctx, client, snapshot)
	if err != nil {
		return observation, err
	}
	file, err := ReadRemoteFile(ctx, client, location)
	observation = reviewObservation{snapshot: snapshot, stored: stored, connection: connection, file: file}
	if err != nil {
		return observation, err
	}
	if err := engine.verifyReview(ctx, client, observation); err != nil {
		return observation, err
	}
	panel, err := snapshot.JSON()
	if err != nil {
		return observation, err
	}
	input, err := connection.input(panel, file.Source)
	if err != nil {
		return observation, err
	}
	decision, err := Reconcile(input)
	if err != nil {
		return observation, invalidSettings(err)
	}
	if decision.Problem == ProblemStaleResolution {
		// An old choice must not prevent a new review of today's actual conflict.
		input.Resolution = nil
		decision, err = Reconcile(input)
		if err == nil && !input.File.Exists {
			// A choice proves that a file existed even before the first shared
			// baseline. Its later deletion must still be an explicit decision.
			decision.Problem = ProblemFileRemoved
		}
	}
	observation.input, observation.decision = input, decision
	return observation, err
}

func (engine Engine) verifyReview(ctx context.Context, client *github.Client, observation reviewObservation) error {
	before := observation.snapshot
	after, err := engine.Snapshot(ctx, before.Target.ID, before.RepositoryID())
	if err != nil {
		return err
	}
	if !connectionEnabled(after) || before.Target.Revision != after.Target.Revision ||
		before.Target.InstallationID != after.Target.InstallationID || before.Target.Kind != after.Target.Kind ||
		before.OwnerRevision() != after.OwnerRevision() ||
		!maps.Equal(before.SyncRevisions(), after.SyncRevisions()) {
		return storage.ErrConflict
	}
	location, err := engine.location(ctx, client, after)
	if err != nil {
		return err
	}
	if location != observation.file.Location {
		return storage.ErrConflict
	}
	stored, err := engine.Store.GetConfigFileState(ctx, before.Target.ID, before.RepositoryID())
	if err != nil {
		return err
	}
	if stored.Revision != observation.stored.Revision {
		return storage.ErrConflict
	}
	return nil
}

func (engine Engine) review(observation reviewObservation) (ConnectionPreview, error) {
	decision, file := observation.decision, observation.file
	preview := ConnectionPreview{Status: StatusPending, CheckedAt: time.Now().UTC(), Path: file.Path, Head: file.Head}
	if preview.Path == "" {
		preview.Path = file.WritePath
	}
	if decision.Problem == "" {
		if err := engine.validateReviewContent(observation, decision.Document); err != nil {
			preview.Status, preview.Problem, preview.Message = StatusBlocked, "invalid_settings", err.Error()
			return preview, nil
		}
		if decision.AdvanceBase && !file.Migrate {
			preview.Status = StatusReady
		}
		return preview, nil
	}
	preview.Status, preview.Problem = StatusBlocked, string(decision.Problem)
	preview.ConflictCount, preview.ConflictPaths = len(decision.Conflicts), conflictSummary(decision.Conflicts)
	token, err := reviewToken(observation)
	if err != nil {
		return preview, err
	}
	preview.ReviewToken = token
	sides := []string{ResolutionPanel}
	if file.Source.Snapshot.Exists {
		sides = append(sides, ResolutionFile)
	}
	for _, side := range sides {
		preview.Choices = append(preview.Choices, engine.previewChoice(observation, side))
	}
	return preview, nil
}

func (engine Engine) previewChoice(observation reviewObservation, side string) PreviewChoice {
	choice := PreviewChoice{Side: side}
	content := observation.input.Panel
	var err error
	if observation.file.Source.Snapshot.Exists {
		content, err = resolvedContent(observation.input, side)
	}
	if err == nil {
		err = engine.validateReviewContent(observation, content)
	}
	if err != nil {
		choice.Message = err.Error()
		return choice
	}
	decision, err := effects(observation.input, Decision{}, content)
	if err != nil {
		choice.Message = err.Error()
		return choice
	}
	choice.Available, choice.Document = true, json.RawMessage(content)
	choice.ImportPanel, choice.PublishFile = decision.ImportPanel, decision.PublishFile || observation.file.Migrate
	return choice
}

func (engine Engine) validateReviewContent(observation reviewObservation, content []byte) error {
	change, err := observation.connection.change(observation.snapshot, observation.stored, time.Now().UTC())
	if err != nil {
		return err
	}
	_, err = observation.snapshot.PrepareImport(content, storage.ConfigFileImport{
		State: change, Path: observation.file.WritePath, HeadSHA: observation.file.Head,
	}, engine.QuietPeriod)
	return err
}

func reviewToken(observation reviewObservation) (string, error) {
	// Polling may update observation metadata without changing a decision. Bind
	// the review to settings revisions, location and immutable source instead;
	// PrepareResolution still CAS-checks the freshly read connection revision.
	snapshot := observation.snapshot
	encoded, err := json.Marshal([]any{
		snapshot.Target.ID, snapshot.Target.InstallationID, snapshot.Target.Kind, snapshot.RepositoryID(),
		snapshot.Target.Revision, snapshot.OwnerRevision(), snapshot.SyncRevisions(),
		observation.file.Location, observation.file.Head, observation.file.Path, observation.file.WritePath,
		observation.decision.Comparison, observation.connection.Resolution,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
