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
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

const scheduleRequestColumns = `
    id, target_id, kind, state, base_revision, base_target_id, profile_id, custom_profile,
    cadence_seconds, default_priority, configuration, reason, requested_by,
    reviewed_by, decision_reason, promoted_profile_id, revision,
    created_at, updated_at, reviewed_at`

func (s *Store) ListScheduleRequests(
	ctx context.Context,
	targetID *string,
) ([]workqueue.ScheduleRequest, error) {
	query := "SELECT" + scheduleRequestColumns + " FROM schedule_requests"
	arguments := []any{}
	if targetID != nil {
		query += " WHERE target_id = ?"
		arguments = append(arguments, *targetID)
	}
	query += " ORDER BY CASE state WHEN 'pending' THEN 0 ELSE 1 END, created_at DESC, id"
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list schedule requests: %w", err)
	}
	requests, err := collectRows(rows, scanScheduleRequest)
	if err != nil {
		return nil, fmt.Errorf("read schedule requests: %w", err)
	}

	return requests, nil
}

func scanScheduleRequest(scanner rowScanner) (workqueue.ScheduleRequest, error) {
	var request workqueue.ScheduleRequest
	var baseTargetID, profileID, custom, reviewedBy, promoted sql.NullString
	var configuration string
	var cadence int64
	var created, updated, reviewed StoredTime
	if err := scanner.Scan(
		&request.ID, &request.TargetID, &request.Kind, &request.State,
		&request.BaseRevision, &baseTargetID, &profileID, &custom, &cadence,
		&request.DefaultPriority, &configuration, &request.Reason,
		&request.RequestedBy, &reviewedBy, &request.DecisionReason, &promoted,
		&request.Revision, &created, &updated, &reviewed,
	); err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	request.ProfileID = stringPointer(profileID)
	request.BaseTargetID = stringPointer(baseTargetID)
	request.Configuration = json.RawMessage(configuration)
	request.ReviewedBy = stringPointer(reviewedBy)
	request.PromotedProfileID = stringPointer(promoted)
	request.Cadence = secondsDuration(cadence)
	request.CreatedAt, request.UpdatedAt = created.Time(), updated.Time()
	request.ReviewedAt = reviewed.Pointer()
	if custom.Valid {
		var profile workqueue.Profile
		if err := json.Unmarshal([]byte(custom.String), &profile); err != nil {
			return workqueue.ScheduleRequest{}, fmt.Errorf("decode custom schedule profile: %w", err)
		}
		request.CustomProfile = &profile
	}

	return request, nil
}

func (s *Store) CreateScheduleRequest(
	ctx context.Context,
	create workqueue.ScheduleRequestCreate,
) (workqueue.ScheduleRequest, error) {
	if err := validateScheduleRequestCreate(create); err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.ScheduleRequest{}, fmt.Errorf("begin schedule request: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := getEffectiveQueuePolicy(ctx, tx, create.Kind, &create.TargetID)
	if err != nil {
		return workqueue.ScheduleRequest{}, noRows(err)
	}
	if current.Revision != create.BaseRevision {
		return workqueue.ScheduleRequest{}, storage.ErrConflict
	}
	if err := s.validateRequestedProfile(ctx, tx, create); err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	request, custom, err := newScheduleRequest(create, current.TargetID)
	if err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	if err := insertScheduleRequest(ctx, tx, request, custom); err != nil {
		if s.dialect.UniqueViolation(err) {
			return workqueue.ScheduleRequest{}, storage.ErrConflict
		}
		return workqueue.ScheduleRequest{}, err
	}
	if err := insertScheduleRequestQueueItem(ctx, tx, request); err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return workqueue.ScheduleRequest{}, fmt.Errorf("commit schedule request: %w", err)
	}

	return request, nil
}

func validateScheduleRequestCreate(create workqueue.ScheduleRequestCreate) error {
	if strings.TrimSpace(create.ID) == "" || strings.TrimSpace(create.TargetID) == "" ||
		strings.TrimSpace(create.Reason) == "" || strings.TrimSpace(create.RequestedBy) == "" {
		return errors.New("schedule request identity, scope, requester, and reason are required")
	}
	if !create.Kind.InstallationConfigurable() ||
		!create.DefaultPriority.Valid() || create.Cadence < 0 ||
		(create.Kind.Recurring() && create.Cadence <= 0) {
		return errors.New("schedule request policy is invalid")
	}
	if (create.ProfileID == nil) == (create.CustomProfile == nil) {
		return errors.New("schedule request needs one existing or custom profile")
	}
	return workqueue.ValidatePolicyConfiguration(create.Kind, create.Configuration)
}

