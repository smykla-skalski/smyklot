package orgsync_test

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

func TestOrgSync(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Org sync")
}

func text(value string) *string { return &value }

var _ = Describe("Label configuration [Unit]", func() {
	config := func(labels ...orgsync.Label) orgsync.LabelConfig {
		return orgsync.LabelConfig{Labels: labels}
	}

	It("accepts what GitHub accepts", func() {
		Expect(config(
			orgsync.Label{Name: "bug", Color: "d73a4a", Description: text("Something broken")},
			orgsync.Label{Name: "kind/feature", Color: "A2EEEF"},
		).Validate()).To(Succeed())
	})

	// Every entry below is a refusal GitHub would have made at apply time,
	// where a 422 abandoned every remaining label on that repository. Answering
	// here means answering beside the field somebody typed it in.
	DescribeTable("refuses configuration GitHub would refuse",
		func(labels []orgsync.Label, because string) {
			err := orgsync.LabelConfig{Labels: labels}.Validate()

			Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
			Expect(err.Error()).To(ContainSubstring(because))
		},
		Entry("a name nobody wrote",
			[]orgsync.Label{{Color: "d73a4a"}}, "has no name"),
		Entry("a colour nobody wrote",
			[]orgsync.Label{{Name: "bug"}}, "has no color"),
		// The mistake somebody actually makes, having copied it out of a
		// stylesheet, so it gets its own message with the answer in it
		Entry("a colour copied out of CSS",
			[]orgsync.Label{{Name: "bug", Color: "#d73a4a"}}, `GitHub wants "d73a4a"`),
		Entry("a colour that is a word",
			[]orgsync.Label{{Name: "bug", Color: "blue"}}, "six hexadecimal digits"),
		Entry("a colour one digit short",
			[]orgsync.Label{{Name: "bug", Color: "12345"}}, "six hexadecimal digits"),
		Entry("a colour that is not hexadecimal",
			[]orgsync.Label{{Name: "bug", Color: "gggggg"}}, "not hexadecimal"),
		// GitHub trims it, so " bug" would be created as "bug", look missing on
		// the next reconcile, and be created again every tick for ever
		Entry("a name GitHub would trim",
			[]orgsync.Label{{Name: " bug", Color: "d73a4a"}}, "whitespace"),
		Entry("a name past the limit",
			[]orgsync.Label{{
				Name:  "0123456789012345678901234567890123456789012345678901",
				Color: "d73a4a",
			}}, "longer than 50"),
		Entry("a description past the limit",
			[]orgsync.Label{{
				Name: "bug", Color: "d73a4a",
				Description: text(string(make([]byte, 101))),
			}}, "longer than 100"),
	)

	// The duplicate reached the apply loop, which iterated the slice while the
	// diff had deduplicated into a map - so it issued two creates and the
	// second 422'd, taking the rest of the repository with it
	It("refuses the same label twice", func() {
		err := config(
			orgsync.Label{Name: "bug", Color: "d73a4a"},
			orgsync.Label{Name: "bug", Color: "000000"},
		).Validate()

		Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
		Expect(err.Error()).To(ContainSubstring(`"bug" is listed twice`))
	})

	// GitHub stores the case it is given and refuses to create "Bug" beside
	// "bug", so a configuration carrying both cannot be applied - and it would
	// fail on whichever came second, which differs per repository
	It("refuses two labels that differ only in case", func() {
		err := config(
			orgsync.Label{Name: "bug", Color: "d73a4a"},
			orgsync.Label{Name: "Bug", Color: "000000"},
		).Validate()

		Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
		Expect(err.Error()).To(ContainSubstring("differ only in case"))
	})

	It("names which entry is wrong when the name is missing", func() {
		err := config(
			orgsync.Label{Name: "bug", Color: "d73a4a"},
			orgsync.Label{Color: "000000"},
		).Validate()

		Expect(err.Error()).To(ContainSubstring("label 2"))
	})

	It("reports every configured name in order", func() {
		Expect(config(
			orgsync.Label{Name: "bug", Color: "d73a4a"},
			orgsync.Label{Name: "chore", Color: "000000"},
		).Names()).To(Equal([]string{"bug", "chore"}))
	})

	It("is an ErrInvalidConfig, whatever the reason", func() {
		Expect(errors.Is(config(orgsync.Label{}).Validate(), orgsync.ErrInvalidConfig)).
			To(BeTrue())
	})
})
