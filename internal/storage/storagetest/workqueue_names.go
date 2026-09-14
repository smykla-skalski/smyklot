package storagetest

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareQueueNameRetentionSpecs(runtime queueRuntime) {
	It("retains queue identity and names when a repository leaves the catalogue", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		id := createQueueFixture(ctx, store, account.ID, target.TargetID, "repo-1", now)
		target.Repositories = nil
		Expect(store.ReconcileInstallation(ctx, target)).To(Succeed())
		for _, scope := range []*string{nil, &target.TargetID} {
			repositoryID := "repo-1"
			page, err := store.ListWorkQueue(ctx, workqueue.Filter{TargetID: scope, RepositoryID: &repositoryID})
			Expect(err).NotTo(HaveOccurred())
			Expect(page.Items).To(HaveLen(1))
			Expect(page.Items[0].ID).To(Equal(id))
			Expect(page.Items[0].RepositoryName).To(Equal("smykla-skalski/smyklot"))
			Expect(page.Facets.RepositoryNames).To(HaveKeyWithValue(repositoryID, "smykla-skalski/smyklot"))
		}
		item, err := store.GetQueueItem(ctx, id)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.RepositoryID).To(HaveValue(Equal("repo-1")))
		Expect(item.RepositoryName).To(Equal("smykla-skalski/smyklot"))
	})

	It("keeps an unknown queue repository identifiable without inventing a name", func() {
		ctx, store, now := runtime()
		account, target := seedInstallation(ctx, store, now)
		repositoryID := "unknown-repository"
		id := createQueueFixture(ctx, store, account.ID, target.TargetID, repositoryID, now)
		for _, scope := range []*string{nil, &target.TargetID} {
			page, err := store.ListWorkQueue(ctx, workqueue.Filter{TargetID: scope, RepositoryID: &repositoryID})
			Expect(err).NotTo(HaveOccurred())
			Expect(page.Items).To(HaveLen(1))
			Expect(page.Items[0].ID).To(Equal(id))
			Expect(page.Items[0].RepositoryName).To(BeEmpty())
			Expect(page.Facets.Repositories).To(ContainElement(repositoryID))
			Expect(page.Facets.RepositoryNames).To(HaveKeyWithValue(repositoryID, ""))
		}
		item, err := store.GetQueueItem(ctx, id)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.RepositoryID).To(HaveValue(Equal(repositoryID)))
		Expect(item.RepositoryName).To(BeEmpty())
	})
}
