package storage

import "errors"

var (
	// ErrNotFound means the requested domain record does not exist.
	ErrNotFound = errors.New("storage record not found")

	// ErrConflict means an optimistic revision no longer matches.
	ErrConflict = errors.New("storage revision conflict")

	// ErrExpired means an authentication record existed but is no longer valid.
	ErrExpired = errors.New("storage record expired")
)
