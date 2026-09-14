package storagetest

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareDeliveryRecoveryInputSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	It("reads delivery recovery input without creating or leasing work", func() {
		ctx, store, now := runtime()
		request, claim := recoveryFixture(ctx, store, now)
		input, err := store.GetDeliveryRecoveryInput(ctx, request.TargetID, request.ExpectedRunID, request.ExpectedRevision)
		Expect(err).NotTo(HaveOccurred())
		Expect(input.RunID).To(Equal(request.ExpectedRunID))
		Expect(input.Revision).To(Equal(request.ExpectedRevision))
		Expect(input.SourceOrder).To(Equal(request.SourceRunID))
		Expect(input.ClaimKey).To(Equal(claim.ClaimKey))
		Expect(input.TargetID).To(Equal(claim.TargetID))
		Expect(input.Event).To(Equal(claim.Event))
		Expect(input.Payload).To(Equal(claim.Payload))
		encoded, err := json.Marshal(input)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring("Payload"))
		lease, err := store.LeaseDelivery(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.Work).To(BeNil())
		_, err = store.GetDeliveryRecoveryInput(ctx, "unrelated-target", request.ExpectedRunID, request.ExpectedRevision)
		Expect(err).To(MatchError(storage.ErrNotFound))
		_, err = store.GetDeliveryRecoveryInput(ctx, request.TargetID, request.ExpectedRunID, request.ExpectedRevision+1)
		Expect(err).To(MatchError(storage.ErrNotFound))
	})
	It("binds delivery recovery input to the current failed execution", func() {
		ctx, store, now := runtime()
		request, claim := recoveryFixture(ctx, store, now)
		recovered, err := store.RecoverDelivery(ctx, request, func() time.Time { return request.RequestedAt })
		Expect(err).NotTo(HaveOccurred())
		_, err = store.GetDeliveryRecoveryInput(ctx, request.TargetID, request.ExpectedRunID, request.ExpectedRevision)
		Expect(err).To(MatchError(storage.ErrNotFound))
		operation, err := store.GetDeliveryOperation(ctx, request.TargetID, request.SourceRunID)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.GetDeliveryRecoveryInput(ctx, request.TargetID, recovered.RunID, operation.Revision)
		Expect(err).To(MatchError(storage.ErrNotFound))
		Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{ClaimID: recovered.RunID, Reason: "still failing", FailedAt: now})).To(Succeed())
		input, err := store.GetDeliveryRecoveryInput(ctx, request.TargetID, recovered.RunID, operation.Revision)
		Expect(err).NotTo(HaveOccurred())
		Expect(input.SourceOrder).To(Equal(request.SourceRunID))
		Expect(input.Payload).To(Equal(claim.Payload))
		Expect(store.PruneDeliveries(ctx, now.Add(time.Second))).To(Succeed())
		_, err = store.GetDeliveryRecoveryInput(ctx, request.TargetID, recovered.RunID, operation.Revision)
		Expect(err).To(MatchError(storage.ErrNotFound))
	})
}
