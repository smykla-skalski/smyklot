package filemerge_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func formattingPolicy(change func(*config.FormattingPolicy)) config.FormattingPolicy {
	policy := config.DefaultFormattingPolicy()
	change(&policy)

	return policy
}

var _ = Describe("Formatting JSON [Unit]", func() {
	It("fast-paths an all-preserve policy without parsing", func() {
		invalid := []byte("{ this is intentionally not JSON\r\n\r\n")

		Expect(filemerge.FormatDocument("broken.json", invalid, config.DefaultFormattingPolicy())).
			To(Equal(invalid))
	})

	It("leaves an unsupported format byte-identical", func() {
		content := []byte("anything\r\nwithout a final newline")
		policy := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.Common.LineEnding = "lf"
			policy.Common.FinalNewline = "insert"
		})

		Expect(filemerge.FormatDocument("notes.txt", content, policy)).To(Equal(content))
	})

	DescribeTable("changes only the selected collection dimension",
		func(content, expected string, change func(*config.FormattingPolicy)) {
			policy := formattingPolicy(change)
			formatted, err := filemerge.FormatDocument("renovate.json", []byte(content), policy)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(formatted)).To(Equal(expected))
			again, err := filemerge.FormatDocument("renovate.json", formatted, policy)
			Expect(err).NotTo(HaveOccurred())
			Expect(again).To(Equal(formatted))
		},
		Entry("compact arrays preserve the surrounding mixed line endings and final blank line",
			"{\r\n  \"renovate\": [ \"a\",\n    \"b\" ],\n  \"keep\" :  true\r\n}\n\n",
			"{\r\n  \"renovate\": [\"a\", \"b\"],\n  \"keep\" :  true\r\n}\n\n",
			func(policy *config.FormattingPolicy) { policy.JSON.Arrays = "compact" }),
		Entry("expanded arrays write one entry per line without expanding objects",
			`{"a":[1,{"b":[2,3]}]}`,
			"{\"a\":[\n    1,\n    {\"b\":[\n        2,\n        3\n      ]}\n  ]}",
			func(policy *config.FormattingPolicy) { policy.JSON.Arrays = "expanded" }),
		Entry("expanded objects write one member per line without expanding arrays",
			`{"a":{"b":1,"c":[2,3]}}`,
			"{\n  \"a\": {\n    \"b\": 1,\n    \"c\": [2,3]\n  }\n}",
			func(policy *config.FormattingPolicy) { policy.JSON.Objects = "expanded" }),
		Entry("automatic arrays stay compact when they fit",
			`{"a":[1,2,3]}`, `{"a":[1, 2, 3]}`,
			func(policy *config.FormattingPolicy) {
				policy.JSON.Arrays = "auto"
				policy.Common.LineWidth = 40
			}),
		Entry("automatic arrays expand when they do not fit",
			`{"a":["this value makes the collection wider than forty columns",2]}`,
			"{\"a\":[\n    \"this value makes the collection wider than forty columns\",\n    2\n  ]}",
			func(policy *config.FormattingPolicy) {
				policy.JSON.Arrays = "auto"
				policy.Common.LineWidth = 40
			}),
	)

	It("reindents existing multiline collections without changing their layout", func() {
		content := []byte("{\n    \"a\": [\n        1,\n        2\n    ]\n}\n")
		policy := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.Common.IndentStyle = "spaces"
			policy.Common.IndentWidth = 2
		})

		formatted, err := filemerge.FormatDocument("a.json", content, policy)

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("{\n  \"a\": [\n    1,\n    2\n  ]\n}\n"))
	})

	DescribeTable("applies line ending and final newline independently",
		func(content, expected string, change func(*config.FormattingPolicy)) {
			policy := formattingPolicy(change)
			formatted, err := filemerge.FormatDocument("a.json", []byte(content), policy)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(formatted)).To(Equal(expected))
		},
		Entry("LF leaves the number of terminal newlines alone",
			"{\r\n  \"a\": 1\n}\r\n\r\n", "{\n  \"a\": 1\n}\n\n",
			func(policy *config.FormattingPolicy) { policy.Common.LineEnding = "lf" }),
		Entry("CRLF leaves the number of terminal newlines alone",
			"{\n  \"a\": 1\n}\n\n", "{\r\n  \"a\": 1\r\n}\r\n\r\n",
			func(policy *config.FormattingPolicy) { policy.Common.LineEnding = "crlf" }),
		Entry("insert produces exactly one nearby-style terminal newline",
			"{\r\n  \"a\": 1\r\n}\r\n\r\n", "{\r\n  \"a\": 1\r\n}\r\n",
			func(policy *config.FormattingPolicy) { policy.Common.FinalNewline = "insert" }),
		Entry("remove removes every terminal newline",
			"{\n  \"a\": 1\n}\n\n", "{\n  \"a\": 1\n}",
			func(policy *config.FormattingPolicy) { policy.Common.FinalNewline = "remove" }),
	)

	It("preserves JSONC comment text while expanding", func() {
		content := []byte("{\n  // before\n  \"a\": [1, // between\n    2 /* after */]\n}\n")
		policy := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.JSON.Arrays = "expanded"
		})

		formatted, err := filemerge.FormatDocument("a.jsonc", content, policy)

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(ContainSubstring("// before"))
		Expect(string(formatted)).To(ContainSubstring("// between"))
		Expect(string(formatted)).To(ContainSubstring("/* after */"))
	})

	It("preserves CRLF line comments without duplicating carriage returns", func() {
		content := []byte("{\r\n  \"a\": [1, // kept\r\n    2]\r\n}\r\n")
		policy := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.JSON.Arrays = "expanded"
		})

		formatted, err := filemerge.FormatDocument("a.jsonc", content, policy)

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("{\r\n  \"a\": [\r\n    1,\r\n    // kept\r\n    2\r\n  ]\r\n}\r\n"))
		Expect(formatted).NotTo(ContainSubstring("\r\r\n"))
	})

	It("fails closed rather than compacting a commented JSONC collection", func() {
		policy := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.JSON.Arrays = "compact"
		})

		_, err := filemerge.FormatDocument("a.jsonc", []byte("{\"a\":[1, // why\n2]}"), policy)

		Expect(err).To(MatchError(filemerge.ErrUnwritable))
	})

	DescribeTable("refuses ambiguous or nonstandard JSON syntax",
		func(path, content string) {
			policy := formattingPolicy(func(policy *config.FormattingPolicy) {
				policy.JSON.Arrays = "expanded"
			})

			_, err := filemerge.FormatDocument(path, []byte(content), policy)

			Expect(err).To(MatchError(filemerge.ErrUnreadable))
		},
		Entry("comments in strict JSON", "a.json", "{\"a\":[1] // comment\n}"),
		Entry("trailing commas in strict JSON", "a.json", "{\"a\":[1,]}"),
		Entry("duplicate keys in JSON", "a.json", "{\"a\":1,\"a\":2}"),
		Entry("duplicate keys in JSONC", "a.jsonc", "{\"a\":1,\"a\":2}"),
	)

	It("inserts and removes JSONC trailing commas without changing comments", func() {
		content := []byte("{\n  \"a\": [\n    1 // kept\n  ]\n}\n")
		insert := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.JSONC.TrailingCommas = "insert"
		})
		remove := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.JSONC.TrailingCommas = "remove"
		})

		withCommas, err := filemerge.FormatDocument("a.jsonc", content, insert)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(withCommas)).To(Equal("{\n  \"a\": [\n    1, // kept\n  ],\n}\n"))

		withoutCommas, err := filemerge.FormatDocument("a.jsonc", withCommas, remove)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(withoutCommas)).To(Equal(string(content)))
	})

	It("preserves compact Renovate arrays when an unrelated value is merged", func() {
		template := []byte(`{"extends":["config:recommended"],"ignorePaths":["vendor/**","dist/**"],"timezone":"UTC"}`)
		merged, err := applyFileMerge("renovate.json", template, filemerge.Spec{
			Overrides: overrides(`{"timezone":"Europe/Warsaw"}`),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal(`{"extends":["config:recommended"],"ignorePaths":["vendor/**","dist/**"],"timezone":"Europe/Warsaw"}`))
	})

	It("inherits array insertion spacing from the nearest sibling", func() {
		merged, err := applyFileMerge("renovate.json", []byte(`{"extends": ["base", "group"]}`), filemerge.Spec{
			Overrides: overrides(`{"extends":["local"]}`),
			Arrays: []filemerge.ArrayRule{
				{Path: "$.extends", Strategy: filemerge.ArrayAppend},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal(`{"extends": ["base", "group", "local"]}`))
	})

	It("inherits collection layout when replacing a value", func() {
		template := []byte(`{"rules": { "one": 1, "two": 2 }, "labels": ["one", "two"]}`)
		merged, err := applyFileMerge("config.json", template, filemerge.Spec{
			Strategy: filemerge.StrategyShallow,
			Overrides: overrides(
				`{"rules":{"two":3,"three":4},"labels":["three","four"]}`,
			),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal(
			`{"rules": { "three": 4, "two": 3 }, "labels": ["three", "four"]}`,
		))
	})

	It("automatic layout compacts a multiline collection when it fits", func() {
		policy := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.JSON.Arrays = "auto"
			policy.Common.LineWidth = 100
		})

		formatted, err := filemerge.FormatDocument(
			"config.json", []byte("{\"labels\": [\n  \"one\",\n  \"two\"\n]}"), policy,
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal(`{"labels": ["one", "two"]}`))
	})

	It("preserves JSONC comments outside a replaced scalar", func() {
		template := []byte("{\n  // managed timezone\n  \"timezone\": \"UTC\", // stays attached\n  \"enabled\": true\n}\n")
		merged, err := applyFileMerge("renovate.jsonc", template, filemerge.Spec{
			Overrides: overrides(`{"timezone":"Europe/Warsaw"}`),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal("{\n  // managed timezone\n  \"timezone\": \"Europe/Warsaw\", // stays attached\n  \"enabled\": true\n}\n"))
	})

	DescribeTable("refuses invalid override documents",
		func(document string) {
			_, err := applyFileMerge("a.json", []byte(`{"a":1}`), filemerge.Spec{
				Overrides: overrides(document),
			})

			Expect(errors.Is(err, filemerge.ErrUnreadable)).To(BeTrue())
		},
		Entry("duplicate keys", `{"a":1,"a":2}`),
		Entry("trailing content", `{"a":1}{"b":2}`),
	)
})
