package orgsync_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

var _ = Describe("Digests [Unit]", func() {
	yes, no := true, false

	Describe("a configuration", func() {
		It("is the same fingerprint for the same configuration", func() {
			Expect(orgsync.DigestConfig(true, []byte(`{"labels":[]}`))).
				To(Equal(orgsync.DigestConfig(true, []byte(`{"labels":[]}`))))
		})

		It("changes when the document changes", func() {
			Expect(orgsync.DigestConfig(true, []byte(`{"labels":[]}`))).
				NotTo(Equal(orgsync.DigestConfig(true, []byte(`{"labels":[{}]}`))))
		})

		It("changes when the kind is switched off", func() {
			Expect(orgsync.DigestConfig(true, []byte(`{}`))).
				NotTo(Equal(orgsync.DigestConfig(false, []byte(`{}`))))
		})

		It("changes when the document gains a key", func() {
			Expect(orgsync.DigestConfig(true, []byte(`{"a":1}`))).
				NotTo(Equal(orgsync.DigestConfig(true, []byte(`{"a":1,"b":2}`))))
		})
	})

	Describe("a scope", func() {
		config := func(kind orgsync.Kind, digest string) orgsync.Config {
			return orgsync.Config{Kind: kind, Digest: digest}
		}
		override := func(repo string, kind orgsync.Kind, enabled *bool) orgsync.RepositoryOverride {
			return orgsync.RepositoryOverride{RepositoryID: repo, Kind: kind, Enabled: enabled}
		}

		configs := []orgsync.Config{
			config(orgsync.KindLabels, "aaa"),
			config(orgsync.KindSettings, "bbb"),
		}
		overrides := []orgsync.RepositoryOverride{
			override("r1", orgsync.KindLabels, &yes),
			override("r2", orgsync.KindLabels, &no),
		}

		It("is the same fingerprint for the same scope", func() {
			Expect(orgsync.DigestScope(configs, overrides)).
				To(Equal(orgsync.DigestScope(configs, overrides)))
		})

		// Neither list arrives in a guaranteed order, so a fingerprint that
		// depended on one would mark every plan stale the first time a query
		// planner changed its mind
		It("does not depend on the order either list arrives in", func() {
			Expect(orgsync.DigestScope(
				[]orgsync.Config{configs[1], configs[0]},
				[]orgsync.RepositoryOverride{overrides[1], overrides[0]},
			)).To(Equal(orgsync.DigestScope(configs, overrides)))
		})

		It("changes when a configuration changes", func() {
			Expect(orgsync.DigestScope(
				[]orgsync.Config{config(orgsync.KindLabels, "zzz"), configs[1]}, overrides,
			)).NotTo(Equal(orgsync.DigestScope(configs, overrides)))
		})

		// Turning a kind off for one repository removes its actions just as
		// surely as removing a label does, so a plan computed before that must
		// not still be approvable
		It("changes when a repository override changes", func() {
			Expect(orgsync.DigestScope(configs, []orgsync.RepositoryOverride{
				overrides[0], override("r2", orgsync.KindLabels, &yes),
			})).NotTo(Equal(orgsync.DigestScope(configs, overrides)))
		})

		It("changes when a repository stops overriding", func() {
			Expect(orgsync.DigestScope(configs, []orgsync.RepositoryOverride{
				overrides[0], override("r2", orgsync.KindLabels, nil),
			})).NotTo(Equal(orgsync.DigestScope(configs, overrides)))
		})

		// "Inherits, and the installation says no" and "this repository says
		// no" are different configurations, and they stop being the same the
		// moment the installation changes its mind
		It("tells inheriting apart from saying no", func() {
			Expect(orgsync.DigestScope(nil, []orgsync.RepositoryOverride{
				override("r1", orgsync.KindLabels, nil),
			})).NotTo(Equal(orgsync.DigestScope(nil, []orgsync.RepositoryOverride{
				override("r1", orgsync.KindLabels, &no),
			})))
		})

		// Nothing reaching this today can construct the collision below - the
		// kinds are a closed set and a digest is hexadecimal - so this guards
		// the framing rather than reporting a live fault. Without it, one
		// entry and two entries serialise to the same bytes, and a digest
		// cannot report that kind of fault about itself
		It("cannot be fooled by an entry that spans where two would end", func() {
			one := []orgsync.Config{{Kind: "k", Digest: "1config\x00k\x002"}}
			two := []orgsync.Config{{Kind: "k", Digest: "1"}, {Kind: "k", Digest: "2"}}

			Expect(orgsync.DigestScope(one, nil)).
				NotTo(Equal(orgsync.DigestScope(two, nil)))
		})

		It("changes when a repository is added to the scope", func() {
			Expect(orgsync.DigestScope(configs, append(
				append([]orgsync.RepositoryOverride{}, overrides...),
				override("r3", orgsync.KindLabels, &yes),
			))).NotTo(Equal(orgsync.DigestScope(configs, overrides)))
		})
	})
})
