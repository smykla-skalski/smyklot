package webhook_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func claimOf(key string) webhook.Claim {
	return webhook.Claim{
		Key: key, DeliveryID: key, Event: webhook.EventIssueComment,
		Payload: []byte(`{}`), At: time.Now().UTC(),
	}
}

func TestMemoryInboxClaimsOnce(t *testing.T) {
	t.Parallel()

	// Given an empty inbox
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})

	// When the same key is claimed twice
	first, err := inbox.Claim(t.Context(), claimOf("k"))
	if err != nil || first.Disposition != webhook.Accepted {
		t.Fatalf("first claim = %v, %v", first, err)
	}
	second, err := inbox.Claim(t.Context(), claimOf("k"))

	// Then the second is told the first is still running
	if err != nil || second.Disposition != webhook.InProgress {
		t.Fatalf("second claim = %v, %v", second, err)
	}
}

func TestMemoryInboxRetainsASettledDelivery(t *testing.T) {
	t.Parallel()

	// Given a delivery that has run to the end
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})
	claimed, _ := inbox.Claim(t.Context(), claimOf("k"))
	if err := inbox.Complete(t.Context(), claimed.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	// When GitHub redelivers it
	again, err := inbox.Claim(t.Context(), claimOf("k"))

	// Then it is retained rather than run a second time
	if err != nil || again.Disposition != webhook.Retained {
		t.Fatalf("redelivery = %v, %v", again, err)
	}
}

func TestMemoryInboxForgetsARetryableFailure(t *testing.T) {
	t.Parallel()

	// Given a delivery that failed and said it was worth retrying
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})
	claimed, _ := inbox.Claim(t.Context(), claimOf("k"))
	err := inbox.Fail(t.Context(), webhook.Failure{
		ClaimID: claimed.ID, Retryable: true, At: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// When GitHub redelivers it
	again, err := inbox.Claim(t.Context(), claimOf("k"))

	// Then it is accepted
	if err != nil || again.Disposition != webhook.Accepted {
		t.Fatalf("redelivery after a retryable failure = %v, %v", again, err)
	}
}

func TestMemoryInboxLeasesExclusively(t *testing.T) {
	t.Parallel()

	// Given one claimed delivery
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})
	if _, err := inbox.Claim(t.Context(), claimOf("k")); err != nil {
		t.Fatal(err)
	}

	// When fifty executors lease at once
	const leasers = 50
	var (
		wait   sync.WaitGroup
		mu     sync.Mutex
		leased int
	)
	now := time.Now().UTC()
	wait.Add(leasers)
	for range leasers {
		go func() {
			defer wait.Done()
			result, err := inbox.Lease(context.Background(), now, now.Add(time.Minute))
			if err != nil {
				t.Error(err)

				return
			}
			if result.Work != nil {
				mu.Lock()
				leased++
				mu.Unlock()
			}
		}()
	}
	wait.Wait()

	// Then exactly one of them holds it
	if leased != 1 {
		t.Fatalf("leased %d times, want exactly one", leased)
	}
}

func TestMemoryInboxReportsWhenWorkBecomesAvailable(t *testing.T) {
	t.Parallel()

	// Given a leased delivery scheduled to be retried in thirty seconds
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{
		Now: func() time.Time { return now },
	})
	claimed, _ := inbox.Claim(t.Context(), claimOf("k"))
	leased, err := inbox.Lease(t.Context(), now, now.Add(time.Minute))
	if err != nil || leased.Work == nil {
		t.Fatalf("lease = %v, %v", leased, err)
	}
	retryAt := now.Add(30 * time.Second)
	if err := inbox.Retry(t.Context(), webhook.Retry{ClaimID: claimed.ID, At: retryAt}); err != nil {
		t.Fatal(err)
	}

	// When it is leased before then
	next, err := inbox.Lease(t.Context(), now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	// Then nothing is ready and the caller is told when to ask again
	if next.Work != nil {
		t.Fatal("leased a delivery scheduled for later")
	}
	if next.AvailableAt == nil || !next.AvailableAt.Equal(retryAt) {
		t.Fatalf("available at = %v, want %v", next.AvailableAt, retryAt)
	}

	// And at that instant it leases as a second attempt
	after, err := inbox.Lease(t.Context(), retryAt, retryAt.Add(time.Minute))
	if err != nil || after.Work == nil {
		t.Fatalf("lease at the retry instant = %v, %v", after, err)
	}
	if after.Work.Attempt != 2 {
		t.Fatalf("attempt = %d, want 2", after.Work.Attempt)
	}
}

func TestMemoryInboxReleasesAnExpiredLease(t *testing.T) {
	t.Parallel()

	// Given a delivery leased for one minute
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{
		Now: func() time.Time { return now },
	})
	if _, err := inbox.Claim(t.Context(), claimOf("k")); err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.Lease(t.Context(), now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	// When another executor leases while that one holds it
	blocked, err := inbox.Lease(t.Context(), now.Add(time.Second), now.Add(time.Minute))

	// Then it gets nothing
	if err != nil || blocked.Work != nil {
		t.Fatalf("leased a delivery another executor holds: %v, %v", blocked, err)
	}

	// And once the lease has expired it gets the delivery
	later := now.Add(2 * time.Minute)
	recovered, err := inbox.Lease(t.Context(), later, later.Add(time.Minute))
	if err != nil || recovered.Work == nil {
		t.Fatalf("expired lease was not re-leasable: %v, %v", recovered, err)
	}
}

func TestMemoryInboxForgetsSettledDeliveriesPastTheTTL(t *testing.T) {
	t.Parallel()

	// Given a settled delivery and a one-minute memory
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	clock := now
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{
		TTL: time.Minute, Now: func() time.Time { return clock },
	})
	claimed, _ := inbox.Claim(t.Context(), claimOf("k"))
	if err := inbox.Complete(t.Context(), claimed.ID, clock); err != nil {
		t.Fatal(err)
	}

	// When two minutes have passed
	clock = now.Add(2 * time.Minute)
	again, err := inbox.Claim(t.Context(), claimOf("k"))

	// Then the same key is accepted as new work
	if err != nil || again.Disposition != webhook.Accepted {
		t.Fatalf("claim past the TTL = %v, %v", again, err)
	}
}

func TestMemoryInboxNeverEvictsLiveWork(t *testing.T) {
	t.Parallel()

	// Given more live deliveries than the inbox is sized for
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{MaxEntries: 2})
	for _, key := range []string{"a", "b", "c", "d"} {
		if _, err := inbox.Claim(t.Context(), claimOf(key)); err != nil {
			t.Fatal(err)
		}
	}

	// When each is claimed again
	// Then every one of them is still held
	for _, key := range []string{"a", "b", "c", "d"} {
		result, err := inbox.Claim(t.Context(), claimOf(key))
		if err != nil {
			t.Fatal(err)
		}
		if result.Disposition != webhook.InProgress {
			t.Fatalf("%s = %s, want in_progress", key, result.Disposition)
		}
	}
}
