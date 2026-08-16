package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

const actionRuntimeSettings = "runtime.settings.updated"

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

// UpdateRuntimeSettings atomically replaces persisted overrides, shortens
// active sessions when required, and appends the Root audit event.
func (s *Store) UpdateRuntimeSettings(
	ctx context.Context,
	change storage.RuntimeSettingsChange,
) (storage.RuntimeSettings, error) {
	botConfig, err := validateRuntimeSettingsChange(change)
	if err != nil {
		return storage.RuntimeSettings{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.RuntimeSettings{}, fmt.Errorf("begin runtime settings update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	current, err := getRuntimeSettings(ctx, tx)
	if errors.Is(err, sql.ErrNoRows) {
		current = storage.RuntimeSettings{}
	} else if err != nil {
		return storage.RuntimeSettings{}, fmt.Errorf("read runtime settings for update: %w", err)
	}
	if current.Revision != change.ExpectedRevision {
		return storage.RuntimeSettings{}, storage.ErrConflict
	}

	if current.Revision == 0 {
		_, err = tx.ExecContext(ctx, `
INSERT INTO runtime_settings (
    singleton, bot_config, log_level,
    poll_interval_seconds, pending_ci_quiet_period_seconds, session_ttl_seconds,
    revision, updated_at, updated_by_account_id
) VALUES (1, ?, ?, ?, ?, ?, 1, ?, ?)`,
			botConfig,
			change.LogLevel,
			durationSeconds(change.PollInterval),
			durationSeconds(change.PendingCIQuietPeriod),
			durationSeconds(change.SessionTTL),
			change.ChangedAt,
			change.ActorAccountID,
		)
	} else {
		_, err = tx.ExecContext(ctx, `
UPDATE runtime_settings SET
    bot_config = ?, log_level = ?, poll_interval_seconds = ?,
    pending_ci_quiet_period_seconds = ?, session_ttl_seconds = ?,
    revision = revision + 1, updated_at = ?, updated_by_account_id = ?
WHERE singleton = 1 AND revision = ?`,
			botConfig,
			change.LogLevel,
			durationSeconds(change.PollInterval),
			durationSeconds(change.PendingCIQuietPeriod),
			durationSeconds(change.SessionTTL),
			change.ChangedAt,
			change.ActorAccountID,
			change.ExpectedRevision,
		)
	}
	if err != nil {
		return storage.RuntimeSettings{}, fmt.Errorf("write runtime settings: %w", err)
	}
	if err := shortenSessions(ctx, tx, change.EffectiveSessionTTL); err != nil {
		return storage.RuntimeSettings{}, err
	}
	if _, err := insertAppAudit(ctx, tx, appAuditInsert{
		Category:       string(storage.AuditCategoryRuntime),
		ActorAccountID: change.ActorAccountID,
		Action:         actionRuntimeSettings,
		Summary:        "Updated runtime settings",
		CreatedAt:      change.ChangedAt,
	}); err != nil {
		return storage.RuntimeSettings{}, err
	}

	updated, err := getRuntimeSettings(ctx, tx)
	if err != nil {
		return storage.RuntimeSettings{}, fmt.Errorf("read updated runtime settings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return storage.RuntimeSettings{}, fmt.Errorf("commit runtime settings update: %w", err)
	}

	return updated, nil
}

func validateRuntimeSettingsChange(change storage.RuntimeSettingsChange) (*string, error) {
	if change.ExpectedRevision < 0 || strings.TrimSpace(change.ActorAccountID) == "" {
		return nil, errors.New("runtime settings identity and revision are required")
	}
	if change.SessionTTL != nil && *change.SessionTTL < time.Minute {
		return nil, errors.New("session lifetime must be at least one minute")
	}
	if change.PollInterval != nil && *change.PollInterval < 0 {
		return nil, errors.New("reaction sweep interval cannot be negative")
	}
	if change.PendingCIQuietPeriod != nil && *change.PendingCIQuietPeriod <= 0 {
		return nil, errors.New("merge-after-CI quiet period must be positive")
	}
	if change.EffectivePollInterval < 0 {
		return nil, errors.New("effective reaction sweep interval cannot be negative")
	}
	if change.EffectivePendingCIQuietPeriod <= 0 {
		return nil, errors.New("effective merge-after-CI quiet period must be positive")
	}
	if change.EffectiveSessionTTL < time.Minute {
		return nil, errors.New("effective session lifetime must be at least one minute")
	}
	if change.LogLevel != nil && !validRuntimeLogLevel(*change.LogLevel) {
		return nil, fmt.Errorf("unsupported runtime log level %q", *change.LogLevel)
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
	var pollSeconds, pendingCIQuietSeconds, sessionSeconds sql.NullInt64
	var updatedByID string
	var updatedAt StoredTime
	err := queryer.QueryRowContext(ctx, `
SELECT bot_config, log_level, poll_interval_seconds, pending_ci_quiet_period_seconds,
       session_ttl_seconds,
       revision, updated_at, updated_by_account_id
FROM runtime_settings WHERE singleton = 1`).Scan(
		&botConfig,
		&logLevel,
		&pollSeconds,
		&pendingCIQuietSeconds,
		&sessionSeconds,
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
