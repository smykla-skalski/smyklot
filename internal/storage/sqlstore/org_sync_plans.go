package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// livePlanStates is the set the partial unique index is built on, spelled here
// so the query and the index cannot disagree about what "live" means.
const livePlanStates = `('computed', 'approved', 'applying')`

const syncPlanColumns = `
    id, target_id, trigger_kind, actor_account_id, digest, state,
    create_count, update_count, delete_count,
    computed_at, approved_at, started_at, finished_at,
    expires_at, lease_expires_at, attempt`

func scanSyncPlan(scanner rowScanner) (orgsync.Plan, error) {
	var (
		plan     orgsync.Plan
		computed StoredTime
		approved StoredTime
		started  StoredTime
		finished StoredTime
		expires  StoredTime
		leased   StoredTime
	)

	if err := scanner.Scan(
		&plan.ID, &plan.TargetID, &plan.Trigger, &plan.ActorAccountID,
		&plan.Digest, &plan.State,
		&plan.Counts.Create, &plan.Counts.Update, &plan.Counts.Delete,
		&computed, &approved, &started, &finished, &expires, &leased, &plan.Attempt,
	); err != nil {
		return orgsync.Plan{}, fmt.Errorf("scan sync plan: %w", err)
	}

	plan.ComputedAt = computed.Time()
	plan.ApprovedAt = approved.Pointer()
	plan.StartedAt = started.Pointer()
	plan.FinishedAt = finished.Pointer()
	plan.ExpiresAt = expires.Time()
	plan.LeaseExpiresAt = leased.Pointer()

	return plan, nil
}

func scanSyncAction(scanner rowScanner) (orgsync.Action, error) {
	var (
		action  orgsync.Action
		payload []byte
	)

	if err := scanner.Scan(
		&action.ID, &action.PlanID, &action.RepositoryID, &action.Kind,
		&action.Operation, &action.Subject, &action.Before, &action.After,
		&payload, &action.State, &action.Error, &action.Blocker,
	); err != nil {
		return orgsync.Action{}, fmt.Errorf("scan sync action: %w", err)
	}

	// An empty column is a deletion, which carries no payload. Left nil rather
	// than an empty slice so a caller that decodes gets a clear failure instead
	// of a zero value that looks like a label called "".
	if len(payload) > 0 {
		action.Payload = payload
	}

	return action, nil
}

const syncActionColumns = `
    id, plan_id, repository_id, kind, operation, subject,
    before_state, after_state, payload, state, error, blocker`

// invalidateLivePlans marks every plan an installation could still apply as
// stale.
//
// Called from inside the transaction that changed the configuration, never
// after it. A plan is approved against the fingerprint a browser rendered, so
// any window where the configuration has moved and the plan has not is a window
// in which somebody can approve work they never saw.
func invalidateLivePlans(
	ctx context.Context,
	tx *transaction,
	targetID string,
	now time.Time,
) error {
	if _, err := tx.ExecContext(ctx, `
UPDATE sync_plans SET state = 'stale', finished_at = ?
WHERE target_id = ? AND state IN `+livePlanStates, now, targetID); err != nil {
		return fmt.Errorf("invalidate live sync plans: %w", err)
	}

	return nil
}

