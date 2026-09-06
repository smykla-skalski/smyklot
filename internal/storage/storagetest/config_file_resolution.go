package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareConfigFileResolutionSpecs(harness Harness, runtime func() (context.Context, storage.Store, time.Time)) {
	It("configuration file resolutions atomically record choices and notify both scopes without changing settings", func() {
		ctx, store, now := runtime()
		account, installation := seedInstallationSettingsBatch(ctx, store, now)
		enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
		drainConfigFileNotifications(ctx, store, now, 2)
		for _, repositoryID := range []string{"", "repo-1"} {
			request := configFileResolution(installation.TargetID, repositoryID, account.ID, now.Add(time.Minute))
			state, err := store.SaveConfigFileResolution(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(state.Revision).To(Equal(int64(1)))
			Expect(state.Document).To(Equal(request.State.Document))
			Expect(state.InitializationRequired).To(BeTrue(), "accepting a choice is not completed reconciliation")
			_, err = store.SaveConfigFileResolution(ctx, request)
			Expect(err).To(MatchError(storage.ErrConflict), "a repeated save cannot create another audit or decision")
		}
		drainConfigFileNotifications(ctx, store, now.Add(time.Minute), 2)
		audit, err := store.ListAudit(ctx, installation.TargetID, storage.AuditPageRequest{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(audit.Items).To(ContainElements(
			And(HaveField("Action", "configuration_file.resolved"), HaveField("RepositoryID", BeNil()), HaveField("Actor.ID", account.ID)),
			And(HaveField("Action", "configuration_file.resolved"), HaveField("RepositoryID", new("repo-1")),
				HaveField("RepositoryFullName", new("smykla-skalski/smyklot")), HaveField("Actor.ID", account.ID)),
		))
		assertResolutionAuditCount(ctx, store, installation.TargetID, 2)
		target, err := store.GetTarget(ctx, installation.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Revision).To(Equal(int64(2)))
		repository, err := store.GetRepository(ctx, installation.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.Revision).To(Equal(int64(2)))
	})
	It("configuration file resolutions roll back choices and notifications when audit attribution fails", func() {
		ctx, store, now := runtime()
		account, installation := seedInstallationSettingsBatch(ctx, store, now)
		enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
		drainConfigFileNotifications(ctx, store, now, 2)
		for _, repositoryID := range []string{"", "repo-1"} {
			request := configFileResolution(installation.TargetID, repositoryID, "unknown-actor", now.Add(time.Minute))
			_, err := store.SaveConfigFileResolution(ctx, request)
			Expect(err).To(HaveOccurred())
			assertNoConfigFileResolution(ctx, store, installation.TargetID, repositoryID, now.Add(time.Minute))
		}
	})
	It("configuration file resolutions reject stale owners, sync revisions, disabled scopes and malformed provenance", func() {
		ctx, store, now := runtime()
		account, installation := seedInstallationSettingsBatch(ctx, store, now)
		enableConfigFileConnections(ctx, store, account, installation.TargetID, now)
		drainConfigFileNotifications(ctx, store, now, 2)
		for _, mutate := range []func(*storage.ConfigFileResolution){
			func(r *storage.ConfigFileResolution) { r.State.OwnerRevision = 1 },
			func(r *storage.ConfigFileResolution) { r.State.ExpectedRevision = 1 },
			func(r *storage.ConfigFileResolution) { r.State.SyncRevisions[orgsync.KindFiles] = 1 },
			func(r *storage.ConfigFileResolution) { r.State.RepositoryID = "repo-2" },
			func(r *storage.ConfigFileResolution) { r.State.RepositoryID = "foreign-repo" },
			func(r *storage.ConfigFileResolution) { r.Side = "unknown" },
			func(r *storage.ConfigFileResolution) { r.ActorAccountID = " " },
			func(r *storage.ConfigFileResolution) { r.State.Initialized = true },
			func(r *storage.ConfigFileResolution) { r.State.Document = []byte(`null`) },
		} {
			request := configFileResolution(installation.TargetID, "repo-1", account.ID, now.Add(time.Minute))
			mutate(&request)
			_, err := store.SaveConfigFileResolution(ctx, request)
			Expect(err).To(HaveOccurred())
			assertNoConfigFileResolution(ctx, store, installation.TargetID, "repo-1", now.Add(time.Minute))
		}
	})
	declareElevatedConfigFileResolutionSpecs(harness, runtime)
}

func configFileResolution(targetID, repositoryID, actorID string, now time.Time) storage.ConfigFileResolution {
	state := configFileStateChange(targetID, repositoryID, now)
	state.Document = []byte(`{"resolution":{"side":"panel","comparison":"reviewed-comparison"},"status":"pending"}`)
	return storage.ConfigFileResolution{State: state, Side: "panel", ActorAccountID: actorID}
}

func drainConfigFileNotifications(ctx context.Context, store storage.Store, now time.Time, expected int) {
	GinkgoHelper()
	count, err := store.DispatchConfigFileNotifications(ctx, now)
	Expect(err).NotTo(HaveOccurred())
	Expect(count).To(Equal(expected))
}

func assertResolutionAuditCount(ctx context.Context, store storage.Store, targetID string, expected int) {
	GinkgoHelper()
	audit, err := store.ListAudit(ctx, targetID, storage.AuditPageRequest{Limit: 100})
	Expect(err).NotTo(HaveOccurred())
	count := 0
	for _, item := range audit.Items {
		if item.Action == "configuration_file.resolved" {
			count++
		}
	}
	Expect(count).To(Equal(expected))
}

func assertNoConfigFileResolution(ctx context.Context, store storage.Store, targetID, repositoryID string, now time.Time) {
	GinkgoHelper()
	state, err := store.GetConfigFileState(ctx, targetID, repositoryID)
	Expect(err).NotTo(HaveOccurred())
	Expect(state.Revision).To(BeZero())
	assertResolutionAuditCount(ctx, store, targetID, 0)
	drainConfigFileNotifications(ctx, store, now, 0)
}
