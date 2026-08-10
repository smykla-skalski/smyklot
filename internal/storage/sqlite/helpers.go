package sqlite

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

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type rowScanner interface {
	Scan(...any) error
}

type storedTimeRange struct {
	createdAt time.Time
	expiresAt time.Time
}

func marshalPatch(patch config.Patch) (string, error) {
	content, err := json.Marshal(patch)
	if err != nil {
		return "", fmt.Errorf("encode config patch: %w", err)
	}

	return string(content), nil
}

func unmarshalPatch(content string) (config.Patch, error) {
	var patch config.Patch
	if err := json.Unmarshal([]byte(content), &patch); err != nil {
		return config.Patch{}, fmt.Errorf("decode config patch: %w", err)
	}

	return patch, nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse stored time: %w", err)
	}

	return parsed, nil
}

func stringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}

	return &value.String
}

func boolPointer(value sql.NullBool) *bool {
	if !value.Valid {
		return nil
	}

	return &value.Bool
}

func noRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return storage.ErrNotFound
	}

	return err
}

func pageLimit(value int) int {
	if value <= 0 {
		return 50
	}

	return min(value, 100)
}

func countHistory(
	ctx context.Context,
	queryer rowQuerier,
	selectQuery string,
	clauses []string,
	arguments []any,
) (int, error) {
	fromIndex := strings.Index(selectQuery, "FROM ")
	if fromIndex < 0 {
		return 0, fmt.Errorf("history select does not contain a FROM clause")
	}
	from := selectQuery[fromIndex:]
	var total int
	err := queryer.QueryRowContext(
		ctx,
		"SELECT COUNT(*) "+from+" WHERE "+strings.Join(clauses, " AND "),
		arguments...,
	).Scan(&total)

	return total, err
}

func scanTimeRange(scanner rowScanner, fields ...any) (storedTimeRange, error) {
	var createdAt, expiresAt string
	fields = append(fields, &createdAt, &expiresAt)

	if err := scanner.Scan(fields...); err != nil {
		return storedTimeRange{}, err
	}

	created, err := parseTime(createdAt)
	if err != nil {
		return storedTimeRange{}, err
	}

	expires, err := parseTime(expiresAt)
	if err != nil {
		return storedTimeRange{}, err
	}

	return storedTimeRange{createdAt: created, expiresAt: expires}, nil
}

func collectRows[T any](
	rows *sql.Rows,
	scan func(rowScanner) (T, error),
) (items []T, err error) {
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close rows: %w", closeErr)
		}
	}()

	items = make([]T, 0)
	for rows.Next() {
		item, scanErr := scan(rows)
		if scanErr != nil {
			return nil, scanErr
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}