func (s *Store) validateRequestedProfile(
	ctx context.Context,
	tx *transaction,
	create workqueue.ScheduleRequestCreate,
) error {
	if create.ProfileID != nil {
		profile, err := getScheduleProfile(ctx, tx, *create.ProfileID)
		if err != nil {
			return noRows(err)
		}

		return validateProfileScope(profile, &create.TargetID)
	}
	profile := *create.CustomProfile
	if strings.TrimSpace(profile.ID) == "" {
		profile.ID = "proposed:" + create.ID
	}
	profile.TargetID = &create.TargetID

	return workqueue.ValidateProfile(profile)
}

func newScheduleRequest(
	create workqueue.ScheduleRequestCreate,
	baseTargetID *string,
) (workqueue.ScheduleRequest, []byte, error) {
	configuration := create.Configuration
	if len(configuration) == 0 {
		configuration = json.RawMessage(`{}`)
	}
	var custom []byte
	var err error
	if create.CustomProfile != nil {
		custom, err = json.Marshal(create.CustomProfile)
		if err != nil {
			return workqueue.ScheduleRequest{}, nil, fmt.Errorf("encode custom schedule profile: %w", err)
		}
	}
	request := workqueue.ScheduleRequest{
		ID: create.ID, TargetID: create.TargetID, Kind: create.Kind,
		State: workqueue.RequestPending, BaseRevision: create.BaseRevision,
		BaseTargetID: baseTargetID,
		ProfileID:    create.ProfileID, CustomProfile: create.CustomProfile,
		Cadence: create.Cadence, DefaultPriority: create.DefaultPriority,
		Configuration: configuration, Reason: create.Reason, RequestedBy: create.RequestedBy,
		Revision: 1, CreatedAt: create.CreatedAt, UpdatedAt: create.CreatedAt,
	}

	return request, custom, nil
}

func insertScheduleRequest(
	ctx context.Context,
	tx *transaction,
	request workqueue.ScheduleRequest,
	custom []byte,
) error {
	var customValue any
	if len(custom) > 0 {
		customValue = string(custom)
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO schedule_requests (`+scheduleRequestColumns+`)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		request.ID, request.TargetID, request.Kind, request.State, request.BaseRevision,
		request.BaseTargetID, request.ProfileID, customValue, int64(request.Cadence/time.Second),
		request.DefaultPriority, string(request.Configuration), request.Reason,
		request.RequestedBy, nil, "", nil, request.Revision,
		request.CreatedAt, request.UpdatedAt, nil,
	)
	if err != nil {
		return fmt.Errorf("insert schedule request: %w", err)
	}

	return nil
}

func insertScheduleRequestQueueItem(
	ctx context.Context,
	tx *transaction,
	request workqueue.ScheduleRequest,
) error {
	requestedBy := request.RequestedBy
	profileID := workqueue.AlwaysOpenProfileID
	details, _ := json.Marshal(map[string]string{"policy_kind": string(request.Kind)})
	item := workqueue.Item{
		ID: "schedule-request:" + request.ID, Kind: workqueue.KindScheduleChange,
		Lane: workqueue.LaneMaintenance, TargetID: &request.TargetID,
		SourceKind: "schedule_request", SourceID: request.ID,
		Title:   "Schedule change for " + strings.ReplaceAll(string(request.Kind), "_", " "),
		Summary: request.Reason, State: workqueue.StateAwaitingApproval,
		Priority: workqueue.PriorityNormal, WindowMode: workqueue.WindowRespect,
		ProfileID: &profileID, NotBefore: request.CreatedAt, EligibleAt: request.CreatedAt,
		RequestedBy: &requestedBy, Details: details, Revision: 1,
		CreatedAt: request.CreatedAt, UpdatedAt: request.CreatedAt,
	}
	if err := insertQueueItem(ctx, tx, item); err != nil {
		return err
	}

	return insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: item.ID, ActorID: &requestedBy, Kind: "approval.requested",
		State: item.State, Summary: "Root approval requested", CreatedAt: request.CreatedAt,
	})
}

