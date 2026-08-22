package gate

import (
	"context"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func TestPendingCISourceClaimWaitsForRepositoryOwnership(t *testing.T) {
	t.Parallel()
	coordinator := bot.NewCoordinator()
	ownerStarted := make(chan struct{})
	releaseOwner := make(chan struct{})
	ownerDone := make(chan error, 1)
	go func() {
		ownerDone <- coordinator.Exclusive(t.Context(), "repository:7", func() error {
			close(ownerStarted)
			<-releaseOwner

			return nil
		})
	}()
	<-ownerStarted

	storeCalled := make(chan struct{})
	store := sourceClaimStoreStub{
		claim: func(pendingci.SourceRevisionRequest) (pendingci.SourceRevisionResult, error) {
			close(storeCalled)

			return pendingci.SourceRevisionResult{Accepted: true, SourceOrder: 17}, nil
		},
	}
	claimDone := make(chan error, 1)
	go func() {
		_, err := ClaimSource(
			t.Context(), store, coordinator,
			pendingci.SourceRevisionRequest{RepositoryID: "repository:7"}, nil,
		)
		claimDone <- err
	}()

	select {
	case <-storeCalled:
		t.Fatal("source claim ran while activation owned the repository")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseOwner)
	if err := <-ownerDone; err != nil {
		t.Fatal(err)
	}
	if err := <-claimDone; err != nil {
		t.Fatal(err)
	}
}

func TestPendingCISourceClaimCancelsWithClaimedOrder(t *testing.T) {
	t.Parallel()
	var cancelled pendingci.CancelRequest
	store := sourceClaimStoreStub{
		claim: func(pendingci.SourceRevisionRequest) (pendingci.SourceRevisionResult, error) {
			return pendingci.SourceRevisionResult{Accepted: true, SourceOrder: 23}, nil
		},
		cancel: func(request pendingci.CancelRequest) (*pendingci.Request, error) {
			cancelled = request

			return &pendingci.Request{ID: 9}, nil
		},
	}
	result, err := ClaimSource(
		t.Context(), store, bot.NewCoordinator(),
		pendingci.SourceRevisionRequest{RepositoryID: "repository:7"},
		&pendingci.CancelRequest{RepositoryID: "repository:7", PullRequest: 198},
	)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.SourceOrder != 23 {
		t.Fatalf("cancellation source order = %d, want 23", cancelled.SourceOrder)
	}
	if result.Cancelled == nil || result.Cancelled.ID != 9 {
		t.Fatalf("cancelled request = %#v", result.Cancelled)
	}
}

func TestPendingCISourceCancellationReportsWebhookCausality(t *testing.T) {
	t.Parallel()
	event := &webhook.IssueCommentEvent{Action: webhook.ActionEdited}
	event.Issue.Number = 198
	event.Comment.ID = 23
	event.Comment.UpdatedAt = "2026-08-16T12:00:00Z"

	change := SourceCancellation(event, "repository:7")
	if change == nil || change.Trigger != pendingci.TriggerWebhook {
		t.Fatalf("source cancellation = %+v", change)
	}
}

type sourceClaimStoreStub struct {
	claim  func(pendingci.SourceRevisionRequest) (pendingci.SourceRevisionResult, error)
	cancel func(pendingci.CancelRequest) (*pendingci.Request, error)
}

func (store sourceClaimStoreStub) ClaimSourceRevision(
	_ context.Context,
	request pendingci.SourceRevisionRequest,
) (pendingci.SourceRevisionResult, error) {
	return store.claim(request)
}

func (store sourceClaimStoreStub) CancelBySource(
	_ context.Context,
	request pendingci.CancelRequest,
) (*pendingci.Request, error) {
	return store.cancel(request)
}
