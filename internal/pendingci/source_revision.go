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

// CompareSourceEvents adds durable receipt order after GitHub's timestamp and
// action sequence. It is used only for revisions of the same mutable comment.
func CompareSourceEvents(
	leftRevision string,
	leftSequence int,
	leftOrder int64,
	rightRevision string,
	rightSequence int,
	rightOrder int64,
) (int, error) {
	comparison, err := CompareSourceRevisions(
		leftRevision, leftSequence, rightRevision, rightSequence,
	)
	if err != nil || comparison != 0 {
		return comparison, err
	}
	if leftOrder < rightOrder {
		return -1, nil
	}
	if leftOrder > rightOrder {
		return 1, nil
	}

	return 0, nil
}

// CompareSourceIntent orders commands by GitHub's source timestamp. GitHub
// timestamps have one-second precision, so distinct comments with the same
// timestamp cannot be ordered safely: one might be a later edit of an older
// comment. Revisions of one comment use durable receipt order after their live
// GitHub state has been verified.
func CompareSourceIntent(
	leftRevision string,
	leftCommentID int64,
	leftOrder int64,
	rightRevision string,
	rightCommentID int64,
	rightOrder int64,
) (int, error) {
	leftTime, err := ParseSourceRevision(leftRevision)
	if err != nil {
		return 0, err
	}
	rightTime, err := ParseSourceRevision(rightRevision)
	if err != nil {
		return 0, err
	}
	if leftTime.Before(rightTime) {
		return -1, nil
	}
	if leftTime.After(rightTime) {
		return 1, nil
	}
	if leftCommentID != rightCommentID {
		return 0, ErrAmbiguousSourceRevision
	}
	if leftOrder < rightOrder {
		return -1, nil
	}
	if leftOrder > rightOrder {
		return 1, nil
	}

	return 0, nil
}
