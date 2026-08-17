package filemerge_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
)

var _ = Describe("Validating a merge [Unit]", func() {
	It("accepts a structured merge", func() {
		Expect(filemerge.Spec{
			Overrides:   overrides(`{"timezone": "Europe/Warsaw", "labels": ["chore"]}`),
			Arrays:      []filemerge.ArrayRule{{Path: "$.labels", Strategy: filemerge.ArrayAppend}},
			Deduplicate: true,
		}.Validate("renovate.json")).To(Succeed())
	})

	It("accepts a Markdown merge", func() {
		Expect(filemerge.Spec{Sections: []filemerge.Section{
			{Action: filemerge.SectionDelete, Heading: "## Setup"},
		}}.Validate("CONTRIBUTING.md")).To(Succeed())
	})

	// A file this cannot merge is refused where it is configured. The engine
	// this replaces discovered it at apply time and wrote the raw template over
	// the repository's copy.
	DescribeTable("refuses a file it has no way to merge",
		func(path string) {
			err := filemerge.Spec{Overrides: overrides(`{"a": 1}`)}.Validate(path)

			Expect(err).To(MatchError(filemerge.ErrUnsupportedFormat))
		},
		Entry("no extension at all", "LICENSE"),
		Entry("an extension nothing here reads", "notes.txt"),
		Entry("a dotfile that only looks like one", ".gitignore"),
	)

	DescribeTable("refuses a merge nobody should be able to configure",
		func(spec filemerge.Spec, path, because string) {
			err := spec.Validate(path)

			Expect(err).To(MatchError(filemerge.ErrInvalidSpec))
			Expect(err.Error()).To(ContainSubstring(because))
		},

		Entry("a strategy this version does not know",
			filemerge.Spec{Strategy: "overlay", Overrides: overrides(`{}`)},
			"renovate.json", `unknown strategy "overlay"`),
		Entry("editing a JSON file by its headings",
			filemerge.Spec{Strategy: filemerge.StrategyMarkdown},
			"renovate.json", "is not Markdown"),
		Entry("merging a Markdown file by its keys",
			filemerge.Spec{Strategy: filemerge.StrategyDeep},
			"CONTRIBUTING.md", "which has no keys to merge"),
		Entry("sections on a file that has no headings",
			filemerge.Spec{Sections: []filemerge.Section{
				{Action: filemerge.SectionAppend, Content: "x"},
			}},
			"renovate.json", "sections edit Markdown headings"),
		Entry("overrides on a file that has no keys",
			filemerge.Spec{Overrides: overrides(`{"a": 1}`), Sections: []filemerge.Section{
				{Action: filemerge.SectionAppend, Content: "x"},
			}},
			"CONTRIBUTING.md", "not by keys and lists"),
		Entry("overrides that are not an object",
			filemerge.Spec{Overrides: overrides(`[1, 2]`)},
			"renovate.json", "not an object"),
		Entry("deduplication with no list to deduplicate",
			filemerge.Spec{Overrides: overrides(`{}`), Deduplicate: true},
			"renovate.json", "nothing is deduplicated without a list rule"),

		// Running it would re-render the file and propose a reordered,
		// comment-stripped copy of it as a change nobody asked for.
		Entry("a merge with nothing to merge",
			filemerge.Spec{Strategy: filemerge.StrategyDeep},
			"renovate.json", "nothing is merged without overrides"),
		Entry("overrides that say nothing",
			filemerge.Spec{Strategy: filemerge.StrategyDeep, Overrides: overrides(`null`)},
			"renovate.json", "nothing is merged without overrides"),
		Entry("a list strategy this version does not know",
			filemerge.Spec{Arrays: []filemerge.ArrayRule{{Path: "$.a", Strategy: "merge"}}},
			"renovate.json", `unknown strategy "merge"`),
		Entry("two rules for one list",
			filemerge.Spec{
				Overrides: overrides(`{"a": ["x"]}`),
				Arrays: []filemerge.ArrayRule{
					{Path: "$.a", Strategy: filemerge.ArrayAppend},
					{Path: "$.a", Strategy: filemerge.ArrayPrepend},
				},
			},
			"renovate.json", "$.a has two list rules"),

		// A rule says what to do with the repository's list, so one whose path
		// no override sets has no list to work with - for every template,
		// always. Left to apply time it lands as a warning in a log that stops
		// that repository's whole file sync, so a typo in one path silently
		// stops every managed file.
		Entry("a list rule no override reaches",
			filemerge.Spec{
				Overrides: overrides(`{"timezone": "Europe/Warsaw"}`),
				Arrays: []filemerge.ArrayRule{
					{Path: "$.labels", Strategy: filemerge.ArrayAppend},
				},
			},
			"renovate.json", "no override sets $.labels"),
		Entry("a list rule whose override is not a list",
			filemerge.Spec{
				Overrides: overrides(`{"labels": "chore"}`),
				Arrays: []filemerge.ArrayRule{
					{Path: "$.labels", Strategy: filemerge.ArrayAppend},
				},
			},
			"renovate.json", "the override at $.labels is not a list"),
		// The overrides reach the path, so the shallow rule is the only thing
		// wrong with this one.
		Entry("a nested list under a shallow merge, which replaces the level above it",
			filemerge.Spec{
				Strategy:  filemerge.StrategyShallow,
				Overrides: overrides(`{"a": {"b": ["x"]}}`),
				Arrays: []filemerge.ArrayRule{
					{Path: "$.a.b", Strategy: filemerge.ArrayAppend},
				},
			},
			"renovate.json", "is below the top level"),
		Entry("a Markdown merge with no sections",
			filemerge.Spec{Strategy: filemerge.StrategyMarkdown},
			"CONTRIBUTING.md", "changes nothing"),
	)

	// Every one of these was a path the engine this replaces resolved to
	// nothing and applied silently, so a typo read as a file that needed no
	// merging.
	DescribeTable("refuses a path that addresses nothing",
		func(path, because string) {
			err := filemerge.Spec{Arrays: []filemerge.ArrayRule{
				{Path: path, Strategy: filemerge.ArrayAppend},
			}}.Validate("renovate.json")

			Expect(err).To(MatchError(filemerge.ErrInvalidSpec))
			Expect(err.Error()).To(ContainSubstring(because))
		},
		Entry("nothing at all", "", "cannot be empty"),
		Entry("a path with no root", "labels", `does not start with "$"`),
		Entry("the root alone", "$", "which is never an array"),
		Entry("a root with no dot after it", "$labels", `needs a "." after`),
		Entry("an empty key", "$.a..b", "has an empty key"),
		Entry("a trailing escape", `$.a\`, "escapes nothing"),
		Entry("an index into a list", "$.packageRules[0].matchPackageNames",
			"an array's positions move when it is merged"),
	)

	DescribeTable("refuses a section that could not be applied",
		func(section filemerge.Section, because string) {
			err := filemerge.Spec{Sections: []filemerge.Section{section}}.
				Validate("CONTRIBUTING.md")

			Expect(err).To(MatchError(filemerge.ErrInvalidSpec))
			Expect(err.Error()).To(ContainSubstring(because))
		},
		Entry("an action this version does not know",
			filemerge.Section{Action: "rewrite", Heading: "## Setup"},
			`unknown action "rewrite"`),
		Entry("a heading written without its marks",
			filemerge.Section{Action: filemerge.SectionDelete, Heading: "Setup"},
			"write it with its # marks"),
		Entry("no heading at all",
			filemerge.Section{Action: filemerge.SectionDelete},
			"write it with its # marks"),
		Entry("a replacement with nothing to put there",
			filemerge.Section{Action: filemerge.SectionReplace, Heading: "## Setup"},
			"has nothing to replace"),
		Entry("an append with nothing to append",
			filemerge.Section{Action: filemerge.SectionAppend},
			"has nothing to append"),
		Entry("an append that addresses a heading it cannot use",
			filemerge.Section{
				Action: filemerge.SectionAppend, Heading: "## Setup", Content: "x",
			},
			"addresses no heading"),
		Entry("a patch with no substitutions",
			filemerge.Section{Action: filemerge.SectionPatch, Heading: "## Setup"},
			"patches nothing"),
		Entry("a substitution that finds nothing",
			filemerge.Section{
				Action: filemerge.SectionPatch, Heading: "## Setup",
				Patches: []filemerge.Patch{{Find: "", Replace: "x"}},
			},
			"finds nothing"),
		Entry("an occurrence below the first",
			filemerge.Section{
				Action: filemerge.SectionDelete, Heading: "## Setup", Occurrence: -1,
			},
			"wants occurrence -1"),
	)

	It("reports the first thing wrong rather than the last", func() {
		err := filemerge.Spec{Sections: []filemerge.Section{
			{Action: filemerge.SectionDelete, Heading: "## First"},
			{Action: "rewrite"},
			{Action: filemerge.SectionAppend},
		}}.Validate("CONTRIBUTING.md")

		Expect(err.Error()).To(ContainSubstring("section 2"))
		Expect(err.Error()).NotTo(ContainSubstring("section 3"))
	})
})
