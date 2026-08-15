package sqlstore

import (
	"errors"
	"fmt"
	"time"
)

// errStoredTimeType reports a timestamp column an engine returned in a shape
// this adapter does not read.
var errStoredTimeType = errors.New("unsupported stored time")

// StoredTime receives a timestamp column from any engine.
//
// One engine has a real timestamp type and hands back a time.Time. Another
// keeps the value as text and hands back a string. Reading through this type
// means a query does not have to know which, and the parse error is raised
// where the row is read rather than carried to every caller.
type StoredTime struct {
	value time.Time
	valid bool
}

// Scan implements sql.Scanner.
func (s *StoredTime) Scan(src any) error {
	s.value, s.valid = time.Time{}, false

	switch value := src.(type) {
	case nil:
		return nil
	case time.Time:
		s.value, s.valid = value.UTC(), true

		return nil
	case string:
		return s.parse(value)
	case []byte:
		return s.parse(string(value))
	default:
		return fmt.Errorf("%w: %T", errStoredTimeType, src)
	}
}

func (s *StoredTime) parse(value string) error {
	// RFC3339Nano reads a fractional part of any width, so it accepts both the
	// fixed-width form written now and the variable-width form written before.
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fmt.Errorf("parse stored time: %w", err)
	}

	s.value, s.valid = parsed.UTC(), true

	return nil
}

// Time returns the scanned value, or the zero time for a NULL column.
func (s StoredTime) Time() time.Time { return s.value }

// Pointer returns the scanned value, or nil for a NULL column.
func (s StoredTime) Pointer() *time.Time {
	if !s.valid {
		return nil
	}

	value := s.value

	return &value
}

// Valid reports whether the column held a value.
func (s StoredTime) Valid() bool { return s.valid }
