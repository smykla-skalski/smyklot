package configsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func (engine Engine) apply(
	ctx context.Context, client *github.Client, snapshot PanelSnapshot, stored storage.ConfigFileState,
	connection Connection, file RemoteFile, input ReconcileInput, decision Decision,
) (Connection, bool, error) {
	// Import preparation also validates publication-only decisions against the
	// installation kind and supported storage ranges, before creating git objects.
	change, err := connection.change(snapshot, stored, time.Now().UTC())
	if err != nil {
		return connection, false, err
	}
	sourcePath := file.Path
	if sourcePath == "" {
		sourcePath = file.WritePath
	}
	request, err := snapshot.PrepareImport(decision.Document, storage.ConfigFileImport{
		State: change, Path: sourcePath, HeadSHA: file.Head,
	}, engine.QuietPeriod)
	if err != nil {
		return engine.block(ctx, snapshot, stored, connection,
			&BlockedError{Code: "invalid_settings", Message: err.Error()})
	}
	if decision.ImportPanel {
		return engine.importPanel(ctx, snapshot, stored, connection, input, decision, request)
	}
	if decision.AdvanceBase && !file.Migrate {
		connection.Base = Snapshot{Exists: true, Document: decision.Document}
		connection.Status, connection.Resolution = StatusReady, nil
		return engine.save(ctx, snapshot, stored, connection)
	}
	return engine.publish(ctx, client, snapshot, stored, connection, file, decision.Document)
}

func (engine Engine) importPanel(
	ctx context.Context, snapshot PanelSnapshot, stored storage.ConfigFileState, connection Connection,
	input ReconcileInput, decision Decision, request storage.SaveInstallationSettingsRequest,
) (Connection, bool, error) {
	connection.Status = StatusPending
	if choice := connection.Resolution; choice != nil {
		// Our atomic import changes the comparison. Preserve this choice only for
		// that exact resulting panel document. Any later save or remote edit still
		// invalidates it on the fresh read performed by Run.
		input.Panel = decision.Document
		comparison, err := comparisonKey(input)
		if err != nil {
			return connection, false, err
		}
		connection.Resolution = &ResolutionChoice{Side: choice.Side, Comparison: comparison}
	}
	change, err := connection.change(snapshot, stored, request.ChangedAt)
	if err != nil {
		return connection, false, err
	}
	request.ConfigFileImport.State = change
	_, err = engine.Store.SaveInstallationSettings(ctx, request)
	if errors.Is(err, orgsync.ErrInvalidConfig) {
		return engine.block(ctx, snapshot, stored, connection, &BlockedError{Code: "invalid_settings", Message: err.Error()})
	}
	return connection, err == nil, err
}

func (engine Engine) publish(
	ctx context.Context, client *github.Client, snapshot PanelSnapshot, stored storage.ConfigFileState,
	connection Connection, file RemoteFile, semantic []byte,
) (Connection, bool, error) {
	if !snapshot.Target.Grants("contents") || !snapshot.Target.Grants("pull_requests") {
		return engine.block(ctx, snapshot, stored, connection, &BlockedError{
			Code: "write_permission_missing", Message: "Give Smyklot Contents and Pull requests write access to publish settings",
		})
	}
	content, err := renderPublication(file, semantic)
	if err != nil {
		return engine.block(ctx, snapshot, stored, connection, err)
	}
	digest := sha256.Sum256(content)
	proposal := connection.Proposal
	if proposal == nil || proposal.DefaultHead != file.Head || proposal.Path != file.WritePath ||
		proposal.Digest != hex.EncodeToString(digest[:]) {
		prepared, err := PrepareProposal(ctx, client, file, semantic, proposal)
		if err != nil {
			return engine.block(ctx, snapshot, stored, connection, err)
		}
		proposal = &prepared
	}
	connection.Proposal, connection.Status = proposal, StatusPending
	connection.Path = proposal.Path
	change, err := connection.change(snapshot, stored, time.Now().UTC())
	if err != nil {
		return connection, false, err
	}
	// This CAS is the publication boundary: no branch or PR can change unless
	// every setting revision and this exact intent are durably confirmed first.
	stored, err = engine.Store.SaveConfigFileState(ctx, change)
	if err != nil {
		return connection, false, err
	}
	published, err := PublishProposal(ctx, client, file.Location, *proposal)
	if err != nil {
		var blocked *BlockedError
		if errors.As(err, &blocked) && blocked.Code == "source_changed" {
			// A moving default branch is ordinary concurrent work. Start over from
			// its new immutable head instead of requesting a user decision.
			return connection, true, nil
		}
		return engine.block(ctx, snapshot, stored, connection, err)
	}
	connection.Proposal, connection.Status = &published, StatusProposed
	return engine.save(ctx, snapshot, stored, connection)
}
