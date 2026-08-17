package filemerge_test

import (
	"encoding/json"
	"fmt"
	"strings"

	yaml "go.yaml.in/yaml/v3"

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
	//
	// Refused as a spec rather than as a merge, because neither depends on the
	// template: a rule the overrides do not reach could never work on any file,
	// which is why Validate answers it and this covers that it still does
	// through the whole entry point.
	DescribeTable("refuses a list rule that addresses nothing",
		func(rule filemerge.ArrayRule, template, override, because string) {
			_, err := filemerge.Apply("renovate.json", []byte(template), filemerge.Spec{
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

			out, err := filemerge.Apply(".github/workflows/ci.yaml", template,
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
			out, err := filemerge.Apply("ci.yaml",
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
				out, err := filemerge.Apply("ci.yaml", []byte(template),
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
				merged, err := filemerge.Apply("ci.yaml", []byte(template), spec)
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
			merged, err := filemerge.Apply("ci.yaml",
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
			_, err := filemerge.Apply("ci.yaml",
				[]byte("defaults: &d\n  runs-on: ubuntu\njobs:\n  build:\n    <<: *d\n"),
				filemerge.Spec{Overrides: overrides(`{"defaults": null}`)})

			Expect(err).To(MatchError(filemerge.ErrUnwritable))
			Expect(err.Error()).To(ContainSubstring(`"*d"`))
		})

		// An alias is what the template says is at that path. Read as the node
		// it literally is, a list rule addressing something reached through one
		// found no mapping, took the template as carrying no list, and appended
		// the repository's items to nothing - a replacement, silently, for a
		// rule that says append.
		It("appends to a list the template reached through an alias", func() {
			merged, err := filemerge.Apply("ci.yaml",
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
			merged, err := filemerge.Apply("ci.yaml",
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
			merged, err := filemerge.Apply("ci.yaml",
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
			merged, err := filemerge.Apply("ci.yaml",
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
				_, err := filemerge.Apply("ci.yaml", []byte(template),
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
		// setting it would write one whose value is not an anchor - a file a
		// good many readers refuse outright.
		It("refuses an override that writes a merge key itself", func() {
			_, err := filemerge.Apply("ci.yaml", []byte("job:\n  name: build\n"),
				filemerge.Spec{Overrides: overrides(`{"job": {"<<": {"a": 1}}}`)})

			Expect(err).To(MatchError(filemerge.ErrUnwritable))
		})

		// An alias binds to the nearest definition above it, so a copy that
		// kept its anchors would take every later `*name` with it. This one
		// changes `x.inner`, and `later` names the same anchor - it has to go
		// on meaning what the template anchored.
		It("does not redefine an anchor nested inside what it copies", func() {
			merged, err := filemerge.Apply("ci.yaml",
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

			// One definition, because two is a document whose meaning depends
			// on where a reader is standing.
			Expect(strings.Count(string(merged), "&i")).To(Equal(1))
		})

		// The tag comes off so the line is written plainly, and a merge key
		// written plainly is still a merge key on the way back in - which is
		// the half that matters and the half a substring assertion cannot see.
		It("leaves a merge key merging", func() {
			out, err := filemerge.Apply("ci.yaml",
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
			merged, err := filemerge.Apply("ci.yaml", []byte("go-version: 1.19\n"),
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
				merged, err := filemerge.Apply("compose.yaml", []byte("restart: keep\n"),
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
				merged, err := filemerge.Apply("ci.yaml", []byte("name: build\n"),
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
			merged, err := filemerge.Apply("ci.yaml", []byte("name: build\n"),
				filemerge.Spec{Overrides: overrides(`{"jobs": {"on": "x"}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring(`"on":`))
		})

		It("keeps a large identifier's digits", func() {
			merged, err := filemerge.Apply("ci.yaml", []byte("app: 1\n"),
				filemerge.Spec{Overrides: overrides(`{"app": 9007199254740995}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("app: 9007199254740995"))
		})

		It("leaves the rest of a mapping alone when one key below it changes", func() {
			merged, err := filemerge.Apply("ci.yaml",
				[]byte("jobs:\n  build:\n    go-version: 1.20\n    timeout: 5\n"),
				filemerge.Spec{Overrides: overrides(
					`{"jobs": {"build": {"timeout": 30}}}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("go-version: 1.20"))
			Expect(string(merged)).To(ContainSubstring("timeout: 30"))
		})

		It("keeps a list item the template wrote as the template wrote it", func() {
			merged, err := filemerge.Apply("ci.yaml",
				[]byte("versions:\n  # supported\n  - 1.20\n"),
				filemerge.Spec{
					Overrides: overrides(`{"versions": ["1.21"]}`),
					Arrays: []filemerge.ArrayRule{
						{Path: "$.versions", Strategy: filemerge.ArrayAppend},
					},
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("- 1.20"))
			Expect(string(merged)).To(ContainSubstring("# supported"))
			Expect(string(merged)).To(MatchRegexp(`- ["']1\.21["']`))
		})

		// Two spellings of one value, which is what the deduplication is for.
		It("removes a repeat written two ways", func() {
			merged, err := filemerge.Apply("ci.yaml",
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
			merged, err := filemerge.Apply("ci.yaml",
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
			merged, err := filemerge.Apply("ci.yaml", []byte("keep: 1\ndrop: 2\n"),
				filemerge.Spec{Overrides: overrides(`{"drop": null}`)})

			Expect(err).NotTo(HaveOccurred())
			Expect(string(merged)).To(ContainSubstring("keep: 1"))
			Expect(string(merged)).NotTo(ContainSubstring("drop"))
		})
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
