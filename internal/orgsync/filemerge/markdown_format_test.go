package filemerge_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("Formatting Markdown [Unit]", func() {
	format := func(path, content string, change func(*config.FormattingPolicy)) ([]byte, error) {
		return filemerge.FormatDocument(path, []byte(content), formattingPolicy(change))
	}

	It("fast-paths all-preserve Markdown without parsing or changing bytes", func() {
		content := []byte("---\r\nnot: [valid\r\n---\r\n\r\nno terminal newline")

		Expect(filemerge.FormatDocument("README.md", content, config.DefaultFormattingPolicy())).
			To(Equal(content))
	})

	DescribeTable("formats only plain top-level prose",
		func(content, expected, wrapping string) {
			formatted, err := format("README.md", content, func(policy *config.FormattingPolicy) {
				policy.Markdown.ProseWrap = wrapping
				policy.Common.LineWidth = 40
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(formatted)).To(Equal(expected))
		},
		Entry("wraps at the configured rune width",
			"A paragraph with enough ordinary words to require wrapping at forty columns.\n",
			"A paragraph with enough ordinary words\nto require wrapping at forty columns.\n", "always"),
		Entry("removes existing soft wraps",
			"A paragraph already\nwrapped over two lines.\n",
			"A paragraph already wrapped over two lines.\n", "never"),
		Entry("uses the paragraph's CRLF ending",
			"A paragraph with enough ordinary words to require wrapping at forty columns.\r\n",
			"A paragraph with enough ordinary words\r\nto require wrapping at forty columns.\r\n", "always"),
	)

	It("preserves hard breaks and non-plain inline syntax", func() {
		content := "Hard break stays here.  \nNext line.\n\n" +
			"Keep [a link](https://example.com), *emphasis*, and `code` exactly.\n"
		formatted, err := format("README.md", content, func(policy *config.FormattingPolicy) {
			policy.Markdown.ProseWrap = "always"
			policy.Common.LineWidth = 40
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal(content))
	})

	It("leaves frontmatter, code, HTML, comments, and link definitions byte-identical", func() {
		content := "---\nvery_long_frontmatter_key: a value that must never be wrapped by Markdown\n---\n\n" +
			"```text\na very long fenced code line that must remain exactly as written\n```\n\n" +
			"    a very long indented code line that must remain exactly as written\n\n" +
			"<div>a very long raw HTML line that must remain exactly as written</div>\n\n" +
			"<!-- a very long HTML comment that must remain exactly as written -->\n\n" +
			"[example]: https://example.com/a/very/long/address \"title\"\n"
		formatted, err := format("README.md", content, func(policy *config.FormattingPolicy) {
			policy.Markdown.ProseWrap = "always"
			policy.Common.LineWidth = 40
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal(content))
	})

	DescribeTable("changes spacing only for simple flat lists",
		func(content, expected, spacing string) {
			formatted, err := format("README.md", content, func(policy *config.FormattingPolicy) {
				policy.Markdown.ListSpacing = spacing
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(formatted)).To(Equal(expected))
		},
		Entry("makes a loose list tight",
			"- one\n\n- two\n\n- three\n", "- one\n- two\n- three\n", "tight"),
		Entry("makes a tight CRLF list loose",
			"- one\r\n- two\r\n", "- one\r\n\r\n- two\r\n", "loose"),
	)

	It("preserves task markers and skips nested or structurally complex lists", func() {
		content := "- [ ] task one\n- [x] task two\n\n" +
			"Between lists.\n\n" +
			"- parent\n  - nested child\n- another parent\n\n" +
			"Between complex lists.\n\n" +
			"- item with a continuation\n\n  second paragraph\n- final item\n"
		formatted, err := format("README.md", content, func(policy *config.FormattingPolicy) {
			policy.Markdown.ListSpacing = "loose"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal(
			"- [ ] task one\n\n- [x] task two\n\n" +
				"Between lists.\n\n" +
				"- parent\n  - nested child\n- another parent\n\n" +
				"Between complex lists.\n\n" +
				"- item with a continuation\n\n  second paragraph\n- final item\n",
		))
	})

	DescribeTable("formats recognized safe GFM tables",
		func(content, expected, tables string) {
			formatted, err := format("README.markdown", content, func(policy *config.FormattingPolicy) {
				policy.Markdown.Tables = tables
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(formatted)).To(Equal(expected))
		},
		Entry("aligns cells and retains alignment markers",
			"| Name | Value |\n| :--- | ---: |\n| a | longer |\n",
			"| Name | Value  |\n| :--- | -----: |\n| a    | longer |\n", "align"),
		Entry("compacts cells without a terminal newline",
			"Name   | Value\n:----- | :---:\na      | two",
			"Name|Value\n:---|:---:\na|two", "compact"),
	)

	It("skips table syntax whose source cannot be safely reconstructed", func() {
		content := "| expression | result |\n| --- | --- |\n| `a | b` | a \\| b |\n"
		formatted, err := format("README.md", content, func(policy *config.FormattingPolicy) {
			policy.Markdown.Tables = "align"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal(content))
	})

	It("preserves nearby mixed endings and a missing final newline", func() {
		content := "Untouched first paragraph.\n\n- one\r\n- two"
		formatted, err := format("README.md", content, func(policy *config.FormattingPolicy) {
			policy.Markdown.ListSpacing = "loose"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("Untouched first paragraph.\n\n- one\r\n\r\n- two"))
	})

	It("applies common endings without disturbing Markdown hard-break spaces", func() {
		formatted, err := format("README.md", "Keep this hard break.  \r\nNext line.\r\n\r\n", func(policy *config.FormattingPolicy) {
			policy.Common.LineEnding = "lf"
			policy.Common.FinalNewline = "insert"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("Keep this hard break.  \nNext line.\n"))
	})

	It("is idempotent across combined dimensions", func() {
		policy := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.Common.LineWidth = 40
			policy.Markdown.ProseWrap = "always"
			policy.Markdown.ListSpacing = "loose"
			policy.Markdown.Tables = "align"
		})
		content := []byte("A paragraph with enough ordinary words to require wrapping at forty columns.\n\n" +
			"- one\n- two\n\n| Name | Value |\n| --- | ---: |\n| a | longer |\n")

		first, err := filemerge.FormatDocument("README.md", content, policy)
		Expect(err).NotTo(HaveOccurred())
		second, err := filemerge.FormatDocument("README.md", first, policy)
		Expect(err).NotTo(HaveOccurred())
		Expect(second).To(Equal(first))
	})
})
