package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestPendingCICommandRecognizesExistingArtifactOwnership(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		request pendingci.Request
		err     error
		label   string
		comment int
		owned   pendingCIArtifactOwnership
	}{
		{
			name: "same label",
			request: pendingci.Request{
				Label: "smyklot:pending:ci:squash", SourceCommentID: 101,
			},
			label: "smyklot:pending:ci:squash", comment: 101,
			owned: pendingCIArtifactOwnership{
				label: true, reaction: true, serviceMarker: true,
			},
		},
		{
			name: "same label with different comment",
			request: pendingci.Request{
				Label: "smyklot:pending:ci:squash", SourceCommentID: 202,
			},
			label: "smyklot:pending:ci:squash", comment: 101,
			owned: pendingCIArtifactOwnership{label: true, serviceMarker: true},
		},
		{
			name: "different label with same comment",
			request: pendingci.Request{
				Label: "smyklot:pending:ci:rebase", SourceCommentID: 101,
			},
			label: "smyklot:pending:ci:squash", comment: 101,
			owned: pendingCIArtifactOwnership{reaction: true, serviceMarker: true},
		},
		{
			name: "no armed request", err: storage.ErrNotFound,
			label: "smyklot:pending:ci:squash", comment: 101,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			command := pendingCICommand{
				store:        pendingCICommandStoreStub{request: test.request, getErr: test.err},
				repositoryID: "repository:7",
			}
			owned, err := command.armedArtifactOwnership(
				t.Context(), 198, test.label, test.comment,
			)
			if err != nil {
				t.Fatal(err)
			}
			if owned != test.owned {
				t.Fatalf("owned = %+v, want %+v", owned, test.owned)
			}
		})
	}
}

func TestPendingCICommandReportsLabelOwnershipReadFailure(t *testing.T) {
	t.Parallel()
	readErr := errors.New("database unavailable")
	command := pendingCICommand{
		store:        pendingCICommandStoreStub{getErr: readErr},
		repositoryID: "repository:7",
	}
	_, err := command.armedArtifactOwnership(
		t.Context(), 198, "smyklot:pending:ci:squash", 101,
	)
	if !errors.Is(err, readErr) {
		t.Fatalf("ownership error = %v, want database error", err)
	}
}

func TestPendingCICommandSuppressesStaleCleanupSideEffects(t *testing.T) {
	t.Parallel()
	called := false
	command := pendingCICommand{
		store: pendingCICommandStoreStub{}, coordinator: newPendingCICoordinator(),
		repositoryID: "repository:7", sourceCommentID: 101,
		sourceRevision: "2026-08-15T12:00:00Z", sourceSequence: 1, sourceOrder: 1,
		now:  func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) },
		wake: func() {},
	}
	accepted, err := command.cancelAndRun(t.Context(), 198, "cleanup command", func() error {
		called = true

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("stale cleanup was accepted")
	}
	if called {
		t.Fatal("stale cleanup ran external side effects")
	}
}

func TestPendingCICommandKeepsCleanupUnderRepositoryOwnership(t *testing.T) {
	t.Parallel()
	coordinator := newPendingCICoordinator()
	command := pendingCICommand{
		store:       pendingCICommandStoreStub{finishResult: &pendingci.Request{ID: 1}},
		coordinator: coordinator, repositoryID: "repository:7",
		sourceCommentID: 101, sourceRevision: "2026-08-15T12:00:00Z",
		sourceSequence: 1, sourceOrder: 1,
		now:  func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) },
		wake: func() {},
	}
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	cleanupDone := make(chan error, 1)
	go func() {
		_, err := command.cancelAndRun(t.Context(), 198, "cleanup command", func() error {
			close(cleanupStarted)
			<-releaseCleanup

			return nil
		})
		cleanupDone <- err
	}()
	<-cleanupStarted

	replacementAttempted := make(chan struct{})
	replacementEntered := make(chan struct{})
	replacementDone := make(chan error, 1)
	go func() {
		close(replacementAttempted)
		replacementDone <- coordinator.Exclusive(
			t.Context(), "repository:7", func() error {
				close(replacementEntered)

				return nil
			},
		)
	}()
	<-replacementAttempted
	enteredEarly := false
	select {
	case <-replacementEntered:
		enteredEarly = true
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCleanup)
	if err := <-cleanupDone; err != nil {
		t.Fatal(err)
	}
	if err := <-replacementDone; err != nil {
		t.Fatal(err)
	}
	if enteredEarly {
		t.Fatal("replacement entered while cleanup still owned the repository")
	}
}

