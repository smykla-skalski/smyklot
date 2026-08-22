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
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})

	first, err := inbox.Claim(t.Context(), claimOf("k"))
	if err != nil || first.Disposition != webhook.Accepted {
		t.Fatalf("first claim = %v, %v", first, err)
	}
	second, err := inbox.Claim(t.Context(), claimOf("k"))
	if err != nil || second.Disposition != webhook.InProgress {
		t.Fatalf("second claim = %v, %v", second, err)
	}
}

// A settled delivery is remembered, so a redelivery of it changes nothing.
func TestMemoryInboxRetainsASettledDelivery(t *testing.T) {
	t.Parallel()
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})

	claimed, _ := inbox.Claim(t.Context(), claimOf("k"))
	if err := inbox.Complete(t.Context(), claimed.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	again, err := inbox.Claim(t.Context(), claimOf("k"))
	if err != nil || again.Disposition != webhook.Retained {
		t.Fatalf("redelivery = %v, %v", again, err)
	}
}

// A retryable failure is the one case where redelivery should be accepted:
// that is what marking it retryable meant.
func TestMemoryInboxForgetsARetryableFailure(t *testing.T) {
	t.Parallel()
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})

	claimed, _ := inbox.Claim(t.Context(), claimOf("k"))
	err := inbox.Fail(t.Context(), webhook.Failure{
		ClaimID: claimed.ID, Retryable: true, At: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	again, err := inbox.Claim(t.Context(), claimOf("k"))
	if err != nil || again.Disposition != webhook.Accepted {
		t.Fatalf("redelivery after a retryable failure = %v, %v", again, err)
	}
}

// Two executors leasing at once must not both get the same row, or the same
// comment is answered twice.
func TestMemoryInboxLeasesExclusively(t *testing.T) {
	t.Parallel()
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})
	if _, err := inbox.Claim(t.Context(), claimOf("k")); err != nil {
		t.Fatal(err)
	}

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

	if leased != 1 {
		t.Fatalf("leased %d times, want exactly one", leased)
	}
}

// Nothing ready yet, so the dispatcher is told when to ask again rather than
// left to poll.
func TestMemoryInboxReportsWhenWorkBecomesAvailable(t *testing.T) {
	t.Parallel()
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

	next, err := inbox.Lease(t.Context(), now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if next.Work != nil {
		t.Fatal("leased a delivery scheduled for later")
	}
	if next.AvailableAt == nil || !next.AvailableAt.Equal(retryAt) {
		t.Fatalf("available at = %v, want %v", next.AvailableAt, retryAt)
	}

	after, err := inbox.Lease(t.Context(), retryAt, retryAt.Add(time.Minute))
	if err != nil || after.Work == nil {
		t.Fatalf("lease at the retry instant = %v, %v", after, err)
	}
	if after.Work.Attempt != 2 {
		t.Fatalf("attempt = %d, want 2", after.Work.Attempt)
	}
}

// An expired lease is leasable again, which is what makes recovery from a
// crashed process an optimisation rather than a requirement.
func TestMemoryInboxRelesesAnExpiredLease(t *testing.T) {
	t.Parallel()
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

	blocked, err := inbox.Lease(t.Context(), now.Add(time.Second), now.Add(time.Minute))
	if err != nil || blocked.Work != nil {
		t.Fatalf("leased a delivery another executor holds: %v, %v", blocked, err)
	}

	later := now.Add(2 * time.Minute)
	recovered, err := inbox.Lease(t.Context(), later, later.Add(time.Minute))
	if err != nil || recovered.Work == nil {
		t.Fatalf("expired lease was not re-leasable: %v, %v", recovered, err)
	}
}

// Past the TTL a settled delivery is forgotten, so memory does not grow for
// ever. GitHub gives up redelivering long before then.
func TestMemoryInboxForgetsSettledDeliveriesPastTheTTL(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	clock := now
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{
		TTL: time.Minute, Now: func() time.Time { return clock },
	})

	claimed, _ := inbox.Claim(t.Context(), claimOf("k"))
	if err := inbox.Complete(t.Context(), claimed.ID, clock); err != nil {
		t.Fatal(err)
	}

	clock = now.Add(2 * time.Minute)
	again, err := inbox.Claim(t.Context(), claimOf("k"))
	if err != nil || again.Disposition != webhook.Accepted {
		t.Fatalf("claim past the TTL = %v, %v", again, err)
	}
}

// A live delivery is never evicted: forgetting one would let a redelivery run
// beside the copy still executing.
func TestMemoryInboxNeverEvictsLiveWork(t *testing.T) {
	t.Parallel()
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{MaxEntries: 2})

	for _, key := range []string{"a", "b", "c", "d"} {
		if _, err := inbox.Claim(t.Context(), claimOf(key)); err != nil {
			t.Fatal(err)
		}
	}
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
