package storagetest

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func claimSyncCheck(ctx context.Context, store storage.Store, target string, now time.Time) workqueue.Item {
	GinkgoHelper()
	item, claimed, err := store.ClaimRecurringWork(ctx, workqueue.RecurringClaim{
		Kind: workqueue.KindSyncScan, TargetID: &target, Title: "Check repositories", Now: now, LeaseDuration: time.Minute,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(claimed).To(BeTrue(), "claim at %s returned %#v", now, item)
	return item
}

func checkPlanInput(item workqueue.Item, target, actor string, action orgsync.Action, now time.Time) orgsync.PlanCreate {
	return orgsync.PlanCreate{
		ID: "from-check", TargetID: target, ActorID: actor, Trigger: orgsync.TriggerManual,
		Digest: "checked", Actions: []orgsync.Action{action}, Now: now, ExpiresAt: now.Add(time.Hour),
		OriginCheck: &orgsync.CheckReference{QueueID: item.ID, Attempt: item.Attempt},
	}
}

func checkResult(item workqueue.Item) string {
	GinkgoHelper()
	if len(item.Details) == 0 {
		return ""
	}
	var details workqueue.SyncScanDetails
	Expect(json.Unmarshal(item.Details, &details)).To(Succeed())
	return details.ResultPlanID
}

func verifySyncCheckResult(ctx context.Context, store storage.Store, target, actor string, action orgsync.Action, now time.Time) {
	GinkgoHelper()
	item := claimSyncCheck(ctx, store, target, now)
	create := checkPlanInput(item, target, actor, action, now)
	plan, err := store.CreateSyncPlan(ctx, create)
	Expect(err).NotTo(HaveOccurred())
	read, err := store.GetQueueItem(ctx, item.ID)
	Expect(err).NotTo(HaveOccurred())
	Expect(checkResult(read)).To(Equal(plan.ID))
	_, err = store.DiscardSyncPlan(ctx, orgsync.PlanDiscard{TargetID: target, PlanID: plan.ID, ActorID: actor, Now: now})
	Expect(err).NotTo(HaveOccurred())
	create.ID = "overwrite-result"
	_, err = store.CreateSyncPlan(ctx, create)
	Expect(err).To(MatchError(orgsync.ErrStaleCheck))
	_, _, err = store.GetSyncPlan(ctx, target, create.ID)
	Expect(err).To(MatchError(storage.ErrNotFound))
	retry, err := store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{Failure: "Lost completion response", Retryable: true}, now)
	Expect(err).NotTo(HaveOccurred())
	retried := claimSyncCheck(ctx, store, target, retry.EligibleAt)
	Expect(retried.ID).To(Equal(item.ID))
	Expect(checkResult(retried)).To(Equal(plan.ID))
	_, err = store.FinishRecurringWork(ctx, item.ID, workqueue.RecurringCompletion{}, retry.EligibleAt)
	Expect(err).NotTo(HaveOccurred())
	policy, err := store.GetEffectiveQueuePolicy(ctx, workqueue.KindSyncScan, &target)
	Expect(err).NotTo(HaveOccurred())
	next := claimSyncCheck(ctx, store, target, now.Add(policy.Cadence))
	Expect(next.ID).NotTo(Equal(item.ID))
	Expect(checkResult(next)).To(BeEmpty())
	read, err = store.GetQueueItem(ctx, item.ID)
	Expect(err).NotTo(HaveOccurred())
	Expect(checkResult(read)).To(Equal(plan.ID))
}

func verifySyncCheckFence(ctx context.Context, store storage.Store, target, actor string, action orgsync.Action, now time.Time) {
	GinkgoHelper()
	item := claimSyncCheck(ctx, store, target, now)
	for _, alter := range []func(*orgsync.PlanCreate){
		func(c *orgsync.PlanCreate) { c.OriginCheck.QueueID = "missing" },
		func(c *orgsync.PlanCreate) { c.OriginCheck.Attempt++ },
		func(c *orgsync.PlanCreate) { c.OriginCheck.Attempt = 0 },
		func(c *orgsync.PlanCreate) { c.TargetID = "another-target" },
		func(c *orgsync.PlanCreate) { c.Now = now.Add(time.Minute) },
	} {
		create := checkPlanInput(item, target, actor, action, now)
		alter(&create)
		_, err := store.CreateSyncPlan(ctx, create)
		Expect(err).To(MatchError(orgsync.ErrStaleCheck))
	}
	_, _, err := store.GetSyncPlan(ctx, target, "from-check")
	Expect(err).To(MatchError(storage.ErrNotFound))
	read, err := store.GetQueueItem(ctx, item.ID)
	Expect(err).NotTo(HaveOccurred())
	Expect(checkResult(read)).To(BeEmpty())
}

func verifySyncCheckRollback(ctx context.Context, store storage.Store, target, actor string, action orgsync.Action, now time.Time) {
	GinkgoHelper()
	item := claimSyncCheck(ctx, store, target, now)
	create := checkPlanInput(item, target, actor, action, now)
	legacy := create
	legacy.ID, legacy.OriginCheck = "other-plan", nil
	_, err := store.CreateSyncPlan(ctx, legacy)
	Expect(err).NotTo(HaveOccurred())
	_, err = store.CreateSyncPlan(ctx, create)
	Expect(err).To(MatchError(storage.ErrConflict))
	read, err := store.GetQueueItem(ctx, item.ID)
	Expect(err).NotTo(HaveOccurred())
	Expect(checkResult(read)).To(BeEmpty())
	_, _, err = store.GetSyncPlan(ctx, target, create.ID)
	Expect(err).To(MatchError(storage.ErrNotFound))
}
