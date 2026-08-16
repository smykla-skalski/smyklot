package orgsync_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

var _ = Describe("Exclusions [Unit]", func() {
	matches := func(pattern, subject string) bool {
		return orgsync.Excludes{Patterns: []string{pattern}}.Matches(subject)
	}

	DescribeTable("matches what somebody meant",
		func(pattern, subject string, expected bool) {
			Expect(matches(pattern, subject)).To(Equal(expected))
		},
		Entry("a literal name", "bug", "bug", true),
		Entry("a literal name that differs", "bug", "chore", false),

		// The whole reason this exists. The tool this replaces compared with
		// `==`, so this entry excluded a label literally called "ci/*" and
		// nothing else, and nothing ever said so
		Entry("a prefix pattern", "ci/*", "ci/lint", true),
		Entry("a prefix pattern against another prefix", "ci/*", "kind/bug", false),
		Entry("a prefix pattern against the bare prefix", "ci/*", "ci/", true),

		Entry("a suffix pattern", "*-wip", "feature-wip", true),
		Entry("a suffix pattern that does not reach", "*-wip", "wip-feature", false),

		Entry("a pattern in the middle", "kind/*/wip", "kind/bug/wip", true),
		Entry("two stars", "*bug*", "a bug here", true),
		Entry("two stars that do not both land", "*bug*", "a beetle here", false),

		Entry("everything", "*", "anything at all", true),
		Entry("everything, against nothing", "*", "", true),

		// `*` crosses `/` here, unlike a path glob. A label is not a path, and
		// borrowing filepath.Match's separator rule would make this read wider
		// than it behaves
		Entry("a star crossing a slash", "kind*", "kind/nested/deep", true),

		// A literal that happens to contain regular-expression punctuation must
		// still be a literal. Compiling patterns into a regular expression is
		// how `v1.0` comes to match `v1x0`
		Entry("a name with a dot", "v1.0", "v1x0", false),
		Entry("a name with a dot, exactly", "v1.0", "v1.0", true),
		Entry("a name with a plus", "c++", "c++", true),
	)

	It("matches when any one pattern does", func() {
		excludes := orgsync.Excludes{Patterns: []string{"ci/*", "wontfix"}}

		Expect(excludes.Matches("wontfix")).To(BeTrue())
		Expect(excludes.Matches("ci/lint")).To(BeTrue())
		Expect(excludes.Matches("bug")).To(BeFalse())
	})

	It("matches nothing when there are no patterns", func() {
		Expect(orgsync.Excludes{}.Matches("bug")).To(BeFalse())
	})

	Describe("validation", func() {
		It("accepts patterns somebody could have meant", func() {
			Expect(orgsync.Excludes{Patterns: []string{"ci/*", "bug", "*"}}.Validate()).
				To(Succeed())
		})

		It("refuses an empty pattern", func() {
			err := orgsync.Excludes{Patterns: []string{"ci/*", "  "}}.Validate()

			Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
			Expect(err.Error()).To(ContainSubstring("exclusion 2 is empty"))
		})
	})
})
