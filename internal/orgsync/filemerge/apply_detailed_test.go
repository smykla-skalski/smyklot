package filemerge_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("Detailed file application [Unit]", func() {
	It("keeps semantic composition separate from final presentation", func() {
		policy := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.JSON.Arrays = "compact"
		})
		result, err := filemerge.ApplyDetailed(
			"renovate.json",
			[]byte("{\n  \"labels\": [\n    \"one\",\n    \"two\"\n  ]\n}\n"),
			filemerge.Spec{},
			policy,
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(string(result.Composed)).To(ContainSubstring("[\n"))
		Expect(string(result.Final)).To(ContainSubstring(`["one", "two"]`))
	})

	It("labels formatting failures without changing their error identity", func() {
		policy := formattingPolicy(func(policy *config.FormattingPolicy) {
			policy.JSON.Arrays = "compact"
		})
		_, err := filemerge.ApplyDetailed(
			"broken.json", []byte(`{"labels":[1,]}`), filemerge.Spec{}, policy,
		)

		var staged *filemerge.ApplyError
		Expect(errors.As(err, &staged)).To(BeTrue())
		Expect(staged.Stage).To(Equal(filemerge.ApplyStageFormat))
	})
})
