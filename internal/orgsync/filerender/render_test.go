package filerender_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/orgsync/filerender"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("Authoritative file rendering [Unit]", func() {
	It("counts the required terminator against the rendered file limit", func() {
		_, err := filerender.Render(filerender.Request{
			Path: "README.md", Draft: []byte(strings.Repeat("x", orgsync.MaxFileContentBytes)), Base: config.Default(),
		})
		Expect(err).To(MatchError(ContainSubstring("rendered file exceeds")))
	})

	It("ignores only the required terminator when comparing formatting", func() {
		lineEnding := "lf"
		result, err := filerender.Render(filerender.Request{
			Path: "README.md", Draft: []byte("first\r\nsecond\r\n"), Base: config.Default(),
			Layers: []config.Layer{{Source: config.SourceTemplate, Patch: config.Patch{Formatting: &config.FormattingPatch{
				Common: &config.FormattingCommonPatch{LineEnding: &lineEnding},
			}}}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.MatchesFormatting).To(BeFalse())
		Expect(string(result.Final)).To(Equal("first\nsecond\n"))
	})

	It("substitutes repository values before checking formatting compliance", func() {
		branch := "trunk"
		lineEnding := "lf"
		result, err := filerender.Render(filerender.Request{
			Path: "README.md", Draft: []byte("Use {{DEFAULT_BRANCH}}.\r\n"),
			DefaultBranch: &branch, Base: config.Default(),
			Layers: []config.Layer{{
				Source: config.SourceTemplate,
				Patch: config.Patch{Formatting: &config.FormattingPatch{
					Common: &config.FormattingCommonPatch{LineEnding: &lineEnding},
				}},
			}},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(result.Composed)).To(Equal("Use trunk.\r\n"))
		Expect(string(result.Final)).To(Equal("Use trunk.\n"))
		Expect(result.MatchesFormatting).To(BeTrue())
		Expect(result.Resolved.Formatting.Common.LineEnding).To(Equal(config.SourceTemplate))
	})
})
