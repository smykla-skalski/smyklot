package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

const scheduleProfileColumns = `
    id, target_id, name, timezone, system, archived_at, revision, created_at, updated_at`

func (s *Store) ListScheduleProfiles(
	ctx context.Context,
	includeArchived bool,
) ([]workqueue.Profile, error) {
	query := "SELECT" + scheduleProfileColumns + " FROM schedule_profiles"
	if !includeArchived {
		query += " WHERE archived_at IS NULL"
	}
	query += " ORDER BY system DESC, lower(name), id"
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list schedule profiles: %w", err)
	}
	profiles, err := collectRows(rows, scanScheduleProfile)
	if err != nil {
		return nil, fmt.Errorf("read schedule profiles: %w", err)
	}
	for index := range profiles {
		if err := loadProfileRules(ctx, s.db, &profiles[index]); err != nil {
			return nil, err
		}
		if err := loadProfileImpact(ctx, s.db, &profiles[index]); err != nil {
			return nil, err
		}
	}

	return profiles, nil
}

func (s *Store) GetScheduleProfile(ctx context.Context, id string) (workqueue.Profile, error) {
	profile, err := getScheduleProfile(ctx, s.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return workqueue.Profile{}, storage.ErrNotFound
	}
	if err != nil {
		return workqueue.Profile{}, fmt.Errorf("get schedule profile: %w", err)
	}
	if err := loadProfileImpact(ctx, s.db, &profile); err != nil {
		return workqueue.Profile{}, err
	}

	return profile, nil
}

func loadProfileImpact(ctx context.Context, runner runner, profile *workqueue.Profile) error {
	if err := runner.QueryRowContext(ctx, `
SELECT COUNT(*) FROM queue_policies WHERE profile_id = ?`, profile.ID,
	).Scan(&profile.AffectedPolicies); err != nil {
		return fmt.Errorf("count schedule profile policies: %w", err)
	}
	if err := runner.QueryRowContext(ctx, `
SELECT COUNT(*) FROM queue_items WHERE profile_id = ? AND immediate_dispatch = FALSE
  AND state IN ('scheduled', 'ready', 'retrying', 'blocked')`, profile.ID,
	).Scan(&profile.AffectedItems); err != nil {
		return fmt.Errorf("count schedule profile items: %w", err)
	}
	if err := runner.QueryRowContext(ctx, `
SELECT COUNT(*) FROM (
  SELECT target_id FROM queue_policies WHERE profile_id = ? AND target_id IS NOT NULL
  UNION
  SELECT target_id FROM queue_items WHERE profile_id = ? AND target_id IS NOT NULL
) affected`, profile.ID, profile.ID).Scan(&profile.AffectedInstallations); err != nil {
		return fmt.Errorf("count schedule profile installations: %w", err)
	}

	return nil
}

func getScheduleProfile(
	ctx context.Context,
	runner runner,
	id string,
) (workqueue.Profile, error) {
	profile, err := scanScheduleProfile(runner.QueryRowContext(
		ctx, "SELECT"+scheduleProfileColumns+" FROM schedule_profiles WHERE id = ?", id,
	))
	if err != nil {
		return workqueue.Profile{}, err
	}
	if err := loadProfileRules(ctx, runner, &profile); err != nil {
		return workqueue.Profile{}, err
	}

	return profile, nil
}

func scanScheduleProfile(scanner rowScanner) (workqueue.Profile, error) {
	var profile workqueue.Profile
	var targetID sql.NullString
	var archived, created, updated StoredTime
	if err := scanner.Scan(
		&profile.ID, &targetID, &profile.Name, &profile.Timezone, &profile.System,
		&archived, &profile.Revision, &created, &updated,
	); err != nil {
		return workqueue.Profile{}, err
	}
	profile.TargetID = stringPointer(targetID)
	profile.ArchivedAt = archived.Pointer()
	profile.CreatedAt, profile.UpdatedAt = created.Time(), updated.Time()

	return profile, nil
}

func loadProfileRules(ctx context.Context, runner runner, profile *workqueue.Profile) error {
	windows, err := runner.QueryContext(ctx, `
SELECT weekday, start_minute, end_minute FROM schedule_windows
WHERE profile_id = ? ORDER BY weekday, start_minute`, profile.ID)
	if err != nil {
		return fmt.Errorf("list schedule windows: %w", err)
	}
	profile.Windows, err = collectRows(windows, scanScheduleWindow)
	if err != nil {
		return fmt.Errorf("read schedule windows: %w", err)
	}
	exceptions, err := runner.QueryContext(ctx, `
SELECT local_date, closed, start_minute, end_minute FROM schedule_exceptions
WHERE profile_id = ? ORDER BY local_date, start_minute`, profile.ID)
	if err != nil {
		return fmt.Errorf("list schedule exceptions: %w", err)
	}
	profile.Exceptions, err = collectRows(exceptions, scanScheduleException)
	if err != nil {
		return fmt.Errorf("read schedule exceptions: %w", err)
	}

	return nil
}

