package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareDeliveryRecoveryClockSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	It("rolls delivery recovery back when authority expires before commit", func() {
		ctx, store, now := runtime()
		request, _ := recoveryFixture(ctx, store, now)
		calls := 0
		clock := func() time.Time {
			calls++
			if calls >= 3 {
				return now.Add(time.Hour)
			}
			return now
		}
		_, err := store.RecoverDelivery(ctx, request, clock)
		Expect(err).To(MatchError(storage.ErrRevoked))
		operation, err := store.GetDeliveryOperation(ctx, request.TargetID, request.SourceRunID)
		Expect(err).NotTo(HaveOccurred())
		Expect(operation.Current.ID).To(Equal(request.SourceRunID))
		Expect(operation.Revision).To(Equal(request.ExpectedRevision))
		receipt, err := store.GetDeliveryRecoveryReceipt(ctx, request, func() time.Time { return now })
		Expect(err).NotTo(HaveOccurred())
		Expect(receipt).To(BeNil())
	})

	It("checks delivery recovery receipt expiry against current time", func() {
		ctx, store, now := runtime()
		request, _ := recoveryFixture(ctx, store, now)
		_, err := store.RecoverDelivery(ctx, request, func() time.Time { return now })
		Expect(err).NotTo(HaveOccurred())
		_, err = store.GetDeliveryRecoveryReceipt(ctx, request, func() time.Time { return now.Add(time.Hour) })
		Expect(err).To(MatchError(storage.ErrRevoked))
	})

	DescribeTable("checks delivery recovery elevation at current time", func(accepted bool) {
		ctx, store, now := runtime()
		request, _ := recoveryFixture(ctx, store, now)
		root := testAccount(now)
		root.ID, root.SubjectID, root.Login = "clock-root", "clock-root", "clock-root"
		Expect(store.UpsertAccount(ctx, root)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, root.ID, now)).To(Succeed())
		request.ActorAccountID, request.SessionTokenHash = root.ID, "clock-root-session"
		Expect(store.CreateSession(ctx, storage.Session{TokenHash: request.SessionTokenHash, AccountID: root.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, 1)).To(Succeed())
		grant, err := store.BeginElevation(ctx, storage.ElevationGrant{ID: "clock-grant", SessionTokenHash: request.SessionTokenHash, RootAccountID: root.ID, TargetID: request.TargetID, StartedAt: now})
		Expect(err).NotTo(HaveOccurred())
		request.ElevationID = &grant.ID
		if accepted {
			_, err = store.RecoverDelivery(ctx, request, func() time.Time { return now })
			Expect(err).NotTo(HaveOccurred())
		}
		before, err := store.GetDeliveryOperation(ctx, request.TargetID, request.SourceRunID)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.RecoverDelivery(ctx, request, func() time.Time { return grant.ExpiresAt })
		Expect(err).To(MatchError(storage.ErrExpired))
		_, err = store.GetDeliveryRecoveryReceipt(ctx, request, func() time.Time { return grant.ExpiresAt })
		Expect(err).To(MatchError(storage.ErrExpired))
		after, err := store.GetDeliveryOperation(ctx, request.TargetID, request.SourceRunID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after.Current.ID).To(Equal(before.Current.ID))
		Expect(after.Revision).To(Equal(before.Revision))
	}, Entry("new recovery", false), Entry("receipt recovery", true))
}
