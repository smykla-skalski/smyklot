package logging

import "errors"

// Configuration errors, matched with errors.Is.
var (
	// ErrUnknownLogFormat is returned when the configured format is neither
	// text nor json
	ErrUnknownLogFormat = errors.New("unknown log format")

	// ErrUnknownLogLevel is returned when the configured level is not one slog
	// recognises
	ErrUnknownLogLevel = errors.New("unknown log level")
)