func scanScheduleWindow(scanner rowScanner) (workqueue.Window, error) {
	var window workqueue.Window
	var weekday int
	if err := scanner.Scan(&weekday, &window.Start, &window.End); err != nil {
		return workqueue.Window{}, err
	}
	window.Weekday = time.Weekday(weekday)

	return window, nil
}

func scanScheduleException(scanner rowScanner) (workqueue.Exception, error) {
	var exception workqueue.Exception
	err := scanner.Scan(&exception.Date, &exception.Closed, &exception.Start, &exception.End)

	return exception, err
}

func (s *Store) SaveScheduleProfile(
	ctx context.Context,
	change workqueue.ProfileChange,
) (workqueue.Profile, error) {
	profile := workqueue.Profile{
		ID: change.ID, TargetID: change.TargetID, Name: change.Name,
		Timezone: change.Timezone, Windows: change.Windows, Exceptions: change.Exceptions,
	}
	if err := workqueue.ValidateProfile(profile); err != nil {
		return workqueue.Profile{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Profile{}, fmt.Errorf("begin schedule profile save: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if change.ExpectedRevision == 0 {
		err = insertScheduleProfile(ctx, tx, profile, change.ActorID, change.ChangedAt)
	} else {
		err = s.updateScheduleProfile(ctx, tx, profile, change)
	}
	if err != nil {
		if s.dialect.UniqueViolation(err) {
			return workqueue.Profile{}, storage.ErrConflict
		}
		return workqueue.Profile{}, err
	}
	if err := replaceProfileRules(ctx, tx, profile); err != nil {
		return workqueue.Profile{}, err
	}
	stored, err := getScheduleProfile(ctx, tx, change.ID)
	if err != nil {
		return workqueue.Profile{}, err
	}
	if err := rescheduleProfileQueueItems(
		ctx, tx, stored, change.ChangedAt, change.ActorID,
	); err != nil {
		return workqueue.Profile{}, err
	}
	if err := tx.Commit(); err != nil {
		return workqueue.Profile{}, fmt.Errorf("commit schedule profile save: %w", err)
	}

	return stored, nil
}

func rescheduleProfileQueueItems(
	ctx context.Context,
	tx *transaction,
	profile workqueue.Profile,
	now time.Time,
	actorID string,
) error {
	rows, err := tx.QueryContext(ctx, "SELECT"+queueItemColumns+`
FROM queue_items WHERE profile_id = ? AND immediate_dispatch = FALSE
  AND window_mode = 'respect'
  AND state IN ('scheduled', 'ready', 'retrying', 'blocked')`, profile.ID)
	if err != nil {
		return fmt.Errorf("list queue items for profile: %w", err)
	}
	items, err := collectRows(rows, scanQueueItem)
	if err != nil {
		return fmt.Errorf("read queue items for profile: %w", err)
	}
	for _, item := range items {
		eligible, eligibleErr := workqueue.NextEligible(profile, item.NotBefore)
		if eligibleErr != nil {
			return eligibleErr
		}
		policy, policyErr := getEffectiveQueuePolicy(ctx, tx, item.Kind, item.TargetID)
		if policyErr != nil {
			return policyErr
		}
		state, blockedReason := stateForEligibility(eligible, now), ""
		if !policy.Enabled {
			state, blockedReason = workqueue.StateBlocked, queueBlockedDisabled
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE queue_items SET eligible_at = ?, state = ?, blocked_reason = ?,
    updated_at = ?, revision = revision + 1 WHERE id = ?`,
			eligible, state, blockedReason, now, item.ID); err != nil {
			return fmt.Errorf("reschedule queue item for profile: %w", err)
		}
		if err := insertQueueEvent(ctx, tx, workqueue.Event{
			ItemID: item.ID, ActorID: queueEventActor(actorID), Kind: "schedule.recomputed",
			State: state, Summary: "Schedule profile changed", CreatedAt: now,
		}); err != nil {
			return err
		}
	}

	return nil
}

func insertScheduleProfile(
	ctx context.Context,
	tx *transaction,
	profile workqueue.Profile,
	actorID string,
	at time.Time,
) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO schedule_profiles (
    id, target_id, name, timezone, system, revision,
    created_by, updated_by, created_at, updated_at
) VALUES (?, ?, ?, ?, FALSE, 1, ?, ?, ?, ?)`,
		profile.ID, profile.TargetID, profile.Name, profile.Timezone,
		actorID, actorID, at, at,
	)
	if err != nil {
		return fmt.Errorf("insert schedule profile: %w", err)
	}

	return nil
}

func (s *Store) updateScheduleProfile(
	ctx context.Context,
	tx *transaction,
	profile workqueue.Profile,
	change workqueue.ProfileChange,
) error {
	current, err := getScheduleProfile(ctx, tx, change.ID)
	if err != nil {
		return noRows(err)
	}
	if current.System || current.Revision != change.ExpectedRevision ||
		!sameOptionalString(current.TargetID, change.TargetID) {
		return storage.ErrConflict
	}
	result, err := tx.ExecContext(ctx, `
UPDATE schedule_profiles SET
    name = ?, timezone = ?, revision = revision + 1, updated_by = ?, updated_at = ?
WHERE id = ? AND revision = ? AND system = FALSE`,
		profile.Name, profile.Timezone, change.ActorID, change.ChangedAt,
		change.ID, change.ExpectedRevision,
	)
	if err != nil {
		return fmt.Errorf("update schedule profile: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return storage.ErrConflict
	}

	return nil
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

func replaceProfileRules(
	ctx context.Context,
	tx *transaction,
	profile workqueue.Profile,
) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM schedule_windows WHERE profile_id = ?", profile.ID); err != nil {
		return fmt.Errorf("clear schedule windows: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM schedule_exceptions WHERE profile_id = ?", profile.ID); err != nil {
		return fmt.Errorf("clear schedule exceptions: %w", err)
	}
	for _, window := range profile.Windows {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO schedule_windows (profile_id, weekday, start_minute, end_minute)
VALUES (?, ?, ?, ?)`, profile.ID, int(window.Weekday), window.Start, window.End); err != nil {
			return fmt.Errorf("insert schedule window: %w", err)
		}
	}
	for _, exception := range profile.Exceptions {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO schedule_exceptions (profile_id, local_date, closed, start_minute, end_minute)
VALUES (?, ?, ?, ?, ?)`, profile.ID, exception.Date, exception.Closed,
			exception.Start, exception.End); err != nil {
			return fmt.Errorf("insert schedule exception: %w", err)
		}
	}

	return nil
}

func (s *Store) ArchiveScheduleProfile(
	ctx context.Context,
	id string,
	expectedRevision int64,
	actorID string,
	at time.Time,
) (workqueue.Profile, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.Profile{}, fmt.Errorf("begin schedule profile archive: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	profile, err := getScheduleProfile(ctx, tx, id)
	if err != nil {
		return workqueue.Profile{}, noRows(err)
	}
	if profile.System || profile.Revision != expectedRevision || profile.ArchivedAt != nil {
		return workqueue.Profile{}, storage.ErrConflict
	}
	var assigned int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM queue_policies WHERE profile_id = ?", id,
	).Scan(&assigned); err != nil {
		return workqueue.Profile{}, fmt.Errorf("count schedule profile assignments: %w", err)
	}
	if assigned > 0 {
		return workqueue.Profile{}, storage.ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE schedule_profiles SET archived_at = ?, revision = revision + 1,
    updated_by = ?, updated_at = ? WHERE id = ? AND revision = ?`,
		at, actorID, at, id, expectedRevision,
	); err != nil {
		return workqueue.Profile{}, fmt.Errorf("archive schedule profile: %w", err)
	}
	profile.ArchivedAt, profile.Revision, profile.UpdatedAt = &at, profile.Revision+1, at
	if err := tx.Commit(); err != nil {
		return workqueue.Profile{}, fmt.Errorf("commit schedule profile archive: %w", err)
	}

	return profile, nil
}

func validateProfileScope(profile workqueue.Profile, targetID *string) error {
	if profile.ArchivedAt != nil {
		return errors.New("schedule profile is archived")
	}
	if targetID == nil && profile.TargetID != nil {
		return errors.New("global policy cannot use an installation profile")
	}
	if profile.TargetID != nil && !sameOptionalString(profile.TargetID, targetID) {
		return errors.New("schedule profile belongs to another installation")
	}
	if strings.TrimSpace(profile.Timezone) == "" {
		return errors.New("schedule profile timezone is required")
	}

	return nil
}
