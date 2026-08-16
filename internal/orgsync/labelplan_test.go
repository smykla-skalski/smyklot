package orgsync_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

var _ = Describe("Label planning [Unit]", func() {
	const repo = "github:repository:1"

	plan := func(
		config orgsync.LabelConfig, current []orgsync.CurrentLabel, exclude orgsync.Excludes,
	) []orgsync.Action {
		return orgsync.PlanLabels(repo, config, current, exclude)
	}

	subjects := func(actions []orgsync.Action) []string {
		out := make([]string, 0, len(actions))
		for _, action := range actions {
			out = append(out, string(action.Operation)+" "+action.Subject)
		}

		return out
	}

	It("creates a label the repository does not have", func() {
		actions := plan(
			orgsync.LabelConfig{Labels: []orgsync.Label{{Name: "bug", Color: "d73a4a"}}},
			nil, orgsync.Excludes{},
		)

		Expect(actions).To(HaveLen(1))
		Expect(actions[0].Operation).To(Equal(orgsync.OperationCreate))
		Expect(actions[0].Kind).To(Equal(orgsync.KindLabels))
		Expect(actions[0].RepositoryID).To(Equal(repo))
		Expect(actions[0].State).To(Equal(orgsync.ActionPending))
		Expect(actions[0].Before).To(BeEmpty())
		Expect(actions[0].After).To(Equal("bug #d73a4a"))
	})

	// A steady state has to cost nothing. A planner that proposed work every
	// tick would make the reconcile a hundred and sixty API calls that change
	// nothing, and would drown the real drift in noise
	It("proposes nothing when the repository already matches", func() {
		Expect(plan(
			orgsync.LabelConfig{Labels: []orgsync.Label{
				{Name: "bug", Color: "d73a4a", Description: text("Broken")},
			}},
			[]orgsync.CurrentLabel{{Name: "bug", Color: "d73a4a", Description: "Broken"}},
			orgsync.Excludes{},
		)).To(BeEmpty())
	})

	// The action is the contract between what somebody read and what runs, so
	// it has to carry the whole answer. Re-reading the configuration when the
	// work runs would apply what it says then, not what was approved
	It("carries what to apply on the action", func() {
		actions := plan(
			orgsync.LabelConfig{Labels: []orgsync.Label{
				{Name: "bug", Color: "D73A4A", Description: text("Broken")},
			}},
			nil, orgsync.Excludes{},
		)

		label, err := orgsync.DecodeLabel(actions[0].Payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(label).To(Equal(orgsync.ResolvedLabel{
			Name: "bug", Color: "d73a4a", Description: "Broken",
		}))
	})

	// "Leave the description alone" is turned into a value here rather than at
	// apply time, because the endpoint replaces whatever it is sent - so the
	// description that will be sent has to be one somebody can read in the plan
	It("resolves a description it was told to leave alone", func() {
		actions := plan(
			orgsync.LabelConfig{Labels: []orgsync.Label{{Name: "bug", Color: "000000"}}},
			[]orgsync.CurrentLabel{
				{Name: "bug", Color: "d73a4a", Description: "written by somebody here"},
			},
			orgsync.Excludes{},
		)

		Expect(actions).To(HaveLen(1))
		label, err := orgsync.DecodeLabel(actions[0].Payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(label.Description).To(Equal("written by somebody here"))
		Expect(actions[0].After).To(Equal("bug #000000 - written by somebody here"))
	})

	// The subject is the whole of the instruction, and a payload would be a
	// second answer nothing reads
	It("carries no payload on a deletion", func() {
		actions := plan(
			orgsync.LabelConfig{AllowRemoval: true},
			[]orgsync.CurrentLabel{{Name: "wontfix", Color: "ffffff"}},
			orgsync.Excludes{},
		)

		Expect(actions[0].Payload).To(BeEmpty())
	})

	It("updates a label whose colour drifted", func() {
		actions := plan(
			orgsync.LabelConfig{Labels: []orgsync.Label{{Name: "bug", Color: "d73a4a"}}},
			[]orgsync.CurrentLabel{{Name: "bug", Color: "000000"}},
			orgsync.Excludes{},
		)

		Expect(actions).To(HaveLen(1))
		Expect(actions[0].Operation).To(Equal(orgsync.OperationUpdate))
		Expect(actions[0].Before).To(Equal("bug #000000"))
		Expect(actions[0].After).To(Equal("bug #d73a4a"))
	})

	// GitHub answers in whatever case it stored, and configuration is what
	// somebody typed. Treating those as different would rewrite the same label
	// on every tick for ever
	It("reads a colour that differs only in case as unchanged", func() {
		Expect(plan(
			orgsync.LabelConfig{Labels: []orgsync.Label{{Name: "bug", Color: "D73A4A"}}},
			[]orgsync.CurrentLabel{{Name: "bug", Color: "d73a4a"}},
			orgsync.Excludes{},
		)).To(BeEmpty())
	})

	// The bug that made the tool this replaces unusable with descriptions: it
	// typed Description as a string, so an entry that said nothing about the
	// description sent an empty one and wiped what the repository had written
	It("leaves a description configuration says nothing about", func() {
		Expect(plan(
			orgsync.LabelConfig{Labels: []orgsync.Label{{Name: "bug", Color: "d73a4a"}}},
			[]orgsync.CurrentLabel{
				{Name: "bug", Color: "d73a4a", Description: "written by somebody here"},
			},
			orgsync.Excludes{},
		)).To(BeEmpty())
	})

	It("clears a description configuration sets to empty", func() {
		actions := plan(
			orgsync.LabelConfig{Labels: []orgsync.Label{
				{Name: "bug", Color: "d73a4a", Description: text("")},
			}},
			[]orgsync.CurrentLabel{{Name: "bug", Color: "d73a4a", Description: "old"}},
			orgsync.Excludes{},
		)

		Expect(actions).To(HaveLen(1))
		Expect(actions[0].After).To(Equal("bug #d73a4a"))
	})

	It("renames a label GitHub stored in another case", func() {
		actions := plan(
			orgsync.LabelConfig{Labels: []orgsync.Label{{Name: "Bug", Color: "d73a4a"}}},
			[]orgsync.CurrentLabel{{Name: "bug", Color: "d73a4a"}},
			orgsync.Excludes{},
		)

		Expect(actions).To(HaveLen(1))
		Expect(actions[0].Operation).To(Equal(orgsync.OperationUpdate))
	})

	Describe("removal", func() {
		surplus := []orgsync.CurrentLabel{{Name: "wontfix", Color: "ffffff"}}

		// Off unless somebody switched it on, because it destroys something a
		// person may have made by hand
		It("leaves a label configuration does not name", func() {
			Expect(plan(
				orgsync.LabelConfig{Labels: []orgsync.Label{{Name: "bug", Color: "d73a4a"}}},
				append(surplus, orgsync.CurrentLabel{Name: "bug", Color: "d73a4a"}),
				orgsync.Excludes{},
			)).To(BeEmpty())
		})

		It("proposes deleting one when removal is switched on", func() {
			actions := plan(
				orgsync.LabelConfig{
					Labels:       []orgsync.Label{{Name: "bug", Color: "d73a4a"}},
					AllowRemoval: true,
				},
				append(surplus, orgsync.CurrentLabel{Name: "bug", Color: "d73a4a"}),
				orgsync.Excludes{},
			)

			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationDelete))
			Expect(actions[0].Subject).To(Equal("wontfix"))
			Expect(actions[0].Before).To(Equal("wontfix #ffffff"))
			Expect(actions[0].After).To(BeEmpty())
		})

		// The tool this replaces built deletions by ranging a map, so a rename
		// could issue the create before the delete, 422 against the label it
		// was about to remove, and do it differently on every run
		It("puts every deletion after every create", func() {
			actions := plan(
				orgsync.LabelConfig{
					Labels:       []orgsync.Label{{Name: "kind/bug", Color: "d73a4a"}},
					AllowRemoval: true,
				},
				[]orgsync.CurrentLabel{{Name: "bug", Color: "d73a4a"}},
				orgsync.Excludes{},
			)

			Expect(subjects(actions)).To(Equal([]string{"create kind/bug", "delete bug"}))
		})

		// Two plans of the same state must be the same plan, or comparing
		// digests means nothing and two runs cannot be told apart
		It("orders deletions the same way every time", func() {
			current := []orgsync.CurrentLabel{
				{Name: "zeta"}, {Name: "alpha"}, {Name: "mu"}, {Name: "beta"},
			}

			first := subjects(plan(
				orgsync.LabelConfig{AllowRemoval: true}, current, orgsync.Excludes{},
			))
			Expect(first).To(Equal([]string{
				"delete alpha", "delete beta", "delete mu", "delete zeta",
			}))

			for range 20 {
				Expect(subjects(plan(
					orgsync.LabelConfig{AllowRemoval: true}, current, orgsync.Excludes{},
				))).To(Equal(first))
			}
		})
	})

	Describe("exclusions", func() {
		It("leaves an excluded label alone rather than creating it", func() {
			Expect(plan(
				orgsync.LabelConfig{Labels: []orgsync.Label{{Name: "ci/lint", Color: "d73a4a"}}},
				nil,
				orgsync.Excludes{Patterns: []string{"ci/*"}},
			)).To(BeEmpty())
		})

		It("leaves an excluded label alone rather than deleting it", func() {
			Expect(plan(
				orgsync.LabelConfig{AllowRemoval: true},
				[]orgsync.CurrentLabel{{Name: "ci/lint"}},
				orgsync.Excludes{Patterns: []string{"ci/*"}},
			)).To(BeEmpty())
		})

		It("still acts on a label the pattern does not cover", func() {
			Expect(plan(
				orgsync.LabelConfig{Labels: []orgsync.Label{{Name: "bug", Color: "d73a4a"}}},
				nil,
				orgsync.Excludes{Patterns: []string{"ci/*"}},
			)).To(HaveLen(1))
		})
	})
})
