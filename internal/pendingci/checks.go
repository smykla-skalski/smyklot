package pendingci

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrSharedHead = errors.New("multiple pull requests share a pending CI head")

const checkStatusCompleted = "completed"

type CheckSlotState string

const (
	CheckSlotProvisioning CheckSlotState = "provisioning"
	CheckSlotReady        CheckSlotState = "ready"
	CheckSlotBlocked      CheckSlotState = "blocked"
)

type CheckAction struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Identifier  string `json:"identifier"`
}

type CheckSlot struct {
	ID                 int64
	TargetID           string
	InstallationID     int64
	RepositoryID       string
	RepositoryFullName string
	PullRequest        int
	HeadSHA            string
	AppID              int64
	Name               string
	ExternalID         string
	Generation         int64
	CheckRunID         *int64
	CheckURL           string
	State              CheckSlotState
	DesiredStatus      string
	DesiredConclusion  string
	DesiredTitle       string
	DesiredSummary     string
	DesiredActions     []CheckAction
	DesiredDigest      string
	AppliedDigest      string
	RetryAt            *time.Time
	LastError          string
	UpdatedAt          time.Time
	Revision           int64
}

type EnsureCheckSlotRequest struct {
	TargetID           string
	InstallationID     int64
	RepositoryID       string
	RepositoryFullName string
	PullRequest        int
	HeadSHA            string
	AppID              int64
	Name               string
	ExternalID         string
	DesiredStatus      string
	DesiredConclusion  string
	DesiredTitle       string
	DesiredSummary     string
	DesiredActions     []CheckAction
	DesiredDigest      string
	ChangedAt          time.Time
}

func (request EnsureCheckSlotRequest) Validate() error {
	if request.InstallationID <= 0 || request.PullRequest <= 0 || request.AppID <= 0 {
		return invalid("check slot installation, pull request, and app IDs must be positive")
	}
	if empty(
		request.TargetID, request.RepositoryID, request.RepositoryFullName,
		request.HeadSHA, request.Name, request.ExternalID, request.DesiredStatus,
		request.DesiredTitle, request.DesiredSummary, request.DesiredDigest,
	) {
		return invalid("check slot identity and desired output are required")
	}
	if request.DesiredStatus != "queued" && request.DesiredStatus != "in_progress" &&
		request.DesiredStatus != checkStatusCompleted {
		return invalid("unsupported check status %q", request.DesiredStatus)
	}
	if (request.DesiredStatus == checkStatusCompleted) != (request.DesiredConclusion != "") {
		return invalid("only completed checks require a conclusion")
	}
	for _, action := range request.DesiredActions {
		if strings.TrimSpace(action.Label) == "" ||
			strings.TrimSpace(action.Description) == "" ||
			strings.TrimSpace(action.Identifier) == "" {
			return invalid("check actions require a label, description, and identifier")
		}
		if utf8.RuneCountInString(action.Label) > 20 ||
			utf8.RuneCountInString(action.Description) > 40 ||
			utf8.RuneCountInString(action.Identifier) > 20 {
			return invalid("check action exceeds GitHub field limits")
		}
	}
	if request.ChangedAt.IsZero() {
		return invalid("check slot change time is required")
	}

	return nil
}

type BindCheckRunRequest struct {
	ID               int64
	ExpectedRevision int64
	CheckRunID       int64
	CheckURL         string
	BoundAt          time.Time
}

func (request BindCheckRunRequest) Validate() error {
	if err := validateCheckTransition(request.ID, request.ExpectedRevision, request.BoundAt); err != nil {
		return err
	}
	if request.CheckRunID <= 0 {
		return invalid("bound check run ID must be positive")
	}

	return nil
}

type ApplyCheckSlotRequest struct {
	ID               int64
	ExpectedRevision int64
	AppliedDigest    string
	CheckRunID       int64
	CheckURL         string
	AppliedAt        time.Time
}

func (request ApplyCheckSlotRequest) Validate() error {
	if err := validateCheckTransition(request.ID, request.ExpectedRevision, request.AppliedAt); err != nil {
		return err
	}
	if request.CheckRunID <= 0 || strings.TrimSpace(request.AppliedDigest) == "" {
		return invalid("applied check run ID and digest are required")
	}

	return nil
}

type RetryCheckSlotRequest struct {
	ID               int64
	ExpectedRevision int64
	RetryAt          time.Time
	Error            string
	FailedAt         time.Time
}

type RenewCheckSlotRequest struct {
	ID               int64
	ExpectedRevision int64
	ExternalID       string
	RenewedAt        time.Time
}

type ReassignCheckSlotRequest struct {
	ID               int64
	ExpectedRevision int64
	PullRequest      int
	ReassignedAt     time.Time
}

type RefreshCheckSlotRequest struct {
	ID               int64
	ExpectedRevision int64
	RefreshedAt      time.Time
}

func (request RefreshCheckSlotRequest) Validate() error {
	return validateCheckTransition(request.ID, request.ExpectedRevision, request.RefreshedAt)
}

func (request ReassignCheckSlotRequest) Validate() error {
	if err := validateCheckTransition(
		request.ID, request.ExpectedRevision, request.ReassignedAt,
	); err != nil {
		return err
	}
	if request.PullRequest <= 0 {
		return invalid("reassigned check pull request must be positive")
	}

	return nil
}

func (request RenewCheckSlotRequest) Validate() error {
	if err := validateCheckTransition(request.ID, request.ExpectedRevision, request.RenewedAt); err != nil {
		return err
	}
	if strings.TrimSpace(request.ExternalID) == "" {
		return invalid("renewed check external ID is required")
	}

	return nil
}

func (request RetryCheckSlotRequest) Validate() error {
	if err := validateCheckTransition(request.ID, request.ExpectedRevision, request.FailedAt); err != nil {
		return err
	}
	if request.RetryAt.IsZero() || strings.TrimSpace(request.Error) == "" {
		return invalid("check retry time and error are required")
	}

	return nil
}

func validateCheckTransition(id, revision int64, at time.Time) error {
	if id <= 0 || revision <= 0 || at.IsZero() {
		return fmt.Errorf("%w: check slot identity, revision, and time are required", ErrInvalidRequest)
	}

	return nil
}

type CheckStore interface {
	EnsureCheckSlot(context.Context, EnsureCheckSlotRequest) (CheckSlot, error)
	GetCheckSlot(context.Context, int64) (CheckSlot, error)
	GetCheckSlotByHead(context.Context, string, string) (CheckSlot, error)
	ListPendingCheckSlots(context.Context, time.Time, int) ([]CheckSlot, error)
	BindCheckRun(context.Context, BindCheckRunRequest) (CheckSlot, error)
	ApplyCheckSlot(context.Context, ApplyCheckSlotRequest) (CheckSlot, error)
	RetryCheckSlot(context.Context, RetryCheckSlotRequest) (CheckSlot, error)
	RenewCheckSlot(context.Context, RenewCheckSlotRequest) (CheckSlot, error)
	ReassignCheckSlot(context.Context, ReassignCheckSlotRequest) (CheckSlot, error)
	RefreshCheckSlot(context.Context, RefreshCheckSlotRequest) (CheckSlot, error)
}
