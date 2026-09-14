package sqlstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// ListSyncCheckObservations pages immutable evidence with the workspace predicate
// checked before any detail is returned. The ordinal is stable across later checks.
func (s *Store) ListSyncCheckObservations(ctx context.Context, targetID, checkID string, after, limit int) (orgsync.CheckObservationPage, error) {
	details, err := s.GetSyncCheckResult(ctx, targetID, checkID)
	if err != nil {
		return orgsync.CheckObservationPage{}, err
	}
	if details.Outcome == nil {
		return orgsync.CheckObservationPage{}, storage.ErrNotFound
	}
	if after < 0 {
		return orgsync.CheckObservationPage{}, fmt.Errorf("check observation cursor cannot be negative")
	}
	limit = pageLimit(limit)
	page := orgsync.CheckObservationPage{Items: []orgsync.CheckObservation{}}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_check_observations WHERE queue_id = ?", checkID).Scan(&page.Total); err != nil {
		return page, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT ordinal, evidence FROM sync_check_observations WHERE queue_id = ? AND ordinal > ? ORDER BY ordinal LIMIT ?", checkID, after, limit+1)
	if err != nil {
		return page, err
	}
	defer func() { _ = rows.Close() }()
	last := after
	for rows.Next() {
		var ordinal int
		var raw string
		if err := rows.Scan(&ordinal, &raw); err != nil {
			return page, err
		}
		if len(page.Items) == limit {
			page.Next = &last
			break
		}
		var observation orgsync.CheckObservation
		if err := json.Unmarshal([]byte(raw), &observation); err != nil {
			return page, fmt.Errorf("read check observation: %w", err)
		}
		page.Items = append(page.Items, observation)
		last = ordinal
	}
	return page, rows.Err()
}

// GetSyncCheckResult survives worker cleanup and never substitutes current data.
// As with plan reads, callers must authorize the target before calling storage.
func (s *Store) GetSyncCheckResult(ctx context.Context, targetID, checkID string) (orgsync.CheckDetails, error) {
	var details orgsync.CheckDetails
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT details FROM sync_check_results WHERE check_id = ? AND target_id = ?", checkID, targetID).Scan(&raw)
	if err != nil {
		return details, noRows(err)
	}
	if err := json.Unmarshal([]byte(raw), &details); err != nil {
		return orgsync.CheckDetails{}, fmt.Errorf("read retained check result: %w", err)
	}
	return details, nil
}
