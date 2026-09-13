package storagetest

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func recoveryFixture(ctx context.Context, store storage.Store, now time.Time) (storage.DeliveryRecovery, storage.DeliveryClaim) {
	account, target := seedInstallation(ctx, store, now)
	_, activateErr := store.CreatePanelUser(ctx, storage.PanelUserCreate{AccountID: account.ID, ActorAccountID: account.ID, ChangedAt: now})
	Expect(activateErr).NotTo(HaveOccurred())
	session := storage.Session{TokenHash: "recovery-session", AccountID: account.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	Expect(store.CreateSession(ctx, session, 2)).To(Succeed())
	claim := storage.DeliveryClaim{ClaimKey: "recover-original", DeliveryID: "original", TargetID: target.TargetID, Event: "issue_comment", Payload: []byte(`{"action":"created"}`), ClaimedAt: now}
	original, err := store.ClaimDelivery(ctx, claim)
	Expect(err).NotTo(HaveOccurred())
	Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{ClaimID: original.ID, Stage: "config", Reason: "fix configuration", FailedAt: now})).To(Succeed())
	operation, err := store.GetDeliveryOperation(ctx, target.TargetID, original.ID)
	Expect(err).NotTo(HaveOccurred())
	return storage.DeliveryRecovery{TargetID: target.TargetID, SourceRunID: original.ID, ExpectedRunID: original.ID, ExpectedRevision: operation.Revision, RequestKey: "retry-once", ActorAccountID: account.ID, SessionTokenHash: session.TokenHash, RequestedAt: now}, claim
}

func declareDeliveryRecoverySpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	It("recovers a permanent delivery failure once and retains its receipt", func() {
		ctx, store, now := runtime()
		request, _ := recoveryFixture(ctx, store, now)
		recovered, err := store.RecoverDelivery(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(recovered.Repeated).To(BeFalse())
		Expect(recovered.RunID).To(BeNumerically(">", request.SourceRunID))
		repeated, err := store.RecoverDelivery(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(repeated.RunID).To(Equal(recovered.RunID))
		Expect(repeated.Repeated).To(BeTrue())
		lease, err := store.LeaseDelivery(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Work).NotTo(BeNil())
		Expect(lease.Work.ID).To(Equal(recovered.RunID))
		Expect(lease.Work.SourceOrder).To(Equal(request.SourceRunID))
		Expect(lease.Work.Attempt).To(Equal(1))
		failures, err := store.ListFailures(ctx, request.TargetID, storage.FailurePageRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(failures.Items).To(HaveLen(1))
		Expect(failures.Items[0].Reason).To(Equal("fix configuration"))
		events, err := store.ListQueueEvents(ctx, fmt.Sprintf("delivery:%d", recovered.RunID), 100)
		Expect(err).NotTo(HaveOccurred())
		Expect(events).To(ContainElement(HaveField("Kind", "recovery_requested")))
		Expect(store.CompleteDelivery(ctx, recovered.RunID, now)).To(Succeed())
		repeated, err = store.RecoverDelivery(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(repeated.RunID).To(Equal(recovered.RunID))
		request.RequestKey = "another-click"
		_, err = store.RecoverDelivery(ctx, request)
		Expect(err).To(MatchError(storage.ErrConflict))
	})
	declareDeliveryRecoveryConcurrency(runtime)
	declareDeliveryRecoveryDenied(runtime)
	declareDeliveryRecoveryAccess(runtime)
}

func declareDeliveryRecoveryConcurrency(runtime func() (context.Context, storage.Store, time.Time)) {
	It("serializes concurrent delivery recovery requests", func() {
		ctx, store, now := runtime()
		request, _ := recoveryFixture(ctx, store, now)
		results := race(func(_ int) (storage.DeliveryRecoveryResult, error) { return store.RecoverDelivery(ctx, request) })
		accepted := 0
		var id int64
		for _, result := range results {
			Expect(result.err).NotTo(HaveOccurred())
			if id == 0 {
				id = result.value.RunID
			}
			Expect(result.value.RunID).To(Equal(id))
			if !result.value.Repeated {
				accepted++
			}
		}
		Expect(accepted).To(Equal(1))
	})
	declareDeliveryRedeliveryRace(runtime)
}

func declareDeliveryRedeliveryRace(runtime func() (context.Context, storage.Store, time.Time)) {
	It("serializes delivery recovery against ordinary redelivery", func() {
		ctx, store, now := runtime()
		request, claim := recoveryFixture(ctx, store, now)
		// A transient failure permits ordinary redelivery, so both paths can win.
		claim.ClaimKey = "transient-recovery"
		original, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{ClaimID: original.ID, Reason: "temporary", Retryable: true, FailedAt: now})).To(Succeed())
		operation, err := store.GetDeliveryOperation(ctx, request.TargetID, original.ID)
		Expect(err).NotTo(HaveOccurred())
		request.SourceRunID, request.ExpectedRunID, request.ExpectedRevision = original.ID, original.ID, operation.Revision
		results := race(func(index int) (int64, error) {
			if index%2 == 0 {
				recovered, err := store.RecoverDelivery(ctx, request)
				if errors.Is(err, storage.ErrConflict) {
					return 0, nil
				}
				return recovered.RunID, err
			}
			result, err := store.ClaimDelivery(ctx, claim)
			if err != nil {
				return 0, err
			}
			if result.Disposition == storage.DeliveryClaimAccepted {
				return result.ID, nil
			}
			return 0, nil
		})
		ids := map[int64]bool{}
		for _, result := range results {
			Expect(result.err).NotTo(HaveOccurred())
			if result.value != 0 {
				ids[result.value] = true
			}
		}
		Expect(ids).To(HaveLen(1))
		Expect(ids).NotTo(HaveKey(request.SourceRunID))
	})
}

func declareDeliveryRecoveryDenied(runtime func() (context.Context, storage.Store, time.Time)) {
	DescribeTable("denies delivery recovery with invalid access or identity", func(kind string) {
		ctx, store, now := runtime()
		request, _ := recoveryFixture(ctx, store, now)
		switch kind {
		case "revoked":
			Expect(store.DeleteSession(ctx, request.SessionTokenHash, storage.ElevationRevoked, now)).To(Succeed())
		case "expired":
			request.RequestedAt = now.Add(2 * time.Hour)
		case "wrong actor":
			request.ActorAccountID = "unknown"
		case "wrong target":
			request.TargetID = "unknown"
		case "stale run":
			request.ExpectedRunID++
		case "stale revision":
			request.ExpectedRevision++
		case "empty key":
			request.RequestKey = ""
		}
		_, err := store.RecoverDelivery(ctx, request)
		Expect(err).To(HaveOccurred())
		operation, err := store.GetDeliveryOperation(ctx, "github:installation:100", request.SourceRunID)
		Expect(err).NotTo(HaveOccurred())
		Expect(operation.Current.ID).To(Equal(request.SourceRunID))
		Expect(operation.Current.Status).To(Equal(storage.DeliveryFailed))
	}, Entry("revoked session", "revoked"), Entry("expired session", "expired"), Entry("wrong actor", "wrong actor"), Entry("wrong target", "wrong target"), Entry("stale run", "stale run"), Entry("stale revision", "stale revision"), Entry("empty key", "empty key"))
}
