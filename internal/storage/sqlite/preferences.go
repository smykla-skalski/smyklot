package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// GetPreferences reads one account's preference document. A missing row is
// the first-class "never stored" state: an empty document at revision 0.
func (s *Store) GetPreferences(
	ctx context.Context,
	accountID string,
) (storage.Preferences, error) {
	preferences, err := getPreferences(ctx, s.db, accountID)
	if err != nil {
		return storage.Preferences{}, err
	}

	return preferences, nil
}

// ApplyPreferences merges per-key changes into an account's document with
// last-write-wins semantics and bumps its revision. The server is the single
// writer, so there is no optimistic-concurrency conflict path.
func (s *Store) ApplyPreferences(
	ctx context.Context,
	change storage.PreferenceChange,
) (storage.Preferences, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Preferences{}, fmt.Errorf("begin preferences update: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	current, err := getPreferences(ctx, tx, change.AccountID)
	if err != nil {
		return storage.Preferences{}, err
	}

	if !mergePreferenceChanges(current.Values, change.Changes) {
		return current, nil
	}

	doc, err := json.Marshal(current.Values)
	if err != nil {
		return storage.Preferences{}, fmt.Errorf("encode preferences doc: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO user_preferences (account_id, doc, revision, updated_at)
VALUES (?, ?, 1, ?)
ON CONFLICT(account_id) DO UPDATE SET
    doc = excluded.doc,
    revision = user_preferences.revision + 1,
    updated_at = excluded.updated_at`,
		change.AccountID,
		string(doc),
		formatTime(change.ChangedAt),
	); err != nil {
		return storage.Preferences{}, fmt.Errorf("update preferences: %w", err)
	}

	updated, err := getPreferences(ctx, tx, change.AccountID)
	if err != nil {
		return storage.Preferences{}, fmt.Errorf("read updated preferences: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return storage.Preferences{}, fmt.Errorf("commit preferences update: %w", err)
	}

	return updated, nil
}

func getPreferences(
	ctx context.Context,
	queryer rowQuerier,
	accountID string,
) (storage.Preferences, error) {
	var doc, updatedAt string
	var revision int64
	err := queryer.QueryRowContext(ctx, `
SELECT doc, revision, updated_at
FROM user_preferences
WHERE account_id = ?`, accountID).Scan(&doc, &revision, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Preferences{
			AccountID: accountID,
			Values:    map[string]json.RawMessage{},
		}, nil
	}
	if err != nil {
		return storage.Preferences{}, fmt.Errorf("read preferences: %w", err)
	}

	var values map[string]json.RawMessage
	if err := json.Unmarshal([]byte(doc), &values); err != nil {
		return storage.Preferences{}, fmt.Errorf("decode preferences doc: %w", err)
	}
	if values == nil {
		values = map[string]json.RawMessage{}
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return storage.Preferences{}, err
	}

	return storage.Preferences{
		AccountID: accountID,
		Values:    values,
		Revision:  revision,
		UpdatedAt: updated,
	}, nil
}

// mergePreferenceChanges applies per-key changes to values and reports
// whether anything changed. A nil or JSON-null value deletes the key.
func mergePreferenceChanges(
	values map[string]json.RawMessage,
	changes map[string]json.RawMessage,
) bool {
	changed := false
	for key, value := range changes {
		if isJSONNull(value) {
			if _, exists := values[key]; exists {
				delete(values, key)
				changed = true
			}

			continue
		}

		if current, exists := values[key]; exists && bytes.Equal(current, value) {
			continue
		}

		values[key] = value
		changed = true
	}

	return changed
}

func isJSONNull(value json.RawMessage) bool {
	return value == nil || bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}
