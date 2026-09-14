package storagetest

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareSyncCheckRequestSpecs(runtime queueRuntime) {
	Describe("sync check request retirement", func() {
		var ctx context.Context
		var store storage.Store
		var now time.Time
		var request workqueue.RecurringRequest
		var create orgsync.PlanCreate
		BeforeEach(func() {
			ctx, store, now = runtime()
			account := testAccount(now)
			Expect(store.UpsertAccount(ctx, account)).To(Succeed())
			Expect(store.ReconcileCatalog(ctx, []storage.InstallationSnapshot{testInstallation(account, now, nil)})).To(Succeed())
			_, err := store.CreatePanelUser(ctx, storage.PanelUserCreate{AccountID: account.ID, ActorAccountID: account.ID, ChangedAt: now})
			Expect(err).NotTo(HaveOccurred())
			Expect(store.CreateSession(ctx, storage.Session{TokenHash: "check-session", AccountID: account.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, 2)).To(Succeed())
			target := "github:installation:100"
			request = workqueue.RecurringRequest{Kind: workqueue.KindSyncScan, TargetID: &target, RequestKey: "fresh-check", SessionTokenHash: "check-session", Title: "Check", ActorID: account.ID, Reason: "Check saved settings", Now: now}
			create = orgsync.PlanCreate{ID: "old-plan", TargetID: target, ActorID: account.ID, Trigger: orgsync.TriggerManual, Digest: "reviewed", Now: now, ExpiresAt: now.Add(time.Minute)}
		})
		declareSyncCheckAuthoritySpecs(func() (context.Context, storage.Store, time.Time, workqueue.RecurringRequest, orgsync.PlanCreate) {
			return ctx, store, now, request, create
		})

		DescribeTable("retires waiting plans at expiry", func(automatic bool) {
			create.Automatic = automatic
			_, err := store.CreateSyncPlan(ctx, create)
			Expect(err).NotTo(HaveOccurred())
			request.Now = create.ExpiresAt
			availability, err := store.GetSyncCheckAvailability(ctx, create.TargetID, request.Now)
			Expect(err).NotTo(HaveOccurred())
			Expect(availability.Reason).To(Equal("available"))
			unchanged, _, err := store.GetSyncPlan(ctx, create.TargetID, create.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(unchanged.State).NotTo(Equal(orgsync.PlanExpired))
			item, err := store.RequestRecurringWork(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(item.Kind).To(Equal(workqueue.KindSyncScan))
			old, _, err := store.GetSyncPlan(ctx, create.TargetID, create.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(old.State).To(Equal(orgsync.PlanExpired))
			queued, err := store.GetQueueItem(ctx, "sync-plan:"+create.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(queued.State).To(Equal(workqueue.StateSuperseded))
			create.ID = "new-plan"
			create.Now = request.Now
			create.ExpiresAt = request.Now.Add(time.Hour)
			_, err = store.CreateSyncPlan(ctx, create)
			Expect(err).NotTo(HaveOccurred())
			repeated, err := store.RequestRecurringWork(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(repeated.ID).To(Equal(item.ID))
		}, Entry("computed", false), Entry("approved", true))
		DescribeTable("preserves current waiting work", func(automatic bool) {
			create.Automatic = automatic
			_, err := store.CreateSyncPlan(ctx, create)
			Expect(err).NotTo(HaveOccurred())
			availability, err := store.GetSyncCheckAvailability(ctx, create.TargetID, request.Now)
			Expect(err).NotTo(HaveOccurred())
			Expect(availability.Reason).To(Equal("changes_pending"))
			Expect(availability.BlockingPlanID).To(Equal(create.ID))
			_, err = store.RequestRecurringWork(ctx, request)
			var blocked *storage.LiveSyncPlanConflict
			Expect(errors.As(err, &blocked)).To(BeTrue())
			Expect(blocked.PlanID).To(Equal(create.ID))
			_, err = store.FindRecurringWorkRequest(ctx, request)
			Expect(err).To(MatchError(storage.ErrNotFound))
		}, Entry("computed", false), Entry("approved", true))
		It("preserves applying work after its approval expiry", func() {
			create.Automatic = true
			_, err := store.CreateSyncPlan(ctx, create)
			Expect(err).NotTo(HaveOccurred())
			lease, err := store.LeaseSyncPlan(ctx, now, now.Add(time.Hour))
			Expect(err).NotTo(HaveOccurred())
			Expect(lease.Plan.ID).To(Equal(create.ID))
			request.Now = create.ExpiresAt
			_, err = store.RequestRecurringWork(ctx, request)
			var blocked *storage.LiveSyncPlanConflict
			Expect(errors.As(err, &blocked)).To(BeTrue())
			Expect(blocked.PlanID).To(Equal(create.ID))
			plan, _, err := store.GetSyncPlan(ctx, create.TargetID, create.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.State).To(Equal(orgsync.PlanApplying))
		})
		It("reports the running check without claiming acceptance", func() {
			item, err := store.RequestRecurringWork(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			leased, found, err := store.ClaimRecurringWork(ctx, workqueue.RecurringClaim{Kind: request.Kind, TargetID: request.TargetID, Title: "Check", Now: now, LeaseDuration: time.Minute})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(leased.ID).To(Equal(item.ID))
			availability, err := store.GetSyncCheckAvailability(ctx, create.TargetID, now)
			Expect(err).NotTo(HaveOccurred())
			Expect(availability).To(Equal(orgsync.CheckAvailability{Reason: "check_running", RunningCheckID: item.ID}))
			request.RequestKey = "another-check"
			_, err = store.RequestRecurringWork(ctx, request)
			Expect(err).To(MatchError(storage.ErrConflict))
		})

		It("accepts concurrent recovery commands once", func() {
			create.Automatic = true
			_, err := store.CreateSyncPlan(ctx, create)
			Expect(err).NotTo(HaveOccurred())
			before, err := store.GetQueueItem(ctx, "sync-plan:"+create.ID)
			Expect(err).NotTo(HaveOccurred())
			request.Now = create.ExpiresAt
			type result struct {
				item workqueue.Item
				err  error
			}
			results := make(chan result, 2)
			start := make(chan struct{})
			for range 2 {
				go func() { <-start; item, err := store.RequestRecurringWork(ctx, request); results <- result{item, err} }()
			}
			close(start)
			first, second := <-results, <-results
			Expect(first.err).NotTo(HaveOccurred())
			Expect(second.err).NotTo(HaveOccurred())
			Expect(first.item.ID).To(Equal(second.item.ID))
			after, err := store.GetQueueItem(ctx, before.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(after.Revision).To(Equal(before.Revision + 1))
		})

		It("preserves waiting work when the actor cannot request a check", func() {
			_, err := store.CreateSyncPlan(ctx, create)
			Expect(err).NotTo(HaveOccurred())
			request.Now = create.ExpiresAt
			request.ActorID = "missing-actor"
			_, err = store.RequestRecurringWork(ctx, request)
			Expect(err).To(HaveOccurred())
			plan, _, err := store.GetSyncPlan(ctx, create.TargetID, create.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.State).To(Equal(orgsync.PlanComputed))
			queued, err := store.GetQueueItem(ctx, "sync-plan:"+create.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(queued.State).To(Equal(workqueue.StateAwaitingApproval))
		})
	})
}
