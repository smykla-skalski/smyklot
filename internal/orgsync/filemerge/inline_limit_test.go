package filemerge_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("Inline collection limits [Unit]", func() {
	DescribeTable("measures the emitted JSONC trailing comma",
		func(content, comma, expected string, limit int) {
			policy := config.DefaultFormattingPolicy()
			policy.Common.InlineMaxChars = limit
			policy.JSON.Arrays = "auto"
			policy.JSONC.TrailingCommas = comma
			actual, err := filemerge.FormatDocument("a.jsonc", []byte(content), policy)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(actual)).To(Equal(expected))
			Expect(filemerge.FormatDocument("a.jsonc", actual, policy)).To(Equal(actual))
		},
		Entry("insert exceeds cap", "[1]", "insert", "[\n  1,\n]", 3),
		Entry("insert fits cap", "[1]", "insert", "[1,]", 4),
		Entry("remove fits cap", "[1,]", "remove", "[1]", 3),
		Entry("preserve exceeds cap", "[1,]", "preserve", "[\n  1,\n]", 3),
		Entry("nested preserved commas exceed cap", "[[1,],]", "preserve", "[\n  [1,],\n]", 6),
	)

	It("formats newly appended Renovate rules after composition", func() {
		policy := config.DefaultFormattingPolicy()
		policy.Common.InlineMaxChars = 40
		policy.JSON.Objects, policy.JSON.Arrays = "auto", "auto"
		spec := filemerge.Spec{
			Strategy:  filemerge.StrategyDeep,
			Overrides: json.RawMessage(`{"packageRules":[{"description":"Group golang.org/x Go modules","groupName":"golang.org/x modules","matchDepTypes":["require"],"matchManagers":["gomod"]}]}`),
			Arrays:    []filemerge.ArrayRule{{Path: "$.packageRules", Strategy: filemerge.ArrayAppend}},
		}
		actual, err := filemerge.Apply("renovate.json", []byte(`{"packageRules":[{"enabled":true}]}`), spec, policy)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(actual)).To(ContainSubstring(`{"enabled": true}`))
		Expect(string(actual)).To(ContainSubstring("{\n      \"description\": \"Group golang.org/x Go modules\","))
		Expect(string(actual)).To(ContainSubstring(`"matchManagers": ["gomod"]`))
		Expect(filemerge.FormatDocument("renovate.json", actual, policy)).To(Equal(actual))
	})

	DescribeTable("limits each JSON collection independently",
		func(content, expected string, limit int) {
			policy := config.DefaultFormattingPolicy()
			policy.JSON.Arrays = "auto"
			policy.JSON.Objects = "auto"
			policy.Common.InlineMaxChars = limit
			for _, path := range []string{"renovate.json", "renovate.jsonc"} {
				actual, err := filemerge.FormatDocument(path, []byte(content), policy)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(actual)).To(Equal(expected))
				Expect(filemerge.FormatDocument(path, actual, policy)).To(Equal(actual))
			}
		},
		Entry("objects at and above the boundary",
			`{"short":{"a":1},"long":{"abcd":1}}`,
			"{\n  \"short\": {\"a\": 1},\n  \"long\": {\n    \"abcd\": 1\n  }\n}", 8),
		Entry("arrays at and above the boundary",
			`[[1,2],[1,2,3]]`, "[\n  [1, 2],\n  [\n    1,\n    2,\n    3\n  ]\n]", 6),
		Entry("rendered characters rather than UTF-8 bytes", `{"é":1}`, `{"é": 1}`, 8),
		Entry("zero retains the line-width-only decision", `{"a":1}`, `{"a": 1}`, 0),
	)

	It("does not override explicit layout choices", func() {
		for _, choice := range []string{"preserve", "compact", "expanded"} {
			policy := config.DefaultFormattingPolicy()
			policy.JSON.Arrays, policy.JSON.Objects = choice, choice
			content := []byte(`{"a":[1,2,3]}`)
			before, err := filemerge.FormatDocument("a.json", content, policy)
			Expect(err).NotTo(HaveOccurred())
			policy.Common.InlineMaxChars = 1
			Expect(filemerge.FormatDocument("a.json", content, policy)).To(Equal(before))
		}
	})

	It("does not move comments in explicitly expanded JSONC", func() {
		policy := config.DefaultFormattingPolicy()
		policy.JSON.Arrays = "expanded"
		policy.JSONC.TrailingCommas = "remove"
		content := []byte(`[1 /* keep */,]`)
		before, err := filemerge.FormatDocument("a.jsonc", content, policy)
		Expect(err).NotTo(HaveOccurred())
		policy.Common.InlineMaxChars = 1
		Expect(filemerge.FormatDocument("a.jsonc", content, policy)).To(Equal(before))
	})

	DescribeTable("caps automatic layouts in other structured formats",
		func(path, content, expected string, limit int) {
			policy := config.DefaultFormattingPolicy()
			policy.Common.InlineMaxChars = limit
			policy.YAML.Sequences = "auto"
			policy.TOML.Arrays = "auto"
			actual, err := filemerge.FormatDocument(path, []byte(content), policy)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(actual)).To(Equal(expected))
			Expect(filemerge.FormatDocument(path, actual, policy)).To(Equal(actual))
		},
		Entry("YAML lists", "a.yaml", "short: [1, 2]\nlong: [1, 2, 3]\n",
			"short: [1, 2]\nlong:\n  - 1\n  - 2\n  - 3\n", 6),
		Entry("YAML retained whitespace", "a.yaml", "a: [1,       2]\n", "a: [1, 2]\n", 6),
		Entry("YAML external comments", "a.yaml", "# before\na: [1,       2] # keep\n# after\n",
			"# before\na: [1, 2] # keep\n# after\n", 6),
		Entry("YAML expanded external comments", "a.yaml", "a: [1, 2] # keep\n",
			"a:\n  - 1\n  - 2 # keep\n", 3),
		Entry("TOML arrays inside nested inline tables", "a.toml", "a = [{ b = [1, 2, 3] }]\n",
			"a = [\n  { b = [\n    1,\n    2,\n    3\n  ] }\n]\n", 6),
		Entry("TOML arrays", "a.toml", "short = [1, 2]\nlong = [1, 2, 3]\n",
			"short = [1, 2]\nlong = [\n  1,\n  2,\n  3\n]\n", 6),
	)

	It("retains line width as a separate cap", func() {
		policy := config.DefaultFormattingPolicy()
		policy.Common.InlineMaxChars = 320
		policy.Common.LineWidth = 40
		policy.JSON.Arrays = "auto"
		actual, err := filemerge.FormatDocument("a.json", []byte(`["a long value wider than the forty character target"]`), policy)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(actual)).To(HavePrefix("[\n"))
	})

	It("keeps commented JSONC collections expanded without losing comments", func() {
		policy := config.DefaultFormattingPolicy()
		policy.Common.InlineMaxChars = 320
		policy.JSON.Arrays = "auto"
		actual, err := filemerge.FormatDocument("a.jsonc", []byte("[1, // keep\n2]"), policy)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(actual)).To(ContainSubstring("// keep"))
		Expect(string(actual)).To(HavePrefix("[\n"))
		Expect(filemerge.FormatDocument("a.jsonc", actual, policy)).To(Equal(actual))
	})
})
