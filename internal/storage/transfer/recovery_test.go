package transfer_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/sqlite"
	"github.com/smykla-skalski/smyklot/internal/storage/transfer"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

type recoveryTransferStore interface {
	storage.Store
	transfer.Engine
}

// Populate the actual command boundaries. Empty receipt tables cannot prove
// that a cutover preserves acceptance or protects a repeated command.
func TestCopyPreservesRecoveryAndCheckEvidence(t *testing.T) {
	for _, crossEngine := range []bool{false, true} {
		name := "sqlite"
		if crossEngine {
			name = "postgres round trip"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			dsn := strings.TrimSpace(os.Getenv(dsnVariable))
			if crossEngine && dsn == "" {
				t.Skip(dsnVariable + " is not set")
			}
			openSQLite := func(name string) *sqlite.Store {
				store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), name+".db"))
				requireRecoveryTransfer(t, err)
				t.Cleanup(func() { _ = store.Close() })
				return store
			}
			source := openSQLite("source")
			fixture := seedRecoveryTransfer(t, source)
			var destination recoveryTransferStore = openSQLite("destination")
			if crossEngine {
				destination = freshPostgres(t, ctx, dsn, "recovery_copy")
			}

			fixture.copyAndCheck(t, source, destination, false)
			// Force must also clear the new child tables before their referenced rows.
			fixture.copyAndCheck(t, source, destination, true)
			if crossEngine {
				returned := openSQLite("returned")
				fixture.copyAndCheck(t, destination, returned, false)
				fixture.verifyCheckAfterPruning(t, returned)
			}
			fixture.verifyCheckAfterPruning(t, destination)
		})
	}
}

type recoveryTransferFixture struct {
	check      workqueue.RecurringRequest
	accepted   workqueue.Item
	evidence   orgsync.CheckObservationPage
	dispatch   orgsync.PlanDispatch
	dispatched orgsync.PlanDispatchReceipt
	recovery   storage.DeliveryRecovery
	recovered  storage.DeliveryRecoveryResult
	operation  storage.DeliveryOperation
}

func seedRecoveryTransfer(t *testing.T, store storage.Store) recoveryTransferFixture {
	t.Helper()
	ctx, now := t.Context(), seededAt
	owner := storage.Account{ID: "recovery-owner", Provider: "github", SubjectID: "recovery-owner", Login: "owner", UpdatedAt: now}
	target := "recovery-target"
	requireRecoveryTransfer(t, store.ReconcileInstallation(ctx, storage.InstallationSnapshot{
		TargetID: target, InstallationID: "900", Kind: storage.TargetOrganization, Account: owner, SyncedAt: now,
		Ownership: storage.OwnershipSnapshot{Source: storage.OwnershipSourceOrganizationAdmin, Status: storage.OwnershipStatusFresh, Owners: []storage.Account{owner}, SyncedAt: now},
	}))
	_, err := store.CreatePanelUser(ctx, storage.PanelUserCreate{AccountID: owner.ID, ActorAccountID: owner.ID, ChangedAt: now})
	requireRecoveryTransfer(t, err)
	requireRecoveryTransfer(t, store.CreateSession(ctx, storage.Session{TokenHash: "recovery-session", AccountID: owner.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, 2))
	f := recoveryTransferFixture{check: workqueue.RecurringRequest{Kind: workqueue.KindSyncScan, TargetID: &target, RequestKey: "check-once", SessionTokenHash: "recovery-session", Title: "Check saved settings", ActorID: owner.ID, Reason: "Verify the saved settings", Now: now}}
	f.accepted, err = store.RequestRecurringWork(ctx, f.check, func() time.Time { return now })
	requireRecoveryTransfer(t, err)
	item, found, err := store.ClaimRecurringWork(ctx, workqueue.RecurringClaim{Kind: f.check.Kind, TargetID: &target, Title: f.check.Title, Now: now, LeaseDuration: time.Minute})
	requireRecoveryTransfer(t, err)
	if !found || item.ID != f.accepted.ID {
		t.Fatal("accepted check was not claimed")
	}
	result := orgsync.CheckResult{Outcome: orgsync.CheckOutcome{CompletedAt: now, Disposition: "checked", Summary: "Saved settings match", Counts: map[orgsync.Observation]int{orgsync.ObservationMatched: 1}}, Observations: []orgsync.CheckObservation{{RepositoryID: "historical-repo", Repository: "owner/original-name", Kind: orgsync.KindLabels, Outcome: orgsync.ObservationMatched, ObservedAt: now.Add(-time.Minute), InputDigest: "original-input"}}}
	requireRecoveryTransfer(t, store.RecordSyncCheckResult(ctx, orgsync.CheckResultCreate{Check: orgsync.CheckReference{QueueID: item.ID, Attempt: item.Attempt}, TargetID: target, Result: result, Now: now}))
	_, err = store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{Attempt: item.Attempt, SuccessSummary: "Checked"}, now)
	requireRecoveryTransfer(t, err)
	f.evidence, err = store.ListSyncCheckObservations(ctx, target, item.ID, 0, 10)
	requireRecoveryTransfer(t, err)
	f.seedDispatch(t, store, target, owner.ID)
	f.seedDelivery(t, store, target, owner.ID)
	return f
}

