package pendingci

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidRequest = errors.New("invalid pending CI request")

// ErrStaleSourceRevision means a delayed delivery no longer represents the
// latest authorized intent for its pull request.
var ErrStaleSourceRevision = errors.New("stale pending CI source revision")

// ErrAmbiguousSourceRevision means GitHub reported commands from different
// comments at the same timestamp precision. Their true order is unknowable,
// so choosing either merge intent would be unsafe.
var ErrAmbiguousSourceRevision = errors.New("ambiguous pending CI source revision")

func (request ArmRequest) Validate() error {
	if empty(request.TargetID, request.RepositoryID, request.RepositoryFullName) {
		return invalid("target and repository identity are required")
	}
	if request.InstallationID <= 0 || request.PullRequest <= 0 {
		return invalid("installation and pull request numbers must be positive")
	}
	if empty(request.HeadSHA, request.BaseBranch, request.Requester) {
		return invalid("head, base branch, and requester are required")
	}
	if request.SourceCommentID <= 0 || request.SourceSequence <= 0 || request.SourceOrder <= 0 ||
		empty(request.SourceRevision, request.Label) {
		return invalid("source comment, revision, and label are required")
	}
	if _, err := ParseSourceRevision(request.SourceRevision); err != nil {
		return err
	}
	if !request.MergeMethod.valid() {
		return invalid("unsupported merge method %q", request.MergeMethod)
	}
	if request.RequestedAt.IsZero() {
		return invalid("request time is required")
	}

	return nil
}

func (request SourceRevisionRequest) Validate() error {
	if strings.TrimSpace(request.RepositoryID) == "" || request.PullRequest <= 0 ||
		request.CommentID <= 0 || request.Sequence <= 0 || request.SourceOrder <= 0 {
		return invalid("source identity, sequence, and order are required")
	}
	if strings.TrimSpace(request.EventKey) == "" || request.ObservedAt.IsZero() {
		return invalid("source event identity and observation time are required")
	}
	if _, err := ParseSourceRevision(request.Revision); err != nil {
		return err
	}

	return nil
}

func (request LegacyDrainRequest) Validate() error {
	if empty(request.TargetID, request.RepositoryID, request.RepositoryFullName) {
		return invalid("target and repository identity are required")
	}
	if request.InstallationID <= 0 || request.PullRequest <= 0 {
		return invalid("installation and pull request numbers must be positive")
	}
	if empty(request.HeadSHA, request.BaseBranch) || len(request.Labels) == 0 {
		return invalid("head, base branch, and labels are required")
	}
	for _, label := range request.Labels {
		if !label.MergeMethod.valid() || strings.TrimSpace(label.Label) == "" {
			return invalid("invalid legacy pending CI label")
		}
	}
	if request.DrainedAt.IsZero() {
		return invalid("drain time is required")
	}

	return nil
}

func (request WakeRequest) Validate() error {
	if strings.TrimSpace(request.RepositoryID) == "" || request.PullRequest <= 0 {
		return invalid("repository identity and pull request number are required")
	}
	if empty(request.EventName, request.EventKey, request.DeliveryID) || request.OccurredAt.IsZero() {
		return invalid("event identity and occurrence time are required")
	}

	return nil
}

func (request WakeHeadRequest) Validate() error {
	if empty(
		request.RepositoryID, request.HeadSHA, request.EventName,
		request.EventKey, request.DeliveryID,
	) || request.OccurredAt.IsZero() {
		return invalid("repository, head SHA, event identity, and occurrence time are required")
	}

	return nil
}

func (request CheckNowRequest) Validate() error {
	if request.ID <= 0 || request.ExpectedRevision <= 0 {
		return invalid("request identity and revision must be positive")
	}
	if strings.TrimSpace(request.EventKey) == "" || request.OccurredAt.IsZero() {
		return invalid("event identity and occurrence time are required")
	}

	return nil
}

func (request ClaimMergeRequest) Validate() error {
	if request.ID <= 0 || request.ExpectedRevision <= 0 {
		return invalid("request identity and revision must be positive")
	}
	if request.ClaimedAt.IsZero() {
		return invalid("merge claim time is required")
	}

	return nil
}

func (request RetuneQuietPeriodRequest) Validate() error {
	if request.PassingQuiet < MinPassingQuiet || request.PassingQuiet > MaxPassingQuiet {
		return invalid(
			"passing quiet period must be between %s and %s",
			MinPassingQuiet,
			MaxPassingQuiet,
		)
	}
	if request.ChangedAt.IsZero() {
		return invalid("quiet period change time is required")
	}

	return nil
}

func (request RescheduleRequest) Validate() error {
	if request.ID <= 0 || request.ExpectedRevision <= 0 {
		return invalid("request identity and revision must be positive")
	}
	if !request.Schedule.valid() {
		return invalid("unsupported schedule %q", request.Schedule)
	}
	if strings.TrimSpace(request.HeadSHA) == "" {
		return invalid("observed head SHA is required")
	}
	if request.NextCheckAt.IsZero() || request.LastProgressAt.IsZero() || request.CheckedAt.IsZero() {
		return invalid("next check, last progress, and checked times are required")
	}
	if strings.TrimSpace(request.LastObservedState) == "" {
		return invalid("last observed state is required")
	}
	if request.NextCheckTrigger != "" && !request.NextCheckTrigger.valid() {
		return invalid("unsupported next check trigger %q", request.NextCheckTrigger)
	}

	return nil
}

