package orgsync_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

var _ = Describe("Scheduling [Unit]", func() {
	act := func(repo string, kind orgsync.Kind, subject string) orgsync.Action {
		return orgsync.Action{RepositoryID: repo, Kind: kind, Subject: subject}
	}

	It("groups a plan's actions by repository", func() {
		work := orgsync.Schedule([]orgsync.Action{
			act("r2", orgsync.KindLabels, "a"),
			act("r1", orgsync.KindLabels, "b"),
			act("r1", orgsync.KindLabels, "c"),
		})

		Expect(work).To(HaveLen(2))
		Expect(work[0].RepositoryID).To(Equal("r1"))
		Expect(work[0].Kinds).To(HaveLen(1))
		Expect(work[0].Kinds[0].Actions).To(HaveLen(2))
		Expect(work[1].RepositoryID).To(Equal("r2"))
	})

	// Files last, because a file change opens a pull request - the only part of
	// this a person sees arrive - and it should not arrive when the rest of the
	// work on that repository failed
	It("runs the kinds in the order they are applied", func() {
		work := orgsync.Schedule([]orgsync.Action{
			act("r1", orgsync.KindFiles, "README.md"),
			act("r1", orgsync.KindRulesets, "main"),
			act("r1", orgsync.KindLabels, "bug"),
			act("r1", orgsync.KindSettings, "merge"),
		})

		Expect(work).To(HaveLen(1))

		var kinds []orgsync.Kind
		for _, item := range work[0].Kinds {
			kinds = append(kinds, item.Kind)
		}
		Expect(kinds).To(Equal(orgsync.Kinds()))
	})

	// An executor resuming after a crash would otherwise interleave
	// differently, and a person comparing two runs could not tell whether
	// anything had changed
	It("orders repositories the same way every time", func() {
		actions := []orgsync.Action{
			act("zeta", orgsync.KindLabels, "a"),
			act("alpha", orgsync.KindLabels, "a"),
			act("mu", orgsync.KindLabels, "a"),
		}

		first := orgsync.Schedule(actions)
		for range 20 {
			Expect(orgsync.Schedule(actions)).To(Equal(first))
		}
		Expect(first[0].RepositoryID).To(Equal("alpha"))
	})

	It("returns nothing for a plan with nothing to do", func() {
		Expect(orgsync.Schedule(nil)).To(BeEmpty())
	})

	Describe("an outcome", func() {
		It("is applied when everything succeeded", func() {
			var outcome orgsync.Outcome
			outcome.Apply(orgsync.Action{ID: 1})

			Expect(outcome.Failed).To(BeZero())
			Expect(outcome.State()).To(Equal(orgsync.PlanApplied))
			Expect(outcome.Actions).To(HaveLen(1))
			Expect(outcome.Actions[0].State).To(Equal(orgsync.ActionApplied))
		})

		It("carries the reason an action failed", func() {
			var outcome orgsync.Outcome
			outcome.Fail(orgsync.Action{ID: 1}, "422 invalid color")

			Expect(outcome.Failed).To(Equal(1))
			Expect(outcome.State()).To(Equal(orgsync.PlanFailed))
			Expect(outcome.Actions[0].Error).To(Equal("422 invalid color"))
		})

		// A skipped action is recorded rather than left pending, because
		// pending is work a later lease picks up and tries - and trying the
		// files of a repository whose labels just failed is what the ordering
		// exists to prevent
		It("names the kind that stopped work it never tried", func() {
			var outcome orgsync.Outcome
			outcome.Skip(orgsync.Action{ID: 2}, orgsync.KindLabels)

			Expect(outcome.Actions[0].State).To(Equal(orgsync.ActionSkipped))
			Expect(outcome.Actions[0].Blocker).To(Equal(orgsync.KindLabels))
			Expect(outcome.Failed).To(Equal(1))
		})

		// A plan's verdict is about the plan, not about whichever attempt
		// happened to close it. A retry that found everything already settled
		// counts no failures of its own, and without carrying the earlier one
		// it would close a failed plan as applied - reporting success for work
		// that never happened, and recording the digest that says so
		It("closes as failed when an earlier attempt failed and this one did nothing", func() {
			var outcome orgsync.Outcome
			outcome.Carry(orgsync.Action{ID: 1, State: orgsync.ActionApplied})
			outcome.Carry(orgsync.Action{ID: 2, State: orgsync.ActionFailed})

			Expect(outcome.State()).To(Equal(orgsync.PlanFailed))
		})

		It("stays applied when everything an earlier attempt settled succeeded", func() {
			var outcome orgsync.Outcome
			outcome.Carry(orgsync.Action{ID: 1, State: orgsync.ActionApplied})

			Expect(outcome.State()).To(Equal(orgsync.PlanApplied))
		})

		// Deletion is off by default and destroys something somebody may have
		// made by hand, so it is counted on its own and audited on its own
		It("counts a removal separately from the rest", func() {
			var outcome orgsync.Outcome
			outcome.Apply(orgsync.Action{ID: 1, Operation: orgsync.OperationCreate})
			outcome.Apply(orgsync.Action{ID: 2, Operation: orgsync.OperationDelete})

			Expect(outcome.Succeeded).To(Equal(2))
			Expect(outcome.Deleted).To(Equal(1))
		})

		// A file action opens or updates a pull request, so nothing has been
		// removed from the repository yet. The count this feeds writes the
		// audit entry that exists to make destruction visible, and reporting
		// one for a proposal nobody merged is reporting a thing that did not
		// happen.
		It("counts nothing removed for a kind that only proposes", func() {
			var outcome orgsync.Outcome
			outcome.Apply(orgsync.Action{
				ID: 1, Kind: orgsync.KindFiles, Operation: orgsync.OperationDelete,
			})

			Expect(outcome.Succeeded).To(Equal(1))
			Expect(outcome.Deleted).To(BeZero())
		})
	})
})
