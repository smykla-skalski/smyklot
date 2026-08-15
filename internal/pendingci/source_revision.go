package pendingci

import (
	"fmt"
	"strings"
	"time"
)

// ParseSourceRevision parses the GitHub timestamp that versions a mutable
// issue comment. Keeping this in the domain makes ordering independent from
// both webhook decoding and SQLite's textual sort rules.
func ParseSourceRevision(revision string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(revision))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid source revision: %w", ErrInvalidRequest, err)
	}

	return parsed.UTC(), nil
}

// CompareSourceRevisions compares timestamp first and event sequence second.
func CompareSourceRevisions(
	left string,
	leftSequence int,
	right string,
	rightSequence int,
) (int, error) {
	leftTime, err := ParseSourceRevision(left)
	if err != nil {
		return 0, err
	}
	rightTime, err := ParseSourceRevision(right)
	if err != nil {
		return 0, err
	}
	if leftTime.Before(rightTime) {
		return -1, nil
	}
	if leftTime.After(rightTime) {
		return 1, nil
	}
	if leftSequence < rightSequence {
		return -1, nil
	}
	if leftSequence > rightSequence {
		return 1, nil
	}

	return 0, nil
}
