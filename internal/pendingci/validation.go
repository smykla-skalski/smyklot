package pendingci

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidRequest = errors.New("invalid pending CI request")

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
	if request.SourceCommentID <= 0 || empty(request.SourceRevision, request.Label) {
		return invalid("source comment, revision, and label are required")
	}
	if !request.MergeMethod.valid() {
		return invalid("unsupported merge method %q", request.MergeMethod)
	}
	if request.RequestedAt.IsZero() {
		return invalid("request time is required")
	}

	return nil
}

func (request WakeRequest) Validate() error {
	if strings.TrimSpace(request.RepositoryID) == "" || request.PullRequest <= 0 {
		return invalid("repository identity and pull request number are required")
	}
	if strings.TrimSpace(request.EventKey) == "" || request.OccurredAt.IsZero() {
		return invalid("event identity and occurrence time are required")
	}

	return nil
}

func (request WakeHeadRequest) Validate() error {
	if empty(request.RepositoryID, request.HeadSHA, request.EventKey) || request.OccurredAt.IsZero() {
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
	if strings.TrimSpace(request.RepositoryID) == "" || request.PullRequest <= 0 || request.CommentID <= 0 {
		return invalid("repository, pull request, and source comment are required")
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
	if strings.TrimSpace(request.Reason) == "" || request.FinishedAt.IsZero() {
		return invalid("pull request finish reason and time are required")
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
