package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareDeliveryRecoveryReceiptSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	It("reads delivery recovery receipts without starting another execution", func() {
		ctx, store, now := runtime()
		request, _ := recoveryFixture(ctx, store, now)
		receipt, err := store.GetDeliveryRecoveryReceipt(ctx, request, func() time.Time { return request.RequestedAt })
		Expect(err).NotTo(HaveOccurred())
		Expect(receipt).To(BeNil())
		operation, err := store.GetDeliveryOperation(ctx, request.TargetID, request.SourceRunID)
		Expect(err).NotTo(HaveOccurred())
		Expect(operation.Current.ID).To(Equal(request.SourceRunID))
		Expect(operation.Revision).To(Equal(request.ExpectedRevision))
		accepted, err := store.RecoverDelivery(ctx, request, func() time.Time { return request.RequestedAt })
		Expect(err).NotTo(HaveOccurred())
		Expect(store.CompleteDelivery(ctx, accepted.RunID, now)).To(Succeed())
		receipt, err = store.GetDeliveryRecoveryReceipt(ctx, request, func() time.Time { return request.RequestedAt })
		Expect(err).NotTo(HaveOccurred())
		Expect(receipt).To(Equal(&storage.DeliveryRecoveryResult{RunID: accepted.RunID, Repeated: true}))
		another := request
		another.RequestKey = "another-request"
		receipt, err = store.GetDeliveryRecoveryReceipt(ctx, another, func() time.Time { return another.RequestedAt })
		Expect(err).NotTo(HaveOccurred())
		Expect(receipt).To(BeNil())
		changed := request
		changed.ExpectedRevision++
		_, err = store.GetDeliveryRecoveryReceipt(ctx, changed, func() time.Time { return changed.RequestedAt })
		Expect(err).To(MatchError(storage.ErrConflict))
		Expect(store.DeleteSession(ctx, request.SessionTokenHash, storage.ElevationRevoked, now)).To(Succeed())
		_, err = store.GetDeliveryRecoveryReceipt(ctx, request, func() time.Time { return request.RequestedAt })
		Expect(err).To(MatchError(storage.ErrRevoked))
	})
	DescribeTable("rejects delivery recovery receipt access changes", func(kind string) {
		ctx, store, now := runtime()
		request, _ := recoveryFixture(ctx, store, now)
		_, err := store.RecoverDelivery(ctx, request, func() time.Time { return request.RequestedAt })
		Expect(err).NotTo(HaveOccurred())
		switch kind {
		case "actor":
			request.ActorAccountID = "another-account"
		case "target":
			request.TargetID = "another-target"
		case "expired":
			request.RequestedAt = now.Add(2 * time.Hour)
		case "source":
			request.SourceRunID++
		case "key":
			request.RequestKey = ""
		}
		_, err = store.GetDeliveryRecoveryReceipt(ctx, request, func() time.Time { return request.RequestedAt })
		Expect(err).To(HaveOccurred())
	}, Entry("actor", "actor"), Entry("target", "target"), Entry("expired", "expired"), Entry("source", "source"), Entry("empty key", "key"))
}
