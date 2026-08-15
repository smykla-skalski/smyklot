package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestPendingCIActivationRollsBackOnlyUnownedArtifacts(t *testing.T) {
	t.Parallel()
	armErr := errors.New("database full")
	tests := []struct {
		name              string
		current           pendingci.Request
		getErr            error
		wantLabels        []string
		wantReactionCount int
	}{
		{
			name: "prior request owns both artifacts",
			current: pendingci.Request{
				Label: "smyklot:pending:ci:squash", SourceCommentID: 101,
			},
		},
		{
			name: "prior request owns only the label",
			current: pendingci.Request{
				Label: "smyklot:pending:ci:squash", SourceCommentID: 202,
			},
			wantReactionCount: 1,
		},
		{
			name: "no prior request owns artifacts", getErr: storage.ErrNotFound,
			wantLabels: []string{"smyklot:pending:ci:squash"}, wantReactionCount: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			artifacts := &pendingCIArtifactsStub{}
			command := &pendingCICommand{
				store: pendingCICommandStoreStub{
					request: test.current, getErr: test.getErr, armErr: armErr,
				},
				coordinator: newPendingCICoordinator(), repositoryID: "repository:7",
				now: func() time.Time { return time.Now().UTC() },
			}
			failures, err := activatePendingCI(
				t.Context(), artifacts, command, pendingCIActivationRequest{
					runtime: &RuntimeConfig{CommentAuthor: "operator"},
					owner:   "owner", repository: "repository", pullRequest: 198,
					commentID: 101, headSHA: "head", baseBranch: "main",
					method: github.MergeMethodSquash, label: "smyklot:pending:ci:squash",
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(failures.command, armErr) {
				t.Fatalf("command failure = %v, want arm error", failures.command)
			}
			if !equalStrings(artifacts.removedLabels, test.wantLabels) {
				t.Fatalf("removed labels = %v, want %v", artifacts.removedLabels, test.wantLabels)
			}
			if len(artifacts.removedReactions) != test.wantReactionCount {
				t.Fatalf(
					"removed reactions = %d, want %d",
					len(artifacts.removedReactions), test.wantReactionCount,
				)
			}
		})
	}
}

type pendingCIArtifactsStub struct {
	removedLabels    []string
	removedReactions []int
}

func (*pendingCIArtifactsStub) AddLabel(context.Context, string, string, int, string) error {
	return nil
}

func (stub *pendingCIArtifactsStub) RemoveLabel(
	_ context.Context,
	_, _ string,
	_ int,
	label string,
) error {
	stub.removedLabels = append(stub.removedLabels, label)

	return nil
}

func (*pendingCIArtifactsStub) AddReaction(
	context.Context,
	string,
	string,
	int,
	github.ReactionType,
) error {
	return nil
}

func (stub *pendingCIArtifactsStub) RemoveReaction(
	_ context.Context,
	_, _ string,
	commentID int,
	_ github.ReactionType,
) error {
	stub.removedReactions = append(stub.removedReactions, commentID)

	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}

	return true
}
