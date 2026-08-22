package webhook_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type classified struct {
	error
	retryable bool
}

func (c classified) Retryable() bool { return c.retryable }

func TestDefaultRetry(t *testing.T) {
	t.Parallel()
	transient := errors.New("github is having a moment")

	tests := []struct {
		name    string
		err     error
		attempt int
		delay   time.Duration
		again   bool
	}{
		// An error that says nothing is assumed transient: the failure that
		// costs a user their command is the one that was retryable and was not
		// retried.
		{"an unclassified error retries", transient, 1, 2 * time.Second, true},
		{"backoff doubles", transient, 2, 4 * time.Second, true},
		{"and again", transient, 3, 8 * time.Second, true},
		// The seventh delay is the last one: attempt eight gives up. The
		// five-minute ceiling in the policy is a guard against a raised
		// attempt budget, not something these attempts reach.
		{"the last delay is two minutes", transient, 7, 128 * time.Second, true},
		{"the budget runs out", transient, 8, 0, false},
		{"and stays out", transient, 99, 0, false},
		{
			"an error that says it is not retryable is believed",
			classified{transient, false},
			1, 0, false,
		},
		{
			"an error that says it is retryable is too",
			classified{transient, true},
			1, 2 * time.Second, true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			delay, again := webhook.DefaultRetry(test.err, test.attempt)
			if again != test.again {
				t.Fatalf("again = %v, want %v", again, test.again)
			}
			if again && delay != test.delay {
				t.Fatalf("delay = %s, want %s", delay, test.delay)
			}
		})
	}
}

func TestNewRefusesAPipelineItCannotRun(t *testing.T) {
	t.Parallel()
	inbox := webhook.NewMemoryInbox(webhook.MemoryInboxOptions{})
	handle := webhook.Handler(nil)

	// The secret is positional for exactly this reason: a pipeline that can be
	// built without one eventually is, and it serves unsigned webhooks.
	if _, err := webhook.New(nil, inbox, func(_ context.Context, _ webhook.Delivery) error {
		return nil
	}, webhook.Options{}); !errors.Is(err, webhook.ErrNoSecret) {
		t.Fatalf("empty secret = %v, want ErrNoSecret", err)
	}
	if _, err := webhook.New([]byte("s"), nil, func(_ context.Context, _ webhook.Delivery) error {
		return nil
	}, webhook.Options{}); !errors.Is(err, webhook.ErrNoInbox) {
		t.Fatalf("nil inbox = %v, want ErrNoInbox", err)
	}
	if _, err := webhook.New([]byte("s"), inbox, handle, webhook.Options{}); !errors.Is(
		err, webhook.ErrNoHandler,
	) {
		t.Fatalf("nil handler = %v, want ErrNoHandler", err)
	}
}
