package sqlstore

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const sampleGrain = time.Hour

func (s *Store) RecordServiceSamples(
	ctx context.Context,
	samples []storage.ServiceSample,
) error {
	return writeEach(ctx, s.db, "service samples", samples, recordServiceSample)
}

func recordServiceSample(
	ctx context.Context,
	tx *transaction,
	sample storage.ServiceSample,
) error {
	if sample.Metric == "" || strings.TrimSpace(sample.Label) == "" {
		return fmt.Errorf("service sample metric and label are required")
	}
	if sample.SampledAt.IsZero() {
		return fmt.Errorf("service sample %s/%s has no time", sample.Metric, sample.Label)
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO service_samples (
    metric, label, sampled_at, observations, failures, total_nanos, max_nanos, value
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (metric, label, sampled_at) DO UPDATE SET
    observations = service_samples.observations + excluded.observations,
    failures = service_samples.failures + excluded.failures,
    total_nanos = service_samples.total_nanos + excluded.total_nanos,
    max_nanos = CASE
        WHEN excluded.max_nanos > service_samples.max_nanos THEN excluded.max_nanos
        ELSE service_samples.max_nanos
      END,
    `+valueFold(sample),
		sample.Metric, sample.Label, sample.SampledAt.UTC().Truncate(sampleGrain),
		sample.Observations, sample.Failures,
		sample.Total, sample.Max, sample.Value,
	)
	if err != nil {
		return fmt.Errorf("record service sample %s/%s: %w", sample.Metric, sample.Label, err)
	}

	return nil
}

// valueFold decides what the readings sharing a point do to each other. A
// reading describes the service at the moment it was taken, so a point covering
// several keeps the highest; a count of what happened since the last reading
// covers only the stretch behind it, so a point covering several holds all of
// them. Composed rather than bound, because which one applies is known before
// the statement is built and neither engine spells either of them differently.
func valueFold(sample storage.ServiceSample) string {
	if sample.Cumulative {
		return "value = service_samples.value + excluded.value"
	}

	return `value = CASE
        WHEN excluded.value > service_samples.value THEN excluded.value
        ELSE service_samples.value
      END`
}

func (s *Store) ListServiceSamples(
	ctx context.Context,
	query storage.ServiceSampleQuery,
) ([]storage.ServiceSample, error) {
	if query.Metric == "" {
		return nil, fmt.Errorf("service sample metric is required")
	}
	clauses := []string{"metric = ?"}
	arguments := []any{query.Metric}
	if !query.Since.IsZero() {
		clauses = append(clauses, "sampled_at >= ?")
		arguments = append(arguments, query.Since.UTC())
	}
	if !query.Until.IsZero() {
		clauses = append(clauses, "sampled_at <= ?")
		arguments = append(arguments, query.Until.UTC())
	}
	where := strings.Join(clauses, " AND ")
	if query.Limit > 0 {
		where += `
  AND label IN (
    SELECT label FROM service_samples
    WHERE ` + where + `
    GROUP BY label
    ORDER BY SUM(total_nanos) DESC, label
    LIMIT ?
  )`
		arguments = append(slices.Concat(arguments, arguments), query.Limit)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT metric, label, sampled_at, observations, failures, total_nanos, max_nanos, value
FROM service_samples
WHERE `+where+`
ORDER BY sampled_at, label`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list service samples: %w", err)
	}
	samples, err := collectRows(rows, scanServiceSample)
	if err != nil {
		return nil, fmt.Errorf("read service samples: %w", err)
	}

	return samples, nil
}

func (s *Store) PruneServiceSamples(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM service_samples WHERE sampled_at < ?", cutoff.UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("prune service samples: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read pruned service samples: %w", err)
	}

	return removed, nil
}

func (s *Store) DrainQueryStats() []storage.QueryStats { return s.db.stats.drain() }

func scanServiceSample(scanner rowScanner) (storage.ServiceSample, error) {
	var sample storage.ServiceSample
	var sampledAt StoredTime
	if err := scanner.Scan(
		&sample.Metric, &sample.Label, &sampledAt, &sample.Observations,
		&sample.Failures, &sample.Total, &sample.Max, &sample.Value,
	); err != nil {
		return storage.ServiceSample{}, err
	}
	sample.SampledAt = sampledAt.Time()

	return sample, nil
}