func (s *Store) DecideScheduleRequest(
	ctx context.Context,
	id string,
	decision workqueue.ScheduleDecision,
) (workqueue.ScheduleRequest, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workqueue.ScheduleRequest{}, fmt.Errorf("begin schedule decision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	request, err := getScheduleRequest(ctx, tx, id, s.dialect.RowLock())
	if err != nil {
		return workqueue.ScheduleRequest{}, noRows(err)
	}
	if request.State != workqueue.RequestPending || request.Revision != decision.ExpectedRevision {
		return workqueue.ScheduleRequest{}, storage.ErrConflict
	}
	current, err := getEffectiveQueuePolicy(ctx, tx, request.Kind, &request.TargetID)
	if err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	if current.Revision != request.BaseRevision ||
		!sameOptionalString(current.TargetID, request.BaseTargetID) {
		return s.markScheduleRequestStale(ctx, tx, request, decision)
	}
	if decision.Approve {
		request, err = s.approveScheduleRequest(ctx, tx, request, current, decision)
	} else {
		request, err = rejectScheduleRequest(ctx, tx, request, decision)
	}
	if err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return workqueue.ScheduleRequest{}, fmt.Errorf("commit schedule decision: %w", err)
	}

	return request, nil
}

func getScheduleRequest(
	ctx context.Context,
	runner runner,
	id, lock string,
) (workqueue.ScheduleRequest, error) {
	return scanScheduleRequest(runner.QueryRowContext(ctx,
		"SELECT"+scheduleRequestColumns+" FROM schedule_requests WHERE id = ?"+lock, id,
	))
}

func (s *Store) GetScheduleRequest(
	ctx context.Context,
	id string,
) (workqueue.ScheduleRequest, error) {
	request, err := getScheduleRequest(ctx, s.db, id, "")
	if err != nil {
		return workqueue.ScheduleRequest{}, noRows(err)
	}

	return request, nil
}

func (s *Store) approveScheduleRequest(
	ctx context.Context,
	tx *transaction,
	request workqueue.ScheduleRequest,
	current workqueue.Policy,
	decision workqueue.ScheduleDecision,
) (workqueue.ScheduleRequest, error) {
	profileID, err := s.approvedRequestProfile(ctx, tx, request, decision)
	if err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	exact, err := getQueuePolicyScope(ctx, tx, request.Kind, &request.TargetID)
	expected := int64(0)
	if err == nil {
		expected = exact.Revision
	} else if !errors.Is(err, sql.ErrNoRows) {
		return workqueue.ScheduleRequest{}, err
	}
	change := workqueue.PolicyChange{
		Kind: request.Kind, TargetID: &request.TargetID, Enabled: current.Enabled,
		Cadence: request.Cadence, ProfileID: profileID,
		DefaultPriority: request.DefaultPriority, RetryDelay: current.RetryDelay,
		Retention: current.Retention, ApprovalTTL: current.ApprovalTTL,
		Configuration: request.Configuration, ExpectedRevision: expected,
		ActorID: decision.ReviewerID, ChangedAt: decision.ReviewedAt,
	}
	if err := saveQueuePolicy(ctx, tx, change); err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	policy, err := getEffectiveQueuePolicy(ctx, tx, request.Kind, &request.TargetID)
	if err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	profile, err := getScheduleProfile(ctx, tx, profileID)
	if err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	if err := s.reschedulePolicyItems(
		ctx, tx, policy, profile, decision.ReviewedAt,
		decision.ReviewerID, "Approved installation schedule request applied",
	); err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	request.State, request.ReviewedBy = workqueue.RequestApproved, &decision.ReviewerID
	request.DecisionReason, request.ReviewedAt = decision.DecisionReason, &decision.ReviewedAt
	request.PromotedProfileID = &profileID
	request.Revision++
	request.UpdatedAt = decision.ReviewedAt
	if err := updateScheduleRequestDecision(ctx, tx, request); err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	if err := finishScheduleRequestQueueItem(
		ctx, tx, request, workqueue.StateSucceeded, "Schedule change approved", decision.ReviewerID,
	); err != nil {
		return workqueue.ScheduleRequest{}, err
	}

	return request, nil
}

