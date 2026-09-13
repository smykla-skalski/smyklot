package sqlstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// ListSyncCheckObservations pages immutable evidence with the workspace predicate
// checked before any detail is returned. The ordinal is stable across later checks.
func (s *Store) ListSyncCheckObservations(ctx context.Context, targetID, checkID string, after, limit int) (orgsync.CheckObservationPage, error) {
	item, err := getQueueItem(ctx, s.db, checkID, "")
	if err != nil {
		return orgsync.CheckObservationPage{}, noRows(err)
	}
	if item.Kind != workqueue.KindSyncScan || item.TargetID == nil || *item.TargetID != targetID {
		return orgsync.CheckObservationPage{}, storage.ErrNotFound
	}
	var details orgsync.CheckDetails
	if len(item.Details) > 0 {
		if err := json.Unmarshal(item.Details, &details); err != nil {
			return orgsync.CheckObservationPage{}, fmt.Errorf("read check outcome: %w", err)
		}
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
