package filemerge_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
)

// overrides writes a spec's adjustments the way they are stored: JSON, whatever
// the file they patch is written in.
func overrides(document string) json.RawMessage { return json.RawMessage(document) }

var _ = Describe("Merging a structured file [Unit]", func() {
	Describe("a deep merge", func() {
		It("takes the template where the repository says nothing", func() {
			merged, err := filemerge.Apply("renovate.json", []byte(`{
  "extends": ["config:recommended"],
  "timezone": "UTC"
}`), filemerge.Spec{Overrides: overrides(`{"timezone": "Europe/Warsaw"}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring(`"extends"`))
			Expect(string(merged)).To(ContainSubstring(`"Europe/Warsaw"`))
		})

		It("merges objects key by key rather than replacing them", func() {
			merged, err := filemerge.Apply("renovate.json", []byte(
				`{"vulnerabilityAlerts": {"enabled": true, "automerge": true}}`,
			), filemerge.Spec{Overrides: overrides(`{"vulnerabilityAlerts": {"automerge": false}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(decoded(merged)).To(HaveKeyWithValue("vulnerabilityAlerts", And(
				HaveKeyWithValue("enabled", BeTrue()),
				HaveKeyWithValue("automerge", BeFalse()),
			)))
		})

		It("removes a key the repository sets to null", func() {
			merged, err := filemerge.Apply("renovate.json",
				[]byte(`{"automergeStrategy": "squash", "timezone": "UTC"}`),
				filemerge.Spec{Overrides: overrides(`{"automergeStrategy": null}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(decoded(merged)).NotTo(HaveKey("automergeStrategy"))
			Expect(decoded(merged)).To(HaveKey("timezone"))
		})

		// RFC 7396: an object patched onto anything that is not an object
		// replaces it, and the nulls inside remove nothing, because there is
		// nothing there to remove.
		It("replaces a scalar with an object, dropping the nulls inside it", func() {
			merged, err := filemerge.Apply("renovate.json", []byte(`{"schedule": "weekly"}`),
				filemerge.Spec{Overrides: overrides(`{"schedule": {"on": "monday", "off": null}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(decoded(merged)).To(HaveKeyWithValue("schedule",
				And(HaveKeyWithValue("on", "monday"), Not(HaveKey("off")))))
		})
	})

	Describe("a shallow merge", func() {
		It("replaces a top-level key whole rather than merging into it", func() {
			merged, err := filemerge.Apply("renovate.json", []byte(
				`{"vulnerabilityAlerts": {"enabled": true, "automerge": true}}`,
			), filemerge.Spec{
				Strategy:  filemerge.StrategyShallow,
				Overrides: overrides(`{"vulnerabilityAlerts": {"automerge": false}}`),
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(decoded(merged)).To(HaveKeyWithValue("vulnerabilityAlerts",
				And(HaveKeyWithValue("automerge", BeFalse()), Not(HaveKey("enabled")))))
		})

		It("removes a top-level key set to null", func() {
			merged, err := filemerge.Apply("renovate.json",
				[]byte(`{"timezone": "UTC", "extends": []}`),
				filemerge.Spec{
					Strategy: filemerge.StrategyShallow, Overrides: overrides(`{"timezone": null}`),
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(decoded(merged)).NotTo(HaveKey("timezone"))
		})
	})

	Describe("lists", func() {
		template := []byte(`{"ignorePaths": ["vendor/**", "dist/**"]}`)

		It("replaces one without a rule, which is what RFC 7396 says", func() {
			merged, err := filemerge.Apply("renovate.json", template,
				filemerge.Spec{Overrides: overrides(`{"ignorePaths": ["crates/**"]}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(list(merged, "ignorePaths")).To(Equal([]any{"crates/**"}))
		})

		It("appends the repository's entries after the template's", func() {
			merged, err := filemerge.Apply("renovate.json", template, filemerge.Spec{
				Overrides: overrides(`{"ignorePaths": ["crates/**"]}`),
				Arrays: []filemerge.ArrayRule{
					{Path: "$.ignorePaths", Strategy: filemerge.ArrayAppend},
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(list(merged, "ignorePaths")).To(Equal(
				[]any{"vendor/**", "dist/**", "crates/**"}))
		})

		It("prepends them before", func() {
			merged, err := filemerge.Apply("renovate.json", template, filemerge.Spec{
				Overrides: overrides(`{"ignorePaths": ["crates/**"]}`),
				Arrays: []filemerge.ArrayRule{
					{Path: "$.ignorePaths", Strategy: filemerge.ArrayPrepend},
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(list(merged, "ignorePaths")).To(Equal(
				[]any{"crates/**", "vendor/**", "dist/**"}))
		})

		It("appends onto a template that has no list of its own", func() {
			merged, err := filemerge.Apply("renovate.json", []byte(`{"timezone": "UTC"}`),
				filemerge.Spec{
					Overrides: overrides(`{"labels": ["automation"]}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.labels", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(list(merged, "labels")).To(Equal([]any{"automation"}))
		})

		It("reaches a list nested inside an object", func() {
			merged, err := filemerge.Apply("renovate.json",
				[]byte(`{"hostRules": {"matchHost": ["github.com"]}}`), filemerge.Spec{
					Overrides: overrides(`{"hostRules": {"matchHost": ["gitlab.com"]}}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.hostRules.matchHost", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())

			nested, _ := decoded(merged)["hostRules"].(map[string]any)
			Expect(nested).To(HaveKeyWithValue("matchHost",
				[]any{"github.com", "gitlab.com"}))
		})

		// The engine this replaces split a path on every dot, so a key with one
		// in it could not be addressed at all and the rule matched nothing.
		It("reaches a key that has a dot in its name", func() {
			merged, err := filemerge.Apply("renovate.json",
				[]byte(`{"example.com": ["one"]}`), filemerge.Spec{
					Overrides: overrides(`{"example.com": ["two"]}`),
					Arrays: []filemerge.ArrayRule{
						{Path: `$.example\.com`, Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(list(merged, "example.com")).To(Equal([]any{"one", "two"}))
		})

		It("removes what appears twice when asked to", func() {
			merged, err := filemerge.Apply("renovate.json",
				[]byte(`{"labels": ["automation", "renovate"]}`), filemerge.Spec{
					Overrides:   overrides(`{"labels": ["renovate", "deps"]}`),
					Arrays:      []filemerge.ArrayRule{{Path: "$.labels", Strategy: filemerge.ArrayAppend}},
					Deduplicate: true,
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(list(merged, "labels")).To(Equal([]any{"automation", "renovate", "deps"}))
		})

		// The engine this replaces marshalled through JSON, so a YAML template's
		// int(1) met an override's float64(1), cmp.Equal called them different,
		// and the deduplication it had been asked for silently did nothing.
		It("removes a repeat the template wrote as YAML and the override as JSON", func() {
			merged, err := filemerge.Apply("renovate.yaml", []byte("ports:\n  - 8080\n"),
				filemerge.Spec{
					Overrides:   overrides(`{"ports": [8080, 9090]}`),
					Arrays:      []filemerge.ArrayRule{{Path: "$.ports", Strategy: filemerge.ArrayAppend}},
					Deduplicate: true,
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal("ports:\n  - 8080\n  - 9090\n"))
		})

		It("applies rules in the order they are configured", func() {
			spec := func(first, second filemerge.ArrayStrategy) filemerge.Spec {
				return filemerge.Spec{
					Overrides: overrides(`{"one": ["b"], "two": ["b"]}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.one", Strategy: first},
						{Path: "$.two", Strategy: second},
					},
				}
			}

			template := []byte(`{"one": ["a"], "two": ["a"]}`)

			merged, err := filemerge.Apply("f.json", template,
				spec(filemerge.ArrayAppend, filemerge.ArrayPrepend))

			Expect(err).NotTo(HaveOccurred())
			Expect(list(merged, "one")).To(Equal([]any{"a", "b"}))
			Expect(list(merged, "two")).To(Equal([]any{"b", "a"}))
		})
	})

	// Every one of these was silence in the engine this replaces: a rule that
	// matched nothing left the list replaced and said so nowhere, so a mistyped
	// path read as a file that needed no merging.
	DescribeTable("refuses a list rule that addresses nothing",
		func(rule filemerge.ArrayRule, template, override, because string) {
			_, err := filemerge.Apply("renovate.json", []byte(template), filemerge.Spec{
				Overrides: overrides(override),
				Arrays:    []filemerge.ArrayRule{rule},
			})

			Expect(err).To(MatchError(filemerge.ErrNothingAddressed))
			Expect(err.Error()).To(ContainSubstring(because))
		},
		Entry("a path no override sets",
			filemerge.ArrayRule{Path: "$.ignorePath", Strategy: filemerge.ArrayAppend},
			`{"ignorePaths": ["a"]}`, `{"ignorePaths": ["b"]}`,
			"no override sets $.ignorePath"),
		Entry("a path whose override is not a list",
			filemerge.ArrayRule{Path: "$.schedule", Strategy: filemerge.ArrayAppend},
			`{"schedule": ["a"]}`, `{"schedule": "weekly"}`,
			"is not a list"),
	)

	Describe("numbers", func() {
		// float64 has 53 bits of mantissa, so an identifier past that comes back
		// from a JSON round trip as a different number. The engine this replaces
		// marshalled through JSON on every merge.
		It("keeps an identifier too large for a float64 exactly as it was", func() {
			merged, err := filemerge.Apply("ids.json",
				[]byte(`{"actor": 9007199254740993}`),
				filemerge.Spec{Overrides: overrides(`{"other": 9007199254740995}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("9007199254740993"))
			Expect(string(merged)).To(ContainSubstring("9007199254740995"))
		})

		It("writes a number into YAML as a number rather than as text", func() {
			merged, err := filemerge.Apply("workflow.yaml", []byte("timeout: 5\n"),
				filemerge.Spec{Overrides: overrides(`{"timeout": 30, "retries": 2}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("timeout: 30"))
			Expect(string(merged)).To(ContainSubstring("retries: 2"))
			Expect(string(merged)).NotTo(ContainSubstring(`"30"`))
		})
	})

	It("takes the template unchanged when nothing is configured", func() {
		template := []byte("# Contributing\n")

		Expect(filemerge.Apply("CONTRIBUTING.md", template, filemerge.Spec{})).
			To(Equal(template))
	})

	DescribeTable("refuses a file it cannot read",
		func(path, template string) {
			_, err := filemerge.Apply(path, []byte(template),
				filemerge.Spec{Overrides: overrides(`{"a": 1}`)})

			Expect(err).To(MatchError(filemerge.ErrUnreadable))
		},
		Entry("JSON that does not parse", "f.json", `{"a":`),
		Entry("YAML that does not parse", "f.yaml", "a: [1\n"),
		Entry("JSON whose top level is a list", "f.json", `[1, 2]`),
		Entry("YAML whose top level is a scalar", "f.yaml", "hello\n"),
		Entry("an empty file", "f.json", ""),
	)
})

func decoded(data []byte) map[string]any {
	GinkgoHelper()

	var document map[string]any
	Expect(json.Unmarshal(data, &document)).To(Succeed())

	return document
}

func list(data []byte, key string) []any {
	GinkgoHelper()

	values, ok := decoded(data)[key].([]any)
	Expect(ok).To(BeTrue(), "expected %s to hold a list", key)

	return values
}
