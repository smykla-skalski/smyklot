package config

import "errors"

// Sentinel errors for configuration.
var (
	// ErrUnknownRunner is returned when the runner key names an entry point
	// that does not exist
	ErrUnknownRunner = errors.New("unknown runner")
)
