package storagetest

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareBlockedQueueTimingSpecs(runtime queueRuntime) {
	It("does not estimate blocked work from eligibility or a saved estimate", func() {
		ctx, store, now := runtime()
		_, target := seedInstallation(ctx, store, now)
		for index, estimate := range []*time.Time{nil, pointer(now.Add(time.Minute))} {
			id := []string{"blocked-without-estimate", "blocked-with-old-estimate"}[index]
			_, err := store.CreateQueueItem(ctx, workqueue.Item{
				ID: id, TargetID: &target.TargetID, Kind: workqueue.KindReactionScan,
				Lane: workqueue.LaneMaintenance, State: workqueue.StateBlocked,
				Title: "Blocked scan", BlockedReason: "dependency unavailable",
				Priority: workqueue.PriorityNormal, WindowMode: workqueue.WindowRespect,
				ProfileID: pointer(workqueue.AlwaysOpenProfileID),
				NotBefore: now, EligibleAt: now, EstimatedStartAt: estimate,
				CreatedAt: now, UpdatedAt: now,
			})
			Expect(err).NotTo(HaveOccurred())
			item, err := store.GetQueueItem(ctx, id)
			Expect(err).NotTo(HaveOccurred())
			Expect(item.EstimatedStartAt).To(BeNil())
			Expect(item.WorkAhead).To(BeZero())
			Expect(item.BlockedReason).To(Equal("dependency unavailable"))
		}
		for _, scope := range []*string{nil, &target.TargetID} {
			page, err := store.ListWorkQueue(ctx, workqueue.Filter{TargetID: scope})
			Expect(err).NotTo(HaveOccurred())
			Expect(page.Items).To(HaveLen(2))
			for _, item := range page.Items {
				Expect(item.EstimatedStartAt).To(BeNil())
				Expect(item.WorkAhead).To(BeZero())
			}
		}
	})
}
