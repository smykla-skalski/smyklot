package sqlite

import (
	"database/sql"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

func scanPendingCI(scanner rowScanner) (pendingci.Request, error) {
	var request pendingci.Request
	var nextCheckAt, lastProgressAt, requestedAt, updatedAt string
	var leaseExpiresAt, finishedAt sql.NullString

	err := scanner.Scan(
		&request.ID,
		&request.TargetID,
		&request.InstallationID,
		&request.RepositoryID,
		&request.RepositoryFullName,
		&request.PullRequest,
		&request.HeadSHA,
		&request.BaseBranch,
		&request.MergeMethod,
		&request.RequiredChecksOnly,
		&request.Requester,
		&request.SourceCommentID,
		&request.SourceRevision,
		&request.Label,
		&request.Lifecycle,
		&request.Schedule,
		&nextCheckAt,
		&leaseExpiresAt,
		&lastProgressAt,
		&request.LastObservedState,
		&request.LastFingerprint,
		&request.LastEventKey,
		&request.Reason,
		&requestedAt,
		&updatedAt,
		&finishedAt,
		&request.Revision,
	)
	if err != nil {
		return pendingci.Request{}, err
	}

	request.NextCheckAt, err = parseTime(nextCheckAt)
	if err != nil {
		return pendingci.Request{}, err
	}
	request.LastProgressAt, err = parseTime(lastProgressAt)
	if err != nil {
		return pendingci.Request{}, err
	}
	request.RequestedAt, err = parseTime(requestedAt)
	if err != nil {
		return pendingci.Request{}, err
	}
	request.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return pendingci.Request{}, err
	}
	request.LeaseExpiresAt, err = parseNullableTime(leaseExpiresAt)
	if err != nil {
		return pendingci.Request{}, err
	}
	request.FinishedAt, err = parseNullableTime(finishedAt)
	if err != nil {
		return pendingci.Request{}, err
	}

	return request, nil
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}

func timePointer(value time.Time) *time.Time {
	return &value
}
