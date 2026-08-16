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

// unmarshalPatch reads a stored patch.
//
// Deliberately lenient, where every decoder reading a document somebody wrote
// is strict. A stored row is not a document: it is written only by marshalPatch
// after the panel has validated it, and a row that somehow could not be decoded
// strictly would take the whole page with it. collectRows abandons a listing on
// the first row it cannot scan, and UpdateRepositorySettings reads the row back
// inside its own transaction - so a strict decode here turns one bad row into
// an installation whose repositories will not render and cannot be repaired
// from the panel that exists to repair them.
//
// The value that made this look worth tightening was the runner, since one
// neither entry point matches takes both of them out. That is closed where the
// damage happens instead: Config.EffectiveRunner reads anything it does not
// recognise as the default, so no stored string can silence a repository.
func unmarshalPatch(content string) (config.Patch, error) {
	var patch config.Patch

	// The column is NOT NULL with an empty object for a default, so this is
	// only reachable from a row nothing here wrote - which is exactly the row
	// that must not take the page with it.
	if strings.TrimSpace(content) == "" {
		return patch, nil
	}

	if err := json.Unmarshal([]byte(content), &patch); err != nil {
		return config.Patch{}, fmt.Errorf("decode config patch: %w", err)
	}

	return patch, nil
}

// marshalPermissions encodes what an installation granted.
//
// An empty map is stored as an empty object rather than as null, so a reader
// gets "nothing was reported" from one shape only. Two spellings of absence is
// how a caller comes to handle one and not the other.
func marshalPermissions(permissions map[string]string) (string, error) {
	if len(permissions) == 0 {
		return "{}", nil
	}

	content, err := json.Marshal(permissions)
	if err != nil {
		return "", fmt.Errorf("encode installation permissions: %w", err)
	}

	return string(content), nil
}

// unmarshalPermissions reads them back.
//
// Lenient, for the reason unmarshalPatch is: a row that will not decode must
// not take the whole listing with it, and collectRows abandons a page on the
// first row it cannot scan. An unreadable column reads as nothing reported,
// which grants nothing - the same answer GitHub's own silence gets, since it
// marks the field required and its absence is a malformed listing rather than
// an installation that granted none.
func unmarshalPermissions(content string) map[string]string {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	var permissions map[string]string
	if err := json.Unmarshal([]byte(content), &permissions); err != nil {
		return nil
	}

	return permissions
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
