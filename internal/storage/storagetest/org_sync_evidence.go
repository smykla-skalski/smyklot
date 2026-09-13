package storagetest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func verifySyncCheckEvidence(ctx context.Context, store storage.Store, target string, now time.Time) {
	GinkgoHelper()
	item := claimSyncCheck(ctx, store, target, now)
	result := orgsync.CheckResult{Outcome: orgsync.CheckOutcome{CompletedAt: now, Disposition: "checked", Summary: "23 checks matched saved settings", Counts: map[orgsync.Observation]int{orgsync.ObservationMatched: 23}}}
	for index := range 23 {
		result.Observations = append(result.Observations, orgsync.CheckObservation{RepositoryID: fmt.Sprintf("repo-%02d", index), Repository: fmt.Sprintf("owner/historical-%02d", index), Kind: orgsync.KindLabels, Outcome: orgsync.ObservationMatched, ObservedAt: now, InputDigest: "checked-input"})
	}
	create := orgsync.CheckResultCreate{Check: orgsync.CheckReference{QueueID: item.ID, Attempt: item.Attempt}, TargetID: target, Result: result, Now: now}
	invalid := create
	invalid.Result.Outcome.Counts = map[orgsync.Observation]int{orgsync.ObservationFailed: 23}
	Expect(store.RecordSyncCheckResult(ctx, invalid)).NotTo(Succeed())
	Expect(store.RecordSyncCheckResult(ctx, create)).To(Succeed())
	read, err := store.GetQueueItem(ctx, item.ID)
	Expect(err).NotTo(HaveOccurred())
	var details orgsync.CheckDetails
	Expect(json.Unmarshal(read.Details, &details)).To(Succeed())
	Expect(details.Outcome).To(Equal(&result.Outcome))
	Expect(details.ResultPlanID).To(BeEmpty())
	Expect(len(read.Details)).To(BeNumerically("<", 1000))
	Expect(store.RecordSyncCheckResult(ctx, create)).To(MatchError(orgsync.ErrStaleCheck))
	_, err = store.ListSyncCheckObservations(ctx, "another-workspace", item.ID, 0, 10)
	Expect(err).To(MatchError(storage.ErrNotFound))
	_, err = store.ListSyncCheckObservations(ctx, target, item.ID, -1, 10)
	Expect(err).To(HaveOccurred())
	var collected []orgsync.CheckObservation
	cursor := 0
	for {
		page, err := store.ListSyncCheckObservations(ctx, target, item.ID, cursor, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Total).To(Equal(23))
		Expect(len(page.Items)).To(BeNumerically("<=", 10))
		collected = append(collected, page.Items...)
		if page.Next == nil {
			break
		}
		Expect(*page.Next).To(BeNumerically(">", cursor))
		cursor = *page.Next
	}
	Expect(collected).To(Equal(result.Observations))
	retry, err := store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{Attempt: item.Attempt, Failure: "Lost completion", Retryable: true}, now)
	Expect(err).NotTo(HaveOccurred())
	retried := claimSyncCheck(ctx, store, target, retry.EligibleAt)
	create.Check.Attempt, create.Now = retried.Attempt, retry.EligibleAt
	Expect(store.RecordSyncCheckResult(ctx, create)).To(MatchError(orgsync.ErrStaleCheck))
	page, err := store.ListSyncCheckObservations(ctx, target, item.ID, 20, 10)
	Expect(err).NotTo(HaveOccurred())
	Expect(page.Items).To(Equal(result.Observations[20:]))
	_, err = store.FinishRecurringWork(ctx, retried.ID, workqueue.RecurringCompletion{Attempt: retried.Attempt, SuccessSummary: result.Outcome.Summary}, retry.EligibleAt)
	Expect(err).NotTo(HaveOccurred())
	policy, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindSyncScan, &target)
	Expect(err).NotTo(HaveOccurred())
	next := claimSyncCheck(ctx, store, target, now.Add(policy.Cadence))
	_, err = store.ListSyncCheckObservations(ctx, target, next.ID, 0, 10)
	Expect(err).To(MatchError(storage.ErrNotFound))
}
