package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareElevatedConfigFileResolutionSpecs(harness Harness, runtime func() (context.Context, storage.Store, time.Time)) {
	It("configuration file resolutions cannot deadlock or undo a concurrent elevation termination", func() {
		ctx, store, now := runtime()
		root, owner, target, session := seedElevationScenario(ctx, store, now)
		enableConfigFileConnections(ctx, store, owner, target.TargetID, now)
		drainConfigFileNotifications(ctx, store, now, 2)
		elevation, err := store.BeginElevation(ctx, storage.ElevationGrant{
			ID: "config-file-resolution-concurrent", SessionTokenHash: session.TokenHash,
			RootAccountID: root.ID, TargetID: target.TargetID, StartedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		request := configFileResolution(target.TargetID, "repo-1", root.ID, now.Add(time.Minute))
		request.ElevationID, request.SessionTokenHash = &elevation.ID, session.TokenHash
		endErr, writeErr := harness.EndElevationBeforeWrite(ctx, func() error {
			_, err := store.EndElevation(ctx, elevation.ID, session.TokenHash, storage.ElevationEnded, now.Add(time.Minute))
			return err
		}, func() error {
			_, err := store.SaveConfigFileResolution(ctx, request)
			return err
		})
		Expect(endErr).NotTo(HaveOccurred(), "revocation must not be rolled back as a deadlock victim")
		Expect(writeErr).To(MatchError(storage.ErrExpired), "an ended grant must reject the write, not deadlock")
		assertNoConfigFileResolution(ctx, store, target.TargetID, "repo-1", now.Add(time.Minute))
	})
	It("configuration file resolutions bind elevated choices to the active Root session and notify Owners", func() {
		ctx, store, now := runtime()
		root, owner, target, session := seedElevationScenario(ctx, store, now)
		enableConfigFileConnections(ctx, store, owner, target.TargetID, now)
		drainConfigFileNotifications(ctx, store, now, 2)
		elevation, err := store.BeginElevation(ctx, storage.ElevationGrant{
			ID: "config-file-resolution", SessionTokenHash: session.TokenHash,
			RootAccountID: root.ID, TargetID: target.TargetID, StartedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		request := configFileResolution(target.TargetID, "repo-1", root.ID, now.Add(time.Minute))
		request.ElevationID, request.SessionTokenHash = &elevation.ID, session.TokenHash
		for _, mutate := range []func(*storage.ConfigFileResolution){
			func(r *storage.ConfigFileResolution) { r.SessionTokenHash = "other-session" },
			func(r *storage.ConfigFileResolution) { r.ActorAccountID = owner.ID },
			func(r *storage.ConfigFileResolution) { r.State.ChangedAt = elevation.ExpiresAt },
		} {
			invalid := request
			mutate(&invalid)
			_, err := store.SaveConfigFileResolution(ctx, invalid)
			Expect(err).To(HaveOccurred())
			assertNoConfigFileResolution(ctx, store, target.TargetID, "repo-1", now.Add(time.Minute))
		}
		_, err = store.SaveConfigFileResolution(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		notifications, err := store.ListSecurityNotifications(ctx, owner.ID, storage.NotificationPageRequest{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(notifications.Items).To(HaveLen(1))
		Expect(notifications.Items[0].ElevationID).To(Equal(elevation.ID))
		Expect(notifications.Items[0].Actor.ID).To(Equal(root.ID))
		audit, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{HistoryPageRequest: storage.HistoryPageRequest{Limit: 20}})
		Expect(err).NotTo(HaveOccurred())
		Expect(audit.Items).To(ContainElement(And(HaveField("Action", "configuration_file.resolved"),
			HaveField("ElevationID", new(elevation.ID)))))
		drainConfigFileNotifications(ctx, store, now.Add(time.Minute), 1)
		_, err = store.EndElevation(ctx, elevation.ID, session.TokenHash, storage.ElevationEnded, now.Add(2*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		request.State.RepositoryID, request.State.ChangedAt = "", now.Add(3*time.Minute)
		_, err = store.SaveConfigFileResolution(ctx, request)
		Expect(err).To(HaveOccurred())
		state, err := store.GetConfigFileState(ctx, target.TargetID, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(state.Revision).To(BeZero())
		assertResolutionAuditCount(ctx, store, target.TargetID, 1)
		drainConfigFileNotifications(ctx, store, now.Add(3*time.Minute), 0)
	})
	It("configuration file resolutions roll back all effects if Owner notification fails", func() {
		ctx, store, now := runtime()
		root, owner, target, session := seedElevationScenario(ctx, store, now)
		enableConfigFileConnections(ctx, store, owner, target.TargetID, now)
		drainConfigFileNotifications(ctx, store, now, 2)
		elevation, err := store.BeginElevation(ctx, storage.ElevationGrant{
			ID: "config-file-resolution-rollback", SessionTokenHash: session.TokenHash,
			RootAccountID: root.ID, TargetID: target.TargetID, StartedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		harness.RejectSecurityNotifications(ctx)
		for _, repositoryID := range []string{"", "repo-1"} {
			request := configFileResolution(target.TargetID, repositoryID, root.ID, now.Add(time.Minute))
			request.ElevationID, request.SessionTokenHash = &elevation.ID, session.TokenHash
			_, err := store.SaveConfigFileResolution(ctx, request)
			Expect(err).To(HaveOccurred())
			assertNoConfigFileResolution(ctx, store, target.TargetID, repositoryID, now.Add(time.Minute))
		}
		notifications, err := store.ListSecurityNotifications(ctx, owner.ID, storage.NotificationPageRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(notifications.Items).To(BeEmpty())
	})
}
