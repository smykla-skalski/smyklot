package filemerge_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// overrides writes a spec's adjustments the way they are stored: JSON, whatever
// the file they patch is written in.
func overrides(document string) json.RawMessage { return json.RawMessage(document) }

func applyFileMerge(filePath string, template []byte, spec filemerge.Spec) ([]byte, error) {
	return filemerge.Apply(filePath, template, spec, config.DefaultFormattingPolicy())
}

var _ = Describe("Merging a structured file [Unit]", func() {
	Describe("a deep merge", func() {
		It("takes the template where the repository says nothing", func() {
			merged, err := applyFileMerge("renovate.json", []byte(`{
  "extends": ["config:recommended"],
  "timezone": "UTC"
}`), filemerge.Spec{Overrides: overrides(`{"timezone": "Europe/Warsaw"}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring(`"extends"`))
			Expect(string(merged)).To(ContainSubstring(`"Europe/Warsaw"`))
		})

		It("merges objects key by key rather than replacing them", func() {
			merged, err := applyFileMerge("renovate.json", []byte(
				`{"vulnerabilityAlerts": {"enabled": true, "automerge": true}}`,
			), filemerge.Spec{Overrides: overrides(`{"vulnerabilityAlerts": {"automerge": false}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(decoded(merged)).To(HaveKeyWithValue("vulnerabilityAlerts", And(
				HaveKeyWithValue("enabled", BeTrue()),
				HaveKeyWithValue("automerge", BeFalse()),
			)))
		})

		It("removes a key the repository sets to null", func() {
			merged, err := applyFileMerge("renovate.json",
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
			merged, err := applyFileMerge("renovate.json", []byte(`{"schedule": "weekly"}`),
				filemerge.Spec{Overrides: overrides(`{"schedule": {"on": "monday", "off": null}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(decoded(merged)).To(HaveKeyWithValue("schedule",
				And(HaveKeyWithValue("on", "monday"), Not(HaveKey("off")))))
		})
	})

	Describe("a shallow merge", func() {
		It("replaces a top-level key whole rather than merging into it", func() {
			merged, err := applyFileMerge("renovate.json", []byte(
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
			merged, err := applyFileMerge("renovate.json",
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
			merged, err := applyFileMerge("renovate.json", template,
				filemerge.Spec{Overrides: overrides(`{"ignorePaths": ["crates/**"]}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(list(merged, "ignorePaths")).To(Equal([]any{"crates/**"}))
		})

		It("appends the repository's entries after the template's", func() {
			merged, err := applyFileMerge("renovate.json", template, filemerge.Spec{
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
			merged, err := applyFileMerge("renovate.json", template, filemerge.Spec{
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
			merged, err := applyFileMerge("renovate.json", []byte(`{"timezone": "UTC"}`),
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
			merged, err := applyFileMerge("renovate.json",
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
			merged, err := applyFileMerge("renovate.json",
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
			merged, err := applyFileMerge("renovate.json",
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
			merged, err := applyFileMerge("renovate.yaml", []byte("ports:\n  - 8080\n"),
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

			merged, err := applyFileMerge("f.json", template,
				spec(filemerge.ArrayAppend, filemerge.ArrayPrepend))

			Expect(err).NotTo(HaveOccurred())
			Expect(list(merged, "one")).To(Equal([]any{"a", "b"}))
			Expect(list(merged, "two")).To(Equal([]any{"b", "a"}))
		})
	})

	// Every one of these was silence in the engine this replaces: a rule that
	// matched nothing left the list replaced and said so nowhere, so a mistyped
	// path read as a file that needed no merging.
	//
	// Refused as a spec rather than as a merge, because neither depends on the
	// template: a rule the overrides do not reach could never work on any file,
	// which is why Validate answers it and this covers that it still does
	// through the whole entry point.
	DescribeTable("refuses a list rule that addresses nothing",
		func(rule filemerge.ArrayRule, template, override, because string) {
			_, err := applyFileMerge("renovate.json", []byte(template), filemerge.Spec{
				Overrides: overrides(override),
				Arrays:    []filemerge.ArrayRule{rule},
			})

			Expect(err).To(MatchError(filemerge.ErrInvalidSpec))
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
			merged, err := applyFileMerge("ids.json",
				[]byte(`{"actor": 9007199254740993}`),
				filemerge.Spec{Overrides: overrides(`{"other": 9007199254740995}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("9007199254740993"))
			Expect(string(merged)).To(ContainSubstring("9007199254740995"))
		})

		It("writes a number into YAML as a number rather than as text", func() {
			merged, err := applyFileMerge("workflow.yaml", []byte("timeout: 5\n"),
				filemerge.Spec{Overrides: overrides(`{"timeout": 30, "retries": 2}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("timeout: 30"))
			Expect(string(merged)).To(ContainSubstring("retries: 2"))
			Expect(string(merged)).NotTo(ContainSubstring(`"30"`))
		})
	})

	It("takes the template unchanged when nothing is configured", func() {
		template := []byte("# Contributing\n")

		Expect(applyFileMerge("CONTRIBUTING.md", template, filemerge.Spec{})).
			To(Equal(template))
	})

	// A YAML file decoded into Go values and encoded back is not the file that
	// was read. Every plain scalar goes through the resolver on the way in and
	// strconv on the way out, and none of what follows needs the merge to have
	// gone anywhere near it: one changed key re-rendered the whole document.
	//
	// A workflow whose Go version silently became 1.2 is a workflow that no
	// longer builds, put there by a bot, in a pull request whose description
	// says it changed something else.
	Describe("a YAML template the merge did not touch", func() {
		template := []byte(`# What every repository builds with.
name: build
on: push
jobs:
  build:
    # The oldest release this still supports.
    go-version: 1.20
    mode: 0644
    since: 2024-01-01
    retries: 1.0
    verbose: yes
    steps:
      - uses: actions/checkout@v4
`)

		merged := func() string {
			GinkgoHelper()

			out, err := applyFileMerge(".github/workflows/ci.yaml", template,
				filemerge.Spec{Overrides: overrides(`{"name": "release"}`)})
			Expect(err).NotTo(HaveOccurred())

			return string(out)
		}

		DescribeTable("writes every value back as it was written",
			func(line string) {
				Expect(merged()).To(ContainSubstring(line))
			},

			// 1.20 read as a float and formatted with %g comes back 1.2
			Entry("a version that reads as a number", "go-version: 1.20"),
			// go-yaml v3 keeps YAML 1.1 octals, so this decoded as 420
			Entry("a mode with a leading zero", "mode: 0644"),
			// A bare date decodes to time.Time and re-encodes with a clock
			Entry("a date", "since: 2024-01-01"),
			Entry("a float that is a whole number", "retries: 1.0"),
			Entry("a word that reads as a boolean", "verbose: yes"),
			Entry("a key that reads as a boolean", "on: push"),
		)

		It("keeps the comments somebody wrote", func() {
			Expect(merged()).To(ContainSubstring("# What every repository builds with."))
			Expect(merged()).To(ContainSubstring("# The oldest release this still supports."))
		})

		// Written back through the document rather than the mapping inside it.
		// A comment with a blank line under it belongs to the file rather than
		// to the first key, so writing the mapping alone dropped exactly the
		// line that says who generated the file - and the spec above passed,
		// because its comment sits directly above a key and rides along with it.
		It("keeps a comment that belongs to the file rather than to a key", func() {
			out, err := applyFileMerge("ci.yaml",
				[]byte("# Generated by Smyklot. Edit the template, not this.\n\nname: build\n"),
				filemerge.Spec{Overrides: overrides(`{"name": "release"}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("# Generated by Smyklot."))
			Expect(string(out)).To(ContainSubstring("name: release"))
		})

		// go-yaml reads `<<` as a scalar tagged !!merge and writes that tag
		// back out, because !!merge is not one of the tags it treats as
		// implied - so a file that used one gained a `!!merge <<:` nobody
		// wrote, and every repository carrying it saw that in the diff.
		DescribeTable("writes a construction back the way it arrived",
			func(template string) {
				out, err := applyFileMerge("ci.yaml", []byte(template),
					filemerge.Spec{Overrides: overrides(`{"name": "release"}`)})

				Expect(err).NotTo(HaveOccurred())
				Expect(string(out)).To(ContainSubstring(
					strings.ReplaceAll(template, "name: build\n", "")))
			},

			Entry("a merge key", "defaults: &d\n  run: make\njobs:\n  <<: *d\nname: build\n"),
			Entry("an anchor and an alias", "defaults: &d\n  run: make\nuse: *d\nname: build\n"),
			Entry("a block scalar", "name: build\nscript: |\n  one\n  two\n"),
			Entry("a flow sequence", "name: build\nports: [80, 443]\n"),
			Entry("a flow mapping", "name: build\nwith: {a: 1}\n"),
			Entry("an empty mapping", "name: build\nempty: {}\n"),
			Entry("a quoted key", "name: build\n\"on\": push\n"),
		)

		// An anchor belongs to the place rather than to the value that was
		// there. Dropped by a replacement, every `*alias` naming it referred to
		// nothing - so the merge produced a file that is not YAML at all, and
		// the bot opened a pull request turning somebody's workflow into a file
		// GitHub will not load.
		DescribeTable("keeps a file readable when it replaces an anchored value",
			func(template string, spec filemerge.Spec) {
				merged, err := applyFileMerge("ci.yaml", []byte(template), spec)
				Expect(err).NotTo(HaveOccurred())

				// Read back rather than matched: a dangling alias is a file
				// that parses nowhere, and only parsing it says so.
				var back any
				Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
			},

			Entry("a scalar",
				"version: &v \"1.2.3\"\nimage: my/app\ntag: *v\n",
				filemerge.Spec{Overrides: overrides(`{"version": "2.0.0"}`)}),
			Entry("a list, through a rule",
				"labels: &l\n  - a\nother: *l\n",
				filemerge.Spec{
					Overrides: overrides(`{"labels": ["c"]}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.labels", Strategy: filemerge.ArrayAppend},
					},
				}),
			Entry("a mapping",
				"defaults: &d\n  runs-on: ubuntu\nuse: *d\n",
				filemerge.Spec{Overrides: overrides(`{"defaults": {"runs-on": "macos"}}`)}),
		)

		// An alias stands for the mapping it names. Read as the node it
		// literally is, it was not a mapping, so a deep merge replaced the
		// whole thing - and every key the alias carried that the override did
		// not mention went with it.
		It("merges into what an alias stands for, rather than over it", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("common: &c\n  labels:\n    - a\n  owner: platform\nx: *c\n"),
				filemerge.Spec{Overrides: overrides(`{"x": {"labels": ["b"]}}`)})

			Expect(err).NotTo(HaveOccurred())

			var back struct {
				X map[string]any `yaml:"x"`
			}
			Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
			Expect(back.X).To(HaveKeyWithValue("owner", "platform"))
			Expect(back.X).To(HaveKey("labels"))

			// Into a copy, not into the anchor: what it names is shared, so
			// merging there would change every other place naming it.
			Expect(string(merged)).To(ContainSubstring("common: &c"))
			Expect(string(merged)).To(ContainSubstring("- a"))
		})

		// Nothing can keep this one: the key carrying the anchor is gone, and
		// what referred to it cannot be left naming nothing.
		It("refuses to remove an anchor something still refers to", func() {
			_, err := applyFileMerge("ci.yaml",
				[]byte("defaults: &d\n  runs-on: ubuntu\njobs:\n  build:\n    <<: *d\n"),
				filemerge.Spec{Overrides: overrides(`{"defaults": null}`)})

			Expect(err).To(MatchError(filemerge.ErrUnwritable))
			Expect(err.Error()).To(ContainSubstring(`"*d"`))
		})

		// The name still exists here - the list rule writes a fresh copy of the
		// template's item, anchor and all - but it lands below the alias, and an
		// alias reads upwards. Asking only whether the name is still somewhere
		// in the document called this fine and wrote a file that will not load.
		It("refuses where the surviving anchor lands below the alias", func() {
			_, err := applyFileMerge("ci.yaml",
				[]byte("defaults: &d\n  labels:\n    - &x keep\nafter: *x\nthing:\n  <<: *d\n"),
				filemerge.Spec{
					Overrides: overrides(`{"defaults": {"labels": ["d"]}, "thing": {"labels": ["t"]}}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.thing.labels", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).To(MatchError(filemerge.ErrUnwritable))
			Expect(err.Error()).To(ContainSubstring(`"*x"`))
		})

		// And a recursive anchor is not that: `&loop` is defined by the node the
		// alias inside it names, so walking in writing order has to record the
		// name before descending, or the merge refuses a document go-yaml reads
		// and writes quite happily.
		It("leaves an anchor named from inside itself alone", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("loop: &loop\n  self: *loop\nname: build\n"),
				filemerge.Spec{Overrides: overrides(`{"name": "test"}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("self: *loop"))
		})

		// An alias is what the template says is at that path. Read as the node
		// it literally is, a list rule addressing something reached through one
		// found no mapping, took the template as carrying no list, and appended
		// the repository's items to nothing - a replacement, silently, for a
		// rule that says append.
		It("appends to a list the template reached through an alias", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("base: &b\n  ports:\n    - 80\nuse: *b\n"),
				filemerge.Spec{
					Overrides: overrides(`{"use": {"ports": [443]}}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.use.ports", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())

			// Counted rather than matched: the template's own list is still in
			// the file under the anchor, so a substring finds "- 80" whether or
			// not the append reached it.
			Expect(strings.Count(string(merged), "- 80")).To(Equal(2))
			Expect(string(merged)).To(ContainSubstring("- 443"))
		})

		// A list is appended to what the template held, and the same override
		// replacing the anchored list must not change that. Taking the template
		// aside by copying its nodes did: a copy carries each alias's pointer to
		// the anchor in the tree it came from, so following one led back into
		// the document being edited and the append landed on what the merge had
		// already written there.
		It("appends to what the template held, not to what the merge wrote", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("base: &b\n  ports:\n    - 80\nuse: *b\n"),
				filemerge.Spec{
					Overrides: overrides(
						`{"base": {"ports": [9999]}, "use": {"ports": [443]}}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.use.ports", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("- 80"))
			Expect(strings.Count(string(merged), "- 9999")).To(Equal(1))
		})

		// A merge key is an alias spelled differently: the mapping holds every
		// key the anchor does, without spelling one of them out. Read for its
		// literal keys it looks very nearly empty, which is how a deep merge
		// came to replace what a job inherited rather than adding to it - and
		// the files this synchronizes are written this way constantly.
		It("merges into what a merge key gives a mapping, rather than over it", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("common: &c\n  with:\n    node: 18\n    cache: npm\n"+
					"job:\n  <<: *c\n  name: build\n"),
				filemerge.Spec{Overrides: overrides(`{"job": {"with": {"node": "20"}}}`)})

			Expect(err).NotTo(HaveOccurred())

			// Read back rather than matched, because what a mapping holds is
			// what a reader resolves and not what any one line says.
			var back struct {
				Job map[string]any `yaml:"job"`
			}
			Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
			Expect(back.Job).To(HaveKeyWithValue("name", "build"))
			Expect(back.Job).To(HaveKeyWithValue("with", map[string]any{
				"node": "20", "cache": "npm",
			}))

			// Into a copy: what the anchor names is shared, and the template's
			// own `common` still says 18.
			Expect(string(merged)).To(ContainSubstring("node: 18"))
		})

		It("appends to a list a merge key gives a mapping", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("base: &b\n  ports:\n    - 80\nuse:\n  <<: *b\n"),
				filemerge.Spec{
					Overrides: overrides(`{"use": {"ports": [443]}}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.use.ports", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())

			var back struct {
				Use map[string]any `yaml:"use"`
			}
			Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
			Expect(back.Use).To(HaveKeyWithValue("ports", []any{80, 443}))
		})

		// A key a mapping only inherits is not that mapping's to remove: it
		// lives where the anchor is, which other places name too. Nor is the
		// literal one, where a merge key carries the same key - removing it
		// does not remove the key, it lets the inherited value back through, so
		// a removal lands as a change of value. Both were silence.
		DescribeTable("refuses to remove a key a merge key carries",
			func(template string) {
				_, err := applyFileMerge("ci.yaml", []byte(template),
					filemerge.Spec{Overrides: overrides(`{"job": {"a": null}}`)})

				Expect(err).To(MatchError(filemerge.ErrUnwritable))
				Expect(err.Error()).To(ContainSubstring(`"<<"`))
			},
			Entry("the key is only inherited",
				"defaults: &d\n  a: 1\n  b: 2\njob:\n  <<: *d\n"),
			Entry("the mapping shadows the inherited key",
				"defaults: &d\n  a: 1\njob:\n  <<: *d\n  a: 5\n"),
		)

		// `<<` is not a key, it is how YAML spells inheritance, and an override
		// setting it writes one whose value is an ordinary object, string or
		// number - a file that does not load at all: "map merge requires map or
		// sequence of maps as the value".
		//
		// Read off the override rather than caught where a mapping is merged,
		// because there are four ways one reaches the file and only the first
		// goes through that code.
		DescribeTable("refuses an override that writes a merge key itself",
			func(template, override string) {
				_, err := applyFileMerge("ci.yaml", []byte(template),
					filemerge.Spec{Overrides: overrides(override)})

				Expect(err).To(MatchError(filemerge.ErrUnwritable))
				Expect(err.Error()).To(ContainSubstring(`"<<"`))
			},
			Entry("merged into a mapping the template has",
				"job:\n  name: build\n", `{"job": {"<<": {"a": 1}}}`),
			Entry("in an object built fresh",
				"name: build\n", `{"jobs": {"<<": {"a": 1}}}`),
			Entry("in an object replacing a scalar",
				"job: 5\n", `{"job": {"<<": "x"}}`),
			Entry("in a list item",
				"name: build\n", `{"steps": [{"<<": "x"}]}`),
		)

		// Plain is what makes a merge key one. Read as inheritance, a quoted
		// `"<<"` had a removal refused for a key the mapping does not have.
		It("reads a quoted merge key as the ordinary key it is", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("d: &d\n  a: 1\nj:\n  \"<<\": *d\n  a: 2\n"),
				filemerge.Spec{Overrides: overrides(`{"j": {"a": null}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring(`"<<": *d`))
			Expect(string(merged)).NotTo(ContainSubstring("a: 2"))
		})

		// A merge key can name several mappings, so the walk branches, and a
		// depth bound leaves the work exponential in the depth. This template
		// is twenty-one bytes and used never to return, on a sync worker with
		// no timeout - a wedged goroutine rather than a failed sync.
		//
		// The override has to name a key the mapping does not spell out, since
		// that is what sends the read looking through the inheritance - a key
		// the mapping has is answered without walking anything.
		DescribeTable("answers a merge key that names itself",
			func(override string) {
				done := make(chan struct{})

				go func() {
					defer GinkgoRecover()
					defer close(done)

					_, err := applyFileMerge("ci.yaml",
						[]byte("a: &a\n  <<: [*a, *a]\n"),
						filemerge.Spec{Overrides: overrides(override)})
					Expect(err).NotTo(HaveOccurred())

					_ = err
				}()

				Eventually(done, "5s").Should(BeClosed())
			},
			Entry("an object under a key it does not have", `{"a": {"k": {"deep": 1}}}`),
			Entry("a removal of a key it does not have", `{"a": {"k": null}}`),
		)

		// An alias binds to the nearest definition above it, so a copy that kept
		// the template's anchor names would take every later `*name` with it.
		// This one changes `x.inner`, and `later` names the same anchor - it has
		// to go on meaning what the template anchored.
		It("does not redefine an anchor nested inside what it copies", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("common: &c\n  inner: &i\n    k: 1\nx: *c\nlater: *i\n"),
				filemerge.Spec{Overrides: overrides(`{"x": {"inner": {"k": 2}}}`)})

			Expect(err).NotTo(HaveOccurred())

			var back struct {
				X     map[string]any `yaml:"x"`
				Later map[string]any `yaml:"later"`
			}
			Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
			Expect(back.X).To(HaveKeyWithValue("inner", map[string]any{"k": 2}))
			Expect(back.Later).To(HaveKeyWithValue("k", 1))

			// One definition of the name, because two is a document whose
			// meaning depends on where the reader is standing.
			Expect(strings.Count(string(merged), "&i\n")).To(Equal(1))
		})

		// The other side of the same copy. `from` means "whatever img is", and
		// that is what the template guarantees inside `zcommon` and therefore
		// inside anything standing for it - so the copy's pair has to keep
		// pointing at each other rather than back at the template's.
		It("keeps a copied alias following the copy it was taken with", func() {
			merged, err := applyFileMerge("compose.yaml",
				[]byte("zcommon: &c\n  img: &i alpine\n  from: *i\nx: *c\n"),
				filemerge.Spec{Overrides: overrides(`{"x": {"img": "debian"}}`)})

			Expect(err).NotTo(HaveOccurred())

			var back struct {
				Zcommon map[string]any `yaml:"zcommon"`
				X       map[string]any `yaml:"x"`
			}
			Expect(yaml.Unmarshal(merged, &back)).To(Succeed())

			Expect(back.X).To(HaveKeyWithValue("img", "debian"))
			Expect(back.X).To(HaveKeyWithValue("from", "debian"))

			// And the template's own pair is untouched, which is the half that
			// keeping the anchor names would have broken.
			Expect(back.Zcommon).To(HaveKeyWithValue("img", "alpine"))
			Expect(back.Zcommon).To(HaveKeyWithValue("from", "alpine"))
		})

		// The copy keeps a name of its own, so merging the template's own
		// definition away in the same override leaves the copy readable rather
		// than refused for an anchor that went with it.
		It("keeps a copy readable when the anchor it came from is removed", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("zbase: &t\n  q: &i 5\n  r: *i\nalias: *t\n"),
				filemerge.Spec{Overrides: overrides(`{"alias": {"x": 1}, "zbase": null}`)})

			Expect(err).NotTo(HaveOccurred())

			var back struct {
				Alias map[string]any `yaml:"alias"`
			}
			Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
			Expect(back.Alias).To(HaveKeyWithValue("q", 5))
			Expect(back.Alias).To(HaveKeyWithValue("r", 5))
			Expect(back.Alias).To(HaveKeyWithValue("x", 1))
		})

		// A list written beside what it was copied from rather than over it
		// carries the copies' anchors, so the file defines one twice. Both say
		// the same thing and it reloads, but a duplicate anchor is what the
		// rename beside this exists to keep out of these files, and a linter in
		// the repository would stop on it. Written *over* it, the copy is the
		// only definition left and has to keep the name, or `after` names
		// nothing - so one name, however the write landed.
		DescribeTable("leaves one definition of an anchor the written list carries",
			func(template string) {
				merged, err := applyFileMerge("ci.yaml", []byte(template),
					filemerge.Spec{
						Overrides: overrides(`{"thing": {"labels": ["extra"]}}`),
						Arrays: []filemerge.ArrayRule{
							{Path: "$.thing.labels", Strategy: filemerge.ArrayAppend},
						},
					})

				Expect(err).NotTo(HaveOccurred())
				Expect(strings.Count(string(merged), "&x")).To(Equal(1))

				var back struct {
					Thing map[string]any `yaml:"thing"`
					After string         `yaml:"after"`
				}
				Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
				Expect(back.Thing).To(HaveKeyWithValue("labels", []any{"keep", "extra"}))
				Expect(back.After).To(Equal("keep"))
			},
			Entry("written beside the list it was copied from, through a merge key",
				"defaults: &d\n  labels:\n    - &x keep\nthing:\n  <<: *d\nafter: *x\n"),
			Entry("written over the list the anchor is defined in",
				"thing:\n  labels:\n    - &x keep\nafter: *x\n"),
		)

		// A template that already defines one name twice. Counting definitions
		// says two whatever the merge did, so the reading that clears a
		// duplicate has to leave these alone - the second definition is not one
		// this merge made, and which one an alias means depends on where the
		// alias is written.
		DescribeTable("leaves a name the template itself defines twice",
			func(template, expected string) {
				merged, err := applyFileMerge("ci.yaml", []byte(template),
					filemerge.Spec{
						Overrides: overrides(`{"labels": ["extra"]}`),
						Arrays: []filemerge.ArrayRule{
							{Path: "$.labels", Strategy: filemerge.ArrayAppend},
						},
					})

				// Read back, because the whole cost of getting this wrong is a
				// file YAML will not load - an alias whose only definition
				// above it was the one that got cleared.
				Expect(err).NotTo(HaveOccurred())

				var back map[string]any
				Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
				Expect(back).To(HaveKeyWithValue("middle", expected))
			},
			Entry("the second definition written after the list",
				"labels:\n  - &x keep\nmiddle: *x\nother: &x elsewhere\n", "keep"),
			Entry("both definitions inside the list being written",
				"labels:\n  - &x a\n  - &x b\nmiddle: *x\n", "b"),
		)

		// The other direction of the same question, and the one a rule reading
		// "the template defines it twice, so leave it alone" got wrong: here
		// the write lands BESIDE the definition it cloned, between an alias and
		// the definition that alias binds to. Keeping the clone's name rebinds
		// `use` to a different value - a change to a key no rule and no
		// override named, in a file the merge reports as written, and one
		// nothing can catch afterwards because the name is still bound.
		It("does not rebind an alias by writing a clone above its definition", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("defs: &base\n  - &a first\nafter: &a second\nlist: *base\nuse: *a\n"),
				filemerge.Spec{
					Overrides: overrides(`{"list": ["extra"]}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.list", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())

			var back map[string]any
			Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
			Expect(back).To(HaveKeyWithValue("use", "second"))
			Expect(back).To(HaveKeyWithValue("list", []any{"first", "extra"}))
		})

		// A rule whose result is what the file already says writes nothing.
		// Writing it stands in for the inherited list, and the flattening is
		// then the whole of the pull request - the same change nobody asked for
		// that the deep merge above refuses to propose, one level over.
		// The deep merge writes the override's own list at the path first, and
		// builds it fresh - so the anchors and the comments the template wrote
		// are gone from it, and the rule's list, built from clones of the
		// template's items, is what puts them back. A skip that asks whether
		// the two say the same thing sees no difference and declines, leaving
		// an alias with nothing above it and somebody's comments deleted.
		DescribeTable("writes a rule's list back over one that lost what it carried",
			func(template, expected string) {
				merged, err := applyFileMerge("ci.yaml", []byte(template),
					filemerge.Spec{
						Overrides:   overrides(`{"a": {"list": ["one", "two"]}}`),
						Arrays:      []filemerge.ArrayRule{{Path: "$.a.list", Strategy: filemerge.ArrayAppend}},
						Deduplicate: true,
					})

				Expect(err).NotTo(HaveOccurred())
				Expect(string(merged)).To(ContainSubstring(expected))
			},
			// Refused outright before this: the anchor went and refuseDanglingAliases
			// stopped the whole repository's plan on a file that merged cleanly
			// one commit earlier.
			Entry("an anchor an alias below it needs",
				"a:\n  list:\n    - &A one\n    - two\nuse: *A\n", "&A one"),
			Entry("a comment somebody wrote on an item",
				"a:\n  list:\n    - one # keep me\n    - two\n", "# keep me"),
		)

		// What the merge left at a key is remembered so a later writer can be
		// told from the merge itself - but the merge's own settle edits inside
		// those copies as it takes inner keys back, and reading that as a later
		// writer blocked the copy around them for ever. Both of these are a
		// flattening that is the whole of the diff.
		DescribeTable("leaves a copy its own settle emptied",
			func(template, override string) {
				merged, err := applyFileMerge("ci.yaml", []byte(template),
					filemerge.Spec{Overrides: overrides(override)})

				Expect(err).NotTo(HaveOccurred())
				Expect(string(merged)).To(Equal(template))
			},
			Entry("an inner key a merge key gave back",
				"zoth: &o\n  k: v\nbase: &b\n  inner:\n    <<: *o\nthing:\n  <<: *b\n",
				`{"thing": {"inner": {"k": "v"}}}`),
			Entry("an inner alias put back",
				"val: &v\n  x: 1\nbase: &b\n  inner:\n    k: *v\nthing:\n  <<: *b\n",
				`{"thing": {"inner": {"k": {"x": 1}}}}`),
		)

		// The same block, where what it holds in place is a value the override
		// contradicts. Keys are merged in sorted order, so the copy is taken
		// before `zbase` moves, and a reader of athing.inner.sib gets the old
		// value for ever - byte-stable across sweeps, so it never repairs.
		It("keeps a stale sibling in a copy its own settle emptied", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("aoth: &o\n  k: v\nzbase: &b\n  inner:\n    <<: *o\n    sib: old\n"+
					"athing:\n  <<: *b\n"),
				filemerge.Spec{Overrides: overrides(
					`{"athing": {"inner": {"k": "v"}}, "zbase": {"inner": {"sib": "new"}}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal(
				"aoth: &o\n  k: v\nzbase: &b\n  inner:\n    <<: *o\n    sib: new\n" +
					"athing:\n  <<: *b\n"))
		})

		// Taking a copy back off takes everything in it, and a list rule reaches
		// into a copy by its own path. Judged only against the override, the
		// copy reads as saying nothing new and the rule's whole result goes
		// with it - a rule configured, validated, run, and silently dropped.
		It("keeps a copy a list rule has written into", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("base: &base\n  list:\n    - t\na: *base\n"),
				filemerge.Spec{
					Overrides: overrides(`{"a": {"list": ["t"]}}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.a.list", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal(
				"base: &base\n  list:\n    - t\na:\n  list:\n    - t\n    - t\n"))
		})

		// The same, where the copy also holds an inner key its own settle takes
		// back. Reading provenance off the file cannot tell that take from a
		// rule's write, and a memory refreshed to stop it blocking the copy for
		// ever refreshed it from the copy - which by then held the rule's work,
		// so the rule's write read as the merge's own and the copy went, taking
		// the rule's result with it.
		It("keeps a copy a rule wrote into, with an inner key settled", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("oth: &o\n  k: v\nbase: &base\n  inner:\n    <<: *o\n"+
					"  list:\n    - t\na: *base\n"),
				filemerge.Spec{
					Overrides: overrides(`{"a": {"inner": {"k": "v"}, "list": ["t"]}}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.a.list", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal(
				"oth: &o\n  k: v\nbase: &base\n  inner:\n    <<: *o\n  list:\n    - t\n" +
					"a:\n  inner:\n    <<: *o\n  list:\n    - t\n    - t\n"))
		})

		// The same loss where what goes is not a duplicate. Deduplicated, so the
		// copy the rule appended into cannot be read as saying only what the
		// anchor already says - and the item dropped is the template's own, from
		// a rule that was told to append to it.
		It("keeps the template's items a rule appended into a copy", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("oth: &o\n  k: v\nbase: &base\n  inner:\n    <<: *o\n"+
					"  list:\n    - old\na: *base\n"),
				filemerge.Spec{
					Overrides: overrides(
						`{"a": {"inner": {"k": "v"}, "list": ["new"]}, ` +
							`"base": {"list": ["new"]}}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.a.list", Strategy: filemerge.ArrayAppend},
					},
					Deduplicate: true,
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal(
				"oth: &o\n  k: v\nbase: &base\n  inner:\n    <<: *o\n  list:\n    - new\n" +
					"a:\n  inner:\n    <<: *o\n  list:\n    - old\n    - new\n"))
		})

		// The other half of the same question. A rule pins the copy it wrote
		// into, and nothing else - "a rule ran" is not an answer, because the
		// second copy says nothing the anchor does not and flattening it is the
		// change nobody asked for.
		It("takes back a copy beside one a rule wrote into", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("base: &base\n  list:\n    - t\n  k: v\na: *base\nb: *base\n"),
				filemerge.Spec{
					Overrides: overrides(`{"a": {"list": ["t"]}, "b": {"k": "v"}}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.a.list", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal(
				"base: &base\n  list:\n    - t\n  k: v\n" +
					"a:\n  list:\n    - t\n    - t\n  k: v\nb: *base\n"))
		})

		// The rebuild runs a merge of its own, and a merge reads the live
		// document - a removal asks whether the key is inherited. So it can
		// refuse where the real merge did not, over a copy that never appears
		// in any file. A rebuild that cannot be made cannot decide, and what
		// cannot be decided is left alone.
		It("does not fail a file over a rebuild it could not make", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("zoth: &o\n  z: 1\nbase: &b\n  inner:\n    <<: *o\n    k: v\n"+
					"thing:\n  <<: *b\n"),
				filemerge.Spec{Overrides: overrides(
					`{"thing": {"inner": {"c": null}}, "zoth": {"c": 2}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal(
				"zoth: &o\n  z: 1\n  c: 2\nbase: &b\n  inner:\n    <<: *o\n    k: v\n" +
					"thing:\n  <<: *b\n"))
		})

		// Judging a copy rebuilds it, that rebuild settles, and settling judges
		// the copies inside it - so the work is exponential in how far into an
		// inheritance the override walks. One alias per level costs 2^(d+1)-1
		// rebuilds, and unbounded, 644 bytes at depth 22 took five and a half
		// seconds while 30 would have taken about an hour.
		//
		// The clock here is a backstop, not a measurement. Losing the bound
		// makes this shape take half a minute and the one below it eight, so
		// what the deadline buys is a named failure instead of a suite that
		// hangs until the Go timeout fires. It is deliberately far looser than
		// the run it is watching: a threshold tight enough to measure anything
		// is one that fails when the machine is busy, and this suite runs beside
		// twenty others. What the bound actually costs is asserted exactly, and
		// without a clock, in merge_internal_test.go.
		It("bounds a deep inheritance chain rather than working through it", func() {
			var template, override strings.Builder

			const depth = 24

			template.WriteString("lvl0: &lvl0\n  leaf: v\n")

			for at := 1; at <= depth; at++ {
				fmt.Fprintf(&template, "lvl%d: &lvl%d\n  down: *lvl%d\n", at, at, at-1)
			}

			fmt.Fprintf(&template, "top: *lvl%d\n", depth)

			override.WriteString(strings.Repeat(`{"down": `, depth))
			override.WriteString(`{"leaf": "v"}`)
			override.WriteString(strings.Repeat("}", depth))

			started := time.Now()

			merged, err := applyFileMerge("ci.yaml", []byte(template.String()),
				filemerge.Spec{Overrides: overrides(`{"top": ` + override.String() + `}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(time.Since(started)).To(BeNumerically("<", time.Minute))

			// And the bound costs nothing here: every copy still comes off, so
			// the file the repository already has is the file it keeps.
			Expect(string(merged)).To(Equal(template.String()))
		})

		// The one that matters, because the template is innocent. An anchor that
		// names itself is forty-three bytes and one level deep, and the depth
		// comes from the override - which is one repository's data, on a sweep
		// with no deadline and one replica. So the organization's file arms this
		// and any repository can fire it, out of a few hundred bytes. Bounding
		// by depth would have missed it, which is why the budget counts work.
		//
		// Deep enough that the budget runs out before the copies have all been
		// taken back, which is the case worth writing down: what a bound cannot
		// finish deciding it keeps. So this asserts what running out is allowed
		// to cost rather than that it costs nothing.
		It("bounds an override walking into an anchor that names itself", func() {
			const (
				depth    = 40
				template = "a: &a\n  self: *a\n  leaf: base\ntop:\n  k: *a\n"
			)

			spec := filemerge.Spec{Overrides: overrides(`{"top": {"k": ` +
				strings.Repeat(`{"self": `, depth) + `{"leaf": "base"}` +
				strings.Repeat("}", depth) + `}}`)}

			started := time.Now()
			merged, err := applyFileMerge("ci.yaml", []byte(template), spec)

			Expect(err).NotTo(HaveOccurred())
			Expect(time.Since(started)).To(BeNumerically("<", time.Minute))

			// Nothing of the organization's file is lost - the flattening is
			// verbose, not different, and it still ends at the anchor rather
			// than expanding it for ever.
			Expect(string(merged)).To(HavePrefix("a: &a\n  self: *a\n  leaf: base\ntop:\n"))
			Expect(string(merged)).To(ContainSubstring("self: *a\n"))

			// And it has converged. A bound that keeps a copy must not then
			// propose a different file next sweep: that is a pull request
			// reopening itself for ever, which is worse than the flattening.
			again, err := applyFileMerge("ci.yaml", merged, spec)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(again)).To(Equal(string(merged)))
		})

		// A key the override adds outright is no candidate for anything, right
		// up until a later key of the same override puts it on the anchor this
		// mapping reads. Recorded either way, for the same reason everything
		// else here is judged at the end rather than where it was written.
		It("takes back a key a later override key made inherited", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("zbase: &b\n  z: 1\nthing:\n  <<: *b\n"),
				filemerge.Spec{Overrides: overrides(`{"thing": {"k": "v"}, "zbase": {"k": "v"}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal("zbase: &b\n  z: 1\n  k: v\nthing:\n  <<: *b\n"))
		})

		// A copy is derived from what it was copied from, so a copy taken at
		// merge time says what the anchor said at merge time. Judged later
		// against an anchor the same run has moved, it differs for a reason
		// nothing to do with the override - so it is kept, and the mapping
		// stops inheriting: every key the copy dragged along is pinned to the
		// old values, including keys the override never named. Judged against
		// what the copy would be if it were taken now instead.
		It("keeps tracking what an alias names when a rule moves it", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("aaa: &a\n  k1: v1\n  list:\n    - t1\nddd:\n  x: *a\n"),
				filemerge.Spec{
					Overrides: overrides(`{"aaa": {"list": ["v3"]}, "ddd": {"x": {"k1": "v1"}}}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.aaa.list", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal(
				"aaa: &a\n  k1: v1\n  list:\n    - t1\n    - v3\nddd:\n  x: *a\n"))
		})

		// The same, through a merge key and with no rule at all: keys run in
		// sorted order, so `thing` is merged before the `zbase` it inherits
		// from. `j` is never named by the override and must not be frozen at
		// the value the template had when the copy was taken.
		It("keeps tracking a merge key when a later override key moves it", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("zbase: &b\n  inner:\n    k: v\n    j: w\nthing:\n  <<: *b\n"),
				filemerge.Spec{Overrides: overrides(
					`{"thing": {"inner": {"k": "v"}}, "zbase": {"inner": {"j": "changed"}}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal(
				"zbase: &b\n  inner:\n    k: v\n    j: changed\nthing:\n  <<: *b\n"))
		})

		// And where the override does say something the moved anchor does not,
		// the copy stays: this is a repository pinning a value against a
		// template that moved under it, which is the whole reason the pane
		// offers adjustments.
		It("keeps a pin the moved anchor disagrees with", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("zbase: &b\n  inner:\n    k: v\nthing:\n  <<: *b\n"),
				filemerge.Spec{Overrides: overrides(
					`{"thing": {"inner": {"k": "v"}}, "zbase": {"inner": {"k": "other"}}}`)})

			Expect(err).NotTo(HaveOccurred())

			var back struct {
				Thing map[string]any `yaml:"thing"`
			}
			Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
			Expect(back.Thing).To(HaveKeyWithValue("inner", map[string]any{"k": "v"}))
		})

		// Whether a key says anything the mapping does not already inherit is
		// settled by the finished file, not by the moment the key was written.
		// The override's own later keys still have to run - and they are run in
		// sorted order, so an anchor named `x-logging` is merged after the
		// `services` that uses it, which is the compose convention exactly -
		// and every list rule runs after all of them. Judged at the write, this
		// dropped a repository's pin because the same sync moved the default it
		// was pinning against, and it re-derived the same wrong file every
		// sweep.
		It("keeps an override the same run's other change would undo", func() {
			merged, err := applyFileMerge("compose.yaml",
				[]byte("x-logging: &logging\n  driver: json-file\n"+
					"services:\n  web:\n    logging:\n      <<: *logging\n"),
				filemerge.Spec{Overrides: overrides(
					`{"services": {"web": {"logging": {"driver": "json-file"}}},` +
						` "x-logging": {"driver": "local"}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal("x-logging: &logging\n  driver: local\n" +
				"services:\n  web:\n    logging:\n      <<: *logging\n      driver: json-file\n"))
		})

		// The same, where a list rule is what moves the ground afterwards.
		// Rules always run after the whole merge, so this one needs no ordering
		// trick at all.
		It("keeps an override a list rule on the anchor would undo", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("base: &b\n  list:\n    - t\nthing:\n  <<: *b\n"),
				filemerge.Spec{
					Overrides: overrides(`{"base": {"list": ["t"]}, "thing": {"list": ["t"]}}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.base.list", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal(
				"base: &b\n  list:\n    - t\n    - t\nthing:\n  <<: *b\n  list:\n    - t\n"))
		})

		// The other half of the same question: where a rule's result really is
		// what the file already holds, down to the anchors and the comments,
		// writing it would stand in for the inherited list and the flattening
		// would be the whole of the pull request.
		//
		// This pair carries the anchor on the list itself, which setKey takes
		// from the position rather than from the value - so the list a rule
		// builds always arrives at the comparison without one, and comparing it
		// read as a change on every sweep.
		It("leaves an inherited list alone where the template anchors it", func() {
			template := "base: &base\n  list: &l\n    - t\na: *base\n"

			merged, err := applyFileMerge("ci.yaml", []byte(template),
				filemerge.Spec{
					Overrides:   overrides(`{"a": {"list": ["t"]}}`),
					Arrays:      []filemerge.ArrayRule{{Path: "$.a.list", Strategy: filemerge.ArrayAppend}},
					Deduplicate: true,
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal(template))
		})

		It("leaves an inherited list alone where a rule changes none of it", func() {
			template := "base: &base\n  list:\n    - t\na: *base\n"

			merged, err := applyFileMerge("ci.yaml", []byte(template),
				filemerge.Spec{
					Overrides:   overrides(`{"a": {"list": ["t"]}}`),
					Arrays:      []filemerge.ArrayRule{{Path: "$.a.list", Strategy: filemerge.ArrayAppend}},
					Deduplicate: true,
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(Equal(template))
		})

		// Merging into what a mapping inherits means writing the inherited keys
		// out literally, and that is a change to the file whatever the merge
		// then does to it. Where the merge asks for what the mapping already
		// gets, the flattening is the only thing in the diff - a pull request
		// proposing a change nobody asked for.
		DescribeTable("changes nothing where an inherited mapping already says it",
			func(override string, own ...string) {
				// The whole file, byte for byte. A substring would match the
				// template's own copy of the inherited keys and pass either way.
				template := "base: &b\n  nested:\n    a: 1\n    b: 2\nthing:\n  <<: *b\n"
				if len(own) > 0 {
					template = own[0]
				}

				merged, err := applyFileMerge("ci.yaml", []byte(template),
					filemerge.Spec{Overrides: overrides(override)})

				Expect(err).NotTo(HaveOccurred())
				Expect(string(merged)).To(Equal(template))
			},
			Entry("an empty patch", `{"thing": {"nested": {}}}`),
			// The inherited subtree carries an anchor and an alias to it, which
			// standInCopy renames as it copies. Counting that as a change made
			// the guard answer "changed" for every template that has one, so an
			// empty patch flattened a whole subtree and minted names for it.
			Entry("an empty patch where the inheritance carries an anchor",
				`{"thing": {"nested": {}}}`, "base: &b\n  nested:\n    inner: &i thing\n"+
					"    ref: *i\nthing:\n  <<: *b\n"),
			// Two nodes under one name, which YAML allows. Recording the
			// renaming by the original name keeps only the last of them, so
			// every alias bound to the earlier one still read as a change.
			Entry("an empty patch where the inheritance defines a name twice",
				`{"thing": {"nested": {}}}`,
				"base: &b\n  nested:\n    p: &x one\n    q: *x\n    r: &x two\n"+
					"    s: *x\nthing:\n  <<: *b\n"),
			// Not everything a merge key gives is a mapping. A list or a scalar
			// took the other path, where the key is written out literally
			// whatever it says - so `a:\n  <<: *base` grew a literal `list:`
			// for an override restating the template, while `a: *base` was left
			// alone. One rule, two answers, decided by how the inheritance was
			// spelled.
			Entry("a list the merge key already gives",
				`{"thing": {"list": ["t"]}}`,
				"base: &b\n  list:\n    - t\nthing:\n  <<: *b\n"),
			Entry("a scalar the merge key already gives",
				`{"thing": {"a": 1}}`, "base: &b\n  a: 1\nthing:\n  <<: *b\n"),
			Entry("every value it already sets", `{"thing": {"nested": {"a": 1, "b": 2}}}`),
			Entry("a null on a key it does not have", `{"thing": {"nested": {"c": null}}}`),
		)

		// Whether the copy says anything new is read off the nodes, not off
		// what they decode to. go-yaml decodes as YAML 1.2 and the readers of
		// these files are not: compose reads a bare `no` as false, which is why
		// the merge quotes what it quotes. Comparing decoded values called
		// these two the same and dropped the override, and the panel reported
		// the sync applied.
		DescribeTable("writes a respelling the inheritance does not already say",
			func(template, override, expected string) {
				merged, err := applyFileMerge("compose.yaml", []byte(template),
					filemerge.Spec{Overrides: overrides(override)})

				Expect(err).NotTo(HaveOccurred())
				Expect(string(merged)).To(ContainSubstring(expected))
			},
			// Nested a level down, and through an alias, because those are the
			// shapes that reach the comparison at all: a scalar directly under
			// a merge key is written by nodeFor without ever asking. Written
			// flat, this whole table passed against the engine it was added
			// to fix.
			Entry("a string YAML 1.1 reads as a boolean",
				"defaults: &d\n  nested:\n    restart: no\nservice:\n  <<: *d\n",
				`{"service": {"nested": {"restart": "no"}}}`, `restart: "no"`),
			Entry("the same, where an alias is what puts it there",
				"defaults: &d\n  restart: no\nservice: *d\n",
				`{"service": {"restart": "no"}}`, `restart: "no"`),
			Entry("a string YAML 1.1 reads as a sexagesimal number",
				"defaults: &d\n  nested:\n    at: 12:30\njob:\n  <<: *d\n",
				`{"job": {"nested": {"at": "12:30"}}}`, `at: "12:30"`),
			// The same override on a literal mapping was always honoured. One
			// spec answering two ways depending on whether the key is inherited
			// is the shape this whole comparison exists to keep out.
			Entry("the same override where the key is spelled out",
				"service:\n  restart: no\n",
				`{"service": {"restart": "no"}}`, `restart: "no"`),
			Entry("a number spelled differently, as the JSON half also writes it",
				"defaults: &d\n  nested:\n    a: 1\nthing:\n  <<: *d\n",
				`{"thing": {"nested": {"a": 1.0}}}`, "a: 1.0"),
		)

		// Reading a value is what the comparison used to do, and a template
		// whose anchor names itself has no value to read - so a merge that
		// never needed one failed, blaming the parse for a file that parsed.
		It("merges into a template whose anchor names itself", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("base: &base\n  self: *base\n  keep: 1\na: *base\nother: 0\n"),
				filemerge.Spec{Overrides: overrides(`{"a": {"added": 2}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("added: 2"))
		})

		// One patch reaching two levels down, each level inherited, so the
		// merge makes a copy inside a copy. A fresh anchor name is minted
		// against what the document already defines, so the outer copy has to
		// be in the document before the inner one is made - otherwise both mint
		// `dup-2` and the file defines that twice.
		//
		// `dup` is defined twice in the template on purpose. That is legal YAML
		// and it is what makes both copies carry a name that has to be renamed.
		It("gives two copies made in one merge two anchor names", func() {
			merged, err := applyFileMerge("ci.yaml", []byte(
				"leaf: &l\n  d: &dup 2\nmid: &m\n  b: *l\n  e: &dup 1\n"+
					"base: &bs\n  a: *m\nthing:\n  <<: *bs\nkeep: *dup\n"),
				filemerge.Spec{Overrides: overrides(`{"thing": {"a": {"b": {"c": 1}}}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Count(string(merged), "&dup-2")).To(Equal(1))

			var back struct {
				Thing map[string]any `yaml:"thing"`
			}
			Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
			Expect(back.Thing).To(HaveKeyWithValue("a", map[string]any{
				"b": map[string]any{"c": 1, "d": 2},
				"e": 1,
			}))
		})

		// And where the patch does say something new, the whole mapping is
		// written out - the inheritance cannot express one key differing.
		It("writes an inherited mapping out where the patch changes it", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("base: &b\n  nested:\n    a: 1\n    b: 2\nthing:\n  <<: *b\n"),
				filemerge.Spec{Overrides: overrides(`{"thing": {"nested": {"a": 9}}}`)})

			Expect(err).NotTo(HaveOccurred())

			var back struct {
				Base  map[string]any `yaml:"base"`
				Thing map[string]any `yaml:"thing"`
			}
			Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
			Expect(back.Thing).To(HaveKeyWithValue("nested", map[string]any{"a": 9, "b": 2}))
			Expect(back.Base).To(HaveKeyWithValue("nested", map[string]any{"a": 1, "b": 2}))
		})

		// A mapping and nothing else. Anything a patch cannot merge into is
		// replaced by it, empty or not - which is what RFC 7396 says and what
		// the JSON side of this engine does with the same patch, so reading
		// "an empty patch changes nothing" as a rule about the patch rather
		// than about what it lands on would split the two apart.
		DescribeTable("replaces what an empty patch cannot merge into",
			func(template string) {
				merged, err := applyFileMerge("ci.yaml", []byte(template),
					filemerge.Spec{Overrides: overrides(`{"jobs": {}}`)})

				Expect(err).NotTo(HaveOccurred())
				Expect(string(merged)).To(ContainSubstring("jobs: {}"))
			},
			Entry("a scalar", "jobs: 5\n"),
			Entry("a sequence", "jobs:\n  - build\n"),
			Entry("a key that is not there at all", "name: build\n"),
		)

		// The same rule where a merge key is what put the value there. This is
		// the pair to the spec above it: the guard that leaves an inherited
		// mapping alone has to read what it is standing on, not merely that
		// something is there, or an inherited scalar survives a patch that
		// replaces the same scalar written literally.
		It("replaces an inherited scalar with an empty patch", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("base: &b\n  jobs: 5\nthing:\n  <<: *b\n"),
				filemerge.Spec{Overrides: overrides(`{"thing": {"jobs": {}}}`)})

			Expect(err).NotTo(HaveOccurred())

			var back struct {
				Base  map[string]any `yaml:"base"`
				Thing map[string]any `yaml:"thing"`
			}
			Expect(yaml.Unmarshal(merged, &back)).To(Succeed())
			Expect(back.Thing).To(HaveKeyWithValue("jobs", map[string]any{}))
			Expect(back.Base).To(HaveKeyWithValue("jobs", 5))
		})

		// The tag comes off so the line is written plainly, and a merge key
		// written plainly is still a merge key on the way back in - which is
		// the half that matters and the half a substring assertion cannot see.
		It("leaves a merge key merging", func() {
			out, err := applyFileMerge("ci.yaml",
				[]byte("defaults: &d\n  run: make\njobs:\n  <<: *d\n  extra: 1\nname: build\n"),
				filemerge.Spec{Overrides: overrides(`{"name": "release"}`)})
			Expect(err).NotTo(HaveOccurred())

			var read struct {
				Jobs map[string]any `yaml:"jobs"`
			}
			Expect(yaml.Unmarshal(out, &read)).To(Succeed())
			Expect(read.Jobs).To(HaveKeyWithValue("run", "make"))
			Expect(read.Jobs).To(HaveKeyWithValue("extra", 1))
		})

		It("keeps the order the file was written in", func() {
			out := merged()

			Expect(strings.Index(out, "name:")).To(BeNumerically("<", strings.Index(out, "on:")))
			Expect(strings.Index(out, "on:")).To(BeNumerically("<", strings.Index(out, "jobs:")))
		})

		It("still makes the change it was asked for", func() {
			Expect(merged()).To(ContainSubstring("name: release"))
			Expect(merged()).NotTo(ContainSubstring("name: build"))
		})
	})

	Describe("a YAML template the merge does touch", func() {
		It("writes a string that reads as a number as a string", func() {
			merged, err := applyFileMerge("ci.yaml", []byte("go-version: 1.19\n"),
				filemerge.Spec{Overrides: overrides(`{"go-version": "1.20"}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(MatchRegexp(`go-version: ["']1\.20["']`))
		})

		// go-yaml decides what to quote against YAML 1.2, and the readers of
		// these files have not all moved. A repository setting `restart: "no"`
		// in a compose file got `restart: no`, and compose reads YAML 1.1,
		// where that is false - a string turned into a boolean on the way into
		// somebody's repository.
		DescribeTable("writes a word an older reader would take for a boolean as a string",
			func(value string) {
				merged, err := applyFileMerge("compose.yaml", []byte("restart: keep\n"),
					filemerge.Spec{Overrides: overrides(
						fmt.Sprintf(`{"restart": %q}`, value))})

				Expect(err).NotTo(HaveOccurred())
				Expect(string(merged)).To(Equal(fmt.Sprintf("restart: %q\n", value)))
			},

			Entry("no", "no"), Entry("yes", "yes"),
			Entry("on", "on"), Entry("off", "off"),
			Entry("a capitalised one", "Off"),
			Entry("the one-letter forms", "n"),

			// YAML 1.1 reads this as the number 750.
			Entry("a time", "12:30"),
		)

		// The same rule on the other half of the line, which nothing applied.
		// `on:` is the commonest key in a workflow file, and written bare a
		// YAML 1.1 reader takes it for the boolean true - so actionlint, and
		// GitHub's own parser on the older spelling, read a workflow with no
		// triggers at all.
		DescribeTable("writes a key an older reader would take for something else as a string",
			func(key string) {
				merged, err := applyFileMerge("ci.yaml", []byte("name: build\n"),
					filemerge.Spec{Overrides: overrides(
						fmt.Sprintf(`{%q: "x"}`, key))})

				Expect(err).NotTo(HaveOccurred())
				Expect(string(merged)).To(ContainSubstring(fmt.Sprintf("%q:", key)))
			},

			Entry("on", "on"), Entry("off", "off"),
			Entry("no", "no"), Entry("yes", "yes"),
			Entry("a time", "12:30"),
		)

		// The same key inside an object the override writes fresh, which is
		// built by a different function and was bare there too.
		It("writes a nested key an older reader would misread as a string", func() {
			merged, err := applyFileMerge("ci.yaml", []byte("name: build\n"),
				filemerge.Spec{Overrides: overrides(`{"jobs": {"on": "x"}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring(`"on":`))
		})

		It("keeps a large identifier's digits", func() {
			merged, err := applyFileMerge("ci.yaml", []byte("app: 1\n"),
				filemerge.Spec{Overrides: overrides(`{"app": 9007199254740995}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("app: 9007199254740995"))
		})

		It("leaves the rest of a mapping alone when one key below it changes", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("jobs:\n  build:\n    go-version: 1.20\n    timeout: 5\n"),
				filemerge.Spec{Overrides: overrides(
					`{"jobs": {"build": {"timeout": 30}}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("go-version: 1.20"))
			Expect(string(merged)).To(ContainSubstring("timeout: 30"))
		})

		It("changes only the requested scalar bytes across mixed endings and no final newline", func() {
			template := []byte("jobs:\r\n  build:\n    go-version: 1.20\r\n    timeout: 5")
			merged, err := applyFileMerge("ci.yaml", template, filemerge.Spec{
				Overrides: overrides(`{"jobs": {"build": {"timeout": 30}}}`),
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(merged).To(Equal([]byte(
				"jobs:\r\n  build:\n    go-version: 1.20\r\n    timeout: 30",
			)))
		})

		It("preserves comments and block scalar spelling around deletion and replacement", func() {
			template := []byte("# top\nname : build # inline\nremove: yes\nscript: |-\n  echo hi  \n")
			merged, err := applyFileMerge("ci.yaml", template, filemerge.Spec{
				Overrides: overrides(`{"name": "release", "remove": null}`),
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(merged).To(Equal([]byte(
				"# top\nname : release # inline\nscript: |-\n  echo hi  \n",
			)))
		})

		It("keeps a list item the template wrote as the template wrote it", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("versions:\n  # supported\n  - 1.20\n"),
				filemerge.Spec{
					Overrides: overrides(`{"versions": ["1.21"]}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.versions", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(merged).To(Equal([]byte(
				"versions:\n  # supported\n  - 1.20\n  - \"1.21\"\n",
			)))
		})

		// Two spellings of one value, which is what the deduplication is for.
		It("removes a repeat written two ways", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("ports:\n  - 8080\n"),
				filemerge.Spec{
					Overrides:   overrides(`{"ports": [8080, 9090]}`),
					Deduplicate: true,
					Arrays: []filemerge.ArrayRule{
						{Path: "$.ports", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Count(string(merged), "8080")).To(Equal(1))
			Expect(string(merged)).To(ContainSubstring("9090"))
		})

		// A mapping with a key that is not a string decodes to a Go map nothing
		// else here handles, and comparing two of them took the process down.
		It("deduplicates beside a mapping whose keys are not strings", func() {
			merged, err := applyFileMerge("ci.yaml",
				[]byte("matrix:\n  - 1: a\n"),
				filemerge.Spec{
					Overrides:   overrides(`{"matrix": [{"go": "1.21"}]}`),
					Deduplicate: true,
					Arrays: []filemerge.ArrayRule{
						{Path: "$.matrix", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("1: a"))
			Expect(string(merged)).To(ContainSubstring("go:"))
		})

		It("removes a key an override sets to null", func() {
			merged, err := applyFileMerge("ci.yaml", []byte("keep: 1\ndrop: 2\n"),
				filemerge.Spec{Overrides: overrides(`{"drop": null}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("keep: 1"))
			Expect(string(merged)).NotTo(ContainSubstring("drop"))
		})
	})

	DescribeTable("refuses a file it cannot read",
		func(path, template string) {
			_, err := applyFileMerge(path, []byte(template),
				filemerge.Spec{Overrides: overrides(`{"a": 1}`)})

			Expect(err).To(MatchError(filemerge.ErrUnreadable))
		},
		Entry("JSON that does not parse", "f.json", `{"a":`),
		Entry("YAML that does not parse", "f.yaml", "a: [1\n"),
		Entry("JSON whose top level is a list", "f.json", `[1, 2]`),
		Entry("YAML whose top level is a scalar", "f.yaml", "hello\n"),
		Entry("an empty file", "f.json", ""),

		// A decoder reads one value and stops, so a file holding two would be
		// merged as its first and written back without the rest - the same
		// silence a second YAML document was dropped in before this.
		Entry("JSON holding two documents", "f.json", `{"a":1}{"b":2}`),
		Entry("YAML holding two documents", "f.yaml", "a: 1\n---\nb: 2\n"),

		// A reader takes the last of two keys with one name and this edits the
		// first, so an override applied to such a file would be written down
		// and then overruled by the line under it - the repository's own
		// adjustment ignored, with the file looking like it had been applied.
		Entry("YAML naming a key twice", "f.yaml", "a: 1\na: 2\n"),
		Entry("YAML naming a nested key twice", "f.yaml", "outer:\n  a: 1\n  a: 2\n"),
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
