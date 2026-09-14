package storagetest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func declareSourcePreviewSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	requestAt := func(now time.Time) pendingci.SourceRevisionRequest {
		return pendingci.SourceRevisionRequest{
			RepositoryID: "repository-20", PullRequest: 198, CommentID: 101,
			Revision: now.Format(time.RFC3339Nano), Sequence: 2, SourceOrder: 2,
			EventKey: "source:original", ObservedAt: now,
		}
	}
	It("previews a source revision without reserving or recording it", func() {
		ctx, store, now := runtime()
		original := requestAt(now)
		preview, err := store.CheckSourceRevision(ctx, original)
		Expect(err).NotTo(HaveOccurred())
		Expect(preview).To(Equal(pendingci.SourceRevisionResult{Accepted: true, SourceOrder: 2}))
		// A different event at exactly the same position can still claim. A
		// preview that inserted a receipt would instead reject this event.
		other := original
		other.EventKey = "source:other"
		claimed, err := store.ClaimSourceRevision(ctx, other)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed.Accepted).To(BeTrue())
		preview, err = store.CheckSourceRevision(ctx, original)
		Expect(err).NotTo(HaveOccurred())
		Expect(preview.Accepted).To(BeFalse())
		claimed, err = store.ClaimSourceRevision(ctx, original)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(Equal(preview))
	})
	DescribeTable("previews source freshness using the execution ordering rules",
		func(change func(*pendingci.SourceRevisionRequest), accepted bool, order int64) {
			ctx, store, now := runtime()
			original := requestAt(now)
			_, err := store.ClaimSourceRevision(ctx, original)
			Expect(err).NotTo(HaveOccurred())
			candidate := original
			change(&candidate)
			preview, err := store.CheckSourceRevision(ctx, candidate)
			Expect(err).NotTo(HaveOccurred())
			Expect(preview).To(Equal(pendingci.SourceRevisionResult{Accepted: accepted, SourceOrder: order}))
			claimed, err := store.ClaimSourceRevision(ctx, candidate)
			Expect(err).NotTo(HaveOccurred())
			Expect(claimed).To(Equal(preview))
		},
		Entry("exact retries preserve receipt order", func(r *pendingci.SourceRevisionRequest) { r.SourceOrder = 999 }, true, int64(2)),
		Entry("distinct equal positions are rejected", func(r *pendingci.SourceRevisionRequest) { r.EventKey = "source:equal" }, false, int64(0)),
		Entry("later receipts break timestamp ties", func(r *pendingci.SourceRevisionRequest) { r.EventKey = "source:later"; r.SourceOrder = 3 }, true, int64(3)),
		Entry("earlier receipts cannot replace an edit", func(r *pendingci.SourceRevisionRequest) { r.EventKey = "source:earlier"; r.SourceOrder = 1 }, false, int64(0)),
		Entry("action sequence precedes receipt order", func(r *pendingci.SourceRevisionRequest) {
			r.EventKey = "source:created"
			r.Sequence = 1
			r.SourceOrder = 999
		}, false, int64(0)),
		Entry("timestamps precede receipt order", func(r *pendingci.SourceRevisionRequest) {
			r.EventKey = "source:old"
			r.Revision = r.ObservedAt.Add(-time.Second).Format(time.RFC3339Nano)
			r.SourceOrder = 999
		}, false, int64(0)),
		Entry("other comments are independent", func(r *pendingci.SourceRevisionRequest) { r.CommentID++; r.SourceOrder = 1 }, true, int64(1)),
		Entry("other pull requests are independent", func(r *pendingci.SourceRevisionRequest) { r.PullRequest++ }, true, int64(2)),
		Entry("other repositories are independent", func(r *pendingci.SourceRevisionRequest) { r.RepositoryID = "repository:other" }, true, int64(2)),
	)
	It("rechecks a source preview after a newer event supersedes it", func() {
		ctx, store, now := runtime()
		original := requestAt(now)
		_, err := store.ClaimSourceRevision(ctx, original)
		Expect(err).NotTo(HaveOccurred())
		preview, err := store.CheckSourceRevision(ctx, original)
		Expect(err).NotTo(HaveOccurred())
		Expect(preview.Accepted).To(BeTrue())
		newer := original
		newer.SourceOrder++
		newer.EventKey = "source:newer"
		_, err = store.ClaimSourceRevision(ctx, newer)
		Expect(err).NotTo(HaveOccurred())
		original.SourceOrder = 999 // A recovery execution ID cannot promote the old command.
		preview, err = store.CheckSourceRevision(ctx, original)
		Expect(err).NotTo(HaveOccurred())
		Expect(preview).To(Equal(pendingci.SourceRevisionResult{Accepted: false, SourceOrder: 2}))
		claimed, err := store.ClaimSourceRevision(ctx, original)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(Equal(preview))
	})
}
