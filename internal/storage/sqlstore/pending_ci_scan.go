package sqlstore

import (
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

func scanPendingCI(scanner rowScanner) (pendingci.Request, error) {
	var request pendingci.Request
	var nextCheckAt, leaseExpiresAt, lastProgressAt StoredTime
	var requestedAt, updatedAt, finishedAt StoredTime

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
		&request.SourceSequence,
		&request.SourceOrder,
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
		&request.CleanupPending,
		&request.CleanupArtifactsDone,
		&request.CleanupAttempts,
		&request.CleanupError,
		&request.Revision,
	)
	if err != nil {
		return pendingci.Request{}, err
	}

	request.NextCheckAt = nextCheckAt.Time()
	request.LastProgressAt = lastProgressAt.Time()
	request.RequestedAt = requestedAt.Time()
	request.UpdatedAt = updatedAt.Time()
	request.LeaseExpiresAt = leaseExpiresAt.Pointer()
	request.FinishedAt = finishedAt.Pointer()

	return request, nil
}

func timePointer(value time.Time) *time.Time {
	return &value
}
