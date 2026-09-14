package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareSyncDispatchAuthoritySpecs(runtime func() (context.Context, storage.Store, time.Time, orgsync.PlanDispatch, workqueue.Item)) {
	DescribeTable("requires current authority before accepting or recovering", func(revoke string) {
		ctx, store, now, request, before := runtime()
		if revoke == "accepted then revoked" {
			_, err := store.DispatchSyncPlan(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			before, err = store.GetQueueItem(ctx, before.ID)
			Expect(err).NotTo(HaveOccurred())
		}
		switch revoke {
		case "missing session":
			request.SessionTokenHash = ""
		case "wrong actor":
			request.ActorID = "different-account"
		case "expired session":
			request.Now = now.Add(24 * time.Hour)
		default:
			Expect(store.DeleteSession(ctx, request.SessionTokenHash, storage.ElevationRevoked, now)).To(Succeed())
		}
		events, err := store.ListQueueEvents(ctx, before.ID, 100)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.DispatchSyncPlan(ctx, request)
		Expect(err).To(MatchError(storage.ErrRevoked))
		_, err = store.FindSyncPlanDispatch(ctx, request)
		Expect(err).To(MatchError(storage.ErrRevoked))
		current, err := store.GetQueueItem(ctx, before.ID)
		Expect(err).NotTo(HaveOccurred())
		assertSameDispatchQueue(current, before)
		after, err := store.ListQueueEvents(ctx, before.ID, 100)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(events))
	}, Entry("missing session", "missing session"), Entry("wrong actor", "wrong actor"), Entry("expired session", "expired session"), Entry("revoked session", "revoked session"), Entry("accepted then revoked", "accepted then revoked"))

	DescribeTable("uses current workspace authority", func(role storage.InstallationRole, suspended bool, allowed bool) {
		ctx, store, now, request, before := runtime()
		owner := request.ActorID
		actor := testAccount(now)
		actor.ID, actor.SubjectID, actor.Login = "dispatch-admin", "dispatch-admin", "dispatch-admin"
		Expect(store.UpsertAccount(ctx, actor)).To(Succeed())
		_, err := store.CreatePanelUser(ctx, storage.PanelUserCreate{AccountID: actor.ID, ActorAccountID: owner, ChangedAt: now})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{TargetID: request.TargetID, SubjectAccountID: actor.ID, ActorAccountID: owner, Role: &role, Suspended: suspended, ChangedAt: now})
		Expect(err).NotTo(HaveOccurred())
		request.ActorID, request.SessionTokenHash = actor.ID, "admin-session"
		Expect(store.CreateSession(ctx, storage.Session{TokenHash: request.SessionTokenHash, AccountID: actor.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, 2)).To(Succeed())
		_, err = store.DispatchSyncPlan(ctx, request)
		if allowed {
			Expect(err).NotTo(HaveOccurred())
		} else {
			Expect(err).To(MatchError(storage.ErrRevoked))
			current, err := store.GetQueueItem(ctx, before.ID)
			Expect(err).NotTo(HaveOccurred())
			assertSameDispatchQueue(current, before)
		}
	}, Entry("Admin", storage.InstallationRoleAdmin, false, true), Entry("Editor", storage.InstallationRoleEditor, false, false), Entry("suspended Admin", storage.InstallationRoleAdmin, true, false))

	It("rejects a ban before acceptance and preserves the original receipt", func() {
		ctx, store, now, request, _ := runtime()
		accepted, err := store.DispatchSyncPlan(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		user, err := store.GetPanelUser(ctx, request.ActorID)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.UpdatePanelUser(ctx, storage.PanelUserChange{AccountID: request.ActorID, ActorAccountID: request.ActorID, Status: storage.PanelUserBanned, ExpectedRevision: user.Revision, ChangedAt: now})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.FindSyncPlanDispatch(ctx, request)
		Expect(err).To(MatchError(storage.ErrRevoked))
		_, err = store.DispatchSyncPlan(ctx, request)
		Expect(err).To(MatchError(storage.ErrRevoked))
		_, err = store.UpdatePanelUser(ctx, storage.PanelUserChange{AccountID: request.ActorID, ActorAccountID: request.ActorID, Status: storage.PanelUserActive, ExpectedRevision: user.Revision + 1, ChangedAt: now})
		Expect(err).NotTo(HaveOccurred())
		repeated, err := store.FindSyncPlanDispatch(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(repeated).To(Equal(accepted))
	})

	It("serializes session revocation with dispatch without accepting later work", func() {
		ctx, store, now, request, before := runtime()
		dispatchResult := make(chan error, 1)
		revokeResult := make(chan error, 1)
		start := make(chan struct{})
		go func() { <-start; _, err := store.DispatchSyncPlan(ctx, request); dispatchResult <- err }()
		go func() {
			<-start
			revokeResult <- store.DeleteSession(ctx, request.SessionTokenHash, storage.ElevationRevoked, now)
		}()
		close(start)
		dispatched, revoked := <-dispatchResult, <-revokeResult
		Expect(revoked).NotTo(HaveOccurred())
		current, err := store.GetQueueItem(ctx, before.ID)
		Expect(err).NotTo(HaveOccurred())
		if dispatched == nil {
			Expect(current.Revision).To(Equal(before.Revision + 1))
		} else {
			Expect(dispatched).To(MatchError(storage.ErrRevoked))
			assertSameDispatchQueue(current, before)
		}
		_, err = store.DispatchSyncPlan(ctx, request)
		Expect(err).To(MatchError(storage.ErrRevoked))
		after, err := store.GetQueueItem(ctx, before.ID)
		Expect(err).NotTo(HaveOccurred())
		assertSameDispatchQueue(after, current)
	})
}
