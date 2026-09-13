package orgsync_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

var _ = Describe("dispatch eligibility", func() {
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	target := "target"
	for _, tc := range []struct {
		name   string
		mutate func(*orgsync.Plan, *workqueue.Item)
		want   orgsync.DispatchReason
	}{
		{"scheduled", func(_ *orgsync.Plan, i *workqueue.Item) { i.State = workqueue.StateScheduled }, orgsync.DispatchAvailable},
		{"blocked", func(_ *orgsync.Plan, i *workqueue.Item) { i.State = workqueue.StateBlocked }, orgsync.DispatchAvailable},
		{"retrying", func(_ *orgsync.Plan, i *workqueue.Item) { i.State = workqueue.StateRetrying }, orgsync.DispatchAvailable},
		{"approved", func(*orgsync.Plan, *workqueue.Item) {}, orgsync.DispatchAvailable},
		{"approval", func(p *orgsync.Plan, _ *workqueue.Item) { p.State = orgsync.PlanComputed }, orgsync.DispatchApprovalRequired},
		{"expired before sweeper", func(p *orgsync.Plan, _ *workqueue.Item) { p.ExpiresAt = now }, orgsync.DispatchPlanExpired},
		{"expired unapproved", func(p *orgsync.Plan, _ *workqueue.Item) { p.State = orgsync.PlanComputed; p.ExpiresAt = now }, orgsync.DispatchPlanExpired},
		{"applying outlives approval", func(p *orgsync.Plan, _ *workqueue.Item) { p.State = orgsync.PlanApplying; p.ExpiresAt = now }, orgsync.DispatchAlreadyRunning},
		{"changed", func(p *orgsync.Plan, _ *workqueue.Item) { p.State = orgsync.PlanStale }, orgsync.DispatchPlanChanged},
		{"expired", func(p *orgsync.Plan, _ *workqueue.Item) { p.State = orgsync.PlanExpired }, orgsync.DispatchPlanExpired},
		{"applied", func(p *orgsync.Plan, _ *workqueue.Item) { p.State = orgsync.PlanApplied }, orgsync.DispatchPlanFinished},
		{"failed", func(p *orgsync.Plan, _ *workqueue.Item) { p.State = orgsync.PlanFailed }, orgsync.DispatchPlanFinished},
		{"discarded", func(p *orgsync.Plan, _ *workqueue.Item) { p.State = orgsync.PlanDiscarded }, orgsync.DispatchPlanFinished},
		{"unknown plan", func(p *orgsync.Plan, _ *workqueue.Item) { p.State = "future" }, orgsync.DispatchStateUnsupported},
		{"running queue", func(_ *orgsync.Plan, i *workqueue.Item) { i.State = workqueue.StateRunning }, orgsync.DispatchAlreadyRunning},
		{"finished queue", func(_ *orgsync.Plan, i *workqueue.Item) { i.State = workqueue.StateSucceeded }, orgsync.DispatchQueueFinished},
		{"queue awaiting approval", func(_ *orgsync.Plan, i *workqueue.Item) { i.State = workqueue.StateAwaitingApproval }, orgsync.DispatchStateUnsupported},
		{"unknown queue", func(_ *orgsync.Plan, i *workqueue.Item) { i.State = "future" }, orgsync.DispatchStateUnsupported},
		{"missing target", func(_ *orgsync.Plan, i *workqueue.Item) { i.TargetID = nil }, orgsync.DispatchQueueUnavailable},
		{"wrong target", func(_ *orgsync.Plan, i *workqueue.Item) { s := "other"; i.TargetID = &s }, orgsync.DispatchQueueUnavailable},
		{"wrong source", func(_ *orgsync.Plan, i *workqueue.Item) { i.SourceID = "other" }, orgsync.DispatchQueueUnavailable},
		{"wrong source kind", func(_ *orgsync.Plan, i *workqueue.Item) { i.SourceKind = "recurring" }, orgsync.DispatchQueueUnavailable},
		{"wrong kind", func(_ *orgsync.Plan, i *workqueue.Item) { i.Kind = workqueue.KindSyncScan }, orgsync.DispatchQueueUnavailable},
	} {
		It(tc.name, func() {
			p := orgsync.Plan{ID: "plan", TargetID: target, State: orgsync.PlanApproved, ExpiresAt: now.Add(time.Hour)}
			i := workqueue.Item{ID: "queue", TargetID: &target, SourceID: p.ID, SourceKind: "sync_plan", Kind: workqueue.KindSyncApply, State: workqueue.StateReady}
			tc.mutate(&p, &i)
			Expect(orgsync.PlanDispatchEligibility(p, &i, now)).To(Equal(tc.want))
		})
	}
})

var _ = Describe("dispatch eligibility without retained queue", func() {
	It("refuses to invent current queue work", func() {
		now := time.Now()
		plan := orgsync.Plan{State: orgsync.PlanApproved, ExpiresAt: now.Add(time.Hour)}
		Expect(orgsync.PlanDispatchEligibility(plan, nil, now)).To(Equal(orgsync.DispatchQueueUnavailable))
	})
})
