package webhook_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type waitingInbox struct {
	*webhook.MemoryInbox
	mu     sync.Mutex
	busy   bool
	waited chan struct{}
	once   sync.Once
}

func (inbox *waitingInbox) Lease(ctx context.Context, now, until time.Time) (webhook.Lease, error) {
	inbox.mu.Lock()
	defer inbox.mu.Unlock()
	if inbox.busy {
		inbox.once.Do(func() { close(inbox.waited) })
		next := now.Add(time.Hour)
		return webhook.Lease{AvailableAt: &next}, nil
	}
	lease, err := inbox.MemoryInbox.Lease(ctx, now, until)
	inbox.busy = lease.Work != nil
	return lease, err
}

func (inbox *waitingInbox) Complete(ctx context.Context, id int64, at time.Time) error {
	inbox.mu.Lock()
	defer inbox.mu.Unlock()
	inbox.busy = false
	return inbox.MemoryInbox.Complete(ctx, id, at)
}

func TestCompletionWakesWorkBlockedByAnActiveLease(t *testing.T) {
	inbox := &waitingInbox{MemoryInbox: webhook.NewMemoryInbox(webhook.MemoryInboxOptions{}), waited: make(chan struct{})}
	for _, id := range []string{"first", "next"} {
		_, err := inbox.Claim(t.Context(), webhook.Claim{
			Key: id, DeliveryID: id, Event: webhook.EventIssueComment,
			Payload: comment("approve"), At: time.Now().UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	handled, release := make(chan string, 2), make(chan struct{})
	var released sync.Once
	releaseFirst := func() { released.Do(func() { close(release) }) }
	pipeline, err := webhook.New([]byte(testSecret), inbox, func(_ context.Context, delivery webhook.Delivery) error {
		handled <- delivery.ID
		if delivery.ID == "first" {
			<-release
		}
		return nil
	}, webhook.Options{Events: []string{webhook.EventIssueComment}, Workers: 1, QueueDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		releaseFirst()
		if err := pipeline.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	pipeline.Start(t.Context())
	select {
	case <-inbox.waited:
	case <-time.After(time.Second):
		t.Fatal("the dispatcher did not wait for the active lease")
	}
	if id := <-handled; id != "first" {
		t.Fatalf("first delivery = %q", id)
	}
	releaseFirst()
	select {
	case id := <-handled:
		if id != "next" {
			t.Fatalf("next delivery = %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("completion did not wake the next delivery")
	}
}
