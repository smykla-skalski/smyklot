package config

import "errors"

// Sentinel errors for configuration.
var (
	// ErrUnknownRunner is returned when the runner key names an entry point
	// that does not exist
	ErrUnknownRunner = errors.New("unknown runner")

	// ErrUnknownFormat is returned for a configuration document written in a
	// format Smyklot does not read, or named with an extension it cannot place
	ErrUnknownFormat = errors.New("unknown configuration format")

	// ErrUnknownSetting is returned for a document naming a setting that does
	// not exist. It is fail-closed on purpose: the file is where a repository
	// narrows allowed_commands, so a typo must not quietly restore a default
	ErrUnknownSetting = errors.New("unknown setting")
)
