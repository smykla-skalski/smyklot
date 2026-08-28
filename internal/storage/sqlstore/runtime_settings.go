package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

const (
	actionRuntimeSettingsSaved    = "runtime.settings.saved"
	actionRuntimeSettingsRestored = "runtime.settings.restored"
)

// GetRuntimeSettings returns the singleton runtime override record. A fresh
// deployment has revision zero and no overrides.
func (s *Store) GetRuntimeSettings(ctx context.Context) (storage.RuntimeSettings, error) {
	settings, err := getRuntimeSettings(ctx, s.db)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.RuntimeSettings{}, nil
	}
	if err != nil {
		return storage.RuntimeSettings{}, fmt.Errorf("get runtime settings: %w", err)
	}

	return settings, nil
}

// SaveRuntimeSettings atomically replaces persisted overrides, records one
// immutable Root checkpoint, and appends its linked Root audit event.
func (s *Store) SaveRuntimeSettings(
	ctx context.Context,
	change storage.RuntimeSettingsChange,
) (storage.SaveRuntimeSettingsResult, error) {
	botConfig, err := validateRuntimeSettingsChange(change)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, fmt.Errorf("begin runtime settings save: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockPendingCIPolicy(ctx, tx, s.dialect); err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	result, err := s.saveRuntimeSettings(ctx, tx, change, botConfig, nil, "")
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.SaveRuntimeSettingsResult{}, fmt.Errorf("commit runtime settings save: %w", err)
	}

	return result, nil
}

// RestoreRuntimeSettings validates the selected Root checkpoint again inside
// the write transaction and applies its runtime document as a new revision.
func (s *Store) RestoreRuntimeSettings(
	ctx context.Context,
	request storage.RestoreRuntimeSettingsRequest,
) (storage.SaveRuntimeSettingsResult, error) {
	if err := request.Validate(); err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{},
			fmt.Errorf("begin runtime settings restore: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockPendingCIPolicy(ctx, tx, s.dialect); err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	inspection, err := inspectRootSettingsCheckpoint(ctx, tx, storage.SettingsCheckpointRef{
		ID: request.CheckpointID, Scope: storage.SettingsCheckpointScopeRoot,
	})
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	item, selected, err := rootRuntimeRestoreItem(inspection, request.Side)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	currentRevision := int64(0)
	if item.Current != nil {
		currentRevision = item.Current.Revision
	}
	if currentRevision != request.ExpectedRevision {
		return storage.SaveRuntimeSettingsResult{}, storage.ErrConflict
	}
	document, err := decodeSettingsDocument[storage.RuntimeSettingsDocument](selected.State.Document)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, fmt.Errorf(
			"%w: runtime checkpoint document is invalid",
			storage.ErrSettingsRestoreBlocked,
		)
	}
	if document.BotConfig != nil {
		botConfig := *document.BotConfig
		botConfig.Runner = request.Runner
		document.BotConfig = &botConfig
	}
	change := storage.RuntimeSettingsChange{
		BackgroundWorkPaused: document.BackgroundWorkPaused,
		BotConfig:            document.BotConfig, LogLevel: document.LogLevel,
		PollInterval:         document.PollInterval,
		PendingCIQuietPeriod: document.PendingCIQuietPeriod,
		SessionTTL:           document.SessionTTL, PathIndexInterval: document.PathIndexInterval,
		ExpectedRevision: request.ExpectedRevision,
		ActorAccountID:   request.ActorAccountID, ChangedAt: request.ChangedAt,
		EffectivePendingCIQuietPeriod: request.EffectivePendingCIQuietPeriod,
		EffectivePollInterval:         request.EffectivePollInterval,
		EffectivePathIndexInterval:    request.EffectivePathIndexInterval,
		EffectiveSessionTTL:           request.EffectiveSessionTTL,
	}
	botConfig, err := validateRuntimeSettingsChange(change)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, fmt.Errorf(
			"%w: runtime checkpoint is incompatible",
			storage.ErrSettingsRestoreBlocked,
		)
	}
	result, err := s.saveRuntimeSettings(
		ctx,
		tx,
		change,
		botConfig,
		&request.CheckpointID,
		request.Side,
	)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	if result.CheckpointID == nil {
		return storage.SaveRuntimeSettingsResult{}, storage.ErrSettingsRestoreNoop
	}
	if err := tx.Commit(); err != nil {
		return storage.SaveRuntimeSettingsResult{},
			fmt.Errorf("commit runtime settings restore: %w", err)
	}

	return result, nil
}

