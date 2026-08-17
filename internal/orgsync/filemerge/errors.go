// Package filemerge builds the copy of a shared template that one repository
// should hold.
//
// A template is the organization's; the adjustments beside it are one
// repository's. This composes the two and answers with bytes, reaching nothing
// and remembering nothing. Whether those bytes are worth writing, and how they
// get to GitHub, is somebody else's question.
//
// Everything here fails closed. The engine this replaces reported a failed
// merge as a warning and wrote the raw template over the repository's file, so
// a heading the template had renamed, a malformed override or a file extension
// nobody had thought about all destroyed the repository's copy - and said so in
// a log line nobody was reading. An error here means no bytes, which means no
// commit, which means the file is left exactly as it was.
package filemerge

import "errors"

var (
	// ErrInvalidSpec is a merge nobody should be able to configure: a path that
	// addresses nothing, a strategy for the wrong sort of file, a section
	// operation missing what it needs. Answered where somebody writes it.
	ErrInvalidSpec = errors.New("invalid merge")

	// ErrUnsupportedFormat is a file this cannot merge. Extensions decide, and
	// a file whose extension is not one of them is refused rather than treated
	// as text and replaced.
	ErrUnsupportedFormat = errors.New("this file cannot be merged")

	// ErrUnreadable is a template or an override that will not parse.
	ErrUnreadable = errors.New("cannot read the file to merge")

	// ErrUnwritable is a merged document that will not marshal back.
	ErrUnwritable = errors.New("cannot write the merged file")

	// ErrNothingAddressed is a merge whose configuration names something the
	// file does not have: a path no override sets, or a heading no section
	// carries.
	//
	// The most important error here. Every one of these was silence in the
	// engine this replaces - a mistyped path left the array replaced, a
	// mistyped heading clobbered the file - and silence is what makes a
	// configuration look like it is working for as long as nobody checks.
	ErrNothingAddressed = errors.New("the merge addresses nothing in this file")
)
