package filemerge_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
)

// merged applies one section to a document and answers with the result.
func merged(document string, sections ...filemerge.Section) (string, error) {
	result, err := filemerge.Apply(
		"CONTRIBUTING.md", []byte(document), filemerge.Spec{Sections: sections})

	return string(result), err
}

var _ = Describe("Merging Markdown [Unit]", func() {
	const document = `# Contributing

Thanks for helping.

## Setup

Run the installer.

### Windows

Use the other installer.

## Review

Somebody reads it.
`

	Describe("the operations", func() {
		It("puts content after a section, past its subsections", func() {
			result, err := merged(document, filemerge.Section{
				Action: filemerge.SectionAfter, Heading: "## Setup", Content: "Then log in.",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring(
				"Use the other installer.\n\nThen log in.\n\n## Review"))
		})

		It("puts content before a heading", func() {
			result, err := merged(document, filemerge.Section{
				Action: filemerge.SectionBefore, Heading: "## Review", Content: "Read this first.",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Read this first.\n\n## Review"))
		})

		It("replaces a section and its subsections", func() {
			result, err := merged(document, filemerge.Section{
				Action:  filemerge.SectionReplace,
				Heading: "## Setup",
				Content: "## Setup\n\nNothing to install.",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("## Setup\n\nNothing to install."))
			Expect(result).NotTo(ContainSubstring("### Windows"))
			Expect(result).To(ContainSubstring("## Review"))
		})

		It("deletes a section and its subsections", func() {
			result, err := merged(document, filemerge.Section{
				Action: filemerge.SectionDelete, Heading: "## Setup",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("## Setup"))
			Expect(result).NotTo(ContainSubstring("### Windows"))
			Expect(result).To(ContainSubstring("## Review"))
		})

		It("substitutes text inside a section", func() {
			result, err := merged(document, filemerge.Section{
				Action:  filemerge.SectionPatch,
				Heading: "## Setup",
				Patches: []filemerge.Patch{{Find: "the installer", Replace: "`make install`"}},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Run `make install`."))
		})

		It("puts content at the end and at the start", func() {
			result, err := merged(document,
				filemerge.Section{Action: filemerge.SectionAppend, Content: "## Licence\n\nMIT."},
				filemerge.Section{Action: filemerge.SectionPrepend, Content: "<!-- generated -->"},
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HavePrefix("<!-- generated -->\n\n# Contributing"))
			Expect(result).To(HaveSuffix("## Licence\n\nMIT.\n"))
		})

		It("applies sections in order, each seeing the last one's work", func() {
			result, err := merged(document,
				filemerge.Section{Action: filemerge.SectionDelete, Heading: "### Windows"},
				filemerge.Section{
					Action: filemerge.SectionAfter, Heading: "## Setup", Content: "Then log in.",
				},
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Run the installer.\n\nThen log in.\n\n## Review"))
		})
	})

	// The engine this replaces included the heading line in the text it
	// substituted over, so a patch whose find string appeared in the heading
	// renamed the section it had just been asked to edit.
	It("leaves the heading alone when a patch would match it", func() {
		result, err := merged("## Setup\n\nSetup is easy.\n", filemerge.Section{
			Action:  filemerge.SectionPatch,
			Heading: "## Setup",
			Patches: []filemerge.Patch{{Find: "Setup", Replace: "Installation"}},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("## Setup\n\nInstallation is easy.\n"))
	})

	Describe("code fences", func() {
		// The engine this replaces toggled one flag on any line starting with
		// three backticks or three tildes, whichever it was. A backtick fence
		// inside a tilde block closed it, and every heading after that point
		// stopped existing - which then clobbered the file.
		It("does not let a backtick fence close a tilde block", func() {
			result, err := merged("# Title\n\n~~~\n```\n~~~\n\n## Real\n\nHere.\n",
				filemerge.Section{
					Action: filemerge.SectionAfter, Heading: "## Real", Content: "Added.",
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Here.\n\nAdded."))
		})

		It("does not let a shorter run close a longer fence", func() {
			result, err := merged("# Title\n\n````\n```\n## Fake\n````\n\n## Real\n\nHere.\n",
				filemerge.Section{
					Action: filemerge.SectionAfter, Heading: "## Real", Content: "Added.",
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Here.\n\nAdded."))
			Expect(result).To(ContainSubstring("## Fake"))
		})

		It("reads a heading inside a fence as text rather than as a heading", func() {
			_, err := merged("# Title\n\n```\n## Fenced\n```\n", filemerge.Section{
				Action: filemerge.SectionDelete, Heading: "## Fenced",
			})

			Expect(err).To(MatchError(filemerge.ErrNothingAddressed))
		})

		// Four spaces makes it an indented code block, so the backticks are a
		// picture of a fence rather than one.
		It("does not open a fence from an indented line", func() {
			result, err := merged("# Title\n\n    ```\n\n## Real\n\nHere.\n",
				filemerge.Section{
					Action: filemerge.SectionAfter, Heading: "## Real", Content: "Added.",
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Here.\n\nAdded."))
		})

		// A backtick fence's info string cannot itself hold a backtick, which is
		// what tells an opener from a line of inline code.
		It("does not open a fence from a line of inline code", func() {
			result, err := merged("# Title\n\n```code``` is inline.\n\n## Real\n\nHere.\n",
				filemerge.Section{
					Action: filemerge.SectionAfter, Heading: "## Real", Content: "Added.",
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Here.\n\nAdded."))
		})
	})

	Describe("addressing a heading", func() {
		const repeated = "## Usage\n\nFirst.\n\n### Usage\n\nDeeper.\n\n## Usage\n\nSecond.\n"

		// The engine this replaces matched on the title alone, so "## Usage"
		// and "### Usage" were one heading and the deeper one won if it came
		// first.
		It("tells one level from another", func() {
			result, err := merged(repeated, filemerge.Section{
				Action: filemerge.SectionPatch, Heading: "### Usage",
				Patches: []filemerge.Patch{{Find: "Deeper", Replace: "Nested"}},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Nested."))
			Expect(result).To(ContainSubstring("First."))
		})

		It("refuses a heading the document carries more than once", func() {
			_, err := merged(repeated, filemerge.Section{
				Action: filemerge.SectionDelete, Heading: "## Usage",
			})

			Expect(err).To(MatchError(filemerge.ErrNothingAddressed))
			Expect(err.Error()).To(ContainSubstring("appears 2 times"))
		})

		It("takes the occurrence a section names", func() {
			result, err := merged(repeated, filemerge.Section{
				Action: filemerge.SectionPatch, Heading: "## Usage", Occurrence: 2,
				Patches: []filemerge.Patch{{Find: "Second", Replace: "Last"}},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Last."))
			Expect(result).To(ContainSubstring("First."))
		})

		It("refuses an occurrence the document does not reach", func() {
			_, err := merged(repeated, filemerge.Section{
				Action: filemerge.SectionDelete, Heading: "## Usage", Occurrence: 3,
			})

			Expect(err).To(MatchError(filemerge.ErrNothingAddressed))
			Expect(err.Error()).To(ContainSubstring("no occurrence 3"))
		})

		It("matches whatever case the document wrote it in", func() {
			result, err := merged("## setup\n\nHere.\n", filemerge.Section{
				Action: filemerge.SectionDelete, Heading: "## Setup",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("setup"))
		})

		It("ignores the closing marks a heading may carry", func() {
			result, err := merged("## Setup ##\n\nHere.\n", filemerge.Section{
				Action: filemerge.SectionDelete, Heading: "## Setup",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("Setup"))
		})
	})

	// The document merged is the template rather than a repository's own copy,
	// so this normally cannot fire - but an operation that changes the document
	// every time it runs is one that cannot be run twice, and that is a
	// property of the operation rather than of who calls it.
	Describe("running twice", func() {
		It("appends content the document already ends with only once", func() {
			result, err := merged("# Title\n\n## Licence\n\nMIT.\n", filemerge.Section{
				Action: filemerge.SectionAppend, Content: "## Licence\n\nMIT.",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("# Title\n\n## Licence\n\nMIT.\n"))
		})

		It("prepends content the document already starts with only once", func() {
			result, err := merged("<!-- generated -->\n\n# Title\n", filemerge.Section{
				Action: filemerge.SectionPrepend, Content: "<!-- generated -->",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("<!-- generated -->\n\n# Title\n"))
		})
	})

	// Fail-closed, which is the whole point. The engine this replaces reported
	// each of these as a warning and wrote the raw template over the
	// repository's file.
	DescribeTable("refuses rather than writing the template over the file",
		func(section filemerge.Section, because string) {
			_, err := merged(document, section)

			Expect(err).To(MatchError(filemerge.ErrNothingAddressed))
			Expect(err.Error()).To(ContainSubstring(because))
		},
		Entry("a heading the document does not have",
			filemerge.Section{Action: filemerge.SectionDelete, Heading: "## Renamed"},
			`no heading "## Renamed"`),
		Entry("a patch that finds nothing",
			filemerge.Section{
				Action: filemerge.SectionPatch, Heading: "## Setup",
				Patches: []filemerge.Patch{{Find: "the uninstaller", Replace: "x"}},
			},
			"does not find"),
	)
})
