package gate

import (
	"testing"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestPendingCISourceMatchesDurableIntent(t *testing.T) {
	t.Parallel()
	request := pendingci.Request{
		SourceCommentID: 101,
		SourceRevision:  "2026-08-15T12:00:00Z",
		Requester:       "operator",
		MergeMethod:     pendingci.MergeMethodSquash,
	}
	tests := []struct {
		name      string
		body      string
		updatedAt string
		author    string
		matches   bool
	}{
		{
			name: "same command", body: "/squash after ci",
			updatedAt: request.SourceRevision, author: "operator", matches: true,
		},
		{
			name: "same semantic command with approval", body: "/approve /squash after ci",
			updatedAt: request.SourceRevision, author: "Operator", matches: true,
		},
		{
			name: "help takes precedence", body: "/squash after ci\n/help",
			updatedAt: request.SourceRevision, author: "operator",
		},
		{
			name: "unapprove revokes approval", body: "/squash after ci\n/unapprove",
			updatedAt: request.SourceRevision, author: "operator",
		},
		{
			name: "conflicting merge methods", body: "/squash /rebase after ci",
			updatedAt: request.SourceRevision, author: "operator",
		},
		{
			name: "revoked within timestamp precision", body: "do not merge",
			updatedAt: request.SourceRevision, author: "operator",
		},
		{
			name: "changed merge method", body: "/merge after ci",
			updatedAt: request.SourceRevision, author: "operator",
		},
		{
			name: "newer revision", body: "/squash after ci",
			updatedAt: "2026-08-15T12:00:01Z", author: "operator",
		},
		{
			name: "different author", body: "/squash after ci",
			updatedAt: request.SourceRevision, author: "intruder",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			comment := github.IssueCommentState{
				ID: request.SourceCommentID, Body: test.body, UpdatedAt: test.updatedAt,
			}
			comment.User.Login = test.author
			if got := sourceMatches(comment, request, config.Default()); got != test.matches {
				t.Fatalf("source match = %t, want %t", got, test.matches)
			}
		})
	}
}

func TestPendingCISourceMatchesRequiredChecksIntent(t *testing.T) {
	t.Parallel()
	request := pendingci.Request{
		SourceCommentID: 101, SourceRevision: "2026-08-15T12:00:00Z",
		Requester: "operator", MergeMethod: pendingci.MergeMethodRebase,
		RequiredChecksOnly: true,
	}
	comment := github.IssueCommentState{
		ID: request.SourceCommentID, Body: "/rebase after required ci",
		UpdatedAt: request.SourceRevision,
	}
	comment.User.Login = request.Requester
	if !sourceMatches(comment, request, config.Default()) {
		t.Fatal("required-checks source did not match its durable intent")
	}
	comment.Body = "/rebase after ci"
	if sourceMatches(comment, request, config.Default()) {
		t.Fatal("changed required-checks policy still matched")
	}
}
