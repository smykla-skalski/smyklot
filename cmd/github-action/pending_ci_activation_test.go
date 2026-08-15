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
			wantLabels: []string{
				"smyklot:pending:ci:squash", github.LabelPendingCIServiceOwner,
			},
			wantReactionCount: 1,
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
			if !equalStrings(artifacts.addedLabels, []string{
				github.LabelPendingCIServiceOwner, "smyklot:pending:ci:squash",
			}) {
				t.Fatalf("added labels = %v", artifacts.addedLabels)
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

func TestPendingCIActivationKeepsOwnershipWhenMethodRollbackFails(t *testing.T) {
	t.Parallel()
	methodLabel := "smyklot:pending:ci:squash"
	artifacts := &pendingCIArtifactsStub{
		removeLabelErrors: map[string]error{methodLabel: errors.New("GitHub unavailable")},
	}
	command := &pendingCICommand{
		store: pendingCICommandStoreStub{
			getErr: storage.ErrNotFound, armErr: errors.New("database full"),
		},
		coordinator: newPendingCICoordinator(), repositoryID: "repository:7",
		now: func() time.Time { return time.Now().UTC() },
	}

	_, err := activatePendingCI(
		t.Context(), artifacts, command, pendingCIActivationRequest{
			runtime: &RuntimeConfig{CommentAuthor: "operator"},
			owner:   "owner", repository: "repository", pullRequest: 198,
			commentID: 101, headSHA: "head", baseBranch: "main",
			method: github.MergeMethodSquash, label: methodLabel,
		},
	)
	if err == nil || !errors.Is(err, artifacts.removeLabelErrors[methodLabel]) {
		t.Fatalf("activation error = %v, want rollback failure", err)
	}
	if !equalStrings(artifacts.removedLabels, []string{methodLabel}) {
		t.Fatalf("removed labels = %v, want method label only", artifacts.removedLabels)
	}
}

func TestPendingCIActivationCleansAmbiguousMethodPublishFailure(t *testing.T) {
	t.Parallel()
	methodLabel := "smyklot:pending:ci:squash"
	publishErr := errors.New("response lost")
	artifacts := &pendingCIArtifactsStub{
		addLabelErrors: map[string]error{methodLabel: publishErr},
	}
	command := &pendingCICommand{
		store:       pendingCICommandStoreStub{getErr: storage.ErrNotFound},
		coordinator: newPendingCICoordinator(), repositoryID: "repository:7",
		now: func() time.Time { return time.Now().UTC() }, wake: func() {},
	}

	failures, err := activatePendingCI(
		t.Context(), artifacts, command, pendingCIActivationRequest{
			runtime: &RuntimeConfig{CommentAuthor: "operator"},
			owner:   "owner", repository: "repository", pullRequest: 198,
			commentID: 101, headSHA: "head", baseBranch: "main",
			method: github.MergeMethodSquash, label: methodLabel,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(failures.label, publishErr) {
		t.Fatalf("label failure = %v, want publish error", failures.label)
	}
	if !equalStrings(artifacts.removedLabels, []string{
		methodLabel, github.LabelPendingCIServiceOwner,
	}) {
		t.Fatalf("removed labels = %v", artifacts.removedLabels)
	}
}

func TestPendingCIActivationCleansAmbiguousMarkerPublishFailure(t *testing.T) {
	t.Parallel()
	publishErr := errors.New("response lost")
	artifacts := &pendingCIArtifactsStub{addLabelErrors: map[string]error{
		github.LabelPendingCIServiceOwner: publishErr,
	}}
	command := &pendingCICommand{
		store:       pendingCICommandStoreStub{getErr: storage.ErrNotFound},
		coordinator: newPendingCICoordinator(), repositoryID: "repository:7",
		now: func() time.Time { return time.Now().UTC() }, wake: func() {},
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
	if !errors.Is(failures.label, publishErr) {
		t.Fatalf("label failure = %v, want publish error", failures.label)
	}
	if !equalStrings(artifacts.removedLabels, []string{github.LabelPendingCIServiceOwner}) {
		t.Fatalf("removed labels = %v", artifacts.removedLabels)
	}
}

func TestPendingCIActivationRemovesActionOwnedConflictingLabels(t *testing.T) {
	t.Parallel()
	artifacts := &pendingCIArtifactsStub{labels: []string{
		github.LabelPendingCIMerge,
		github.LabelPendingCISquash,
		github.LegacyLabelPendingCIRebase,
		"unrelated",
	}}
	command := &pendingCICommand{
		store:       pendingCICommandStoreStub{getErr: storage.ErrNotFound},
		coordinator: newPendingCICoordinator(), repositoryID: "repository:7",
		now: func() time.Time { return time.Now().UTC() }, wake: func() {},
	}

	failures, err := activatePendingCI(
		t.Context(), artifacts, command, pendingCIActivationRequest{
			runtime: &RuntimeConfig{CommentAuthor: "operator"},
			owner:   "owner", repository: "repository", pullRequest: 198,
			commentID: 101, headSHA: "head", baseBranch: "main",
			method: github.MergeMethodSquash, label: github.LabelPendingCISquash,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if failures != (pendingCIActivationErrors{}) {
		t.Fatalf("activation failures = %+v", failures)
	}
	if !equalStrings(artifacts.removedLabels, []string{
		github.LabelPendingCIMerge,
		github.LegacyLabelPendingCIRebase,
	}) {
		t.Fatalf("removed labels = %v", artifacts.removedLabels)
	}
}

func TestPendingCIActivationKeepsCurrentLabelWhenIncomingCommandIsStale(t *testing.T) {
	t.Parallel()
	currentLabel := github.LabelPendingCIMerge
	incomingLabel := github.LabelPendingCISquash
	artifacts := &pendingCIArtifactsStub{labels: []string{currentLabel}}
	command := &pendingCICommand{
		store: pendingCICommandStoreStub{
			request: pendingci.Request{
				Label: currentLabel, SourceCommentID: 202,
			},
			armErr: pendingci.ErrStaleSourceRevision,
		},
		coordinator: newPendingCICoordinator(), repositoryID: "repository:7",
		now: func() time.Time { return time.Now().UTC() }, wake: func() {},
	}

	failures, err := activatePendingCI(
		t.Context(), artifacts, command, pendingCIActivationRequest{
			runtime: &RuntimeConfig{CommentAuthor: "operator"},
			owner:   "owner", repository: "repository", pullRequest: 198,
			commentID: 101, headSHA: "head", baseBranch: "main",
			method: github.MergeMethodSquash, label: incomingLabel,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !failures.stale || failures.command != nil {
		t.Fatalf("activation failures = %+v", failures)
	}
	if !equalStrings(artifacts.removedLabels, []string{incomingLabel}) {
		t.Fatalf("removed labels = %v, want only stale incoming label", artifacts.removedLabels)
	}
}

func TestPendingCIActivationRollbackTreatsMissingLabelsAsClean(t *testing.T) {
	t.Parallel()
	methodLabel := github.LabelPendingCISquash
	missing := &github.APIError{StatusCode: 404}
	artifacts := &pendingCIArtifactsStub{
		removeLabelErrors: map[string]error{methodLabel: missing},
	}
	command := &pendingCICommand{
		store: pendingCICommandStoreStub{
			getErr: storage.ErrNotFound, armErr: errors.New("database full"),
		},
		coordinator: newPendingCICoordinator(), repositoryID: "repository:7",
		now: func() time.Time { return time.Now().UTC() },
	}

	_, err := activatePendingCI(
		t.Context(), artifacts, command, pendingCIActivationRequest{
			runtime: &RuntimeConfig{CommentAuthor: "operator"},
			owner:   "owner", repository: "repository", pullRequest: 198,
			commentID: 101, headSHA: "head", baseBranch: "main",
			method: github.MergeMethodSquash, label: methodLabel,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(artifacts.removedLabels, []string{
		methodLabel, github.LabelPendingCIServiceOwner,
	}) {
		t.Fatalf("removed labels = %v", artifacts.removedLabels)
	}
}

func TestPendingCIActivationCancelsAmbiguousCommands(t *testing.T) {
	t.Parallel()
	current := pendingci.Request{
		ID: 7, Label: github.LabelPendingCIMerge, SourceCommentID: 202,
	}
	artifacts := &pendingCIArtifactsStub{}
	command := &pendingCICommand{
		store: pendingCICommandStoreStub{
			request: current, armErr: pendingci.ErrAmbiguousSourceRevision,
			finishResult: &current,
		},
		coordinator: newPendingCICoordinator(), repositoryID: "repository:7",
		now: func() time.Time { return time.Now().UTC() }, wake: func() {},
	}

	failures, err := activatePendingCI(
		t.Context(), artifacts, command, pendingCIActivationRequest{
			runtime: &RuntimeConfig{CommentAuthor: "operator"},
			owner:   "owner", repository: "repository", pullRequest: 198,
			commentID: 101, headSHA: "head", baseBranch: "main",
			method: github.MergeMethodSquash, label: github.LabelPendingCISquash,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !failures.ambiguous || failures.command != nil {
		t.Fatalf("activation failures = %+v", failures)
	}
	if !equalStrings(artifacts.removedLabels, []string{github.LabelPendingCISquash}) {
		t.Fatalf("removed labels = %v", artifacts.removedLabels)
	}
}

type pendingCIArtifactsStub struct {
	labels            []string
	addedLabels       []string
	removedLabels     []string
	removedReactions  []int
	addLabelErrors    map[string]error
	removeLabelErrors map[string]error
}

func (stub *pendingCIArtifactsStub) GetLabels(
	context.Context,
	string,
	string,
	int,
) ([]string, error) {
	return append([]string(nil), stub.labels...), nil
}

func (stub *pendingCIArtifactsStub) AddLabel(
	_ context.Context,
	_, _ string,
	_ int,
	label string,
) error {
	stub.addedLabels = append(stub.addedLabels, label)

	return stub.addLabelErrors[label]
}

func (stub *pendingCIArtifactsStub) RemoveLabel(
	_ context.Context,
	_, _ string,
	_ int,
	label string,
) error {
	stub.removedLabels = append(stub.removedLabels, label)

	return stub.removeLabelErrors[label]
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
