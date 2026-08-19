package filemerge_test

import (
	"encoding/json"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
)

// The panel composes the file a reader is looking at, and lets them edit it -
// so its copy of this engine has to produce the same bytes this one does. The
// two are read from one table, here and in
// internal/panel/frontend/tests/merge-parity.test.ts, because two
// implementations of one rule drift the moment nothing compares them: the
// panel's had RFC 7396 and nothing else, and drew a repository's own list
// replacing the template's where the service appended to it.
type parityCase struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Template string          `json:"template"`
	Spec     json.RawMessage `json:"spec"`
	Expected string          `json:"expected"`
	// Refused is a merge neither side will compose; Verbatim is one that
	// composes nothing, so the template is what the repository holds.
	Refused  bool `json:"refused"`
	Verbatim bool `json:"verbatim"`
	// Unsupported is a merge this engine composes and the panel declines to,
	// because the panel reads JSON and this also reads YAML and Markdown. Held
	// to the same standard as any other composed case here - the verb says
	// where the panel stops, not that anything about this side is uncertain.
	Unsupported bool `json:"unsupported"`
}

var _ = Describe("Panel parity [Unit]", func() {
	var cases []parityCase

	BeforeEach(func() {
		data, err := os.ReadFile("testdata/panel-parity.json")
		Expect(err).NotTo(HaveOccurred())

		var table struct {
			Cases []parityCase `json:"cases"`
		}
		Expect(json.Unmarshal(data, &table)).To(Succeed())
		Expect(table.Cases).NotTo(BeEmpty())
		cases = table.Cases
	})

	It("composes every case in the shared table the way the table says", func() {
		for _, one := range cases {
			var spec filemerge.Spec
			Expect(json.Unmarshal(one.Spec, &spec)).To(Succeed(), one.Name)

			got, err := filemerge.Apply(one.Path, []byte(one.Template), spec)

			switch {
			case one.Refused:
				Expect(err).To(HaveOccurred(), one.Name)

			case one.Verbatim:
				Expect(err).NotTo(HaveOccurred(), one.Name)
				Expect(string(got)).To(Equal(one.Template), one.Name)

			default:
				Expect(err).NotTo(HaveOccurred(), one.Name)
				Expect(string(got)).To(Equal(one.Expected), one.Name)
			}
		}
	})
})
