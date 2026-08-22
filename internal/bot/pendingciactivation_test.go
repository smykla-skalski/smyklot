package bot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type pendingCIActivationGuardFunc func(context.Context) (bool, error)

func (guard pendingCIActivationGuardFunc) AllowsActivation(
	ctx context.Context,
	_ pendingci.ArtifactKind,
	_ string,
	_ bool,
) (bool, error) {
	return guard(ctx)
}

var allowPendingCIActivation pendingCIActivationGuardFunc = func(context.Context) (bool, error) {
	return true, nil
}

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
			name: "prior request owns the shared service fence",
			current: pendingci.Request{
				Label: "smyklot:pending:ci:squash", SourceCommentID: 202,
			},
		},
		{
			name: "no prior request owns artifacts", getErr: storage.ErrNotFound,
			wantLabels:        []string{"smyklot:pending:ci:squash"},
			wantReactionCount: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			artifacts := &pendingCIArtifactsStub{}
			command := &PendingCICommand{
				Store: pendingCICommandStoreStub{
					request: test.current, getErr: test.getErr, armErr: armErr,
				},
				Coordinator: NewCoordinator(), RepositoryID: "repository:7",
				Now: func() time.Time { return time.Now().UTC() },
			}
			failures, err := activatePendingCI(
				t.Context(), artifacts, command, allowPendingCIActivation,
				PendingCIActivationRequest{
					Runtime: &RuntimeConfig{CommentAuthor: "operator"},
					Owner:   "owner", Repository: "repository", PullRequest: 198,
					CommentID: 101, HeadSHA: "head", BaseBranch: "main",
					Method: github.MergeMethodSquash, Label: "smyklot:pending:ci:squash",
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(failures.Command, armErr) {
				t.Fatalf("command failure = %v, want arm error", failures.Command)
			}
			if !equalStrings(artifacts.addedLabels, []string{"smyklot:pending:ci:squash"}) {
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
	command := &PendingCICommand{
		Store: pendingCICommandStoreStub{
			getErr: storage.ErrNotFound, armErr: errors.New("database full"),
		},
		Coordinator: NewCoordinator(), RepositoryID: "repository:7",
		Now: func() time.Time { return time.Now().UTC() },
	}

	_, err := activatePendingCI(
		t.Context(), artifacts, command, allowPendingCIActivation,
		PendingCIActivationRequest{
			Runtime: &RuntimeConfig{CommentAuthor: "operator"},
			Owner:   "owner", Repository: "repository", PullRequest: 198,
			CommentID: 101, HeadSHA: "head", BaseBranch: "main",
			Method: github.MergeMethodSquash, Label: methodLabel,
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
	command := &PendingCICommand{
		Store:       pendingCICommandStoreStub{getErr: storage.ErrNotFound},
		Coordinator: NewCoordinator(), RepositoryID: "repository:7",
		Now: func() time.Time { return time.Now().UTC() }, Wake: func() {},
	}

	failures, err := activatePendingCI(
		t.Context(), artifacts, command, allowPendingCIActivation,
		PendingCIActivationRequest{
			Runtime: &RuntimeConfig{CommentAuthor: "operator"},
			Owner:   "owner", Repository: "repository", PullRequest: 198,
			CommentID: 101, HeadSHA: "head", BaseBranch: "main",
			Method: github.MergeMethodSquash, Label: methodLabel,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(failures.Label, publishErr) {
		t.Fatalf("label failure = %v, want publish error", failures.Label)
	}
	if !equalStrings(artifacts.removedLabels, []string{methodLabel}) {
		t.Fatalf("removed labels = %v", artifacts.removedLabels)
	}
}

func TestPendingCIActivationStopsWhenWaitingReactionCannotBePublished(t *testing.T) {
	t.Parallel()
	publishErr := errors.New("response lost")
	artifacts := &pendingCIArtifactsStub{addReactionErr: publishErr}
	command := &PendingCICommand{
		Store:       pendingCICommandStoreStub{getErr: storage.ErrNotFound},
		Coordinator: NewCoordinator(), RepositoryID: "repository:7",
		Now: func() time.Time { return time.Now().UTC() }, Wake: func() {},
	}

	failures, err := activatePendingCI(
		t.Context(), artifacts, command, allowPendingCIActivation,
		PendingCIActivationRequest{
			Runtime: &RuntimeConfig{CommentAuthor: "operator"},
			Owner:   "owner", Repository: "repository", PullRequest: 198,
			CommentID: 101, HeadSHA: "head", BaseBranch: "main",
			Method: github.MergeMethodSquash, Label: "smyklot:pending:ci:squash",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(failures.Reaction, publishErr) {
		t.Fatalf("reaction failure = %v, want publish error", failures.Reaction)
	}
	if len(artifacts.addedLabels) != 0 || len(artifacts.removedLabels) != 0 {
		t.Fatalf(
			"reaction failure touched labels: added=%v removed=%v",
			artifacts.addedLabels, artifacts.removedLabels,
		)
	}
	if len(artifacts.removedReactions) != 1 || artifacts.removedReactions[0] != 198 {
		t.Fatalf(
			"removed reactions = %v, want ambiguous reaction cleanup",
			artifacts.removedReactions,
		)
	}
}

func TestPendingCIActivationRemovesActionOwnedConflictingLabels(t *testing.T) {
	t.Parallel()
	artifacts := &pendingCIArtifactsStub{labels: []string{
		LabelPendingCIMerge,
		LabelPendingCISquash,
		LegacyLabelPendingCIRebase,
		"unrelated",
	}}
	command := &PendingCICommand{
		Store:       pendingCICommandStoreStub{getErr: storage.ErrNotFound},
		Coordinator: NewCoordinator(), RepositoryID: "repository:7",
		Now: func() time.Time { return time.Now().UTC() }, Wake: func() {},
	}

	failures, err := activatePendingCI(
		t.Context(), artifacts, command, allowPendingCIActivation,
		PendingCIActivationRequest{
			Runtime: &RuntimeConfig{CommentAuthor: "operator"},
			Owner:   "owner", Repository: "repository", PullRequest: 198,
			CommentID: 101, HeadSHA: "head", BaseBranch: "main",
			Method: github.MergeMethodSquash, Label: LabelPendingCISquash,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if failures != (pendingCIActivationErrors{}) {
		t.Fatalf("activation failures = %+v", failures)
	}
	if !equalStrings(artifacts.removedLabels, []string{
		LabelPendingCIMerge,
		LegacyLabelPendingCIRebase,
	}) {
		t.Fatalf("removed labels = %v", artifacts.removedLabels)
	}
}

func TestPendingCIActivationKeepsCurrentLabelWhenIncomingCommandIsStale(t *testing.T) {
	t.Parallel()
	currentLabel := LabelPendingCIMerge
	incomingLabel := LabelPendingCISquash
	artifacts := &pendingCIArtifactsStub{labels: []string{currentLabel}}
	command := &PendingCICommand{
		Store: pendingCICommandStoreStub{
			request: pendingci.Request{
				Label: currentLabel, SourceCommentID: 202,
			},
			armErr: pendingci.ErrStaleSourceRevision,
		},
		Coordinator: NewCoordinator(), RepositoryID: "repository:7",
		Now: func() time.Time { return time.Now().UTC() }, Wake: func() {},
	}

	failures, err := activatePendingCI(
		t.Context(), artifacts, command, allowPendingCIActivation,
		PendingCIActivationRequest{
			Runtime: &RuntimeConfig{CommentAuthor: "operator"},
			Owner:   "owner", Repository: "repository", PullRequest: 198,
			CommentID: 101, HeadSHA: "head", BaseBranch: "main",
			Method: github.MergeMethodSquash, Label: incomingLabel,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !failures.Stale || failures.Command != nil {
		t.Fatalf("activation failures = %+v", failures)
	}
	if !equalStrings(artifacts.removedLabels, []string{incomingLabel}) {
		t.Fatalf("removed labels = %v, want only stale incoming label", artifacts.removedLabels)
	}
}

func TestPendingCIActivationRejectsStaleSourceBeforeApproval(t *testing.T) {
	t.Parallel()
	approved := false
	artifacts := &pendingCIArtifactsStub{
		info: &github.PRInfo{},
		approve: func() error {
			approved = true

			return nil
		},
	}
	command := &PendingCICommand{
		Store: pendingCICommandStoreStub{
			getErr: storage.ErrNotFound, checkArmErr: pendingci.ErrStaleSourceRevision,
		},
		Coordinator: NewCoordinator(), RepositoryID: "repository:7",
		Now: func() time.Time { return time.Now().UTC() }, Wake: func() {},
	}

	failures, err := activatePendingCI(
		t.Context(), artifacts, command, allowPendingCIActivation,
		PendingCIActivationRequest{
			Runtime: &RuntimeConfig{CommentAuthor: "operator"},
			Owner:   "owner", Repository: "repository", PullRequest: 198,
			CommentID: 101, HeadSHA: "head", BaseBranch: "main",
			Method: github.MergeMethodSquash, Label: LabelPendingCISquash,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !failures.Stale || failures.Command != nil {
		t.Fatalf("activation failures = %+v", failures)
	}
	if approved {
		t.Fatal("stale activation approved the pull request")
	}
	if len(artifacts.addedLabels) != 0 {
		t.Fatalf("stale activation added labels: %v", artifacts.addedLabels)
	}
}

func TestPendingCIActivationRollbackTreatsMissingLabelsAsClean(t *testing.T) {
	t.Parallel()
	methodLabel := LabelPendingCISquash
	missing := &github.APIError{StatusCode: 404}
	artifacts := &pendingCIArtifactsStub{
		removeLabelErrors: map[string]error{methodLabel: missing},
	}
	command := &PendingCICommand{
		Store: pendingCICommandStoreStub{
			getErr: storage.ErrNotFound, armErr: errors.New("database full"),
		},
		Coordinator: NewCoordinator(), RepositoryID: "repository:7",
		Now: func() time.Time { return time.Now().UTC() },
	}

	_, err := activatePendingCI(
		t.Context(), artifacts, command, allowPendingCIActivation,
		PendingCIActivationRequest{
			Runtime: &RuntimeConfig{CommentAuthor: "operator"},
			Owner:   "owner", Repository: "repository", PullRequest: 198,
			CommentID: 101, HeadSHA: "head", BaseBranch: "main",
			Method: github.MergeMethodSquash, Label: methodLabel,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(artifacts.removedLabels, []string{methodLabel}) {
		t.Fatalf("removed labels = %v", artifacts.removedLabels)
	}
}

func TestPendingCIActivationCancelsAmbiguousCommands(t *testing.T) {
	t.Parallel()
	current := pendingci.Request{
		ID: 7, Label: LabelPendingCIMerge, SourceCommentID: 202,
	}
	artifacts := &pendingCIArtifactsStub{}
	command := &PendingCICommand{
		Store: pendingCICommandStoreStub{
			request: current, armErr: pendingci.ErrAmbiguousSourceRevision,
			finishResult: &current,
		},
		Coordinator: NewCoordinator(), RepositoryID: "repository:7",
		Now: func() time.Time { return time.Now().UTC() }, Wake: func() {},
	}

	failures, err := activatePendingCI(
		t.Context(), artifacts, command, allowPendingCIActivation,
		PendingCIActivationRequest{
			Runtime: &RuntimeConfig{CommentAuthor: "operator"},
			Owner:   "owner", Repository: "repository", PullRequest: 198,
			CommentID: 101, HeadSHA: "head", BaseBranch: "main",
			Method: github.MergeMethodSquash, Label: LabelPendingCISquash,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !failures.Ambiguous || failures.Command != nil {
		t.Fatalf("activation failures = %+v", failures)
	}
	if !equalStrings(artifacts.removedLabels, []string{LabelPendingCISquash}) {
		t.Fatalf("removed labels = %v", artifacts.removedLabels)
	}
}

func TestPendingCIActivationSerializesApprovalWithCleanup(t *testing.T) {
	t.Parallel()
	coordinator := NewCoordinator()
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	cleanupDone := make(chan error, 1)
	go func() {
		cleanupDone <- coordinator.Exclusive(
			t.Context(), "repository:7", func() error {
				close(cleanupStarted)
				<-releaseCleanup

				return nil
			},
		)
	}()
	<-cleanupStarted

	approvalStarted := make(chan struct{})
	artifacts := &pendingCIArtifactsStub{info: &github.PRInfo{}, approve: func() error {
		close(approvalStarted)

		return nil
	}}
	command := &PendingCICommand{
		Store:       pendingCICommandStoreStub{getErr: storage.ErrNotFound},
		Coordinator: coordinator, RepositoryID: "repository:7",
		Now: func() time.Time { return time.Now().UTC() }, Wake: func() {},
	}
	activationAttempted := make(chan struct{})
	activationDone := make(chan error, 1)
	go func() {
		close(activationAttempted)
		_, err := activatePendingCI(
			t.Context(), artifacts, command, allowPendingCIActivation,
			PendingCIActivationRequest{
				Runtime: &RuntimeConfig{CommentAuthor: "operator"},
				Owner:   "owner", Repository: "repository", PullRequest: 198,
				CommentID: 101, HeadSHA: "head", BaseBranch: "main",
				Method: github.MergeMethodSquash, Label: LabelPendingCISquash,
			},
		)
		activationDone <- err
	}()
	<-activationAttempted
	approvedEarly := false
	select {
	case <-approvalStarted:
		approvedEarly = true
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCleanup)
	if err := <-cleanupDone; err != nil {
		t.Fatal(err)
	}
	if err := <-activationDone; err != nil {
		t.Fatal(err)
	}
	if approvedEarly {
		t.Fatal("approval ran while cleanup still owned the repository")
	}
}

func TestPendingCIActivationRechecksOwnershipAfterHandoff(t *testing.T) {
	t.Parallel()
	coordinator := NewCoordinator()
	handoffStarted := make(chan struct{})
	releaseHandoff := make(chan struct{})
	handoffDone := make(chan error, 1)
	go func() {
		handoffDone <- coordinator.Exclusive(
			t.Context(), "repository:7", func() error {
				close(handoffStarted)
				<-releaseHandoff

				return nil
			},
		)
	}()
	<-handoffStarted

	guardChecked := make(chan struct{})
	guard := pendingCIActivationGuardFunc(func(context.Context) (bool, error) {
		close(guardChecked)

		return false, nil
	})
	approved := false
	artifacts := &pendingCIArtifactsStub{info: &github.PRInfo{}, approve: func() error {
		approved = true

		return nil
	}}
	command := &PendingCICommand{
		Store:       pendingCICommandStoreStub{getErr: storage.ErrNotFound},
		Coordinator: coordinator, RepositoryID: "repository:7",
		Now: func() time.Time { return time.Now().UTC() }, Wake: func() {},
	}
	type activationResult struct {
		failures pendingCIActivationErrors
		err      error
	}
	activationDone := make(chan activationResult, 1)
	go func() {
		failures, err := activatePendingCI(
			t.Context(), artifacts, command, guard, PendingCIActivationRequest{
				Runtime: &RuntimeConfig{CommentAuthor: "operator"},
				Owner:   "owner", Repository: "repository", PullRequest: 198,
				CommentID: 101, HeadSHA: "head", BaseBranch: "main",
				Method: github.MergeMethodSquash, Label: LabelPendingCISquash,
			},
		)
		activationDone <- activationResult{failures: failures, err: err}
	}()
	checkedDuringHandoff := false
	select {
	case <-guardChecked:
		checkedDuringHandoff = true
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseHandoff)
	if err := <-handoffDone; err != nil {
		t.Fatal(err)
	}
	result := <-activationDone
	if result.err != nil {
		t.Fatal(result.err)
	}
	if checkedDuringHandoff {
		t.Fatal("runner ownership was checked while handoff still owned the repository")
	}
	if !result.failures.StoodDown {
		t.Fatalf("activation failures = %+v, want stood down", result.failures)
	}
	if approved || len(artifacts.addedLabels) != 0 {
		t.Fatalf(
			"activation ran after handoff: approved=%t labels=%v",
			approved, artifacts.addedLabels,
		)
	}
}

func TestPendingCIActivationStopsWhenApprovalFails(t *testing.T) {
	t.Parallel()
	approvalErr := errors.New("approval refused")
	artifacts := &pendingCIArtifactsStub{
		info: &github.PRInfo{}, approve: func() error { return approvalErr },
	}
	command := &PendingCICommand{
		Store:       pendingCICommandStoreStub{getErr: storage.ErrNotFound},
		Coordinator: NewCoordinator(), RepositoryID: "repository:7",
		Now: func() time.Time { return time.Now().UTC() }, Wake: func() {},
	}
	failures, err := activatePendingCI(
		t.Context(), artifacts, command, allowPendingCIActivation,
		PendingCIActivationRequest{
			Runtime: &RuntimeConfig{CommentAuthor: "operator"},
			Owner:   "owner", Repository: "repository", PullRequest: 198,
			CommentID: 101, HeadSHA: "head", BaseBranch: "main",
			Method: github.MergeMethodSquash, Label: LabelPendingCISquash,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(failures.Approval, approvalErr) {
		t.Fatalf("approval failure = %v, want %v", failures.Approval, approvalErr)
	}
	if len(artifacts.addedLabels) != 0 {
		t.Fatalf("labels added after approval failure: %v", artifacts.addedLabels)
	}
}

type pendingCIArtifactsStub struct {
	labels            []string
	addedLabels       []string
	removedLabels     []string
	removedReactions  []int
	addLabelErrors    map[string]error
	removeLabelErrors map[string]error
	addReactionErr    error
	approve           func() error
	info              *github.PRInfo
	infoErr           error
}

func (stub *pendingCIArtifactsStub) ApprovePR(
	context.Context,
	string,
	string,
	int,
) error {
	if stub.approve != nil {
		return stub.approve()
	}

	return nil
}

func (stub *pendingCIArtifactsStub) GetPRInfo(
	context.Context,
	string,
	string,
	int,
) (*github.PRInfo, error) {
	if stub.info != nil || stub.infoErr != nil {
		return stub.info, stub.infoErr
	}

	return &github.PRInfo{ApprovedBy: []string{"operator"}}, nil
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

func (stub *pendingCIArtifactsStub) AddPullRequestReaction(
	context.Context,
	string,
	string,
	int,
	github.ReactionType,
) error {
	return stub.addReactionErr
}

func (stub *pendingCIArtifactsStub) RemovePullRequestReactionByUser(
	_ context.Context,
	_, _ string,
	pullRequest int,
	_ string,
	_ github.ReactionType,
) error {
	stub.removedReactions = append(stub.removedReactions, pullRequest)

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
