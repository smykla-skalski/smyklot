package filemerge_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("Formatting TOML [Unit]", func() {
	format := func(content string, change func(*config.FormattingPolicy)) ([]byte, error) {
		return filemerge.FormatDocument("config.toml", []byte(content), formattingPolicy(change))
	}

	It("compacts arrays without respelling their values", func() {
		formatted, err := format("values = [ 1_000, 0xDEAD, 'kept' ]\n", func(policy *config.FormattingPolicy) {
			policy.TOML.Arrays = "compact"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("values = [1_000, 0xDEAD, 'kept']\n"))
	})

	It("expands arrays and keeps inline comments and CRLF", func() {
		formatted, err := format("values = [1, # kept\r\n  2]\r\n", func(policy *config.FormattingPolicy) {
			policy.TOML.Arrays = "expanded"
			policy.TOML.TrailingCommas = "multiline"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("values = [\r\n  1, # kept\r\n  2,\r\n]\r\n"))
	})

	It("fails closed rather than compacting a commented array", func() {
		_, err := format("values = [1, # kept\n  2]\n", func(policy *config.FormattingPolicy) {
			policy.TOML.Arrays = "compact"
		})

		Expect(errors.Is(err, filemerge.ErrUnwritable)).To(BeTrue())
	})

	It("applies the trailing comma dimension without changing layout", func() {
		removed, err := format("values = [\n  1,\n  2, # kept\n]\n", func(policy *config.FormattingPolicy) {
			policy.TOML.TrailingCommas = "remove"
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(removed)).To(Equal("values = [\n  1,\n  2 # kept\n]\n"))

		inserted, err := format(string(removed), func(policy *config.FormattingPolicy) {
			policy.TOML.TrailingCommas = "multiline"
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(inserted)).To(Equal("values = [\n  1,\n  2, # kept\n]\n"))
	})

	It("uses automatic layout and configured width", func() {
		compact, err := format("values = [\n  'one',\n  'two',\n]\n", func(policy *config.FormattingPolicy) {
			policy.TOML.Arrays = "auto"
			policy.Common.LineWidth = 40
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(compact)).To(Equal("values = ['one', 'two',]\n"))

		expanded, err := format("values = ['a value wider than forty columns', 'two']\n", func(policy *config.FormattingPolicy) {
			policy.TOML.Arrays = "auto"
			policy.Common.LineWidth = 40
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(expanded)).To(Equal(
			"values = [\n  'a value wider than forty columns',\n  'two'\n]\n",
		))

		prefixed, err := format("very_long_configuration_name = ['one', 'two']\n", func(policy *config.FormattingPolicy) {
			policy.TOML.Arrays = "auto"
			policy.Common.LineWidth = 40
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(prefixed)).To(Equal(
			"very_long_configuration_name = [\n  'one',\n  'two'\n]\n",
		))
	})

	It("uses the array's nearby line ending in a mixed-ending document", func() {
		formatted, err := format("title = 'kept'\nvalues = [1, 2]\r\n", func(policy *config.FormattingPolicy) {
			policy.TOML.Arrays = "expanded"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal(
			"title = 'kept'\nvalues = [\r\n  1,\r\n  2\r\n]\r\n",
		))
	})

	It("changes quote preference through validated library fragments", func() {
		basic, err := format("one = 'plain'\ntwo = \"has ' quote\"\n", func(policy *config.FormattingPolicy) {
			policy.TOML.QuoteStyle = "prefer_basic"
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(basic)).To(Equal("one = \"plain\"\ntwo = \"has ' quote\"\n"))

		literal, err := format(string(basic), func(policy *config.FormattingPolicy) {
			policy.TOML.QuoteStyle = "prefer_literal"
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(literal)).To(Equal("one = 'plain'\ntwo = \"has ' quote\"\n"))
	})

	It("aligns entries and comments independently", func() {
		formatted, err := format("a=1  # one\nlong = 2 # two\n", func(policy *config.FormattingPolicy) {
			policy.TOML.AlignEntries = "align"
			policy.TOML.AlignComments = "align"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("a    = 1 # one\nlong = 2 # two\n"))

		compacted, err := format(string(formatted), func(policy *config.FormattingPolicy) {
			policy.TOML.AlignEntries = "compact"
			policy.TOML.AlignComments = "compact"
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(compacted)).To(Equal("a = 1 # one\nlong = 2 # two\n"))
	})

	It("sorts keys without changing table, date, dotted-key, or array-table spelling", func() {
		content := "z = 1\na = 2\nwhen = 1979-05-27T07:32:00Z\n" +
			"dotted.key = 'value'\n\n[[products]]\nname = 'one'\n"
		formatted, err := format(content, func(policy *config.FormattingPolicy) {
			policy.TOML.KeyOrder = "sort"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal(
			"a = 2\ndotted.key = 'value'\nwhen = 1979-05-27T07:32:00Z\nz = 1\n\n" +
				"[[products]]\nname = 'one'\n",
		))
	})

	It("keeps a leading comment fixed and moves an attached comment with its key", func() {
		formatted, err := format("# document\nz = 1\n# belongs to a\na = 2\n", func(policy *config.FormattingPolicy) {
			policy.TOML.KeyOrder = "sort"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("# document\n# belongs to a\na = 2\nz = 1\n"))
	})

	It("formats nested arrays and arrays inside inline tables", func() {
		formatted, err := format("nested = [[1, 2], [3, 4]]\ninline = { values = [5, 6] }\n", func(policy *config.FormattingPolicy) {
			policy.TOML.Arrays = "expanded"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal(
			"nested = [\n  [\n    1,\n    2\n  ],\n  [\n    3,\n    4\n  ]\n]\n" +
				"inline = { values = [\n  5,\n  6\n] }\n",
		))
	})

	It("can safely format a document containing TOML NaN", func() {
		formatted, err := format("z = nan\na = [ 1, 2 ]\n", func(policy *config.FormattingPolicy) {
			policy.TOML.Arrays = "compact"
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("z = nan\na = [1, 2]\n"))
	})

	It("rejects duplicate keys before applying presentation edits", func() {
		_, err := format("value = 1\nvalue = 2\n", func(policy *config.FormattingPolicy) {
			policy.TOML.Arrays = "compact"
		})

		Expect(errors.Is(err, filemerge.ErrUnreadable)).To(BeTrue())
	})

	It("is idempotent across combined dimensions", func() {
		policy := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.TOML.Arrays = "expanded"
			policy.TOML.QuoteStyle = "prefer_basic"
			policy.TOML.AlignEntries = "align"
			policy.TOML.AlignComments = "align"
			policy.TOML.KeyOrder = "sort"
		})
		first, err := filemerge.FormatDocument(
			"a.toml", []byte("z=['one','two'] # z\na='x' # a\n"), policy,
		)
		Expect(err).NotTo(HaveOccurred())
		second, err := filemerge.FormatDocument("a.toml", first, policy)
		Expect(err).NotTo(HaveOccurred())
		Expect(second).To(Equal(first))
	})
})
