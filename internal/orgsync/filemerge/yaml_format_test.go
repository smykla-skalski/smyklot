package filemerge_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("Formatting YAML [Unit]", func() {
	format := func(content string, change func(*config.FormattingPolicy)) ([]byte, error) {
		policy := formattingPolicy(change)

		return filemerge.FormatDocument("workflow.yaml", []byte(content), policy)
	}

	It("changes quotes through scalar source spans without moving comments", func() {
		formatted, err := format("# top\nnaïve: café # kept\nnumber: 01\n", func(policy *config.FormattingPolicy) {
			policy.YAML.QuoteStyle = "prefer_double"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("# top\n\"naïve\": \"café\" # kept\n\"number\": 01\n"))
	})

	It("keeps quotes when a scalar cannot safely become plain", func() {
		formatted, err := format("one: 'plain'\ntwo: 'true'\nthree: 'a: b'\n", func(policy *config.FormattingPolicy) {
			policy.YAML.QuoteStyle = "prefer_plain"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("one: plain\ntwo: 'true'\nthree: 'a: b'\n"))
	})

	It("turns a flow sequence into a block sequence without trailing key whitespace", func() {
		formatted, err := format("items: [one, two]\nkeep : true\n", func(policy *config.FormattingPolicy) {
			policy.YAML.Sequences = "block"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("items:\n  - one\n  - two\nkeep : true\n"))
	})

	It("turns a block sequence into a flow sequence without rewriting its sibling", func() {
		formatted, err := format("items:\n  - one\n  - two\nkeep : true\n", func(policy *config.FormattingPolicy) {
			policy.YAML.Sequences = "flow"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("items:\n  [one, two]\nkeep : true\n"))
	})

	It("applies mapping style recursively from the outer source span", func() {
		formatted, err := format("config:\n  a: 1\n  b: 2\nother: keep\n", func(policy *config.FormattingPolicy) {
			policy.YAML.Mappings = "flow"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("{config: {a: 1, b: 2}, other: keep}\n"))
	})

	It("uses line width for automatic collection layout", func() {
		formatted, err := format("items:\n  - one\n  - two\n", func(policy *config.FormattingPolicy) {
			policy.YAML.Sequences = "auto"
			policy.Common.LineWidth = 40
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("items:\n  [one, two]\n"))
	})

	It("supports indentless sequences independently of mapping indentation", func() {
		formatted, err := format("items:\n  - one\n  - two\n", func(policy *config.FormattingPolicy) {
			policy.YAML.SequenceIndent = "indentless"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("items:\n- one\n- two\n"))
	})

	It("inserts and removes a document start with the nearby line ending", func() {
		inserted, err := format("key: value\r\n", func(policy *config.FormattingPolicy) {
			policy.YAML.DocumentStart = "insert"
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(inserted)).To(Equal("---\r\nkey: value\r\n"))

		removed, err := format(string(inserted), func(policy *config.FormattingPolicy) {
			policy.YAML.DocumentStart = "remove"
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(removed)).To(Equal("key: value\r\n"))
	})

	It("reindents comments and values by changing only leading spaces", func() {
		formatted, err := format("root:\n    # kept\n    child:\n        value: yes\n", func(policy *config.FormattingPolicy) {
			policy.Common.IndentStyle = "spaces"
			policy.Common.IndentWidth = 2
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("root:\n  # kept\n  child:\n    value: yes\n"))
	})

	It("lets the YAML encoder place comments in a changed collection", func() {
		formatted, err := format("items:\n  - one # kept\n  - two\n", func(policy *config.FormattingPolicy) {
			policy.YAML.Sequences = "flow"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("items:\n  [one, # kept\n  two]\n"))
	})

	DescribeTable("fails closed on rich YAML syntax it cannot prove byte-safe",
		func(content string) {
			_, err := format(content, func(policy *config.FormattingPolicy) {
				policy.YAML.QuoteStyle = "prefer_single"
			})

			Expect(errors.Is(err, filemerge.ErrUnwritable)).To(BeTrue())
		},
		Entry("anchors and aliases", "base: &base\n  value: yes\ncopy: *base\n"),
		Entry("explicit tags", "value: !!str yes\n"),
		Entry("merge keys", "base: &base {value: yes}\ncopy:\n  <<: *base\n"),
	)

	It("keeps block-scalar spelling while changing an unrelated quote", func() {
		formatted, err := format("script: |-\n  echo hello\n", func(policy *config.FormattingPolicy) {
			policy.YAML.QuoteStyle = "prefer_single"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("'script': |-\n  echo hello\n"))
	})

	It("is idempotent after a structural edit", func() {
		policy := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.YAML.Sequences = "block"
			policy.YAML.QuoteStyle = "prefer_single"
		})
		first, err := filemerge.FormatDocument("a.yml", []byte("items: [one, two]\n"), policy)
		Expect(err).NotTo(HaveOccurred())
		second, err := filemerge.FormatDocument("a.yml", first, policy)
		Expect(err).NotTo(HaveOccurred())
		Expect(second).To(Equal(first))
	})
})
