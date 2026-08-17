package orgsync

import "errors"

var (
	// ErrInvalidConfig is configuration that could never be applied. It is
	// returned where somebody wrote it, so the answer arrives beside the field
	// rather than in a sweep log an hour later.
	ErrInvalidConfig = errors.New("invalid sync configuration")

	// ErrInvalidPlan is a plan or an action that does not describe work.
	ErrInvalidPlan = errors.New("invalid sync plan")

	// ErrRepositoryConflict is a repository whose own contents make a
	// configured change impossible: a path the configuration writes a file to
	// holds a directory, a symbolic link or a submodule there, or sits under a
	// file. The configuration is fine; this one repository cannot take it.
	ErrRepositoryConflict = errors.New("this repository cannot take that change")

	// ErrStalePlan is an apply of a plan whose configuration has since changed.
	// The digest the browser rendered no longer matches what is stored, so what
	// somebody reviewed is not what would run.
	ErrStalePlan = errors.New("stale sync plan")
)
