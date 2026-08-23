package sqlstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

type settingsBaselineTargetDocument struct {
	document storage.TargetSettingsDocument
	revision int64
}

type settingsBaselineRepository struct {
	id       string
	fullName string
	document storage.RepositorySettingsDocument
	revision int64
}

func readSettingsBaselineTarget(
	ctx context.Context,
	queryer rowQuerier,
	targetID string,
) (settingsBaselineTargetDocument, error) {
	return scanSettingsBaselineTarget(queryer.QueryRowContext(ctx, `
SELECT repository_default_enabled, pending_ci_mode_default,
       pending_ci_branch_patterns_default,
       pending_ci_quiet_period_seconds_override,
       path_index_interval_seconds_override, config_patch, revision
FROM targets
WHERE id = ?`, targetID))
}

func scanSettingsBaselineTarget(scanner rowScanner) (settingsBaselineTargetDocument, error) {
	var row settingsBaselineTargetDocument
	var branchPatterns, patch string
	var quietPeriod, pathIndexInterval sql.NullInt64
	if err := scanner.Scan(
		&row.document.RepositoryDefaultEnabled,
		&row.document.PendingCIModeDefault,
		&branchPatterns,
		&quietPeriod,
		&pathIndexInterval,
		&patch,
		&row.revision,
	); err != nil {
		return settingsBaselineTargetDocument{}, err
	}
	patterns, err := unmarshalPendingCIBranchPatterns(branchPatterns)
	if err != nil {
		return settingsBaselineTargetDocument{}, err
	}
	configPatch, err := decodeSettingsBaselinePatch(patch)
	if err != nil {
		return settingsBaselineTargetDocument{}, err
	}
	row.document.PendingCIBranchPatternsDefault = patterns
	row.document.PendingCIQuietPeriodOverride = durationPointer(quietPeriod)
	row.document.PathIndexIntervalOverride = durationPointer(pathIndexInterval)
	row.document.ConfigPatch = configPatch

	return row, nil
}

func listSettingsBaselineRepositories(
	ctx context.Context,
	queryer runner,
	targetID string,
) ([]settingsBaselineRepository, error) {
	rows, err := queryer.QueryContext(ctx, `
SELECT id, full_name, enabled_override, pending_ci_mode_override,
       pending_ci_branch_patterns_override,
       pending_ci_quiet_period_seconds_override,
       path_index_interval_seconds_override, config_patch,
       ignore_repository_file, revision
FROM repositories
WHERE target_id = ?
ORDER BY id`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list repository settings: %w", err)
	}
	repositories, err := collectRows(rows, scanSettingsBaselineRepository)
	if err != nil {
		return nil, fmt.Errorf("read repository settings: %w", err)
	}

	return repositories, nil
}

func scanSettingsBaselineRepository(scanner rowScanner) (settingsBaselineRepository, error) {
	var row settingsBaselineRepository
	var enabled sql.NullBool
	var mode, branchPatterns sql.NullString
	var quietPeriod, pathIndexInterval sql.NullInt64
	var patch string
	if err := scanner.Scan(
		&row.id, &row.fullName, &enabled, &mode, &branchPatterns,
		&quietPeriod, &pathIndexInterval, &patch,
		&row.document.IgnoreRepositoryFile, &row.revision,
	); err != nil {
		return settingsBaselineRepository{}, err
	}
	row.document.EnabledOverride = boolPointer(enabled)
	if mode.Valid {
		value := storage.PendingCIMode(mode.String)
		row.document.PendingCIModeOverride = &value
	}
	if branchPatterns.Valid {
		value, err := unmarshalPendingCIBranchPatterns(branchPatterns.String)
		if err != nil {
			return settingsBaselineRepository{}, err
		}
		row.document.PendingCIBranchPatternsOverride = &value
	}
	configPatch, err := decodeSettingsBaselinePatch(patch)
	if err != nil {
		return settingsBaselineRepository{}, err
	}
	row.document.PendingCIQuietPeriodOverride = durationPointer(quietPeriod)
	row.document.PathIndexIntervalOverride = durationPointer(pathIndexInterval)
	row.document.ConfigPatch = configPatch

	return row, nil
}

func decodeSettingsBaselinePatch(document string) (config.Patch, error) {
	var patch config.Patch
	decoder := json.NewDecoder(bytes.NewBufferString(document))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&patch); err != nil {
		return config.Patch{}, fmt.Errorf("decode settings baseline config patch: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return config.Patch{}, errors.New("settings baseline config patch has trailing JSON")
	}

	return patch, nil
}
