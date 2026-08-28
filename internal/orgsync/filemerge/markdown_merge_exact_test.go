package filemerge_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
)

var _ = Describe("Preserving Markdown merge bytes [Unit]", func() {
	It("preserves untouched CRLF bytes around a replaced section", func() {
		content := "# Title\r\n\r\n<!-- before -->\r\n\r\n## Setup\r\n\r\nOld.\r\n\r\n## Keep\r\n\r\nKeep  \r\n"
		merged, err := merged(content, filemerge.Section{
			Action: filemerge.SectionReplace, Heading: "## Setup", Content: "## Setup\n\nNew.",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(merged).To(Equal(
			"# Title\r\n\r\n<!-- before -->\r\n\r\n## Setup\r\n\r\nNew.\r\n\r\n## Keep\r\n\r\nKeep  \r\n",
		))
	})

	It("uses the addressed section's local ending in a mixed-ending document", func() {
		content := "# LF title\n\n## CRLF section\r\n\r\nOld.\r\n\r\n## Next\r\n\r\nKeep.\r\n"
		merged, err := merged(content, filemerge.Section{
			Action: filemerge.SectionReplace, Heading: "## CRLF section", Content: "## CRLF section\n\nNew.",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(merged).To(Equal(
			"# LF title\n\n## CRLF section\r\n\r\nNew.\r\n\r\n## Next\r\n\r\nKeep.\r\n",
		))
	})

	It("preserves a missing final newline", func() {
		merged, err := merged("# Title\n\n## Setup\n\nOld.", filemerge.Section{
			Action: filemerge.SectionPatch, Heading: "## Setup",
			Patches: []filemerge.Patch{{Find: "Old", Replace: "New"}},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(merged).To(Equal("# Title\n\n## Setup\n\nNew."))
	})

	It("normalizes only inserted patch lines to the section's local ending", func() {
		content := "# Title\n\n## Setup\r\n\r\nOld value.\r\n\r\n## Keep\n\nKeep.\n"
		merged, err := merged(content, filemerge.Section{
			Action: filemerge.SectionPatch, Heading: "## Setup",
			Patches: []filemerge.Patch{{Find: "Old value.", Replace: "New line.\nSecond line."}},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(merged).To(Equal(
			"# Title\n\n## Setup\r\n\r\nNew line.\r\nSecond line.\r\n\r\n## Keep\n\nKeep.\n",
		))
	})

	It("deletes comments inside an explicitly deleted section and no others", func() {
		content := "<!-- document -->\n\n## Delete\n\n<!-- belongs to deleted section -->\n\nGone.\n\n" +
			"## Keep\n\n<!-- keep exactly -->\n\nHere.\n"
		merged, err := merged(content, filemerge.Section{
			Action: filemerge.SectionDelete, Heading: "## Delete",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(merged).To(Equal(
			"<!-- document -->\n\n## Keep\n\n<!-- keep exactly -->\n\nHere.\n",
		))
	})
})
