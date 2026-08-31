package sqlstore

import (
	"database/sql"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

func scanPendingCI(scanner rowScanner) (pendingci.Request, error) {
	var request pendingci.Request
	var nextCheckAt, leaseExpiresAt, lastProgressAt StoredTime
	var requestedAt, updatedAt, finishedAt StoredTime
	var label, candidateHead, candidateBase sql.NullString
	var checkSlotID, retiredCheckSlotID sql.NullInt64
	var authorizedAt StoredTime

	err := scanner.Scan(
		&request.ID,
		&request.TargetID,
		&request.InstallationID,
		&request.RepositoryID,
		&request.RepositoryFullName,
		&request.PullRequest,
		&request.PullRequestTitle,
		&request.HeadSHA,
		&request.BaseBranch,
		&request.MergeMethod,
		&request.RequiredChecksOnly,
		&request.Requester,
		&request.SourceCommentID,
		&request.SourceRevision,
		&request.SourceSequence,
		&request.SourceOrder,
		&request.ArtifactKind,
		&label,
		&checkSlotID,
		&retiredCheckSlotID,
		&request.AuthorizationState,
		&request.GateState,
		&candidateHead,
		&candidateBase,
		&request.AuthorizedBy,
		&authorizedAt,
		&request.MergePhase,
		&request.Lifecycle,
		&request.Schedule,
		&request.NextCheckTrigger,
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
	request.Label = label.String
	if checkSlotID.Valid {
		request.CheckSlotID = &checkSlotID.Int64
	}
	if retiredCheckSlotID.Valid {
		request.RetiredCheckSlotID = &retiredCheckSlotID.Int64
	}
	request.CandidateHeadSHA = candidateHead.String
	request.CandidateBaseBranch = candidateBase.String
	request.AuthorizedAt = authorizedAt.Time()

	return request, nil
}

func timePointer(value time.Time) *time.Time {
	return &value
}
