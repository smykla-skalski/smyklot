package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareRepositoryFileObservationSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	It("retains the observed file state and clock while bypassed", func() {
		ctx, store, now := runtime()
		account, target := seedInstallationSettingsBatch(ctx, store, now)
		repository, err := store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigFileObservedAt).To(BeNil())
		changed, err := store.UpdateRepositoryFileState(ctx, storage.RepositoryFileState{
			TargetID: target.TargetID, RepositoryID: "repo-1",
			Status: storage.RepositoryFileMissing, ObservedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue(), "the first confirmed missing observation must announce")
		changed, err = store.UpdateRepositoryFileState(ctx, storage.RepositoryFileState{
			TargetID: target.TargetID, RepositoryID: "repo-1",
			Status: storage.RepositoryFileMissing, ObservedAt: now.Add(time.Second),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse(), "later unchanged observations must stay quiet")
		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID, ChangedAt: now,
			Repositories: []storage.InstallationRepositorySettingsChange{{
				RepositoryID: "repo-1", IgnoreRepositoryFile: true, ExpectedRevision: repository.Revision,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		for _, status := range []storage.RepositoryFileStatus{
			storage.RepositoryFileValid, storage.RepositoryFileInvalid, storage.RepositoryFileMissing,
		} {
			_, err = store.UpdateRepositoryFileState(ctx, storage.RepositoryFileState{
				TargetID: target.TargetID, RepositoryID: "repo-1", Status: status, ObservedAt: now,
			})
			Expect(err).NotTo(HaveOccurred())
			repository, err = store.GetRepository(ctx, target.TargetID, "repo-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(repository.ConfigFileStatus).To(Equal(storage.RepositoryFileBypassed))
			Expect(repository.ConfigFileObservedStatus).To(Equal(status))
			Expect(repository.ConfigFileObservedAt).To(Equal(&now))
		}
	})
}
