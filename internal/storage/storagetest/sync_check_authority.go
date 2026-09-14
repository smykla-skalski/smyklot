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

func declareSyncCheckAuthoritySpecs(runtime func() (context.Context, storage.Store, time.Time, workqueue.RecurringRequest, orgsync.PlanCreate)) {
	DescribeTable("requires live check authority before retiring work", func(cause string) {
		ctx, store, now, request, create := runtime()
		_, err := store.CreateSyncPlan(ctx, create)
		Expect(err).NotTo(HaveOccurred())
		request.Now = create.ExpiresAt
		switch cause {
		case "missing":
			request.SessionTokenHash = ""
		case "mismatch":
			request.ActorID = "different-actor"
		case "expired":
			request.Now = now.Add(time.Hour)
		default:
			Expect(store.DeleteSession(ctx, request.SessionTokenHash, storage.ElevationRevoked, now)).To(Succeed())
		}
		_, err = store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
		Expect(err).To(MatchError(storage.ErrRevoked))
		_, err = store.FindRecurringWorkRequest(ctx, request, func() time.Time { return request.Now })
		Expect(err).To(MatchError(storage.ErrRevoked))
		plan, _, err := store.GetSyncPlan(ctx, create.TargetID, create.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan.State).To(Equal(orgsync.PlanComputed))
		item, err := store.GetQueueItem(ctx, "sync-plan:"+create.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.State).To(Equal(workqueue.StateAwaitingApproval))
	}, Entry("missing session", "missing"), Entry("actor mismatch", "mismatch"), Entry("expired session", "expired"), Entry("deleted session", "deleted"))

	It("recovers the same check through a new authorized session", func() {
		ctx, store, now, request, _ := runtime()
		accepted, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
		Expect(err).NotTo(HaveOccurred())
		Expect(store.DeleteSession(ctx, request.SessionTokenHash, storage.ElevationRevoked, now)).To(Succeed())
		_, err = store.FindRecurringWorkRequest(ctx, request, func() time.Time { return request.Now })
		Expect(err).To(MatchError(storage.ErrRevoked))
		_, err = store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
		Expect(err).To(MatchError(storage.ErrRevoked))
		request.SessionTokenHash = "replacement-session"
		Expect(store.CreateSession(ctx, storage.Session{TokenHash: request.SessionTokenHash, AccountID: request.ActorID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, 2)).To(Succeed())
		repeated, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
		Expect(err).NotTo(HaveOccurred())
		Expect(repeated).To(Equal(accepted))
		recovered, err := store.FindRecurringWorkRequest(ctx, request, func() time.Time { return request.Now })
		Expect(err).NotTo(HaveOccurred())
		Expect(recovered).To(Equal(accepted))
	})

	DescribeTable("rechecks changed workspace roles for check receipts", func(role storage.InstallationRole, suspended bool) {
		ctx, store, now, request, _ := runtime()
		owner := request.ActorID
		actor := testAccount(now)
		actor.ID, actor.SubjectID, actor.Login = "check-admin", "check-admin", "check-admin"
		Expect(store.UpsertAccount(ctx, actor)).To(Succeed())
		_, err := store.CreatePanelUser(ctx, storage.PanelUserCreate{AccountID: actor.ID, ActorAccountID: owner, ChangedAt: now})
		Expect(err).NotTo(HaveOccurred())
		admin := storage.InstallationRoleAdmin
		change := storage.TargetAccessChange{TargetID: *request.TargetID, SubjectAccountID: actor.ID, ActorAccountID: owner, Role: &admin, ChangedAt: now}
		_, err = store.SetTargetAccess(ctx, change)
		Expect(err).NotTo(HaveOccurred())
		request.ActorID, request.SessionTokenHash = actor.ID, "admin-check-session"
		Expect(store.CreateSession(ctx, storage.Session{TokenHash: request.SessionTokenHash, AccountID: actor.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, 2)).To(Succeed())
		accepted, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
		Expect(err).NotTo(HaveOccurred())
		change.Role, change.Suspended, change.ExpectedRevision = &role, suspended, 1
		_, err = store.SetTargetAccess(ctx, change)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.FindRecurringWorkRequest(ctx, request, func() time.Time { return request.Now })
		Expect(err).To(MatchError(storage.ErrRevoked))
		_, err = store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
		Expect(err).To(MatchError(storage.ErrRevoked))
		current, err := store.GetQueueItem(ctx, accepted.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.Revision).To(Equal(accepted.Revision))
	}, Entry("demoted Admin", storage.InstallationRoleViewer, false), Entry("suspended Admin", storage.InstallationRoleAdmin, true), Entry("removed access", storage.InstallationRoleNone, false))

	It("rolls retirement and acceptance back when authority expires before commit", func() {
		ctx, store, now, request, create := runtime()
		_, err := store.CreateSyncPlan(ctx, create)
		Expect(err).NotTo(HaveOccurred())
		calls := 0
		clock := func() time.Time {
			calls++
			if calls >= 6 {
				return now.Add(time.Hour)
			}
			return create.ExpiresAt
		}
		_, err = store.RequestRecurringWork(ctx, request, clock)
		Expect(err).To(MatchError(storage.ErrRevoked))
		plan, _, err := store.GetSyncPlan(ctx, create.TargetID, create.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan.State).To(Equal(orgsync.PlanComputed))
		item, err := store.GetQueueItem(ctx, "sync-plan:"+create.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.State).To(Equal(workqueue.StateAwaitingApproval))
		_, err = store.FindRecurringWorkRequest(ctx, request, func() time.Time { return now })
		Expect(err).To(MatchError(storage.ErrNotFound))
	})

	It("checks receipt authority against the current clock", func() {
		ctx, store, now, request, _ := runtime()
		_, err := store.RequestRecurringWork(ctx, request, func() time.Time { return now })
		Expect(err).NotTo(HaveOccurred())
		_, err = store.FindRecurringWorkRequest(ctx, request, func() time.Time { return now.Add(time.Hour) })
		Expect(err).To(MatchError(storage.ErrRevoked))
	})

	It("serializes check acceptance with session deletion", func() {
		ctx, store, now, request, _ := runtime()
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		type result struct {
			item workqueue.Item
			err  error
		}
		accepted := make(chan result, 1)
		revoked := make(chan error, 1)
		start := make(chan struct{})
		go func() {
			<-start
			item, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
			accepted <- result{item, err}
		}()
		go func() {
			<-start
			revoked <- store.DeleteSession(ctx, request.SessionTokenHash, storage.ElevationRevoked, now)
		}()
		close(start)
		first, revokeErr := <-accepted, <-revoked
		Expect(revokeErr).NotTo(HaveOccurred())
		if first.err != nil {
			Expect(first.err).To(MatchError(storage.ErrRevoked))
		}
		_, err := store.RequestRecurringWork(ctx, request, func() time.Time { return request.Now })
		Expect(err).To(MatchError(storage.ErrRevoked))
		if first.err == nil {
			current, err := store.GetQueueItem(ctx, first.item.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(current.Revision).To(Equal(first.item.Revision))
		}
	})
}
