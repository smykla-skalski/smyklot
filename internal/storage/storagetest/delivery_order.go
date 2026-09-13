package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareDeliveryOrderSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	It("preserves delivery source order across redelivery and pruning", func() {
		ctx, store, now := runtime()
		_, target := seedInstallation(ctx, store, now)
		claim := storage.DeliveryClaim{ClaimKey: "original-command", DeliveryID: "original-delivery", TargetID: target.TargetID, RepositoryFullName: "smykla-skalski/smyklot", Event: "issue_comment", Payload: []byte(`{"action":"created"}`), ClaimedAt: now}
		original, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		first, err := store.LeaseDelivery(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Work).NotTo(BeNil())
		Expect(first.Work.SourceOrder).To(Equal(original.ID))
		Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{ClaimID: original.ID, Stage: "execute", Reason: "temporary failure", Retryable: true, FailedAt: now})).To(Succeed())
		abandoned, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.AbandonDelivery(ctx, abandoned.ID)).To(Succeed())
		later := claim
		later.ClaimKey = "later-command"
		later.DeliveryID = "later-delivery"
		newer, err := store.ClaimDelivery(ctx, later)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.CompleteDelivery(ctx, newer.ID, now)).To(Succeed())
		repeated, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(repeated.ID).To(BeNumerically(">", newer.ID))
		Expect(store.PruneDeliveries(ctx, now.Add(time.Second))).To(Succeed())
		work, err := store.LeaseDelivery(ctx, now.Add(time.Second), now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(work.Work).NotTo(BeNil())
		Expect(work.Work.ID).To(Equal(repeated.ID))
		Expect(work.Work.SourceOrder).To(Equal(original.ID))
		Expect(work.Work.SourceOrder).To(BeNumerically("<", newer.ID))
		Expect(work.Work.Attempt).To(Equal(1))
		Expect(work.Work.ClaimKey).To(Equal(claim.ClaimKey))
	})
}