func (s *Store) saveRuntimeSettings(
	ctx context.Context,
	tx *transaction,
	change storage.RuntimeSettingsChange,
	botConfig *string,
	restoredFromID *int64,
	restoredSide storage.SettingsCheckpointRestoreSide,
) (storage.SaveRuntimeSettingsResult, error) {
	current, err := getRuntimeSettings(ctx, tx)
	if errors.Is(err, sql.ErrNoRows) {
		current = storage.RuntimeSettings{}
	} else if err != nil {
		return storage.SaveRuntimeSettingsResult{},
			fmt.Errorf("read runtime settings for save: %w", err)
	}
	if current.Revision != change.ExpectedRevision {
		return storage.SaveRuntimeSettingsResult{}, storage.ErrConflict
	}
	proposed := storage.RuntimeSettings{
		BackgroundWorkPaused: change.BackgroundWorkPaused,
		BotConfig:            change.BotConfig, LogLevel: change.LogLevel,
		PollInterval:         change.PollInterval,
		PendingCIQuietPeriod: change.PendingCIQuietPeriod,
		SessionTTL:           change.SessionTTL, PathIndexInterval: change.PathIndexInterval,
	}
	before, err := runtimeSettingsState(runtimeSettingsDocument(current), current.Revision)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	proposedState, err := runtimeSettingsState(runtimeSettingsDocument(proposed), current.Revision)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	if sameSettingsCheckpointState(before, proposedState) {
		return storage.SaveRuntimeSettingsResult{Settings: current}, nil
	}
	if err := ensureRuntimeSettingsBaseline(ctx, tx, change, before); err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}

	if err := s.writeRuntimeSettings(ctx, tx, current, change, botConfig); err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	if err := invalidatePlansForRuntimeFormatting(ctx, tx, current, change); err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	if err := s.syncRuntimeQueueAliases(ctx, tx, current, change); err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	if err := shortenSessions(ctx, tx, change.EffectiveSessionTTL); err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	if !sameOptionalDuration(current.PendingCIQuietPeriod, change.PendingCIQuietPeriod) {
		request := pendingci.RetuneQuietPeriodRequest{
			PassingQuiet:  change.EffectivePendingCIQuietPeriod,
			ChangedAt:     change.ChangedAt,
			InheritedOnly: true,
		}
		if err := request.Validate(); err != nil {
			return storage.SaveRuntimeSettingsResult{}, err
		}
		if _, err := retuneQuietPeriod(ctx, tx, s.dialect, request); err != nil {
			return storage.SaveRuntimeSettingsResult{}, fmt.Errorf(
				"retune runtime pending CI quiet period: %w",
				err,
			)
		}
	}

	updated, err := getRuntimeSettings(ctx, tx)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{},
			fmt.Errorf("read saved runtime settings: %w", err)
	}
	after, err := runtimeSettingsState(runtimeSettingsDocument(updated), updated.Revision)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}
	checkpointID, err := s.recordRuntimeSettingsChange(
		ctx, tx, change, restoredFromID, restoredSide, before, after,
	)
	if err != nil {
		return storage.SaveRuntimeSettingsResult{}, err
	}

	return storage.SaveRuntimeSettingsResult{
		Settings:     updated,
		CheckpointID: &checkpointID,
	}, nil
}

func runtimeFormattingChanged(current, proposed *config.Config) bool {
	currentPolicy := config.DefaultFormattingPolicy()
	if current != nil {
		currentPolicy = current.Formatting
	}
	proposedPolicy := config.DefaultFormattingPolicy()
	if proposed != nil {
		proposedPolicy = proposed.Formatting
	}

	return currentPolicy != proposedPolicy
}

func invalidatePlansForRuntimeFormatting(
	ctx context.Context,
	tx *transaction,
	current storage.RuntimeSettings,
	change storage.RuntimeSettingsChange,
) error {
	if !runtimeFormattingChanged(current.BotConfig, change.BotConfig) {
		return nil
	}

	return invalidateAllLivePlans(ctx, tx, change.ChangedAt)
}

func ensureRuntimeSettingsBaseline(
	ctx context.Context,
	tx *transaction,
	change storage.RuntimeSettingsChange,
	before *storage.SettingsCheckpointState,
) error {
	exists, err := settingsBaselineExists(ctx, tx, storage.SettingsCheckpointScopeRoot, "")
	if err != nil || exists {
		return err
	}

	return insertSettingsBaseline(ctx, tx, storage.SettingsCheckpointCreate{
		Scope:          storage.SettingsCheckpointScopeRoot,
		ActorAccountID: change.ActorAccountID,
		Action:         storage.SettingsCheckpointActionBaseline,
		CreatedAt:      change.ChangedAt,
		Items: []storage.SettingsCheckpointItem{{
			Kind:            storage.SettingsCheckpointItemRuntime,
			DocumentVersion: storage.SettingsCheckpointDocumentVersion,
			After:           before,
		}},
	})
}

