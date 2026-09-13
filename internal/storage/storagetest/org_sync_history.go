package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func verifySyncHistoryPages(ctx context.Context, store storage.Store, now time.Time, target, actorID string, action orgsync.Action) {
	for _, id := range []string{"history-a", "history-b", "history-c"} {
		_, err := store.CreateSyncPlan(ctx, orgsync.PlanCreate{
			ID: id, TargetID: target, Trigger: orgsync.TriggerManual, ActorID: actorID,
			Digest: id, Now: now, ExpiresAt: now.Add(time.Hour),
			Actions: []orgsync.Action{action},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.InvalidateSyncPlans(ctx, target, now)).To(Succeed())
	}
	first, err := store.ListSyncPlans(ctx, target, orgsync.PlanPageRequest{Limit: 2})
	Expect(err).NotTo(HaveOccurred())
	Expect(first.Total).To(Equal(3))
	Expect(first.Items).To(HaveLen(2))
	Expect(first.Items[0].ID).To(Equal("history-c"))
	Expect(first.Items[1].ID).To(Equal("history-b"))
	Expect(first.Items[0].Counts.Create).To(Equal(1))
	Expect(first.Next).NotTo(BeNil())
	_, err = store.CreateSyncPlan(ctx, orgsync.PlanCreate{
		ID: "history-new", TargetID: target, Trigger: orgsync.TriggerManual, ActorID: actorID,
		Digest: "new", Now: now.Add(time.Minute), ExpiresAt: now.Add(time.Hour),
	})
	Expect(err).NotTo(HaveOccurred())
	second, err := store.ListSyncPlans(ctx, target, orgsync.PlanPageRequest{Limit: 2, Before: first.Next})
	Expect(err).NotTo(HaveOccurred())
	Expect(second.Total).To(Equal(4))
	Expect(second.Items).To(HaveLen(1))
	Expect(second.Items[0].ID).To(Equal("history-a"))
	Expect(second.Next).To(BeNil())
	empty, err := store.ListSyncPlans(ctx, "another-installation", orgsync.PlanPageRequest{Limit: 2, Before: first.Next})
	Expect(err).NotTo(HaveOccurred())
	Expect(empty.Total).To(BeZero())
	Expect(empty.Items).To(BeEmpty())
	Expect(empty.Next).To(BeNil())
}
