package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareDeliveryRecoveryAccess(runtime func() (context.Context, storage.Store, time.Time)) {
	It("requires live elevation for Root delivery recovery outside owned targets", func() {
		ctx, store, now := runtime()
		request, _ := recoveryFixture(ctx, store, now)
		root := testAccount(now)
		root.ID, root.SubjectID, root.Login = "root-recovery", "root-recovery", "root-recovery"
		Expect(store.UpsertAccount(ctx, root)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, root.ID, now)).To(Succeed())
		ownerID := request.ActorAccountID
		request.ActorAccountID, request.SessionTokenHash = root.ID, "root-recovery-session"
		Expect(store.CreateSession(ctx, storage.Session{TokenHash: request.SessionTokenHash, AccountID: root.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, 1)).To(Succeed())
		_, err := store.RecoverDelivery(ctx, request)
		Expect(err).To(MatchError(storage.ErrRevoked))
		grant, err := store.BeginElevation(ctx, storage.ElevationGrant{ID: "recovery-grant", SessionTokenHash: request.SessionTokenHash, RootAccountID: root.ID, TargetID: request.TargetID, StartedAt: now})
		Expect(err).NotTo(HaveOccurred())
		request.ElevationID = &grant.ID
		recovered, err := store.RecoverDelivery(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(recovered.RunID).To(BeNumerically(">", request.SourceRunID))
		notices, err := store.ListSecurityNotifications(ctx, ownerID, storage.NotificationPageRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(notices.Items).To(HaveLen(1))
		_, err = store.EndElevation(ctx, grant.ID, request.SessionTokenHash, storage.ElevationRevoked, now)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.RecoverDelivery(ctx, request)
		Expect(err).To(HaveOccurred())
	})
	DescribeTable("checks target roles for delivery recovery", func(role storage.InstallationRole, suspended, allowed bool) {
		ctx, store, now := runtime()
		request, _ := recoveryFixture(ctx, store, now)
		actor := testAccount(now)
		actor.ID, actor.SubjectID, actor.Login = "recovery-member", "recovery-member", "recovery-member"
		Expect(store.UpsertAccount(ctx, actor)).To(Succeed())
		_, err := store.CreatePanelUser(ctx, storage.PanelUserCreate{AccountID: actor.ID, ActorAccountID: request.ActorAccountID, ChangedAt: now})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{TargetID: request.TargetID, SubjectAccountID: actor.ID, ActorAccountID: request.ActorAccountID, Role: &role, Suspended: suspended, ChangedAt: now})
		Expect(err).NotTo(HaveOccurred())
		request.ActorAccountID, request.SessionTokenHash = actor.ID, "member-recovery-session"
		Expect(store.CreateSession(ctx, storage.Session{TokenHash: request.SessionTokenHash, AccountID: actor.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, 1)).To(Succeed())
		_, err = store.RecoverDelivery(ctx, request)
		if allowed {
			Expect(err).NotTo(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
		}
	}, Entry("Admin", storage.InstallationRoleAdmin, false, true), Entry("Editor", storage.InstallationRoleEditor, false, false), Entry("Viewer", storage.InstallationRoleViewer, false, false), Entry("suspended Admin", storage.InstallationRoleAdmin, true, false))
}
