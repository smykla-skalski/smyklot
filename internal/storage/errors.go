package storage

import "errors"

var (
	// ErrNotFound means the requested domain record does not exist.
	ErrNotFound = errors.New("storage record not found")

	// ErrConflict means an optimistic revision no longer matches.
	ErrConflict = errors.New("storage revision conflict")

	// ErrExpired means an authentication record existed but is no longer valid.
	ErrExpired = errors.New("storage record expired")

	// ErrRevoked means an authentication record was explicitly revoked.
	ErrRevoked = errors.New("storage record revoked")
)

// SessionRevokedError preserves the safe reason shown to the affected user.
type SessionRevokedError struct {
	Code   string
	Reason string
}

func (e SessionRevokedError) Error() string {
	return e.Reason
}

func (e SessionRevokedError) Is(target error) bool {
	return target == ErrRevoked
}
