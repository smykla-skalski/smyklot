package webhook_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

var _ = Describe("ParseSource [Unit]", func() {
	body := func(repository, installation string) []byte {
		return fmt.Appendf(nil,
			`{"action":"created","repository":%s,"installation":%s}`,
			repository, installation,
		)
	}

	full := fmt.Sprintf(
		`{"id":%d,"name":%q,"full_name":"%s/%s","owner":{"login":%q}}`,
		testRepoID, testRepo, testOwner, testRepo, testOwner,
	)
	installed := fmt.Sprintf(`{"id":%d}`, testInstallation)

	It("should read the identity every delivery carries", func() {
		source, err := webhook.ParseSource(body(full, installed))

		Expect(err).NotTo(HaveOccurred())
		Expect(source.InstallationID).To(BeEquivalentTo(testInstallation))
		Expect(source.Action).To(Equal("created"))
		Expect(source.Repository.ID).To(BeEquivalentTo(testRepoID))
		Expect(source.Repository.Owner).To(Equal(testOwner))
		Expect(source.Repository.Name).To(Equal(testRepo))
		Expect(source.Repository.FullName).To(Equal(testOwner + "/" + testRepo))
	})

	It("should build the full name from the owner when GitHub omits it", func() {
		partial := fmt.Sprintf(
			`{"id":%d,"name":%q,"owner":{"login":%q}}`, testRepoID, testRepo, testOwner,
		)

		source, err := webhook.ParseSource(body(partial, installed))

		Expect(err).NotTo(HaveOccurred())
		Expect(source.Repository.FullName).To(Equal(testOwner + "/" + testRepo))
	})

	It("should name the byte a malformed body went wrong at", func() {
		_, err := webhook.ParseSource([]byte(`{"action":`))

		Expect(err).To(MatchError(webhook.ErrMalformedPayload))
		Expect(err.Error()).To(ContainSubstring("unexpected end of JSON input"))
	})

	It("should refuse a delivery carrying no installation", func() {
		_, err := webhook.ParseSource(body(full, `{}`))

		Expect(err).To(MatchError(webhook.ErrNoInstallation))
	})

	DescribeTable("should refuse a repository it cannot name",
		func(repository string) {
			_, err := webhook.ParseSource(body(repository, installed))

			Expect(err).To(MatchError(webhook.ErrNoRepository))
		},
		Entry("no id", fmt.Sprintf(`{"name":%q,"owner":{"login":%q}}`, testRepo, testOwner)),
		Entry("no name", fmt.Sprintf(`{"id":%d,"owner":{"login":%q}}`, testRepoID, testOwner)),
		Entry("no owner", fmt.Sprintf(`{"id":%d,"name":%q}`, testRepoID, testRepo)),
	)
})
