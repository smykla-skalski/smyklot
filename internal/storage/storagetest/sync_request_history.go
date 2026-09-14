package storagetest

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func declareSyncRequestHistorySpecs(runtime func() (context.Context, storage.Store, time.Time, orgsync.PlanDispatch, workqueue.Item)) {
	Describe("accepted sync request discovery", func() {
		var ctx context.Context
		var store storage.Store
		var now time.Time
		var dispatch orgsync.PlanDispatch
		var check workqueue.RecurringRequest
		var query orgsync.RequestHistoryQuery
		var clock func() time.Time
		BeforeEach(func() {
			ctx, store, now, dispatch, _ = runtime()
			clock = func() time.Time { return now }
			_, err := store.DispatchSyncPlan(ctx, dispatch, clock)
			Expect(err).NotTo(HaveOccurred())
			_, err = store.DiscardSyncPlan(ctx, orgsync.PlanDiscard{TargetID: dispatch.TargetID, PlanID: dispatch.PlanID, ActorID: dispatch.ActorID, Now: now})
			Expect(err).NotTo(HaveOccurred())
			check = workqueue.RecurringRequest{Kind: workqueue.KindSyncScan, TargetID: &dispatch.TargetID, RequestKey: "check-00", SessionTokenHash: dispatch.SessionTokenHash, Title: "Check settings", ActorID: dispatch.ActorID, Reason: "Verify current configuration", Now: now}
			_, err = store.RequestRecurringWork(ctx, check, clock)
			Expect(err).NotTo(HaveOccurred())
			query = orgsync.RequestHistoryQuery{ActorID: dispatch.ActorID, SessionTokenHash: dispatch.SessionTokenHash, TargetID: dispatch.TargetID, Limit: 1}
		})

		declareSyncRequestLookupSpecs(func() (context.Context, storage.Store, time.Time, workqueue.RecurringRequest, orgsync.RequestHistoryQuery) {
			return ctx, store, now, check, query
		})

		It("discovers both command effects and original identities without a request key", func() {
			query.Limit = 10
			page, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(page.Next).To(BeNil())
			Expect(page.Items).To(HaveLen(2))
			Expect(page.Items[0]).To(Equal(orgsync.RequestAcceptance{Action: "dispatch", RequestKey: dispatch.RequestKey, QueueID: "sync-plan:" + dispatch.PlanID, PlanID: dispatch.PlanID, ExpectedRevision: dispatch.ExpectedRevision, Reason: dispatch.Reason, AcceptedAt: now}))
			accepted, err := store.FindRecurringWorkRequest(ctx, check, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(page.Items[1]).To(Equal(orgsync.RequestAcceptance{Action: "check", RequestKey: check.RequestKey, QueueID: accepted.ID, Reason: check.Reason, AcceptedAt: now}))
		})

		It("pages equal timestamps without skipping commands and ignores newer inserts", func() {
			for index := 1; index <= 5; index++ {
				check.RequestKey = fmt.Sprintf("check-%02d", index)
				_, err := store.RequestRecurringWork(ctx, check, clock)
				Expect(err).NotTo(HaveOccurred())
			}
			page, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(page.Items[0].Action).To(Equal("dispatch"))
			check.RequestKey = "newer-request"
			now = now.Add(time.Second)
			check.Now = now
			_, err = store.RequestRecurringWork(ctx, check, clock)
			Expect(err).NotTo(HaveOccurred())
			var keys []string
			for page.Next != nil {
				query.After = page.Next
				page, err = store.ListSyncRequests(ctx, query, clock)
				Expect(err).NotTo(HaveOccurred())
				for _, item := range page.Items {
					keys = append(keys, item.RequestKey)
				}
			}
			Expect(keys).To(Equal([]string{"check-05", "check-04", "check-03", "check-02", "check-01", "check-00"}))
		})

		It("caps a large history page and continues without losing entries", func() {
			for index := 1; index <= 105; index++ {
				check.RequestKey = fmt.Sprintf("large-%03d", index)
				_, err := store.RequestRecurringWork(ctx, check, clock)
				Expect(err).NotTo(HaveOccurred())
			}
			query.Limit = 10000
			first, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(first.Items).To(HaveLen(100))
			Expect(first.Next).NotTo(BeNil())
			query.After = first.Next
			second, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(second.Items).To(HaveLen(7))
			Expect(second.Next).To(BeNil())
			seen := map[string]bool{}
			for _, item := range append(first.Items, second.Items...) {
				key := item.Action + ":" + item.RequestKey
				Expect(seen[key]).To(BeFalse())
				seen[key] = true
			}
		})

		It("does not expose another workspace or unrelated recurring work", func() {
			other := testInstallation(testAccount(now), now, nil)
			other.TargetID, other.InstallationID = "github:installation:101", "101"
			Expect(store.ReconcileInstallation(ctx, other)).To(Succeed())
			query.TargetID = other.TargetID
			empty, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(empty.Items).To(BeEmpty())
			_, err = store.GetSyncRequest(ctx, orgsync.RequestLookup{ActorID: query.ActorID, TargetID: query.TargetID, SessionTokenHash: query.SessionTokenHash, Action: "check", RequestKey: check.RequestKey}, clock)
			Expect(err).To(MatchError(storage.ErrNotFound))
			Expect(empty.Next).To(BeNil())
			unrelated := check
			unrelated.Kind, unrelated.RequestKey = workqueue.KindPathRefresh, "unrelated-request"
			_, err = store.RequestRecurringWork(ctx, unrelated, clock)
			Expect(err).NotTo(HaveOccurred())
			query.TargetID, query.Limit = dispatch.TargetID, 10
			page, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(page.Items).To(HaveLen(2))
		})

		It("rejects continuation reused for a different actor or workspace", func() {
			page, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			query.After = page.Next
			query.After.TargetID = "another-workspace"
			_, err = store.ListSyncRequests(ctx, query, clock)
			Expect(err).To(MatchError(storage.ErrConflict))
			query.After.TargetID = query.TargetID
			query.After.ActorID = "another-actor"
			_, err = store.ListSyncRequests(ctx, query, clock)
			Expect(err).To(MatchError(storage.ErrConflict))
		})

		It("keeps accepted history after queue pruning without issuing another command", func() {
			query.Limit = 10
			before, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			item, found, err := store.ClaimRecurringWork(ctx, workqueue.RecurringClaim{Kind: check.Kind, TargetID: check.TargetID, Title: check.Title, Now: now, LeaseDuration: time.Minute})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			_, err = store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{Attempt: item.Attempt}, now)
			Expect(err).NotTo(HaveOccurred())
			_, err = store.PruneWorkQueue(ctx, now.AddDate(2, 0, 0))
			Expect(err).NotTo(HaveOccurred())
			_, err = store.GetQueueItem(ctx, item.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
			after, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(before))
			assertExactSyncRequests(ctx, store, query, before.Items, clock)
			_, err = store.GetQueueItem(ctx, item.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		It("keeps accepted check evidence independently of queue pruning", func() {
			query.Limit = 10
			before, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			item, found, err := store.ClaimRecurringWork(ctx, workqueue.RecurringClaim{Kind: check.Kind, TargetID: check.TargetID, Title: check.Title, Now: now, LeaseDuration: time.Minute})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			result := orgsync.CheckResult{
				Outcome:      orgsync.CheckOutcome{CompletedAt: now, Disposition: "checked", Summary: "Reused the earlier label comparison", Counts: map[orgsync.Observation]int{}},
				Observations: []orgsync.CheckObservation{{RepositoryID: "historical-repository", Repository: "owner/original-name", Kind: orgsync.KindLabels, Outcome: orgsync.ObservationMatched, ObservedAt: now.Add(-time.Hour), InputDigest: "original-input", Cached: true}},
			}
			result.Outcome.Cached = 1
			Expect(store.RecordSyncCheckResult(ctx, orgsync.CheckResultCreate{Check: orgsync.CheckReference{QueueID: item.ID, Attempt: item.Attempt}, TargetID: *check.TargetID, Result: result, Now: now})).To(Succeed())
			_, err = store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{Attempt: item.Attempt, SuccessSummary: result.Outcome.Summary}, now)
			Expect(err).NotTo(HaveOccurred())
			evidence, err := store.ListSyncCheckObservations(ctx, query.TargetID, item.ID, 0, 10)
			Expect(err).NotTo(HaveOccurred())
			Expect(evidence.Items).To(Equal(result.Observations))
			removed, err := store.PruneWorkQueue(ctx, now.AddDate(2, 0, 0))
			Expect(err).NotTo(HaveOccurred())
			Expect(removed).To(BeNumerically(">", 0))
			_, err = store.GetQueueItem(ctx, item.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
			after, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(before))
			assertExactSyncRequests(ctx, store, query, before.Items, clock)
			retained, err := store.ListSyncCheckObservations(ctx, query.TargetID, item.ID, 0, 10)
			Expect(err).NotTo(HaveOccurred(), "accepted check history must retain its evidence when worker rows are pruned")
			Expect(retained).To(Equal(evidence))
			summary, err := store.GetSyncCheckResult(ctx, query.TargetID, item.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(summary.Outcome).To(Equal(&result.Outcome))
			Expect(summary.ResultPlanID).To(BeEmpty())
			_, err = store.GetSyncCheckResult(ctx, "another-workspace", item.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
			_, err = store.ListSyncCheckObservations(ctx, "another-workspace", item.ID, 0, 10)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		It("requires a current matching session even for empty history", func() {
			query.SessionTokenHash = "missing-session"
			_, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).To(MatchError(storage.ErrRevoked))
			query.SessionTokenHash = dispatch.SessionTokenHash
			query.ActorID = "different-actor"
			_, err = store.ListSyncRequests(ctx, query, clock)
			Expect(err).To(MatchError(storage.ErrRevoked))
		})

		It("withholds results when authority expires during the read", func() {
			calls := 0
			page, err := store.ListSyncRequests(ctx, query, func() time.Time {
				calls++
				if calls > 1 {
					return now.Add(48 * time.Hour)
				}
				return now
			})
			Expect(err).To(MatchError(storage.ErrRevoked))
			Expect(page.Items).To(BeEmpty())
		})

		It("scopes history to its actor and permits read-only workspace access", func() {
			actor := testAccount(now)
			actor.ID, actor.SubjectID = "history-viewer", "history-viewer"
			Expect(store.UpsertAccount(ctx, actor)).To(Succeed())
			_, err := store.CreatePanelUser(ctx, storage.PanelUserCreate{AccountID: actor.ID, ActorAccountID: dispatch.ActorID, ChangedAt: now})
			Expect(err).NotTo(HaveOccurred())
			role := storage.InstallationRoleAdmin
			_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{TargetID: query.TargetID, SubjectAccountID: actor.ID, ActorAccountID: dispatch.ActorID, Role: &role, ChangedAt: now})
			Expect(err).NotTo(HaveOccurred())
			Expect(store.CreateSession(ctx, storage.Session{TokenHash: "history-viewer-session", AccountID: actor.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, 2)).To(Succeed())
			check.ActorID, check.SessionTokenHash = actor.ID, "history-viewer-session"
			_, err = store.RequestRecurringWork(ctx, check, clock)
			Expect(err).NotTo(HaveOccurred())
			role = storage.InstallationRoleViewer
			_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{TargetID: query.TargetID, SubjectAccountID: actor.ID, ActorAccountID: dispatch.ActorID, Role: &role, ExpectedRevision: 1, ChangedAt: now})
			Expect(err).NotTo(HaveOccurred())
			query.ActorID, query.SessionTokenHash, query.Limit = actor.ID, check.SessionTokenHash, 10
			page, err := store.ListSyncRequests(ctx, query, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(page.Items).To(HaveLen(1))
			Expect(page.Items[0].Action).To(Equal("check"))
			lookup := orgsync.RequestLookup{ActorID: query.ActorID, TargetID: query.TargetID, SessionTokenHash: query.SessionTokenHash, Action: "check", RequestKey: check.RequestKey}
			got, err := store.GetSyncRequest(ctx, lookup, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(page.Items[0]))
			lookup.Action, lookup.RequestKey = "dispatch", dispatch.RequestKey
			_, err = store.GetSyncRequest(ctx, lookup, clock)
			Expect(err).To(MatchError(storage.ErrNotFound))
			_, err = store.RequestRecurringWork(ctx, check, clock)
			Expect(err).To(MatchError(storage.ErrRevoked))
			_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{TargetID: query.TargetID, SubjectAccountID: actor.ID, ActorAccountID: dispatch.ActorID, Role: &role, Suspended: true, ExpectedRevision: 2, ChangedAt: now})
			Expect(err).NotTo(HaveOccurred())
			_, err = store.ListSyncRequests(ctx, query, clock)
			Expect(err).To(MatchError(storage.ErrRevoked))
		})
	})
}
