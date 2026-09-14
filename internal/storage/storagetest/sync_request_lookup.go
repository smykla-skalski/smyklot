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

func declareSyncRequestLookupSpecs(runtime func() (context.Context, storage.Store, time.Time, workqueue.RecurringRequest, orgsync.RequestHistoryQuery)) {
	It("looks up exact acceptance without scanning history or starting work", func() {
		ctx, store, now, _, query := runtime()
		clock := func() time.Time { return now }
		query.Limit = 10
		page, err := store.ListSyncRequests(ctx, query, clock)
		Expect(err).NotTo(HaveOccurred())
		for _, want := range page.Items {
			lookup := orgsync.RequestLookup{ActorID: query.ActorID, TargetID: query.TargetID, SessionTokenHash: query.SessionTokenHash, Action: want.Action, RequestKey: want.RequestKey}
			got, err := store.GetSyncRequest(ctx, lookup, clock)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(want))
			lookup.RequestKey = "never-accepted"
			_, err = store.GetSyncRequest(ctx, lookup, clock)
			Expect(err).To(MatchError(storage.ErrNotFound))
			lookup.RequestKey, lookup.Action = want.RequestKey, "invalid"
			_, err = store.GetSyncRequest(ctx, lookup, clock)
			Expect(err).To(MatchError(storage.ErrConflict))
		}
		after, err := store.ListSyncRequests(ctx, query, clock)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(page))
	})

	It("withholds an exact receipt when the session expires during its read", func() {
		ctx, store, now, check, query := runtime()
		lookup := orgsync.RequestLookup{ActorID: query.ActorID, TargetID: query.TargetID, SessionTokenHash: query.SessionTokenHash, Action: "check", RequestKey: check.RequestKey}
		calls := 0
		got, err := store.GetSyncRequest(ctx, lookup, func() time.Time {
			calls++
			if calls > 1 {
				return now.Add(48 * time.Hour)
			}
			return now
		})
		Expect(err).To(MatchError(storage.ErrRevoked))
		Expect(got).To(Equal(orgsync.RequestAcceptance{}))
	})
}

func assertExactSyncRequests(ctx context.Context, store storage.Store, query orgsync.RequestHistoryQuery, items []orgsync.RequestAcceptance, clock func() time.Time) {
	GinkgoHelper()
	for _, want := range items {
		got, err := store.GetSyncRequest(ctx, orgsync.RequestLookup{ActorID: query.ActorID, TargetID: query.TargetID, SessionTokenHash: query.SessionTokenHash, Action: want.Action, RequestKey: want.RequestKey}, clock)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(want))
	}
}