// CreateSyncPlan records a plan and its actions together.
//
// One transaction, because a plan holds the installation's single live slot the
// moment it exists. Writing the plan and then its actions would leave a window
// where another caller reads a plan that proposes nothing and approves it.
func (s *Store) CreateSyncPlan(
	ctx context.Context,
	create orgsync.PlanCreate,
) (orgsync.Plan, error) {
	if err := validatePlanCreate(create); err != nil {
		return orgsync.Plan{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return orgsync.Plan{}, fmt.Errorf("begin sync plan create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	counts := countActions(create.Actions)

	_, err = tx.ExecContext(ctx, `
INSERT INTO sync_plans (
    id, target_id, trigger_kind, actor_account_id, digest, state,
    create_count, update_count, delete_count, computed_at, expires_at, attempt
) VALUES (?, ?, ?, ?, ?, 'computed', ?, ?, ?, ?, ?, 0)`,
		create.ID, create.TargetID, create.Trigger, create.ActorID, create.Digest,
		counts.Create, counts.Update, counts.Delete, create.Now, create.ExpiresAt,
	)
	if err != nil {
		// The partial unique index is what makes "one live plan per
		// installation" a fact rather than a convention, so a second one
		// arriving is a conflict to report rather than a bug to log.
		if s.dialect.UniqueViolation(err) {
			return orgsync.Plan{}, storage.ErrConflict
		}

		return orgsync.Plan{}, fmt.Errorf("insert sync plan: %w", err)
	}

	for _, action := range create.Actions {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO sync_plan_actions (
    plan_id, repository_id, kind, operation, subject,
    before_state, after_state, payload, state, error, blocker
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', '', '')`,
			create.ID, action.RepositoryID, action.Kind, action.Operation,
			action.Subject, action.Before, action.After, string(action.Payload),
		); err != nil {
			return orgsync.Plan{}, fmt.Errorf("insert sync plan action: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return orgsync.Plan{}, fmt.Errorf("commit sync plan create: %w", err)
	}

	return orgsync.Plan{
		ID:             create.ID,
		TargetID:       create.TargetID,
		Trigger:        create.Trigger,
		ActorAccountID: create.ActorID,
		Digest:         create.Digest,
		State:          orgsync.PlanComputed,
		Counts:         counts,
		ComputedAt:     create.Now,
		ExpiresAt:      create.ExpiresAt,
	}, nil
}

func validatePlanCreate(create orgsync.PlanCreate) error {
	switch {
	case create.ID == "" || create.TargetID == "" || create.ActorID == "":
		return fmt.Errorf("%w: plan identity, installation and actor are required",
			orgsync.ErrInvalidPlan)
	case !create.Trigger.Valid():
		return fmt.Errorf("%w: unknown trigger %q", orgsync.ErrInvalidPlan, create.Trigger)
	case create.Digest == "":
		return fmt.Errorf("%w: a plan without a fingerprint could never be approved safely",
			orgsync.ErrInvalidPlan)
	case create.Now.IsZero() || create.ExpiresAt.IsZero():
		return fmt.Errorf("%w: a plan needs a time and an expiry", orgsync.ErrInvalidPlan)
	}

	for index, action := range create.Actions {
		if !action.Kind.Valid() || action.RepositoryID == "" || action.Subject == "" {
			return fmt.Errorf("%w: action %d names no repository, kind or subject",
				orgsync.ErrInvalidPlan, index+1)
		}
	}

	return nil
}

func countActions(actions []orgsync.Action) orgsync.Counts {
	var counts orgsync.Counts
	for _, action := range actions {
		switch action.Operation {
		case orgsync.OperationCreate:
			counts.Create++
		case orgsync.OperationUpdate:
			counts.Update++
		case orgsync.OperationDelete:
			counts.Delete++
		}
	}

	return counts
}

// GetSyncPlan reads a plan and its actions, scoped to its installation.
//
// Both, always. A plan identifier is a name for something the caller may never
// have been authorized against, so reading by identifier alone is how one
// installation's plan reaches somebody who has rights over another.
func (s *Store) GetSyncPlan(
	ctx context.Context,
	targetID string,
	planID string,
) (orgsync.Plan, []orgsync.Action, error) {
	plan, err := scanSyncPlan(s.db.QueryRowContext(ctx, `
SELECT`+syncPlanColumns+` FROM sync_plans WHERE id = ? AND target_id = ?`, planID, targetID))
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.Plan{}, nil, storage.ErrNotFound
	}
	if err != nil {
		return orgsync.Plan{}, nil, err
	}

	actions, err := s.listSyncActions(ctx, planID)

	return plan, actions, err
}

// GetLiveSyncPlan answers the one plan an installation may have in flight.
func (s *Store) GetLiveSyncPlan(
	ctx context.Context,
	targetID string,
) (orgsync.Plan, []orgsync.Action, error) {
	plan, err := scanSyncPlan(s.db.QueryRowContext(ctx, `
SELECT`+syncPlanColumns+`
FROM sync_plans
WHERE target_id = ? AND state IN `+livePlanStates, targetID))
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.Plan{}, nil, storage.ErrNotFound
	}
	if err != nil {
		return orgsync.Plan{}, nil, err
	}

	actions, err := s.listSyncActions(ctx, plan.ID)

	return plan, actions, err
}

func (s *Store) listSyncActions(ctx context.Context, planID string) ([]orgsync.Action, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT`+syncActionColumns+`
FROM sync_plan_actions
WHERE plan_id = ?
ORDER BY repository_id, kind, id`, planID)
	if err != nil {
		return nil, fmt.Errorf("list sync plan actions: %w", err)
	}

	return collectRows(rows, scanSyncAction)
}

// ApproveSyncPlan accepts a plan somebody has read.
//
// Expiry is re-checked here rather than left to the sweeper, so a plan that
// outlived its window cannot be approved because nothing had got round to
// retiring it. The fingerprint is checked for the stronger reason: it is what
// says the plan on the screen is the plan in the database.
func (s *Store) ApproveSyncPlan(
	ctx context.Context,
	approval orgsync.PlanApproval,
) (orgsync.Plan, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return orgsync.Plan{}, fmt.Errorf("begin sync plan approval: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Scoped to the installation the approver was authorized against. The plan
	// identifier alone would let somebody with rights over one installation
	// approve another's work, which then runs against that other's repositories.
	plan, err := scanSyncPlan(tx.QueryRowContext(ctx, `
SELECT`+syncPlanColumns+`
FROM sync_plans
WHERE id = ? AND target_id = ?`+s.dialect.RowLock(), approval.PlanID, approval.TargetID))
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.Plan{}, storage.ErrNotFound
	}
	if err != nil {
		return orgsync.Plan{}, err
	}

	switch {
	case plan.State != orgsync.PlanComputed:
		return orgsync.Plan{}, storage.ErrConflict
	case plan.Digest != approval.Digest:
		return orgsync.Plan{}, orgsync.ErrStalePlan
	case !approval.Now.Before(plan.ExpiresAt):
		return orgsync.Plan{}, orgsync.ErrStalePlan
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE sync_plans SET state = 'approved', approved_at = ? WHERE id = ?`,
		approval.Now, approval.PlanID,
	); err != nil {
		return orgsync.Plan{}, fmt.Errorf("approve sync plan: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return orgsync.Plan{}, fmt.Errorf("commit sync plan approval: %w", err)
	}

	plan.State = orgsync.PlanApproved
	plan.ApprovedAt = &approval.Now

	return plan, nil
}

// DiscardSyncPlan takes a live plan off the slot because somebody declined it.
func (s *Store) DiscardSyncPlan(
	ctx context.Context,
	discard orgsync.PlanDiscard,
) (orgsync.Plan, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return orgsync.Plan{}, fmt.Errorf("begin sync plan discard: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Scoped to the installation the reader was authorized against, for the
	// same reason approval is: the identifier alone would let somebody with
	// rights over one installation retire another's work.
	plan, err := scanSyncPlan(tx.QueryRowContext(ctx, `
SELECT`+syncPlanColumns+`
FROM sync_plans
WHERE id = ? AND target_id = ?`+s.dialect.RowLock(), discard.PlanID, discard.TargetID))
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.Plan{}, storage.ErrNotFound
	}
	if err != nil {
		return orgsync.Plan{}, err
	}

	// A plan an executor already holds is not declined from a browser: the
	// work may be half done, and "discarded" would say it never ran.
	if plan.State != orgsync.PlanComputed && plan.State != orgsync.PlanApproved {
		return orgsync.Plan{}, storage.ErrConflict
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE sync_plans SET state = 'discarded', finished_at = ? WHERE id = ?`,
		discard.Now, discard.PlanID,
	); err != nil {
		return orgsync.Plan{}, fmt.Errorf("discard sync plan: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return orgsync.Plan{}, fmt.Errorf("commit sync plan discard: %w", err)
	}

	plan.State = orgsync.PlanDiscarded
	plan.FinishedAt = &discard.Now

	return plan, nil
}

// LeaseSyncPlan claims an approved plan for a bounded time.
//
// A lease rather than a flag, exactly as LeaseDelivery works: an executor that
// dies leaves work whose lease runs out and which somebody else can pick up,
// rather than a plan stuck in applying for as long as nobody notices.
func (s *Store) LeaseSyncPlan(
	ctx context.Context,
	now time.Time,
	until time.Time,
) (orgsync.PlanLease, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return orgsync.PlanLease{}, fmt.Errorf("begin sync plan lease: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	plan, err := scanSyncPlan(tx.QueryRowContext(ctx, `
SELECT`+syncPlanColumns+`
FROM sync_plans
WHERE (state = 'approved' AND lease_expires_at IS NULL)
   OR (state = 'applying' AND lease_expires_at <= ?)
ORDER BY approved_at
LIMIT 1`+s.dialect.RowLock(), now))
	if errors.Is(err, sql.ErrNoRows) {
		// Nothing due is the ordinary answer on most ticks, not a failure.
		return orgsync.PlanLease{}, nil
	}
	if err != nil {
		return orgsync.PlanLease{}, err
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE sync_plans SET
    state = 'applying', started_at = COALESCE(started_at, ?),
    lease_expires_at = ?, attempt = attempt + 1
WHERE id = ?`, now, until, plan.ID,
	); err != nil {
		return orgsync.PlanLease{}, fmt.Errorf("lease sync plan: %w", err)
	}

	// Every action, not only the pending ones.
	//
	// A retry after a lease ran out needs to see what the previous attempt
	// finished, for two reasons. It must not do that work again - re-creating a
	// label GitHub already made is a 422 that fails a repository for having
	// succeeded - and it must still record the digest for a kind that finished,
	// which it can only do if that kind still appears in the work. Filtering to
	// pending here made an interrupted plan leave a repository looking
	// permanently unsynchronised, and re-read from GitHub on every tick after.
	rows, err := tx.QueryContext(ctx, `
SELECT`+syncActionColumns+`
FROM sync_plan_actions
WHERE plan_id = ?
ORDER BY repository_id, kind, id`, plan.ID)
	if err != nil {
		return orgsync.PlanLease{}, fmt.Errorf("read leased sync actions: %w", err)
	}

	actions, err := collectRows(rows, scanSyncAction)
	if err != nil {
		return orgsync.PlanLease{}, err
	}

	if err := tx.Commit(); err != nil {
		return orgsync.PlanLease{}, fmt.Errorf("commit sync plan lease: %w", err)
	}

	plan.State = orgsync.PlanApplying
	plan.Attempt++
	plan.LeaseExpiresAt = &until

	return orgsync.PlanLease{Plan: plan, Actions: actions, Found: true}, nil
}

// RecordSyncActionOutcome writes what became of one action.
//
// One action at a time, because actions fail alone. Nothing is unwound when one
// does: undoing a settings change because a later ruleset failed leaves a
// repository in a state nobody chose, which is worse than the partial state it
// replaces.
func (s *Store) RecordSyncActionOutcome(
	ctx context.Context,
	outcome orgsync.ActionOutcome,
) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE sync_plan_actions SET state = ?, error = ?, blocker = ? WHERE id = ?`,
		outcome.State, outcome.Error, string(outcome.Blocker), outcome.ActionID,
	)
	if err != nil {
		return fmt.Errorf("record sync action outcome: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record sync action outcome: %w", err)
	}
	if affected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

// FinishSyncPlan closes a plan and records what each repository now has.
//
// The applied digests are written in the same transaction as the plan's own
// state, because they are what the next reconcile trusts: a plan recorded as
// applied whose digests were not would have every repository re-planned, and
// digests recorded without the plan closing would have them skipped by a plan
// that never finished.
func (s *Store) FinishSyncPlan(ctx context.Context, outcome orgsync.PlanOutcome) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync plan finish: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Only a plan this executor still holds. A configuration saved while the
	// work was running marks it stale, and closing it anyway would report
	// "applied" for a plan the service had already decided was out of date -
	// and, worse, record what each repository now has from a scope that has
	// since moved, so the next reconcile would skip repositories that need
	// exactly the change nobody applied.
	var state orgsync.PlanState
	err = tx.QueryRowContext(ctx,
		`SELECT state FROM sync_plans WHERE id = ?`+s.dialect.RowLock(), outcome.PlanID,
	).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read sync plan to finish: %w", err)
	}
	if state != orgsync.PlanApplying {
		return storage.ErrConflict
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE sync_plans SET state = ?, finished_at = ?, lease_expires_at = NULL WHERE id = ?`,
		outcome.State, outcome.Now, outcome.PlanID,
	); err != nil {
		return fmt.Errorf("finish sync plan: %w", err)
	}

	for _, state := range outcome.Applied {
		if err := upsertSyncRepositoryState(ctx, tx, state); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sync plan finish: %w", err)
	}

	return nil
}

// RecordSyncRepositoryState writes what repositories have, outside any plan.
//
// One transaction for the batch, because a planner records everything it found
// matching in one pass and a half-written batch would leave some of them asking
// GitHub again next tick while the rest did not.
func (s *Store) RecordSyncRepositoryState(
	ctx context.Context,
	states []orgsync.RepositoryState,
) error {
	if len(states) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync repository state: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, state := range states {
		if err := upsertSyncRepositoryState(ctx, tx, state); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sync repository state: %w", err)
	}

	return nil
}

// upsertSyncRepositoryState records what a repository has had applied.
//
// Written as a delete and an insert rather than an engine's own upsert clause,
// which the two spell differently: ON CONFLICT is not portable enough to be
// worth a Dialect method for one statement, and the pair runs inside the
// caller's transaction, so nothing can read between them.
func upsertSyncRepositoryState(
	ctx context.Context,
	tx *transaction,
	state orgsync.RepositoryState,
) error {
	if _, err := tx.ExecContext(ctx, `
DELETE FROM sync_repository_state WHERE repository_id = ? AND kind = ?`,
		state.RepositoryID, state.Kind,
	); err != nil {
		return fmt.Errorf("clear sync repository state: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO sync_repository_state (repository_id, kind, applied_digest, applied_at, problem)
VALUES (?, ?, ?, ?, ?)`,
		state.RepositoryID, state.Kind, state.AppliedDigest, state.AppliedAt, state.Problem,
	); err != nil {
		return fmt.Errorf("record sync repository state: %w", err)
	}

	return nil
}

// ExpireSyncPlans retires plans nobody acted on.
//
// Only ones still waiting for somebody. A plan an executor holds is not
// abandoned because its expiry passed - it is being applied, and its lease is
// what says whether that is still true.
func (s *Store) ExpireSyncPlans(ctx context.Context, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, `
UPDATE sync_plans SET state = 'expired', finished_at = ?
WHERE state IN ('computed', 'approved') AND expires_at <= ?`, now, now); err != nil {
		return fmt.Errorf("expire sync plans: %w", err)
	}

	return nil
}
