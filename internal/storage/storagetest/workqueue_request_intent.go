package storagetest

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareQueueRequestIntentSpecs(runtime queueRuntime) {
	It("retains explicit run intent after leasing and isolates successor intent", func() {
		ctx, store, now := runtime()
		account := storage.Account{ID: "github:user:42", Login: "owner"}
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		requested, err := store.QueueRunWasRequested(ctx, "missing")
		Expect(err).NotTo(HaveOccurred())
		Expect(requested).To(BeFalse())
		item, err := store.RequestRecurringWork(ctx, workqueue.RecurringRequest{Kind: workqueue.KindCatalogRefresh, Title: "Refresh", ActorID: account.ID, Reason: "Check changed GitHub state", Now: now})
		Expect(err).NotTo(HaveOccurred())
		claim := workqueue.RecurringClaim{Kind: workqueue.KindCatalogRefresh, Title: "Refresh", Now: now, LeaseDuration: time.Minute}
		leased, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(leased.ID).To(Equal(item.ID))
		Expect(leased.Immediate).To(BeFalse())
		requested, err = store.QueueRunWasRequested(ctx, leased.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(requested).To(BeTrue())
		_, err = store.FinishRecurringWork(ctx, leased.ID, workqueue.RecurringCompletion{Attempt: leased.Attempt, Failure: "temporary", Retryable: true}, now)
		Expect(err).NotTo(HaveOccurred())
		claim.Now = now.Add(time.Minute)
		retried, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(retried.ID).To(Equal(item.ID))
		requested, err = store.QueueRunWasRequested(ctx, retried.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(requested).To(BeTrue())
		_, err = store.FinishRecurringWork(ctx, retried.ID, workqueue.RecurringCompletion{Attempt: retried.Attempt}, claim.Now)
		Expect(err).NotTo(HaveOccurred())
		claim.Now = now.Add(time.Hour)
		next, claimed, err := store.ClaimRecurringWork(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		Expect(next.ID).NotTo(Equal(item.ID))
		requested, err = store.QueueRunWasRequested(ctx, next.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(requested).To(BeFalse())
	})
}