func (s *Store) recordRuntimeSettingsChange(
	ctx context.Context,
	tx *transaction,
	change storage.RuntimeSettingsChange,
	restoredFromID *int64,
	restoredSide storage.SettingsCheckpointRestoreSide,
	before, after *storage.SettingsCheckpointState,
) (int64, error) {
	action := storage.SettingsCheckpointActionSave
	auditAction := actionRuntimeSettingsSaved
	summary := "Saved runtime settings"
	if restoredFromID != nil {
		action = storage.SettingsCheckpointActionRestore
		auditAction = actionRuntimeSettingsRestored
		summary = "Restored runtime settings"
	}
	checkpointID, err := s.createSettingsCheckpoint(ctx, tx, storage.SettingsCheckpointCreate{
		Scope:          storage.SettingsCheckpointScopeRoot,
		ActorAccountID: change.ActorAccountID,
		Action:         action, RestoredFromID: restoredFromID, RestoredSide: restoredSide,
		CreatedAt: change.ChangedAt,
		Items: []storage.SettingsCheckpointItem{{
			Kind:            storage.SettingsCheckpointItemRuntime,
			DocumentVersion: storage.SettingsCheckpointDocumentVersion,
			Before:          before, After: after,
		}},
	})
	if err != nil {
		return 0, err
	}
	sourceKind := settingsCheckpointSourceKind
	if _, err := insertAppAudit(ctx, tx, appAuditInsert{
		Category:   string(storage.AuditCategoryRuntime),
		SourceKind: &sourceKind, SourceID: &checkpointID,
		ActorAccountID: change.ActorAccountID,
		Action:         auditAction, Summary: summary, CreatedAt: change.ChangedAt,
	}); err != nil {
		return 0, err
	}

	return checkpointID, nil
}

