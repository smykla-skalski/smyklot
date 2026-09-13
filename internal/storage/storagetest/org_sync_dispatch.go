package storagetest

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareSyncDispatchSpecs(runtime queueRuntime) {
	Describe("sync dispatch receipts", func() {
		var ctx context.Context
		var cancel context.CancelFunc
		var store storage.Store
		var now time.Time
		var request orgsync.PlanDispatch
		var before workqueue.Item
		BeforeEach(func() {
			ctx, store, now = runtime()
			ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
			account := testAccount(now)
			Expect(store.UpsertAccount(ctx, account)).To(Succeed())
			Expect(store.ReconcileCatalog(ctx, []storage.InstallationSnapshot{testInstallation(account, now, nil)})).To(Succeed())
			plan, err := store.CreateSyncPlan(ctx, orgsync.PlanCreate{ID: "dispatch-plan", TargetID: "github:installation:100", ActorID: account.ID, Trigger: orgsync.TriggerManual, Digest: "reviewed", Automatic: true, Now: now, ExpiresAt: now.Add(time.Hour)})
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.State).To(Equal(orgsync.PlanApproved))
			before, err = store.GetQueueItem(ctx, "sync-plan:"+plan.ID)
			Expect(err).NotTo(HaveOccurred())
			request = orgsync.PlanDispatch{TargetID: plan.TargetID, PlanID: plan.ID, ActorID: account.ID, RequestKey: "dispatch-1", ExpectedRevision: before.Revision, Reason: "Apply reviewed changes", Now: now}
		})
		AfterEach(func() { cancel() })

		It("accepts once and preserves the exact receipt without changing events", func() {
			_, err := store.FindSyncPlanDispatch(ctx, request)
			Expect(err).To(MatchError(storage.ErrNotFound))
			events, err := store.ListQueueEvents(ctx, before.ID, 100)
			Expect(err).NotTo(HaveOccurred())
			accepted, err := store.DispatchSyncPlan(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(accepted).To(Equal(orgsync.PlanDispatchReceipt{PlanID: request.PlanID, QueueID: before.ID, AcceptedAt: now}))
			current, err := store.GetQueueItem(ctx, before.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(current.Revision).To(Equal(before.Revision + 1))
			Expect(current.Immediate).To(BeTrue())
			request.Now = now.Add(2 * time.Hour)
			repeated, err := store.DispatchSyncPlan(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(repeated).To(Equal(accepted))
			after, err := store.GetQueueItem(ctx, before.ID)
			Expect(err).NotTo(HaveOccurred())
			assertSameDispatchQueue(after, current)
			later, err := store.ListQueueEvents(ctx, before.ID, 100)
			Expect(err).NotTo(HaveOccurred())
			Expect(later).To(HaveLen(len(events) + 1))
		})

		It("binds actor key to target plan revision and reason", func() {
			_, err := store.DispatchSyncPlan(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			for _, change := range []func(*orgsync.PlanDispatch){
				func(r *orgsync.PlanDispatch) { r.TargetID = "another-target" },
				func(r *orgsync.PlanDispatch) { r.PlanID = "another-plan" },
				func(r *orgsync.PlanDispatch) { r.ExpectedRevision++ },
				func(r *orgsync.PlanDispatch) { r.Reason = "different" },
			} {
				changed := request
				change(&changed)
				_, err := store.FindSyncPlanDispatch(ctx, changed)
				Expect(err).To(MatchError(storage.ErrConflict))
				_, err = store.DispatchSyncPlan(ctx, changed)
				Expect(err).To(MatchError(storage.ErrConflict))
			}
			request.ActorID = "other-actor"
			_, err = store.FindSyncPlanDispatch(ctx, request)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		It("retains acceptance after discard pruning and newer work", func() {
			accepted, err := store.DispatchSyncPlan(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			_, err = store.DiscardSyncPlan(ctx, orgsync.PlanDiscard{TargetID: request.TargetID, PlanID: request.PlanID, ActorID: request.ActorID, Now: now})
			Expect(err).NotTo(HaveOccurred())
			_, err = store.PruneWorkQueue(ctx, now.AddDate(2, 0, 0))
			Expect(err).NotTo(HaveOccurred())
			_, err = store.GetQueueItem(ctx, before.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
			_, err = store.CreateSyncPlan(ctx, orgsync.PlanCreate{ID: "newer", TargetID: request.TargetID, ActorID: request.ActorID, Trigger: orgsync.TriggerManual, Digest: "new", Automatic: true, Now: now, ExpiresAt: now.Add(time.Hour)})
			Expect(err).NotTo(HaveOccurred())
			newer, err := store.GetQueueItem(ctx, "sync-plan:newer")
			Expect(err).NotTo(HaveOccurred())
			repeated, err := store.DispatchSyncPlan(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(repeated).To(Equal(accepted))
			current, err := store.GetQueueItem(ctx, newer.ID)
			Expect(err).NotTo(HaveOccurred())
			assertSameDispatchQueue(current, newer)
			read, err := store.FindSyncPlanDispatch(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(read).To(Equal(accepted))
		})

		It("rolls back scheduling and events when receipt ownership is invalid", func() {
			request.ActorID = "missing-account"
			events, err := store.ListQueueEvents(ctx, before.ID, 100)
			Expect(err).NotTo(HaveOccurred())
			_, err = store.DispatchSyncPlan(ctx, request)
			Expect(err).To(HaveOccurred())
			current, err := store.GetQueueItem(ctx, before.ID)
			Expect(err).NotTo(HaveOccurred())
			assertSameDispatchQueue(current, before)
			after, err := store.ListQueueEvents(ctx, before.ID, 100)
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(events))
			Expect(store.UpsertAccount(ctx, storage.Account{ID: request.ActorID, Provider: "github", SubjectID: "missing-account", Login: "owner"})).To(Succeed())
			_, err = store.DispatchSyncPlan(ctx, request)
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects expired stale running and wrong-workspace requests without acceptance", func() {
			for _, change := range []func(*orgsync.PlanDispatch){
				func(r *orgsync.PlanDispatch) { r.Now = now.Add(time.Hour) },
				func(r *orgsync.PlanDispatch) { r.ExpectedRevision++ },
				func(r *orgsync.PlanDispatch) { r.TargetID = "another-target" },
			} {
				changed := request
				change(&changed)
				_, err := store.DispatchSyncPlan(ctx, changed)
				Expect(err).To(HaveOccurred())
			}
			current, err := store.GetQueueItem(ctx, before.ID)
			Expect(err).NotTo(HaveOccurred())
			assertSameDispatchQueue(current, before)
			lease, err := store.LeaseSyncPlan(ctx, now, now.Add(time.Minute))
			Expect(err).NotTo(HaveOccurred())
			Expect(lease.Found).To(BeTrue())
			running, err := store.GetQueueItem(ctx, before.ID)
			Expect(err).NotTo(HaveOccurred())
			request.ExpectedRevision = running.Revision
			_, err = store.DispatchSyncPlan(ctx, request)
			Expect(err).To(MatchError(storage.ErrConflict))
			_, err = store.FindSyncPlanDispatch(ctx, request)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		It("requires an approved plan and never accepts invalidated changes", func() {
			Expect(store.InvalidateSyncPlans(ctx, request.TargetID, now)).To(Succeed())
			_, err := store.DispatchSyncPlan(ctx, request)
			Expect(err).To(MatchError(storage.ErrConflict))
			_, err = store.CreateSyncPlan(ctx, orgsync.PlanCreate{ID: "unapproved", TargetID: request.TargetID, ActorID: request.ActorID, Trigger: orgsync.TriggerManual, Digest: "new", Now: now, ExpiresAt: now.Add(time.Hour)})
			Expect(err).NotTo(HaveOccurred())
			request.PlanID = "unapproved"
			_, err = store.DispatchSyncPlan(ctx, request)
			Expect(err).To(MatchError(storage.ErrConflict))
			_, err = store.FindSyncPlanDispatch(ctx, request)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		It("rejects malformed identities before changing the queue", func() {
			for _, key := range []string{"", " ", " padded", strings.Repeat("ą", 101)} {
				request.RequestKey = key
				_, err := store.DispatchSyncPlan(ctx, request)
				Expect(err).To(MatchError(storage.ErrConflict))
			}
			current, err := store.GetQueueItem(ctx, before.ID)
			Expect(err).NotTo(HaveOccurred())
			assertSameDispatchQueue(current, before)
		})

		It("deduplicates concurrent submissions in the accepting transaction", func() {
			type result struct {
				receipt orgsync.PlanDispatchReceipt
				err     error
			}
			results := make(chan result, 2)
			start := make(chan struct{})
			for range 2 {
				go func() { <-start; r, e := store.DispatchSyncPlan(ctx, request); results <- result{r, e} }()
			}
			close(start)
			first, second := <-results, <-results
			Expect(first.err).NotTo(HaveOccurred())
			Expect(second.err).NotTo(HaveOccurred())
			Expect(second.receipt).To(Equal(first.receipt))
			current, err := store.GetQueueItem(ctx, before.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(current.Revision).To(Equal(before.Revision + 1))
		})

		It("serializes dispatch with leasing without reviving running work", func() {
			dispatchResult := make(chan error, 1)
			type leaseResult struct {
				lease orgsync.PlanLease
				err   error
			}
			leaseResults := make(chan leaseResult, 1)
			start := make(chan struct{})
			go func() { <-start; _, err := store.DispatchSyncPlan(ctx, request); dispatchResult <- err }()
			go func() {
				<-start
				l, e := store.LeaseSyncPlan(ctx, now, now.Add(time.Minute))
				leaseResults <- leaseResult{l, e}
			}()
			close(start)
			dispatched, leased := <-dispatchResult, <-leaseResults
			Expect(leased.err).NotTo(HaveOccurred())
			Expect(leased.lease.Found).To(BeTrue())
			if dispatched != nil {
				Expect(dispatched).To(MatchError(storage.ErrConflict))
			}
			current, err := store.GetQueueItem(ctx, before.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(current.State).To(Equal(workqueue.StateRunning))
			Expect(current.Attempt).To(Equal(1))
		})
	})
}

// EstimatedStartAt is computed at read time, not persisted mutation state.
func assertSameDispatchQueue(actual, expected workqueue.Item) {
	GinkgoHelper()
	actual.EstimatedStartAt, expected.EstimatedStartAt = nil, nil
	Expect(actual).To(Equal(expected))
}
