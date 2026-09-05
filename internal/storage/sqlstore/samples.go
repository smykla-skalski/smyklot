package sqlstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// sampleGrain is the period every stored sample is folded into.
const sampleGrain = time.Hour

// RecordServiceSamples folds observations into the hour they belong to.
//
// The hour is computed here rather than in SQL: one engine has date_trunc and
// the other does not, and a bound argument needs neither. Writing is an upsert
// because a five-minute sampler visits the same hour twelve times, and the
// twelfth visit has to add to the first rather than replace it.
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
    metric, label, sampled_at, observations, failures, total_millis, max_millis, value
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (metric, label, sampled_at) DO UPDATE SET
    observations = service_samples.observations + excluded.observations,
    failures = service_samples.failures + excluded.failures,
    total_millis = service_samples.total_millis + excluded.total_millis,
    max_millis = CASE
        WHEN excluded.max_millis > service_samples.max_millis THEN excluded.max_millis
        ELSE service_samples.max_millis
      END,
    value = excluded.value`,
		sample.Metric, sample.Label, sample.SampledAt.UTC().Truncate(sampleGrain),
		sample.Observations, sample.Failures,
		millis(sample.Total), millis(sample.Max), sample.Value,
	)
	if err != nil {
		return fmt.Errorf("record service sample %s/%s: %w", sample.Metric, sample.Label, err)
	}

	return nil
}

// ListServiceSamples reads one metric's series over a window, oldest first.
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
	if len(query.Labels) > 0 {
		clause, labels := queueInClause("label", query.Labels)
		clauses = append(clauses, clause)
		arguments = append(arguments, labels...)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT metric, label, sampled_at, observations, failures, total_millis, max_millis, value
FROM service_samples
WHERE `+strings.Join(clauses, " AND ")+`
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

// PruneServiceSamples removes samples older than the cutoff.
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

// DrainQueryStats returns what the store's statements have cost since the last
// call, and resets the counters.
func (s *Store) DrainQueryStats() []storage.QueryStats { return s.stats.drain() }

func scanServiceSample(scanner rowScanner) (storage.ServiceSample, error) {
	var sample storage.ServiceSample
	var sampledAt StoredTime
	var total, max float64
	if err := scanner.Scan(
		&sample.Metric, &sample.Label, &sampledAt, &sample.Observations,
		&sample.Failures, &total, &max, &sample.Value,
	); err != nil {
		return storage.ServiceSample{}, err
	}
	sample.SampledAt = sampledAt.Time()
	sample.Total, sample.Max = duration(total), duration(max)

	return sample, nil
}

func millis(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}

func duration(value float64) time.Duration {
	return time.Duration(value * float64(time.Millisecond))
}
