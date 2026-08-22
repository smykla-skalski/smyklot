package main

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func TestIssueCommentIsCurrent(t *testing.T) {
	t.Parallel()
	payload := githubtest.Command("/squash after ci")
	event, err := webhook.ParseIssueComment(payload)
	if err != nil {
		t.Fatal(err)
	}
	current := github.IssueCommentState{
		ID: event.Comment.ID, Body: event.Comment.Body, UpdatedAt: event.Comment.UpdatedAt,
	}
	current.User.Login = event.Comment.User.Login
	current.User.Type = event.Comment.User.Type

	tests := []struct {
		name    string
		state   github.IssueCommentState
		err     error
		action  string
		current bool
		wantErr bool
	}{
		{name: "matching comment", state: current, action: webhook.ActionCreated, current: true},
		{
			name: "stale body", state: func() github.IssueCommentState {
				changed := current
				changed.Body = "/merge after ci"

				return changed
			}(), action: webhook.ActionEdited,
		},
		{
			name: "confirmed deletion", err: &github.APIError{StatusCode: http.StatusNotFound},
			action: webhook.ActionDeleted, current: true,
		},
		{
			name: "stale deletion", state: current, action: webhook.ActionDeleted,
		},
		{
			name: "GitHub unavailable", err: errors.New("unavailable"),
			action: webhook.ActionEdited, wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			eventCopy := *event
			eventCopy.Action = test.action
			actual, err := issueCommentIsCurrent(
				context.Background(), issueCommentReaderStub{state: test.state, err: test.err},
				&eventCopy,
			)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, want error %t", err, test.wantErr)
			}
			if actual != test.current {
				t.Fatalf("current = %t, want %t", actual, test.current)
			}
		})
	}
}

type issueCommentReaderStub struct {
	state github.IssueCommentState
	err   error
}

func (stub issueCommentReaderStub) GetIssueComment(
	context.Context,
	string,
	string,
	int64,
) (github.IssueCommentState, error) {
	return stub.state, stub.err
}
