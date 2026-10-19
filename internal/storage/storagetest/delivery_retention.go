package storagetest

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareDeliveryRetentionSpecs(runtime queueRuntime) {
	It("expires successful delivery payloads but retains deduplication and recovery input", func() {
		ctx, store, now := runtime()
		_, target := seedInstallation(ctx, store, now)
		cutoff := now.Add(-24 * time.Hour)
		for _, test := range []struct {
			key     string
			status  storage.DeliveryStatus
			at      time.Time
			payload bool
		}{
			{"success-old", storage.DeliverySucceeded, cutoff.Add(-time.Second), false},
			{"success-boundary", storage.DeliverySucceeded, cutoff, true},
			{"failure-old", storage.DeliveryFailed, cutoff.Add(-time.Second), true},
			{"active-old", storage.DeliveryRunning, cutoff.Add(-time.Second), true},
		} {
			claim := storage.DeliveryClaim{
				ClaimKey: test.key, DeliveryID: test.key, TargetID: target.TargetID,
				Event: "check_run", Payload: []byte(`{"action":"completed"}`), ClaimedAt: test.at,
			}
			accepted, err := store.ClaimDelivery(ctx, claim)
			Expect(err).NotTo(HaveOccurred())
			switch test.status {
			case storage.DeliverySucceeded:
				Expect(store.CompleteDelivery(ctx, accepted.ID, test.at)).To(Succeed())
			case storage.DeliveryFailed:
				Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{
					ClaimID: accepted.ID, Stage: "execute", Reason: "temporary", Retryable: true, FailedAt: test.at,
				})).To(Succeed())
			}
			Expect(store.PruneDeliveryPayloads(ctx, cutoff)).To(Succeed())
			operation, err := store.GetDeliveryOperation(ctx, target.TargetID, accepted.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(operation.Current).NotTo(BeNil())
			Expect(operation.Current.Status).To(Equal(test.status))
			Expect(operation.Current.PayloadAvailable).To(Equal(test.payload), test.key)
			if test.status == storage.DeliverySucceeded {
				repeated, err := store.ClaimDelivery(ctx, claim)
				Expect(err).NotTo(HaveOccurred())
				Expect(repeated.Disposition).To(Equal(storage.DeliveryClaimRetained))
			}
		}
	})

	It("bounds delivery cleanup passes and retains active work until later passes", func() {
		ctx, store, now := runtime()
		_, target := seedInstallation(ctx, store, now)
		Expect(store.InitializeQueuePolicies(ctx, workqueue.DeploymentDefaults{}, now)).To(Succeed())
		var finalID int64
		for index := range 1001 {
			claim, err := store.ClaimDelivery(ctx, storage.DeliveryClaim{
				ClaimKey: fmt.Sprint(index), DeliveryID: fmt.Sprint(index), TargetID: target.TargetID,
				Event: "check_run", ClaimedAt: now,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(store.CompleteDelivery(ctx, claim.ID, now)).To(Succeed())
			finalID = claim.ID
		}
		active, err := store.ClaimDelivery(ctx, storage.DeliveryClaim{
			ClaimKey: "active", DeliveryID: "active", TargetID: target.TargetID,
			Event: "check_run", ClaimedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		cutoff := now.Add(time.Second)
		Expect(store.PruneDeliveries(ctx, cutoff)).To(Succeed())
		_, err = store.GetDeliveryOperation(ctx, target.TargetID, finalID)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.PruneDeliveries(ctx, cutoff)).To(Succeed())
		_, err = store.GetDeliveryOperation(ctx, target.TargetID, finalID)
		Expect(err).To(MatchError(storage.ErrNotFound))
		_, err = store.GetDeliveryOperation(ctx, target.TargetID, active.ID)
		Expect(err).NotTo(HaveOccurred())
		removed, err := store.PruneWorkQueue(ctx, now.Add(8*24*time.Hour))
		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(Equal(int64(1000)))
		removed, err = store.PruneWorkQueue(ctx, now.Add(8*24*time.Hour))
		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(Equal(int64(1)))
		Expect(workqueue.Retention(workqueue.KindWebhookDelivery)).To(Equal(7 * 24 * time.Hour))
	})
}
