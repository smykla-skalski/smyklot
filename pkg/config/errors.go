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

	// ErrUnknownSetting restates go-toml's refusal so it names the keys, which
	// its own wording does not. Every format refuses an unknown setting -
	// fail-closed on purpose, since the file is where a repository narrows
	// allowed_commands and a typo must not quietly restore a default - but the
	// other two decoders already say which key, so only this one is rewritten
	ErrUnknownSetting = errors.New("unknown setting")

	// ErrMultipleDocuments is returned for a YAML file that carries settings
	// past its first document, which a single decode would silently ignore
	ErrMultipleDocuments = errors.New("settings after the first YAML document")

	// ErrTrailingContent is returned for a configuration document with
	// something after it. A decoder reads one document and says nothing about
	// the rest, so without this a second one would be silently discarded
	ErrTrailingContent = errors.New("content after the configuration document")

	// ErrInvalidValue is returned for a setting whose text cannot be read as
	// the type it takes. It names the setting, because the text came from an
	// environment variable somebody has to go and find
	ErrInvalidValue = errors.New("invalid value")
)