func TestPendingCIReactionCleanupFinishesCurrentRequest(t *testing.T) {
	t.Parallel()
	var change pendingci.FinishPRRequest
	command := pendingCICommand{
		store: pendingCICommandStoreStub{finish: func(
			request pendingci.FinishPRRequest,
		) (*pendingci.Request, error) {
			change = request

			return &pendingci.Request{ID: 1}, nil
		}},
		coordinator: newPendingCICoordinator(), repositoryID: "repository:7",
		now:  func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) },
		wake: func() {},
	}
	called := false
	accepted, err := command.cancelAndRun(t.Context(), 198, "cleanup reaction", func() error {
		called = true

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !accepted || !called {
		t.Fatalf("accepted = %t, cleanup called = %t", accepted, called)
	}
	if change.RepositoryID != "repository:7" || change.PullRequest != 198 ||
		change.Lifecycle != pendingci.LifecycleCancelled || change.Reason != "cleanup reaction" {
		t.Fatalf("finish request = %+v", change)
	}
}

func TestPendingCICleanupWakesDurableCleanupAfterExternalFailure(t *testing.T) {
	t.Parallel()
	woke := false
	externalErr := errors.New("GitHub unavailable")
	command := pendingCICommand{
		store:       pendingCICommandStoreStub{finishResult: &pendingci.Request{ID: 1}},
		coordinator: newPendingCICoordinator(), repositoryID: "repository:7",
		sourceCommentID: 101, sourceRevision: "2026-08-15T12:00:00Z",
		sourceSequence: 1, sourceOrder: 1,
		now:  func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) },
		wake: func() { woke = true },
	}
	_, err := command.cancelAndRun(t.Context(), 198, "cleanup command", func() error {
		return externalErr
	})
	if !errors.Is(err, externalErr) {
		t.Fatalf("cleanup error = %v, want external failure", err)
	}
	if !woke {
		t.Fatal("durable cleanup was not woken after external failure")
	}
}

type pendingCICommandStoreStub struct {
	request      pendingci.Request
	getErr       error
	checkArmErr  error
	armErr       error
	finishResult *pendingci.Request
	finishErr    error
	finish       func(pendingci.FinishPRRequest) (*pendingci.Request, error)
}

func (store pendingCICommandStoreStub) CheckArm(
	context.Context,
	pendingci.ArmRequest,
) error {
	return store.checkArmErr
}

func (store pendingCICommandStoreStub) GetArmed(
	context.Context,
	string,
	int,
) (pendingci.Request, error) {
	return store.request, store.getErr
}

func (store pendingCICommandStoreStub) Arm(
	context.Context,
	pendingci.ArmRequest,
) (pendingci.ArmResult, error) {
	return pendingci.ArmResult{}, store.armErr
}

func (pendingCICommandStoreStub) CancelBySource(
	context.Context,
	pendingci.CancelRequest,
) (*pendingci.Request, error) {
	return nil, nil
}

func (store pendingCICommandStoreStub) CancelByIntent(
	context.Context,
	pendingci.CancelIntentRequest,
) (pendingci.CancelIntentResult, error) {
	return pendingci.CancelIntentResult{
		Accepted: store.finishResult != nil,
		Request:  store.finishResult,
	}, store.finishErr
}

func (store pendingCICommandStoreStub) FinishPR(
	_ context.Context,
	request pendingci.FinishPRRequest,
) (*pendingci.Request, error) {
	if store.finish != nil {
		return store.finish(request)
	}

	return store.finishResult, store.finishErr
}
