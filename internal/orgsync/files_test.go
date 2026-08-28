package orgsync_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	appconfig "github.com/smykla-skalski/smyklot/pkg/config"
)

func file(path, content string) orgsync.File {
	return orgsync.File{Path: path, Content: content}
}

var _ = Describe("File configuration [Unit]", func() {
	It("accepts files a repository could carry", func() {
		Expect(orgsync.FileConfig{
			Files: []orgsync.File{
				file("CONTRIBUTING.md", "# Contributing\n"),
				file(".github/workflows/ci.yaml", "on: push\n"),
			},
			Retired: []string{".github/workflows/sync-trigger.yml"},
			Excludes: []string{
				"LICENSE",
			},
		}.Validate()).To(Succeed())
	})

	// Doubled braces are ordinary text in the files an organization shares
	// most. Read as placeholders, they were refused - so a workflow, which is
	// the canonical shared file and the one this feature exists for, could not
	// be configured at all.
	DescribeTable("accepts braces that are somebody else's",
		func(path, content string) {
			Expect(orgsync.FileConfig{Files: []orgsync.File{file(path, content)}}.Validate()).
				To(Succeed())
		},

		Entry("a workflow expression", ".github/workflows/ci.yaml",
			"jobs:\n  build:\n    steps:\n      - run: echo ${{ github.sha }}\n"),
		Entry("a workflow secret", ".github/workflows/ci.yaml",
			"        token: ${{ secrets.GITHUB_TOKEN }}\n"),
		Entry("a Renovate commit message", "renovate.json",
			`{"commitMessage": "chore: {{depName}} to {{newVersion}}"}`),
		Entry("a chart value", "chart.yaml", "image: {{ .Values.image }}\n"),
		Entry("a Go template", "README.md", "Hello {{ .Name }}\n"),

		// Smyklot's own, spelled the way somebody else's expression is. The `$`
		// is what says whose it is.
		Entry("the branch placeholder inside an expression", ".github/workflows/ci.yaml",
			"        ref: ${{ DEFAULT_BRANCH }}\n"),
	)

	// Render substitutes the exact spelling, so a spaced one would pass and
	// then be committed to every repository with its braces still on.
	It("refuses its own placeholder written with spaces", func() {
		err := orgsync.FileConfig{
			Files: []orgsync.File{file("README.md", "See {{ DEFAULT_BRANCH }}.")},
		}.Validate()

		Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
		Expect(err.Error()).To(ContainSubstring("asks for {{DEFAULT_BRANCH}}"))
	})

	// The typo this rule is for is still caught: shaped like one of Smyklot's,
	// spelled as none of them.
	It("refuses a placeholder of its own shape that it cannot fill", func() {
		err := orgsync.FileConfig{
			Files: []orgsync.File{file("README.md", "See {{DEFAULT_BRANC}}.")},
		}.Validate()

		Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
		Expect(err.Error()).To(ContainSubstring("asks for {{DEFAULT_BRANC}}"))
	})

	It("fills in the branch a repository calls its own", func() {
		Expect(orgsync.Render("See {{DEFAULT_BRANCH}} for more.", "trunk")).
			To(Equal("See trunk for more."))
	})

	It("leaves a placeholder alone where GitHub named no branch", func() {
		Expect(orgsync.Render("See {{DEFAULT_BRANCH}}.", "")).
			To(Equal("See {{DEFAULT_BRANCH}}."))
	})

	It("leaves line endings to the effective formatting policy", func() {
		Expect(orgsync.Render("one\r\ntwo\r\n", "main")).To(Equal("one\r\ntwo\r\n"))
		Expect(orgsync.Render("one\r\ntwo\r\n", "")).To(Equal("one\r\ntwo\r\n"))
	})

	It("rejects shared formatting rules for an unsupported extension", func() {
		compact := "compact"
		err := orgsync.FileConfig{Files: []orgsync.File{{
			Path: "notes.txt", Content: "text",
			Formatting: &appconfig.FormattingPatch{
				JSON: &appconfig.FormattingJSONPatch{Arrays: &compact},
			},
		}}}.Validate()

		Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
		Expect(err.Error()).To(ContainSubstring("unsupported extension"))
	})

	// The tool this replaces validated the file list not at all: no empty path,
	// no duplicate, no rejection of "..", no size limit. Each of these was
	// something it would have discovered while writing to a repository, or
	// would never have discovered.
	DescribeTable("refuses configuration nobody should be able to save",
		func(config orgsync.FileConfig, because string) {
			err := config.Validate()

			Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
			Expect(err.Error()).To(ContainSubstring(because))
		},

		Entry("a file with no path",
			orgsync.FileConfig{Files: []orgsync.File{file("", "x")}},
			"file 1 has no path"),
		Entry("a path with whitespace around it",
			orgsync.FileConfig{Files: []orgsync.File{file(" README.md", "x")}},
			"leading or trailing whitespace"),
		Entry("a path that climbs out of the repository",
			orgsync.FileConfig{Files: []orgsync.File{file("../secrets.env", "x")}},
			"climbs out of the repository"),
		Entry("a path that climbs out halfway along",
			orgsync.FileConfig{Files: []orgsync.File{file(".github/../../x", "x")}},
			"climbs out of the repository"),
		Entry("a path that names the repository itself",
			orgsync.FileConfig{Files: []orgsync.File{file(".", "x")}},
			"names the repository rather than a file"),
		Entry("a path written with a leading dot-slash",
			orgsync.FileConfig{Files: []orgsync.File{file("./README.md", "x")}},
			`"README.md" is the same place spelled once`),
		Entry("a path anchored at the root",
			orgsync.FileConfig{Files: []orgsync.File{file("/etc/passwd", "x")}},
			"starts at the root"),
		Entry("a path inside git's own directory",
			orgsync.FileConfig{Files: []orgsync.File{file(".git/config", "x")}},
			"inside git's own directory"),
		Entry("a hook inside git's own directory",
			orgsync.FileConfig{Files: []orgsync.File{file(".git/hooks/pre-commit", "x")}},
			"inside git's own directory"),
		Entry("git's own directory, spelled to fool a folded filesystem",
			orgsync.FileConfig{Files: []orgsync.File{file(".GIT/config", "x")}},
			"inside git's own directory"),
		Entry("git's own directory itself",
			orgsync.FileConfig{Retired: []string{".git"}},
			"inside git's own directory"),
		Entry("a path written with backslashes",
			orgsync.FileConfig{Files: []orgsync.File{file(`.github\ci.yaml`, "x")}},
			"git separates paths with /"),
		// Invisible in the box somebody typed it into, and every other check
		// here waves it through: not a separator, not a dot, not whitespace.
		Entry("a path with a byte nobody can see",
			orgsync.FileConfig{Files: []orgsync.File{file("a\x00b.md", "x")}},
			"cannot be printed"),
		Entry("a path with a doubled separator",
			orgsync.FileConfig{Files: []orgsync.File{file(".github//ci.yaml", "x")}},
			"is not a plain path"),
		Entry("the same file twice",
			orgsync.FileConfig{Files: []orgsync.File{
				file("README.md", "x"), file("README.md", "y"),
			}},
			`"README.md" is configured twice`),
		Entry("two files a checkout could not tell apart",
			orgsync.FileConfig{Files: []orgsync.File{
				file("Readme.md", "x"), file("README.md", "y"),
			}},
			"differ only in case"),
		// A repository that has neither path yet passes every conflict check,
		// because those read what it holds and it holds nothing here. The two
		// entries then reach one commit, asking git for a path that is a file
		// and a directory at once.
		Entry("a file inside another file",
			orgsync.FileConfig{Files: []orgsync.File{
				file("docs", "x"), file("docs/index.md", "y"),
			}},
			`"docs/index.md" sits under "docs"`),
		Entry("a file inside another file, ordered the other way",
			orgsync.FileConfig{Files: []orgsync.File{
				file("docs/index.md", "y"), file("docs", "x"),
			}},
			`"docs/index.md" sits under "docs"`),
		Entry("a file inside another file, several levels down",
			orgsync.FileConfig{Files: []orgsync.File{
				file(".github", "x"), file(".github/workflows/ci.yaml", "y"),
			}},
			`".github/workflows/ci.yaml" sits under ".github"`),
		// Sorting alone would put these three in this order and compare only
		// the neighbours, which is the pair that is fine.
		Entry("a file inside another file, with a name between them",
			orgsync.FileConfig{Files: []orgsync.File{
				file("docs", "x"), file("docs-2.md", "y"), file("docs/index.md", "z"),
			}},
			`"docs/index.md" sits under "docs"`),
		Entry("a file inside another file, differing in case",
			orgsync.FileConfig{Files: []orgsync.File{
				file("Docs", "x"), file("docs/index.md", "y"),
			}},
			`"docs/index.md" sits under "Docs"`),
		Entry("a retired path inside a file",
			orgsync.FileConfig{
				Files:   []orgsync.File{file("docs", "x")},
				Retired: []string{"docs/old.md"},
			},
			`"docs/old.md" sits under "docs"`),
		Entry("a file inside a retired path",
			orgsync.FileConfig{
				Files:   []orgsync.File{file("docs/index.md", "x")},
				Retired: []string{"docs"},
			},
			`"docs/index.md" sits under "docs"`),
		Entry("a file with nothing in it",
			orgsync.FileConfig{Files: []orgsync.File{file("README.md", "")}},
			"has no content"),
		Entry("a template asking for something nothing fills in",
			orgsync.FileConfig{Files: []orgsync.File{file("README.md", "See {{REPO_NAME}}.")}},
			"asks for {{REPO_NAME}}"),
		Entry("a retired path that is also a file",
			orgsync.FileConfig{
				Files:   []orgsync.File{file("renovate.json", "{}")},
				Retired: []string{"renovate.json"},
			},
			"written and removed by the same change"),
		Entry("a retired path listed twice",
			orgsync.FileConfig{Retired: []string{"a.yml", "a.yml"}},
			"listed twice"),
		Entry("a retired path that climbs out",
			orgsync.FileConfig{Retired: []string{"../a.yml"}},
			"climbs out of the repository"),
		Entry("an exclusion that says nothing",
			orgsync.FileConfig{Excludes: []string{"  "}},
			"exclusion 1 is empty"),
	)

	// A plan carries what it will write, once per repository it would write it
	// to, so an installation of two hundred repositories multiplies this by two
	// hundred. Bounded together as well as one at a time, so that number is one
	// somebody could have predicted.
	It("refuses files that come to more than a megabyte together", func() {
		half := strings.Repeat("x", 600_000)

		err := orgsync.FileConfig{Files: []orgsync.File{
			file("one.md", half), file("two.md", half),
		}}.Validate()

		Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
		Expect(err.Error()).To(ContainSubstring("come to more than"))
	})

	Describe("a repository's own adjustments", func() {
		config := orgsync.FileConfig{Files: []orgsync.File{
			file("renovate.json", `{"extends": ["config:recommended"]}`),
		}}

		It("accepts one for a file the installation syncs", func() {
			Expect(orgsync.FileOverride{Merges: []orgsync.FileMerge{{
				Path: "renovate.json",
				Spec: filemerge.Spec{Overrides: []byte(`{"timezone": "Europe/Warsaw"}`)},
			}}}.Validate(config)).To(Succeed())
		})

		It("accepts one formatting overlay for an exact managed path", func() {
			compact := "compact"
			Expect(orgsync.FileOverride{Formats: []orgsync.FileFormat{{
				Path: "renovate.json",
				Formatting: appconfig.FormattingPatch{
					JSON: &appconfig.FormattingJSONPatch{Arrays: &compact},
				},
			}}}.Validate(config)).To(Succeed())
		})

		DescribeTable("refuses invalid formatting overlays",
			func(fileConfig orgsync.FileConfig, override orgsync.FileOverride, because string) {
				err := override.Validate(fileConfig)

				Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
				Expect(err.Error()).To(ContainSubstring(because))
			},
			Entry("an unmanaged path", config, orgsync.FileOverride{Formats: []orgsync.FileFormat{{
				Path: "other.json",
			}}}, "is not one of the files synchronized"),
			Entry("a duplicate path", config, orgsync.FileOverride{Formats: []orgsync.FileFormat{
				{Path: "renovate.json"}, {Path: "renovate.json"},
			}}, "formatting configured twice"),
			Entry("an unsupported extension",
				orgsync.FileConfig{Files: []orgsync.File{file("README.txt", "text")}},
				orgsync.FileOverride{Formats: []orgsync.FileFormat{{Path: "README.txt"}}},
				"unsupported extension"),
		)

		DescribeTable("refuses one that could never be applied",
			func(override orgsync.FileOverride, because string) {
				err := override.Validate(config)

				Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
				Expect(err.Error()).To(ContainSubstring(because))
			},

			// The same silence as a mistyped path: it reads as configured and
			// does nothing, and the repository quietly gets the raw template.
			Entry("one for a file nobody syncs",
				orgsync.FileOverride{Merges: []orgsync.FileMerge{{Path: "package.json"}}},
				"is not one of the files synchronized"),
			Entry("two for one file",
				orgsync.FileOverride{Merges: []orgsync.FileMerge{
					{Path: "renovate.json", Spec: filemerge.Spec{Overrides: []byte(`{"a":1}`)}},
					{Path: "renovate.json", Spec: filemerge.Spec{Overrides: []byte(`{"b":2}`)}},
				}},
				"is adjusted twice"),
			Entry("one that merges nothing",
				orgsync.FileOverride{Merges: []orgsync.FileMerge{{Path: "renovate.json"}}},
				"nothing is merged without overrides"),
			Entry("a merge the file could not take",
				orgsync.FileOverride{Merges: []orgsync.FileMerge{{
					Path: "renovate.json",
					Spec: filemerge.Spec{Strategy: filemerge.StrategyMarkdown},
				}}},
				"is not Markdown"),
			Entry("an exclusion that says nothing",
				orgsync.FileOverride{Excludes: []string{""}},
				"exclusion 1 is empty"),
		)
	})
})
