package sqlstore

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

type queueMetricRow struct {
	lane      workqueue.Lane
	profileID string
	state     workqueue.State
	created   time.Time
	eligible  time.Time
	started   *time.Time
	lease     *time.Time
}

func (s *Store) WorkQueueMetrics(
	ctx context.Context,
	now time.Time,
) (workqueue.MetricsSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT lane, COALESCE(profile_id, ''), state, created_at, eligible_at,
       started_at, lease_expires_at FROM queue_items`)
	if err != nil {
		return workqueue.MetricsSnapshot{}, fmt.Errorf("list queue metrics: %w", err)
	}
	defer func() { _ = rows.Close() }()
	groups := map[string]workqueue.BacklogMetric{}
	snapshot := workqueue.MetricsSnapshot{}
	for rows.Next() {
		row, scanErr := scanQueueMetricRow(rows)
		if scanErr != nil {
			return workqueue.MetricsSnapshot{}, scanErr
		}
		accumulateQueueMetric(&snapshot, groups, row, now)
	}
	if err := rows.Err(); err != nil {
		return workqueue.MetricsSnapshot{}, fmt.Errorf("read queue metrics: %w", err)
	}
	for _, metric := range groups {
		snapshot.Backlogs = append(snapshot.Backlogs, metric)
	}
	sort.Slice(snapshot.Backlogs, func(left, right int) bool {
		first, second := snapshot.Backlogs[left], snapshot.Backlogs[right]
		return first.Lane < second.Lane || first.Lane == second.Lane && first.ProfileID < second.ProfileID
	})

	return snapshot, nil
}

func scanQueueMetricRow(scanner rowScanner) (queueMetricRow, error) {
	var row queueMetricRow
	var created, eligible, started, lease StoredTime
	if err := scanner.Scan(
		&row.lane, &row.profileID, &row.state, &created, &eligible, &started, &lease,
	); err != nil {
		return queueMetricRow{}, fmt.Errorf("scan queue metric: %w", err)
	}
	row.created, row.eligible = created.Time(), eligible.Time()
	row.started, row.lease = started.Pointer(), lease.Pointer()

	return row, nil
}

func accumulateQueueMetric(
	snapshot *workqueue.MetricsSnapshot,
	groups map[string]workqueue.BacklogMetric,
	row queueMetricRow,
	now time.Time,
) {
	if row.state == workqueue.StateFailed {
		snapshot.Failures++
	}
	if row.state == workqueue.StateRunning && row.lease != nil && row.lease.After(now) {
		snapshot.RunningLeases++
	}
	key := string(row.lane) + "\x00" + row.profileID
	metric := groups[key]
	metric.Lane, metric.ProfileID = row.lane, row.profileID
	if row.started != nil {
		latency := row.started.Sub(row.eligible)
		if latency > metric.EligibleToStartLatency {
			metric.EligibleToStartLatency = latency
		}
	}
	if row.state.Terminal() || row.state == workqueue.StateRunning ||
		row.state == workqueue.StateAwaitingApproval {
		groups[key] = metric
		return
	}
	if row.eligible.Before(now) && row.state == workqueue.StateScheduled {
		snapshot.MissedWindows++
	}
	metric.Depth++
	age := now.Sub(row.created)
	if age > metric.OldestAge {
		metric.OldestAge = age
	}
	groups[key] = metric
}