func (s *Store) approvedRequestProfile(
	ctx context.Context,
	tx *transaction,
	request workqueue.ScheduleRequest,
	decision workqueue.ScheduleDecision,
) (string, error) {
	if request.ProfileID != nil {
		return *request.ProfileID, nil
	}
	if request.CustomProfile == nil {
		return "", errors.New("schedule request has no profile")
	}
	profile := *request.CustomProfile
	profile.ID = "profile:" + request.ID
	if decision.ProfileID != nil && strings.TrimSpace(*decision.ProfileID) != "" {
		profile.ID = *decision.ProfileID
	}
	if decision.PromoteProfile {
		profile.TargetID = nil
	} else {
		profile.TargetID = &request.TargetID
	}
	if err := workqueue.ValidateProfile(profile); err != nil {
		return "", err
	}
	if err := insertScheduleProfile(ctx, tx, profile, decision.ReviewerID, decision.ReviewedAt); err != nil {
		return "", err
	}
	if err := replaceProfileRules(ctx, tx, profile); err != nil {
		return "", err
	}

	return profile.ID, nil
}

func getQueuePolicyScope(
	ctx context.Context,
	runner runner,
	kind workqueue.Kind,
	targetID *string,
) (workqueue.Policy, error) {
	if targetID == nil {
		return scanQueuePolicy(runner.QueryRowContext(ctx, "SELECT"+queuePolicyColumns+`
FROM queue_policies WHERE kind = ? AND target_id IS NULL`, kind))
	}

	return scanQueuePolicy(runner.QueryRowContext(ctx, "SELECT"+queuePolicyColumns+`
FROM queue_policies WHERE kind = ? AND target_id = ?`, kind, *targetID))
}

func rejectScheduleRequest(
	ctx context.Context,
	tx *transaction,
	request workqueue.ScheduleRequest,
	decision workqueue.ScheduleDecision,
) (workqueue.ScheduleRequest, error) {
	request.State, request.ReviewedBy = workqueue.RequestRejected, &decision.ReviewerID
	request.DecisionReason, request.ReviewedAt = decision.DecisionReason, &decision.ReviewedAt
	request.Revision++
	request.UpdatedAt = decision.ReviewedAt
	if err := updateScheduleRequestDecision(ctx, tx, request); err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	if err := finishScheduleRequestQueueItem(
		ctx, tx, request, workqueue.StateCancelled, "Schedule change rejected", decision.ReviewerID,
	); err != nil {
		return workqueue.ScheduleRequest{}, err
	}

	return request, nil
}

func updateScheduleRequestDecision(
	ctx context.Context,
	tx *transaction,
	request workqueue.ScheduleRequest,
) error {
	_, err := tx.ExecContext(ctx, `
UPDATE schedule_requests SET
    state = ?, reviewed_by = ?, decision_reason = ?, promoted_profile_id = ?,
    revision = ?, updated_at = ?, reviewed_at = ? WHERE id = ?`,
		request.State, request.ReviewedBy, request.DecisionReason,
		request.PromotedProfileID, request.Revision, request.UpdatedAt,
		request.ReviewedAt, request.ID,
	)
	if err != nil {
		return fmt.Errorf("update schedule request decision: %w", err)
	}

	return nil
}

func (s *Store) markScheduleRequestStale(
	ctx context.Context,
	tx *transaction,
	request workqueue.ScheduleRequest,
	decision workqueue.ScheduleDecision,
) (workqueue.ScheduleRequest, error) {
	request.State, request.ReviewedBy = workqueue.RequestStale, &decision.ReviewerID
	request.DecisionReason = "The effective policy changed after this request was submitted"
	request.ReviewedAt, request.UpdatedAt = &decision.ReviewedAt, decision.ReviewedAt
	request.Revision++
	if err := updateScheduleRequestDecision(ctx, tx, request); err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	if err := finishScheduleRequestQueueItem(
		ctx, tx, request, workqueue.StateSuperseded, "Schedule request became stale", decision.ReviewerID,
	); err != nil {
		return workqueue.ScheduleRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return workqueue.ScheduleRequest{}, fmt.Errorf("commit stale schedule request: %w", err)
	}

	return request, nil
}

func finishScheduleRequestQueueItem(
	ctx context.Context,
	tx *transaction,
	request workqueue.ScheduleRequest,
	state workqueue.State,
	summary, actorID string,
) error {
	itemID := "schedule-request:" + request.ID
	if _, err := tx.ExecContext(ctx, `
UPDATE queue_items SET state = ?, finished_at = ?, updated_at = ?,
    revision = revision + 1 WHERE id = ?`,
		state, request.UpdatedAt, request.UpdatedAt, itemID,
	); err != nil {
		return fmt.Errorf("finish schedule request queue item: %w", err)
	}

	return insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: itemID, ActorID: &actorID, Kind: "approval.decided",
		State: state, Summary: summary, CreatedAt: request.UpdatedAt,
	})
}
