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

// unmarshalPatch reads a stored patch, refusing one it cannot vouch for.
//
// It was a bare json.Unmarshal, which accepts an unknown key without a word
// and any string at all for the runner. A row saying `{"runner": "workflow"}` -
// written before the panel refused that key, or by hand - resolved to a runner
// neither entry point matches, so both stood down and the repository went
// silent. The file layer has always been fail-closed; this is the same rule for
// the layer beside it.
func unmarshalPatch(content string) (config.Patch, error) {
	patch, err := config.ParseStoredPatch([]byte(content))
	if err != nil {
		return config.Patch{}, fmt.Errorf("decode config patch: %w", err)
	}

	return patch, nil
}

// marshalPaths encodes a list of file paths for storage.
//
// A JSON array rather than a joined string, because a path is text a
// repository chooses and a separator is a bet that it never contains one.
func marshalPaths(paths []string) (string, error) {
	if len(paths) == 0 {
		return "[]", nil
	}

	content, err := json.Marshal(paths)
	if err != nil {
		return "", fmt.Errorf("encode file paths: %w", err)
	}

	return string(content), nil
}

func unmarshalPaths(content string) ([]string, error) {
	if content == "" {
		return nil, nil
	}

	var paths []string
	if err := json.Unmarshal([]byte(content), &paths); err != nil {
		return nil, fmt.Errorf("decode file paths: %w", err)
	}

	return paths, nil
}

func intPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}

	number := int(value.Int64)

	return &number
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
	var createdAt, expiresAt StoredTime
	fields = append(fields, &createdAt, &expiresAt)

	if err := scanner.Scan(fields...); err != nil {
		return storedTimeRange{}, err
	}

	return storedTimeRange{createdAt: createdAt.Time(), expiresAt: expiresAt.Time()}, nil
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
