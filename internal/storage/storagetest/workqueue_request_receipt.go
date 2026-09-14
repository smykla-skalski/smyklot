package storagetest

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareRecurringRequestReceiptSpecs(runtime queueRuntime) {
	Describe("recurring request receipts", func() {
		var ctx context.Context
		var store storage.Store
		var now time.Time
		var request workqueue.RecurringRequest
		BeforeEach(func() {
			ctx, store, now = runtime()
			Expect(store.UpsertAccount(ctx, storage.Account{ID: "receipt-actor", Provider: "github", SubjectID: "receipt-actor", Login: "owner"})).To(Succeed())
			request = workqueue.RecurringRequest{RequestKey: "desktop-request", Kind: workqueue.KindCatalogRefresh, Title: "Refresh", ActorID: "receipt-actor", Reason: "Check changed GitHub state", Now: now}
		})

		It("keeps original acceptance through execution, completion and queue pruning", func() {
			_, err := store.FindRecurringWorkRequest(ctx, request, func() time.Time { return request.Now })
			Expect(err).To(MatchError(storage.ErrNotFound))
			accepted, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
			Expect(err).NotTo(HaveOccurred())
			events, err := store.ListQueueEvents(ctx, accepted.ID, 100)
			Expect(err).NotTo(HaveOccurred())
			request.Now = now.Add(time.Second)
			repeated, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
			Expect(err).NotTo(HaveOccurred())
			Expect(repeated).To(Equal(accepted))
			afterEvents, err := store.ListQueueEvents(ctx, accepted.ID, 100)
			Expect(err).NotTo(HaveOccurred())
			Expect(afterEvents).To(Equal(events))
			leased, found, err := store.ClaimRecurringWork(ctx, workqueue.RecurringClaim{Kind: request.Kind, Title: request.Title, Now: now, LeaseDuration: time.Minute})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			repeated, err = store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
			Expect(err).NotTo(HaveOccurred())
			Expect(repeated).To(Equal(accepted))
			_, err = store.FinishRecurringWork(ctx, leased.ID, workqueue.RecurringCompletion{Attempt: leased.Attempt}, now)
			Expect(err).NotTo(HaveOccurred())
			_, err = store.PruneWorkQueue(ctx, now.AddDate(2, 0, 0))
			Expect(err).NotTo(HaveOccurred())
			_, err = store.GetQueueItem(ctx, accepted.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
			repeated, err = store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
			Expect(err).NotTo(HaveOccurred())
			Expect(repeated).To(Equal(accepted))
			recovered, err := store.FindRecurringWorkRequest(ctx, request, func() time.Time { return request.Now })
			Expect(err).NotTo(HaveOccurred())
			Expect(recovered).To(Equal(accepted))
			request.RequestKey = "new-user-request"
			fresh, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
			Expect(err).NotTo(HaveOccurred())
			Expect(fresh.ID).NotTo(Equal(accepted.ID))
		})

		It("rejects changed input and never exposes another actor's receipt", func() {
			_, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
			Expect(err).NotTo(HaveOccurred())
			target := "another-workspace"
			changed := []workqueue.RecurringRequest{request, request, request, request}
			changed[0].Reason = "different reason"
			changed[1].Title = "Different title"
			changed[2].TargetID = &target
			changed[3].Kind = workqueue.KindSyncScan
			for _, input := range changed {
				_, err := store.FindRecurringWorkRequest(ctx, input, func() time.Time { return input.Now })
				Expect(err).To(MatchError(storage.ErrConflict))
			}
			changedRequest := request
			changedRequest.Reason = "different reason"
			_, err = store.RequestRecurringWork(ctx, changedRequest, func() time.Time { return changedRequest.Now })
			Expect(err).To(MatchError(storage.ErrConflict))
			request.ActorID = "another-actor"
			_, err = store.FindRecurringWorkRequest(ctx, request, func() time.Time { return request.Now })
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		It("serializes concurrent acceptance without repeating queue events", func() {
			type result struct {
				item workqueue.Item
				err  error
			}
			results := make(chan result, 2)
			start := make(chan struct{})
			for range 2 {
				go func() {
					<-start
					item, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
					results <- result{item, err}
				}()
			}
			close(start)
			first, second := <-results, <-results
			Expect(first.err).NotTo(HaveOccurred())
			Expect(second.err).NotTo(HaveOccurred())
			Expect(first.item).To(Equal(second.item))
			events, err := store.ListQueueEvents(ctx, first.item.ID, 100)
			Expect(err).NotTo(HaveOccurred())
			actions := 0
			for _, event := range events {
				if event.Kind == "action.run_now" {
					actions++
				}
			}
			Expect(actions).To(Equal(1))
		})

		It("advances the revision for a distinct request on the same waiting occurrence", func() {
			first, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
			Expect(err).NotTo(HaveOccurred())
			request.RequestKey = "another-command"
			request.Now = now.Add(time.Second)
			second, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
			Expect(err).NotTo(HaveOccurred())
			Expect(second.ID).To(Equal(first.ID))
			Expect(second.Revision).To(Equal(first.Revision + 1))
			Expect(second.UpdatedAt).To(Equal(request.Now))
			_, err = store.ApplyQueueAction(ctx, first.ID, workqueue.ItemAction{Type: workqueue.ActionCancel, ExpectedRevision: first.Revision, ActorID: request.ActorID, Reason: "stale cancellation", ChangedAt: request.Now})
			Expect(err).To(MatchError(storage.ErrConflict))
		})

		It("does not retain acceptance when the transaction fails", func() {
			request.ActorID = "missing-account"
			_, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
			Expect(err).To(HaveOccurred())
			_, err = store.FindRecurringWorkRequest(ctx, request, func() time.Time { return request.Now })
			Expect(err).To(MatchError(storage.ErrNotFound))
			Expect(store.UpsertAccount(ctx, storage.Account{ID: request.ActorID, Provider: "github", SubjectID: request.ActorID, Login: "operator"})).To(Succeed())
			accepted, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
			Expect(err).NotTo(HaveOccurred())
			Expect(accepted.Revision).To(Equal(int64(2)))
		})

		It("rejects malformed keys without acceptance", func() {
			for _, key := range []string{" padded", "padded ", strings.Repeat("x", 201)} {
				request.RequestKey = key
				_, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
				Expect(err).To(MatchError(storage.ErrConflict))
			}
		})
	})
}
