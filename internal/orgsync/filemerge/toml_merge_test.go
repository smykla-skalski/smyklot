package filemerge_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

var _ = Describe("Merging TOML [Unit]", func() {
	It("replaces a scalar through its value span", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"# top\ntimezone = \"UTC\" # kept\nenabled=true\n",
		), filemerge.Spec{Overrides: overrides(`{"timezone":"Europe/Warsaw"}`)})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal(
			"# top\ntimezone = \"Europe/Warsaw\" # kept\nenabled=true\n",
		))
	})

	It("deep-merges a regular table without rewriting siblings", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"[service]\nenabled = true\nretries=2\n",
		), filemerge.Spec{Overrides: overrides(`{"service":{"retries":3}}`)})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal("[service]\nenabled = true\nretries=3\n"))
	})

	It("supports shallow and null deletion", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"keep = 1\nremove = 2 # removed with the node\n",
		), filemerge.Spec{
			Strategy:  filemerge.StrategyShallow,
			Overrides: overrides(`{"remove":null}`),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal("keep = 1\n"))
	})

	It("applies array rules while retaining multiline layout", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"labels = [\n  'base',\n]\n",
		), filemerge.Spec{
			Overrides: overrides(`{"labels":["local"]}`),
			Arrays:    []filemerge.ArrayRule{{Path: "$.labels", Strategy: filemerge.ArrayAppend}},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal("labels = [\n  'base',\n  'local',\n]\n"))
	})

	It("preserves typed dates and dotted keys during unrelated changes", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"when = 1979-05-27T07:32:00Z\ndotted.key = 'old'\nkeep = 1\n",
		), filemerge.Spec{Overrides: overrides(`{"keep":2}`)})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal(
			"when = 1979-05-27T07:32:00Z\ndotted.key = 'old'\nkeep = 2\n",
		))
	})

	It("updates an existing dotted key through its exact path", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"dotted.key = 'old'\n",
		), filemerge.Spec{Overrides: overrides(`{"dotted":{"key":"new"}}`)})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal("dotted.key = 'new'\n"))
	})

	It("adds below implicit tables with encoder-produced dotted key syntax", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"service.name = 'api'\n",
		), filemerge.Spec{Overrides: overrides(`{"service":{"port":8080}}`)})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal("service.name = 'api'\nservice.port = 8080\n"))
	})

	It("adds a relative dotted key inside the closest explicit table", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"[service]\ndatabase.host = 'db'\n",
		), filemerge.Spec{Overrides: overrides(`{"service":{"database":{"port":5432}}}`)})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal(
			"[service]\ndatabase.host = 'db'\ndatabase.port = 5432\n",
		))
	})

	It("rejects replacing a typed TOML date from JSON", func() {
		_, err := applyFileMerge("config.toml", []byte(
			"when = 1979-05-27T07:32:00Z\n",
		), filemerge.Spec{Overrides: overrides(`{"when":"tomorrow"}`)})

		Expect(errors.Is(err, filemerge.ErrUnwritable)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("$.when"))
	})

	It("rejects integers outside TOML's signed 64-bit range", func() {
		_, err := applyFileMerge("config.toml", []byte("value = 1\n"), filemerge.Spec{
			Overrides: overrides(`{"value":9223372036854775808}`),
		})

		Expect(errors.Is(err, filemerge.ErrUnwritable)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("$.value"))
	})

	It("adds new objects as tables from encoder-produced syntax", func() {
		merged, err := applyFileMerge("config.toml", []byte("enabled = true\n"), filemerge.Spec{
			Overrides: overrides(`{"service":{"port":8080,"name":"api"}}`),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal(
			"enabled = true\n\n[service]\nname = 'api'\nport = 8080\n",
		))
	})

	It("preserves a missing final newline when adding values and tables", func() {
		value, err := applyFileMerge("config.toml", []byte("enabled = true"), filemerge.Spec{
			Overrides: overrides(`{"port":8080}`),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(value)).To(Equal("enabled = true\nport = 8080"))

		table, err := applyFileMerge("config.toml", []byte("enabled = true"), filemerge.Spec{
			Overrides: overrides(`{"service":{"port":8080}}`),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(table)).To(Equal("enabled = true\n\n[service]\nport = 8080"))
	})

	It("does not treat a comma inside a trailing comment as a trailing comma", func() {
		formatted, err := filemerge.FormatDocument(
			"config.toml",
			[]byte("values = [\n  1 # comma, is comment text\n]\n"),
			formattingPolicy(func(policy *config.FormattingPolicy) {
				policy.TOML.TrailingCommas = "remove"
			}),
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(string(formatted)).To(Equal("values = [\n  1 # comma, is comment text\n]\n"))
	})

	It("retains inline-table style for replacements", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"service = { name = 'old', port = 80 }\n",
		), filemerge.Spec{
			Strategy:  filemerge.StrategyShallow,
			Overrides: overrides(`{"service":{"name":"new","port":8080}}`),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal("service = { name = 'new', port = 8080 }\n"))
	})

	It("changes one inline member without respelling untouched members", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"service = { name = 'old', port = 0x50 }\n",
		), filemerge.Spec{Overrides: overrides(`{"service":{"name":"new"}}`)})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal("service = { name = 'new', port = 0x50 }\n"))
	})

	It("adds and changes inline members without respelling untouched members", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"service = { name = 'old', port = 0x50 }\n",
		), filemerge.Spec{Overrides: overrides(
			`{"service":{"name":"new","enabled":true}}`,
		)})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal(
			"service = { name = 'new', port = 0x50, enabled = true }\n",
		))
	})

	It("deletes inline members without respelling survivors", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"service = { name = 'old', port = 0x50 }\n",
		), filemerge.Spec{Overrides: overrides(`{"service":{"name":null}}`)})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal("service = { port = 0x50 }\n"))
	})

	It("preserves outer and sibling syntax during nested inline structural changes", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"service = { database = { host = 'db', port = 0x1538 }, enabled = true }\n",
		), filemerge.Spec{Overrides: overrides(`{"service":{"database":{"tls":true}}}`)})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal(
			"service = { database = { host = 'db', port = 0x1538, tls = true }, enabled = true }\n",
		))
	})

	It("replaces array tables with encoder-produced array tables", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"[[products]]\nname = 'old'\n",
		), filemerge.Spec{
			Strategy:  filemerge.StrategyShallow,
			Overrides: overrides(`{"products":[{"name":"new"}]}`),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal("[[products]]\nname = 'new'\n"))
	})

	It("leaves unrelated array tables byte-identical", func() {
		merged, err := applyFileMerge("config.toml", []byte(
			"enabled = true\n\n[[products]]\nname = 'one'\n\n[[products]]\nname = 'two'\n",
		), filemerge.Spec{Overrides: overrides(`{"enabled":false}`)})

		Expect(err).NotTo(HaveOccurred())
		Expect(string(merged)).To(Equal(
			"enabled = false\n\n[[products]]\nname = 'one'\n\n[[products]]\nname = 'two'\n",
		))
	})
})