func (f *recoveryTransferFixture) seedDispatch(t *testing.T, store storage.Store, target, actor string) {
	t.Helper()
	ctx, now := t.Context(), seededAt
	plan, err := store.CreateSyncPlan(ctx, orgsync.PlanCreate{ID: "recovery-plan", TargetID: target, ActorID: actor, Trigger: orgsync.TriggerManual, Digest: "reviewed", Automatic: true, Now: now, ExpiresAt: now.Add(time.Hour)})
	requireRecoveryTransfer(t, err)
	item, err := store.GetQueueItem(ctx, "sync-plan:"+plan.ID)
	requireRecoveryTransfer(t, err)
	f.dispatch = orgsync.PlanDispatch{TargetID: target, PlanID: plan.ID, ActorID: actor, SessionTokenHash: f.check.SessionTokenHash, RequestKey: "dispatch-once", ExpectedRevision: item.Revision, Reason: "Apply reviewed changes", Now: now}
	f.dispatched, err = store.DispatchSyncPlan(ctx, f.dispatch, func() time.Time { return now })
	requireRecoveryTransfer(t, err)
}

func (f *recoveryTransferFixture) seedDelivery(t *testing.T, store storage.Store, target, actor string) {
	t.Helper()
	ctx, now := t.Context(), seededAt
	original, err := store.ClaimDelivery(ctx, storage.DeliveryClaim{ClaimKey: "original-event", DeliveryID: "github-event", TargetID: target, Event: "issue_comment", Payload: []byte(`{"action":"created"}`), ClaimedAt: now})
	requireRecoveryTransfer(t, err)
	requireRecoveryTransfer(t, store.FailDelivery(ctx, storage.DeliveryFailureChange{ClaimID: original.ID, Stage: "config", Reason: "Correct the settings", FailedAt: now}))
	operation, err := store.GetDeliveryOperation(ctx, target, original.ID)
	requireRecoveryTransfer(t, err)
	f.recovery = storage.DeliveryRecovery{TargetID: target, SourceRunID: original.ID, ExpectedRunID: original.ID, ExpectedRevision: operation.Revision, RequestKey: "recover-once", ActorAccountID: actor, SessionTokenHash: f.check.SessionTokenHash, RequestedAt: now}
	f.recovered, err = store.RecoverDelivery(ctx, f.recovery, func() time.Time { return now })
	requireRecoveryTransfer(t, err)
	requireRecoveryTransfer(t, store.CompleteDelivery(ctx, f.recovered.RunID, now))
	f.operation, err = store.GetDeliveryOperation(ctx, target, original.ID)
	requireRecoveryTransfer(t, err)
}

func (f recoveryTransferFixture) verify(t *testing.T, store storage.Store) {
	t.Helper()
	ctx := t.Context()
	clock := func() time.Time { return seededAt }
	check, err := store.FindRecurringWorkRequest(ctx, f.check, clock)
	requireRecoveryTransfer(t, err)
	if !reflect.DeepEqual(check, f.accepted) {
		t.Fatal("check acceptance changed")
	}
	repeated, err := store.RequestRecurringWork(ctx, f.check, clock)
	requireRecoveryTransfer(t, err)
	if !reflect.DeepEqual(repeated, f.accepted) {
		t.Fatal("repeated check created different work")
	}
	evidence, err := store.ListSyncCheckObservations(ctx, *f.check.TargetID, f.accepted.ID, 0, 10)
	requireRecoveryTransfer(t, err)
	if !reflect.DeepEqual(evidence, f.evidence) {
		t.Fatal("historical evidence changed")
	}
	before, err := store.ListQueueEvents(ctx, f.dispatched.QueueID, 100)
	requireRecoveryTransfer(t, err)
	dispatch, err := store.DispatchSyncPlan(ctx, f.dispatch, clock)
	requireRecoveryTransfer(t, err)
	if dispatch != f.dispatched {
		t.Fatal("dispatch acceptance changed")
	}
	after, err := store.ListQueueEvents(ctx, f.dispatched.QueueID, 100)
	requireRecoveryTransfer(t, err)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("repeated dispatch changed queue events")
	}
	recovery, err := store.RecoverDelivery(ctx, f.recovery, clock)
	requireRecoveryTransfer(t, err)
	if !recovery.Repeated || recovery.RunID != f.recovered.RunID {
		t.Fatal("recovery did not retain its original run")
	}
	operation, err := store.GetDeliveryOperation(ctx, f.recovery.TargetID, f.recovery.SourceRunID)
	requireRecoveryTransfer(t, err)
	if !reflect.DeepEqual(operation, f.operation) {
		t.Fatal("delivery operation changed")
	}
}

func requireRecoveryTransfer(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (f recoveryTransferFixture) copyAndCheck(t *testing.T, from, to recoveryTransferStore, force bool) {
	t.Helper()
	report, err := transfer.Copy(t.Context(), from, to, transfer.Options{Force: force})
	requireRecoveryTransfer(t, err)
	for _, table := range []string{"delivery_operations", "delivery_recovery_receipts", "recurring_request_receipts", "sync_dispatch_receipts", "sync_check_results", "sync_check_observations"} {
		if report.Rows[table] == 0 {
			t.Errorf("copy omitted populated %s", table)
		}
	}
	f.verify(t, to)
}

func (f recoveryTransferFixture) verifyCheckAfterPruning(t *testing.T, to storage.Store) {
	t.Helper()
	before, err := to.GetSyncCheckResult(t.Context(), *f.check.TargetID, f.accepted.ID)
	requireRecoveryTransfer(t, err)
	_, err = to.PruneWorkQueue(t.Context(), seededAt.AddDate(2, 0, 0))
	requireRecoveryTransfer(t, err)
	after, err := to.GetSyncCheckResult(t.Context(), *f.check.TargetID, f.accepted.ID)
	requireRecoveryTransfer(t, err)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("queue cleanup changed the transferred comparison")
	}
	evidence, err := to.ListSyncCheckObservations(t.Context(), *f.check.TargetID, f.accepted.ID, 0, 10)
	requireRecoveryTransfer(t, err)
	if !reflect.DeepEqual(evidence, f.evidence) {
		t.Fatal("queue cleanup changed transferred repository evidence")
	}
}
