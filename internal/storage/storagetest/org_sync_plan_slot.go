package storagetest

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func verifySyncPlanContention(ctx context.Context, store storage.Store, target, actor string, now time.Time) {
	GinkgoHelper()
	create := orgsync.PlanCreate{TargetID: target, ActorID: actor, Trigger: orgsync.TriggerManual, Digest: "input", Now: now, ExpiresAt: now.Add(time.Hour)}
	type result struct {
		plan orgsync.Plan
		err  error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, id := range []string{"contender-a", "contender-b"} {
		go func() {
			<-start
			input := create
			input.ID = id
			plan, err := store.CreateSyncPlan(ctx, input)
			results <- result{plan, err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	if first.err != nil {
		first, second = second, first
	}
	Expect(first.err).NotTo(HaveOccurred())
	var occupied *storage.LiveSyncPlanConflict
	Expect(errors.As(second.err, &occupied)).To(BeTrue(), "second creation: %v", second.err)
	Expect(occupied.PlanID).To(Equal(first.plan.ID))
	live, _, err := store.GetLiveSyncPlan(ctx, target)
	Expect(err).NotTo(HaveOccurred())
	Expect(live.ID).To(Equal(first.plan.ID))
	// The same identity is a collision even while its plan is live.
	create.ID = first.plan.ID
	_, err = store.CreateSyncPlan(ctx, create)
	Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
	Expect(errors.As(err, &occupied)).To(BeFalse())
	_, err = store.DiscardSyncPlan(ctx, orgsync.PlanDiscard{TargetID: target, PlanID: first.plan.ID, ActorID: actor, Now: now})
	Expect(err).NotTo(HaveOccurred())
	// A retained terminal identifier must not become a fictitious live blocker.
	_, err = store.CreateSyncPlan(ctx, create)
	Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
	Expect(errors.As(err, &occupied)).To(BeFalse())
	create.ID = "next-plan"
	_, err = store.CreateSyncPlan(ctx, create)
	Expect(err).NotTo(HaveOccurred())
}

func verifyDeferredSyncCheck(ctx context.Context, store storage.Store, target, actor string, now time.Time) {
	GinkgoHelper()
	create := orgsync.PlanCreate{ID: "earlier-plan", TargetID: target, ActorID: actor, Trigger: orgsync.TriggerManual, Digest: "input", Now: now, ExpiresAt: now.Add(time.Hour)}
	_, err := store.CreateSyncPlan(ctx, create)
	Expect(err).NotTo(HaveOccurred())
	item := claimSyncCheck(ctx, store, target, now)
	outcome := orgsync.CheckOutcome{CompletedAt: now, Disposition: "deferred", Summary: "Earlier changes are in progress", BlockingPlanID: create.ID}
	input := orgsync.CheckResultCreate{Check: orgsync.CheckReference{QueueID: item.ID, Attempt: item.Attempt}, TargetID: target, Result: orgsync.CheckResult{Outcome: outcome}, Now: now}
	// A blocker cannot be attached to an outcome claiming the check proceeded.
	input.Result.Outcome.Disposition = "checked"
	Expect(store.RecordSyncCheckResult(ctx, input)).NotTo(Succeed())
	input.Result.Outcome.Disposition = "deferred"
	Expect(store.RecordSyncCheckResult(ctx, input)).To(Succeed())
	_, err = store.DiscardSyncPlan(ctx, orgsync.PlanDiscard{TargetID: target, PlanID: create.ID, ActorID: actor, Now: now})
	Expect(err).NotTo(HaveOccurred())
	create.ID = "newer-plan"
	_, err = store.CreateSyncPlan(ctx, create)
	Expect(err).NotTo(HaveOccurred())
	read, err := store.GetQueueItem(ctx, item.ID)
	Expect(err).NotTo(HaveOccurred())
	var details orgsync.CheckDetails
	Expect(json.Unmarshal(read.Details, &details)).To(Succeed())
	Expect(details.Outcome).NotTo(BeNil())
	Expect(details.Outcome.BlockingPlanID).To(Equal("earlier-plan"))
	Expect(details.ResultPlanID).To(BeEmpty())
	input.Result.Outcome.BlockingPlanID = "newer-plan"
	Expect(store.RecordSyncCheckResult(ctx, input)).To(MatchError(orgsync.ErrStaleCheck))
}
