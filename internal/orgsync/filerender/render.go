// Package filerender owns the authoritative preview pipeline shared by sync
// planning, the panel render endpoint, and the development bridge.
package filerender

import (
	"bytes"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// Request is every typed input needed after the caller has loaded stored
// state. Layers are already named but are always resolved in their given order.
type Request struct {
	Path          string
	Draft         []byte
	DefaultBranch *string
	Merge         filemerge.Spec
	Base          *config.Config
	Layers        []config.Layer
}

// Result is the resolved policy, semantic composition, and exact final bytes.
type Result struct {
	Resolved          config.Resolved
	Composed          []byte
	Final             []byte
	MatchesFormatting bool
}

// Render applies the one backend-authoritative file pipeline.
func Render(request Request) (Result, error) {
	resolved := config.Resolve(request.Base, request.Layers...)
	policy := resolved.Values.Formatting
	if err := policy.AsPatch().Validate(); err != nil {
		return Result{Resolved: resolved}, fmt.Errorf("resolve formatting policy: %w", err)
	}

	content := string(request.Draft)
	if request.DefaultBranch != nil {
		content = orgsync.Render(content, *request.DefaultBranch)
	}

	applied, err := filemerge.ApplyTemplate(
		request.Path,
		[]byte(content),
		request.Merge,
		policy,
	)
	if err != nil {
		return Result{Resolved: resolved}, err
	}
	if len(applied.Final) > orgsync.MaxFileContentBytes {
		return Result{Resolved: resolved}, fmt.Errorf("%w: rendered file exceeds %d bytes", orgsync.ErrInvalidConfig, orgsync.MaxFileContentBytes)
	}

	return Result{
		Resolved:          resolved,
		Composed:          applied.Composed,
		Final:             applied.Final,
		MatchesFormatting: bytes.Equal(filemerge.TemplateBody(applied.Composed), filemerge.TemplateBody(applied.Final)),
	}, nil
}
