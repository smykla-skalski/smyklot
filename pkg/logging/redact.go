package logging

import (
	"context"
	"log/slog"
	"strings"
)

const (
	// placeholder is what a secret is replaced with
	placeholder = "[REDACTED]"

	// minSecretLength is the shortest value worth substituting. Below it the
	// value is likelier to be a fragment of ordinary text than a credential,
	// and replacing it would corrupt every line it appeared in
	minSecretLength = 8
)

// Redactor replaces known secrets wherever they appear in text.
//
// The service hands one to its logger and to anything else it serves, so a
// credential that leaks into an error message is caught once rather than at
// every place that message might surface.
type Redactor struct {
	secrets []string
}

// NewRedactor builds a redactor for the given secrets.
//
// A value too short to be a credential is ignored, since substituting it would
// mangle ordinary text that happened to contain it.
func NewRedactor(secrets ...[]byte) *Redactor {
	kept := make([]string, 0, len(secrets))

	for _, secret := range secrets {
		if len(secret) >= minSecretLength {
			kept = append(kept, string(secret))
		}
	}

	return &Redactor{secrets: kept}
}

// String returns s with every known secret replaced.
func (r *Redactor) String(s string) string {
	if r == nil {
		return s
	}

	for _, secret := range r.secrets {
		s = strings.ReplaceAll(s, secret, placeholder)
	}

	return s
}

// Error returns err's message with every known secret replaced, or empty when
// there is no error.
func (r *Redactor) Error(err error) string {
	if err == nil {
		return ""
	}

	return r.String(err.Error())
}

// empty reports whether this redactor has nothing to hide.
func (r *Redactor) empty() bool {
	return r == nil || len(r.secrets) == 0
}

// redactingHandler runs every log line through a Redactor.
type redactingHandler struct {
	inner    slog.Handler
	redactor *Redactor
}

// redacting wraps inner so the redactor's secrets never reach it.
//
// With nothing to hide it returns inner untouched, so the Action pays nothing
// for a guard the service needs.
func redacting(inner slog.Handler, redactor *Redactor) slog.Handler {
	if redactor.empty() {
		return inner
	}

	return &redactingHandler{inner: inner, redactor: redactor}
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, rec slog.Record) error {
	clean := slog.NewRecord(rec.Time, rec.Level, h.redactor.String(rec.Message), rec.PC)

	rec.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(h.scrubAttr(attr))

		return true
	})

	return h.inner.Handle(ctx, clean)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	scrubbed := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		scrubbed[i] = h.scrubAttr(attr)
	}

	return &redactingHandler{inner: h.inner.WithAttrs(scrubbed), redactor: h.redactor}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{inner: h.inner.WithGroup(name), redactor: h.redactor}
}

func (h *redactingHandler) scrubAttr(attr slog.Attr) slog.Attr {
	attr.Value = h.scrubValue(attr.Value)

	return attr
}

// scrubValue rewrites the parts of a value that can carry text.
//
// Numbers and times cannot hold a credential, so they are passed through as
// they are rather than being formatted only to be searched.
func (h *redactingHandler) scrubValue(value slog.Value) slog.Value {
	switch value.Kind() {
	case slog.KindString:
		return slog.StringValue(h.redactor.String(value.String()))

	case slog.KindGroup:
		attrs := value.Group()

		scrubbed := make([]slog.Attr, len(attrs))
		for i, attr := range attrs {
			scrubbed[i] = h.scrubAttr(attr)
		}

		return slog.GroupValue(scrubbed...)

	case slog.KindLogValuer:
		return h.scrubValue(value.Resolve())

	case slog.KindAny:
		// An error repeats whatever the failing call was given, which is the
		// most likely way a token reaches a log line by accident
		if err, ok := value.Any().(error); ok {
			return slog.StringValue(h.redactor.Error(err))
		}

		return value

	default:
		return value
	}
}
