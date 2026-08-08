package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/pkg/logging"
)

var _ = Describe("Logging [Unit]", func() {
	var buf *bytes.Buffer

	BeforeEach(func() {
		buf = &bytes.Buffer{}
	})

	// line decodes the single JSON object the logger wrote
	line := func() map[string]any {
		GinkgoHelper()

		var decoded map[string]any
		Expect(json.Unmarshal(buf.Bytes(), &decoded)).To(Succeed())

		return decoded
	}

	Describe("ParseFormat", func() {
		DescribeTable("accepts the formats the service offers",
			func(name string, expected logging.Format) {
				format, err := logging.ParseFormat(name)
				Expect(err).ToNot(HaveOccurred())
				Expect(format).To(Equal(expected))
			},
			Entry("text", "text", logging.FormatText),
			Entry("json", "json", logging.FormatJSON),
			Entry("ignores case", "JSON", logging.FormatJSON),
			Entry("ignores surrounding space", " text ", logging.FormatText),
		)

		It("rejects anything else", func() {
			_, err := logging.ParseFormat("logfmt")
			Expect(err).To(MatchError(logging.ErrUnknownLogFormat))
		})
	})

	Describe("ParseLevel", func() {
		It("accepts a level name", func() {
			level, err := logging.ParseLevel("debug")
			Expect(err).ToNot(HaveOccurred())
			Expect(level).To(Equal(slog.LevelDebug))
		})

		It("rejects anything else", func() {
			_, err := logging.ParseLevel("chatty")
			Expect(err).To(MatchError(logging.ErrUnknownLogLevel))
		})
	})

	Describe("New", func() {
		It("writes JSON when asked for it", func() {
			logging.New(buf, logging.FormatJSON, slog.LevelInfo, nil).Info("served", "repo", "o/r")

			Expect(line()).To(HaveKeyWithValue("msg", "served"))
			Expect(line()).To(HaveKeyWithValue("repo", "o/r"))
		})

		It("writes plain text when asked for it", func() {
			logging.New(buf, logging.FormatText, slog.LevelInfo, nil).Info("served", "repo", "o/r")

			Expect(buf.String()).To(ContainSubstring(`msg=served`))
			Expect(buf.String()).To(ContainSubstring(`repo=o/r`))
		})

		It("drops lines below the configured level", func() {
			logging.New(buf, logging.FormatJSON, slog.LevelInfo, nil).Debug("noisy")

			Expect(buf.String()).To(BeEmpty())
		})
	})

	Describe("context propagation", func() {
		It("returns the process default when nothing was set", func() {
			Expect(logging.From(context.Background())).To(Equal(slog.Default()))
		})

		It("returns the logger that was put in", func() {
			logger := logging.New(buf, logging.FormatJSON, slog.LevelInfo, nil)
			ctx := logging.Into(context.Background(), logger)

			Expect(logging.From(ctx)).To(Equal(logger))
		})

		It("puts an attribute added once on every later line", func() {
			ctx := logging.Into(context.Background(), logging.New(buf, logging.FormatJSON, slog.LevelInfo, nil))
			ctx = logging.With(ctx, "delivery_id", "abc-123")

			logging.From(ctx).Info("executing")

			Expect(line()).To(HaveKeyWithValue("delivery_id", "abc-123"))
		})

		It("keeps attributes from an outer scope when an inner scope adds its own", func() {
			ctx := logging.Into(context.Background(), logging.New(buf, logging.FormatJSON, slog.LevelInfo, nil))
			ctx = logging.With(ctx, "delivery_id", "abc-123")
			ctx = logging.With(ctx, "repo", "o/r")

			logging.From(ctx).Info("executing")

			Expect(line()).To(HaveKeyWithValue("delivery_id", "abc-123"))
			Expect(line()).To(HaveKeyWithValue("repo", "o/r"))
		})
	})

	Describe("redaction", func() {
		const secret = "ghs_16C7e42F292c6912E7710c838347Ae178B4a"

		var logger *slog.Logger

		BeforeEach(func() {
			logger = logging.New(buf, logging.FormatJSON, slog.LevelInfo, logging.NewRedactor([]byte(secret)))
		})

		It("hides a secret in the message", func() {
			logger.Info("minted " + secret)

			Expect(buf.String()).ToNot(ContainSubstring(secret))
			Expect(line()).To(HaveKeyWithValue("msg", "minted [REDACTED]"))
		})

		It("hides a secret in an attribute", func() {
			logger.Info("minted", "token", secret)

			Expect(buf.String()).ToNot(ContainSubstring(secret))
			Expect(line()).To(HaveKeyWithValue("token", "[REDACTED]"))
		})

		It("hides a secret an error quotes", func() {
			logger.Info("failed", "error", errors.New("bad credentials for "+secret))

			Expect(buf.String()).ToNot(ContainSubstring(secret))
			Expect(line()).To(HaveKeyWithValue("error", "bad credentials for [REDACTED]"))
		})

		It("hides a secret attached with With", func() {
			logger.With("token", secret).Info("minted")

			Expect(buf.String()).ToNot(ContainSubstring(secret))
		})

		It("hides a secret inside a group", func() {
			logger.Info("minted", slog.Group("auth", "token", secret))

			Expect(buf.String()).ToNot(ContainSubstring(secret))
		})

		It("hides a multi-line private key", func() {
			key := "-----BEGIN RSA PRIVATE KEY-----\nMIIEow==\n-----END RSA PRIVATE KEY-----"
			keyed := logging.New(buf, logging.FormatJSON, slog.LevelInfo, logging.NewRedactor([]byte(key)))

			keyed.Info("starting", "key", key)

			Expect(buf.String()).ToNot(ContainSubstring("MIIEow=="))
		})

		It("leaves ordinary text alone", func() {
			logger.Info("approved", "repo", "smykla-skalski/smyklot")

			Expect(line()).To(HaveKeyWithValue("repo", "smykla-skalski/smyklot"))
		})

		It("ignores a value too short to be a credential", func() {
			short := logging.New(buf, logging.FormatJSON, slog.LevelInfo, logging.NewRedactor([]byte("abc")))
			short.Info("abcdef")

			Expect(line()).To(HaveKeyWithValue("msg", "abcdef"))
		})
	})

	Describe("Redactor", func() {
		const secret = "ghs_16C7e42F292c6912E7710c838347Ae178B4a"

		It("replaces a secret in arbitrary text", func() {
			redactor := logging.NewRedactor([]byte(secret))

			Expect(redactor.String("using " + secret)).To(Equal("using [REDACTED]"))
		})

		It("replaces a secret in an error message", func() {
			redactor := logging.NewRedactor([]byte(secret))

			Expect(redactor.Error(errors.New("rejected " + secret))).To(Equal("rejected [REDACTED]"))
		})

		It("returns empty for no error", func() {
			Expect(logging.NewRedactor([]byte(secret)).Error(nil)).To(BeEmpty())
		})

		// Callers hold one for the whole process, and a nil one must behave
		// like a redactor with nothing to hide rather than panic
		It("passes text through when there is nothing to hide", func() {
			var redactor *logging.Redactor

			Expect(redactor.String("plain")).To(Equal("plain"))
		})
	})
})
