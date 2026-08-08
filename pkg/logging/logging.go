// Package logging builds the process logger and carries it through a request.
//
// The Action writes to a workflow log a person reads, and the service writes to
// a collector a query reads. Both go through the same logger so shared code
// logs once and suits either, and so a delivery identifier attached at the edge
// reaches every line the delivery produces.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Format selects how a log line is rendered.
type Format string

const (
	// FormatText is one line of key=value pairs, meant to be read directly
	FormatText Format = "text"

	// FormatJSON is one JSON object per line, meant to be queried
	FormatJSON Format = "json"
)

// ParseFormat resolves a configured format name.
func ParseFormat(name string) (Format, error) {
	switch Format(strings.ToLower(strings.TrimSpace(name))) {
	case FormatText:
		return FormatText, nil

	case FormatJSON:
		return FormatJSON, nil

	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownLogFormat, name)
	}
}

// ParseLevel resolves a configured level name.
func ParseLevel(name string) (slog.Level, error) {
	var level slog.Level

	if err := level.UnmarshalText([]byte(strings.TrimSpace(name))); err != nil {
		return 0, fmt.Errorf("%w: %q", ErrUnknownLogLevel, name)
	}

	return level, nil
}

// New builds a logger.
//
// Anything the redactor knows about is replaced with a placeholder wherever it
// would appear in a line. That is a backstop, not a licence to log credentials:
// the code above never passes one deliberately, and this catches the case where
// an error message quotes what it was given.
func New(w io.Writer, format Format, level slog.Level, redactor *Redactor) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if format == FormatJSON {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	return slog.New(redacting(handler, redactor))
}

// contextKey is unexported so nothing outside this package can collide with it.
type contextKey struct{}

// Into returns a context carrying logger.
func Into(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

// With returns a context whose logger carries the extra attributes.
//
// The point of the whole package: an attribute added once at the edge, such as
// the delivery identifier, is on every line the work below produces.
func With(ctx context.Context, args ...any) context.Context {
	return Into(ctx, From(ctx).With(args...))
}

// From returns the logger a context carries, or the process default.
//
// Falling back rather than returning nil means code that logs does not have to
// know whether its caller set one up.
func From(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if logger, ok := ctx.Value(contextKey{}).(*slog.Logger); ok {
			return logger
		}
	}

	return slog.Default()
}