func (request FinishRequest) Validate() error {
	if request.ID <= 0 || request.ExpectedRevision <= 0 {
		return invalid("request identity and revision must be positive")
	}
	if request.Lifecycle != LifecycleMerged && request.Lifecycle != LifecycleCancelled {
		return invalid("finish lifecycle must be merged or cancelled")
	}
	if strings.TrimSpace(request.Reason) == "" || request.FinishedAt.IsZero() {
		return invalid("finish reason and time are required")
	}

	return nil
}

func (request CancelRequest) Validate() error {
	if strings.TrimSpace(request.RepositoryID) == "" || request.PullRequest <= 0 ||
		request.CommentID <= 0 || request.SourceSequence <= 0 || request.SourceOrder <= 0 {
		return invalid("repository, pull request, and source comment are required")
	}
	if _, err := ParseSourceRevision(request.SourceRevision); err != nil {
		return err
	}
	if !request.Trigger.valid() {
		return invalid("unsupported source cancellation trigger %q", request.Trigger)
	}
	if strings.TrimSpace(request.Reason) == "" || request.CancelledAt.IsZero() {
		return invalid("cancellation reason and time are required")
	}

	return nil
}

func (request CancelIntentRequest) Validate() error {
	if strings.TrimSpace(request.RepositoryID) == "" || request.PullRequest <= 0 ||
		request.CommentID <= 0 || request.SourceSequence <= 0 || request.SourceOrder <= 0 {
		return invalid("repository, pull request, and cancellation source are required")
	}
	if _, err := ParseSourceRevision(request.SourceRevision); err != nil {
		return err
	}
	if strings.TrimSpace(request.Reason) == "" || request.CancelledAt.IsZero() {
		return invalid("cancellation reason and time are required")
	}

	return nil
}

func (request FinishPRRequest) Validate() error {
	if strings.TrimSpace(request.RepositoryID) == "" || request.PullRequest <= 0 {
		return invalid("repository identity and pull request number are required")
	}
	if request.Lifecycle != LifecycleMerged && request.Lifecycle != LifecycleCancelled {
		return invalid("pull request finish lifecycle must be merged or cancelled")
	}
	if !request.Trigger.valid() {
		return invalid("unsupported pull request finish trigger %q", request.Trigger)
	}
	if strings.TrimSpace(request.Reason) == "" || request.FinishedAt.IsZero() {
		return invalid("pull request finish reason and time are required")
	}

	return nil
}

func (request CancelRepositoryRequest) Validate() error {
	if strings.TrimSpace(request.RepositoryID) == "" {
		return invalid("repository identity is required")
	}
	if strings.TrimSpace(request.Reason) == "" || request.CancelledAt.IsZero() {
		return invalid("repository cancellation reason and time are required")
	}

	return nil
}

func (filter CleanupFilter) Validate() error {
	if strings.TrimSpace(filter.RepositoryID) == "" {
		return invalid("cleanup repository identity is required")
	}
	if filter.PullRequest < 0 || filter.ExcludeID < 0 {
		return invalid("cleanup scope cannot be negative")
	}

	return nil
}

func (filter HistoryFilter) Validate() error {
	if filter.Limit < 0 || filter.Limit > 200 {
		return invalid("history limit must be between 0 and 200")
	}

	return nil
}

func (filter EventFilter) Validate() error {
	if filter.RequestID <= 0 {
		return invalid("event request identity must be positive")
	}
	if filter.Limit < 0 || filter.Limit > 500 {
		return invalid("event limit must be between 0 and 500")
	}

	return nil
}

func (request MarkCleanupArtifactsDoneRequest) Validate() error {
	if request.ID <= 0 || request.ExpectedRevision <= 0 {
		return invalid("cleanup identity and revision must be positive")
	}
	if request.MarkedAt.IsZero() {
		return invalid("cleanup artifact completion time is required")
	}

	return nil
}

func (request CompleteCleanupRequest) Validate() error {
	if request.ID <= 0 || request.ExpectedRevision <= 0 {
		return invalid("cleanup identity and revision must be positive")
	}
	if request.CompletedAt.IsZero() {
		return invalid("cleanup completion time is required")
	}

	return nil
}

func (request RetryCleanupRequest) Validate() error {
	if request.ID <= 0 || request.ExpectedRevision <= 0 {
		return invalid("cleanup identity and revision must be positive")
	}
	if request.NextAttemptAt.IsZero() || request.FailedAt.IsZero() {
		return invalid("cleanup retry and failure times are required")
	}
	if strings.TrimSpace(request.Error) == "" {
		return invalid("cleanup failure is required")
	}

	return nil
}

func (filter QueueFilter) Validate() error {
	if filter.Schedule != nil && !filter.Schedule.valid() {
		return invalid("unsupported schedule %q", *filter.Schedule)
	}

	return nil
}

func (method MergeMethod) valid() bool {
	return method == MergeMethodMerge || method == MergeMethodSquash || method == MergeMethodRebase
}

func (schedule Schedule) valid() bool {
	return schedule == ScheduleActive || schedule == ScheduleDeferred
}

func empty(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}

	return false
}

func invalid(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, arguments...))
}
