package orgsync_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
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

	It("fills in the branch a repository calls its own", func() {
		Expect(orgsync.Render("See {{DEFAULT_BRANCH}} for more.", "trunk")).
			To(Equal("See trunk for more."))
	})

	It("leaves a template alone where GitHub named no branch", func() {
		Expect(orgsync.Render("See {{DEFAULT_BRANCH}}.", "")).
			To(Equal("See {{DEFAULT_BRANCH}}."))
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
		Entry("a path written with backslashes",
			orgsync.FileConfig{Files: []orgsync.File{file(`.github\ci.yaml`, "x")}},
			"git separates paths with /"),
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
					{Path: "renovate.json"}, {Path: "renovate.json"},
				}},
				"is adjusted twice"),
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