func (s *Store) writeRuntimeSettings(
	ctx context.Context,
	tx runner,
	current storage.RuntimeSettings,
	change storage.RuntimeSettingsChange,
	botConfig *string,
) error {
	var result sql.Result
	var err error
	if current.Revision == 0 {
		result, err = tx.ExecContext(ctx, `
INSERT INTO runtime_settings (
    singleton, background_work_paused, bot_config, log_level,
    poll_interval_seconds, pending_ci_quiet_period_seconds, session_ttl_seconds,
    path_index_interval_seconds,
    revision, updated_at, updated_by_account_id
) VALUES (1, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			change.BackgroundWorkPaused,
			botConfig,
			change.LogLevel,
			durationSeconds(change.PollInterval),
			durationSeconds(change.PendingCIQuietPeriod),
			durationSeconds(change.SessionTTL),
			durationSeconds(change.PathIndexInterval),
			change.ChangedAt,
			change.ActorAccountID,
		)
	} else {
		result, err = tx.ExecContext(ctx, `
UPDATE runtime_settings SET
    background_work_paused = ?, bot_config = ?, log_level = ?, poll_interval_seconds = ?,
    pending_ci_quiet_period_seconds = ?, session_ttl_seconds = ?,
    path_index_interval_seconds = ?,
    revision = revision + 1, updated_at = ?, updated_by_account_id = ?
WHERE singleton = 1 AND revision = ?`,
			change.BackgroundWorkPaused,
			botConfig,
			change.LogLevel,
			durationSeconds(change.PollInterval),
			durationSeconds(change.PendingCIQuietPeriod),
			durationSeconds(change.SessionTTL),
			durationSeconds(change.PathIndexInterval),
			change.ChangedAt,
			change.ActorAccountID,
			change.ExpectedRevision,
		)
	}
	if err != nil {
		if s.dialect.UniqueViolation(err) {
			return storage.ErrConflict
		}
		return fmt.Errorf("write runtime settings: %w", err)
	}
	return requireOneRow(result)
}

func sameOptionalDuration(left, right *time.Duration) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

func validateRuntimeSettingsChange(change storage.RuntimeSettingsChange) (*string, error) {
	if change.ExpectedRevision < 0 || strings.TrimSpace(change.ActorAccountID) == "" {
		return nil, errors.New("runtime settings identity and revision are required")
	}
	if err := validateRuntimeSettingsDocumentValue(storage.RuntimeSettingsDocument{
		BackgroundWorkPaused: change.BackgroundWorkPaused,
		BotConfig:            change.BotConfig, LogLevel: change.LogLevel,
		PollInterval: change.PollInterval, PendingCIQuietPeriod: change.PendingCIQuietPeriod,
		SessionTTL: change.SessionTTL, PathIndexInterval: change.PathIndexInterval,
	}); err != nil {
		return nil, err
	}
	if change.EffectivePendingCIQuietPeriod < 0 {
		return nil, errors.New("effective merge-after-CI quiet period cannot be negative")
	}
	if change.EffectiveSessionTTL < time.Minute {
		return nil, errors.New("effective session lifetime must be at least one minute")
	}
	if change.BotConfig == nil {
		return nil, nil
	}
	content, err := json.Marshal(change.BotConfig)
	if err != nil {
		return nil, fmt.Errorf("encode runtime bot config: %w", err)
	}
	value := string(content)

	return &value, nil
}

func getRuntimeSettings(
	ctx context.Context,
	queryer rowQuerier,
) (storage.RuntimeSettings, error) {
	var settings storage.RuntimeSettings
	var botConfig, logLevel sql.NullString
	var pollSeconds, pendingCIQuietSeconds, sessionSeconds, pathIndexSeconds sql.NullInt64
	var updatedByID string
	var updatedAt StoredTime
	err := queryer.QueryRowContext(ctx, `
SELECT background_work_paused, bot_config, log_level,
       poll_interval_seconds, pending_ci_quiet_period_seconds,
       session_ttl_seconds, path_index_interval_seconds,
       revision, updated_at, updated_by_account_id
	FROM runtime_settings WHERE singleton = 1`).Scan(
		&settings.BackgroundWorkPaused,
		&botConfig,
		&logLevel,
		&pollSeconds,
		&pendingCIQuietSeconds,
		&sessionSeconds,
		&pathIndexSeconds,
		&settings.Revision,
		&updatedAt,
		&updatedByID,
	)
	if err != nil {
		return storage.RuntimeSettings{}, err
	}
	if botConfig.Valid {
		var value config.Config
		if err := json.Unmarshal([]byte(botConfig.String), &value); err != nil {
			return storage.RuntimeSettings{}, fmt.Errorf("decode runtime bot config: %w", err)
		}
		settings.BotConfig = &value
	}
	settings.LogLevel = stringPointer(logLevel)
	settings.PollInterval = durationPointer(pollSeconds)
	settings.PendingCIQuietPeriod = durationPointer(pendingCIQuietSeconds)
	settings.SessionTTL = durationPointer(sessionSeconds)
	settings.PathIndexInterval = durationPointer(pathIndexSeconds)
	settings.UpdatedAt = updatedAt.Pointer()
	account, err := getAccount(ctx, queryer, updatedByID)
	if err != nil {
		return storage.RuntimeSettings{}, err
	}
	settings.UpdatedBy = &account

	return settings, nil
}

func durationPointer(value sql.NullInt64) *time.Duration {
	if !value.Valid {
		return nil
	}
	duration := time.Duration(value.Int64) * time.Second

	return &duration
}

func durationSeconds(value *time.Duration) any {
	if value == nil {
		return nil
	}

	return int64(*value / time.Second)
}

func validRuntimeLogLevel(value string) bool {
	switch value {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}

func shortenSessions(ctx context.Context, tx runner, lifetime time.Duration) error {
	rows, err := tx.QueryContext(ctx, "SELECT token_hash, created_at, expires_at FROM sessions")
	if err != nil {
		return fmt.Errorf("list sessions for lifetime update: %w", err)
	}
	type sessionWindow struct {
		tokenHash string
		createdAt time.Time
		expiresAt time.Time
	}
	sessions, err := collectRows(rows, func(scanner rowScanner) (sessionWindow, error) {
		var item sessionWindow
		var createdAt, expiresAt StoredTime
		if scanErr := scanner.Scan(&item.tokenHash, &createdAt, &expiresAt); scanErr != nil {
			return sessionWindow{}, scanErr
		}
		item.createdAt, item.expiresAt = createdAt.Time(), expiresAt.Time()

		return item, nil
	})
	if err != nil {
		return fmt.Errorf("read sessions for lifetime update: %w", err)
	}
	for _, session := range sessions {
		maximum := session.createdAt.Add(lifetime)
		if !session.expiresAt.After(maximum) {
			continue
		}
		if _, err := tx.ExecContext(
			ctx,
			"UPDATE sessions SET expires_at = ? WHERE token_hash = ?",
			maximum,
			session.tokenHash,
		); err != nil {
			return fmt.Errorf("shorten session lifetime: %w", err)
		}
	}

	return nil
}
