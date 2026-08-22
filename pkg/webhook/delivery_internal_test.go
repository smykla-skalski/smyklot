package webhook

import (
	"strings"
	"testing"
)

// The delivery header is not covered by the signature, so its value is whatever
// the caller sent. It reaches log lines, metric labels and database rows, and a
// newline in a log line is a forged log line.
func TestSanitizeDeliveryID(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, raw, want string }{
		{
			"a real GitHub identifier survives untouched",
			"7d1b0f00-1234-11ef-9f6b-0242ac120002",
			"7d1b0f00-1234-11ef-9f6b-0242ac120002",
		},
		{"a newline is removed", "abc\ninjected", "abcinjected"},
		{"a carriage return is removed", "abc\r\nlevel=ERROR", "abclevelERROR"},
		{"an empty identifier becomes a placeholder", "", unknownDeliveryID},
		{"nothing usable becomes a placeholder", "\n\t", unknownDeliveryID},
		{
			"an overlong identifier is cut",
			strings.Repeat("a", 200),
			strings.Repeat("a", maxDeliveryIDLength),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeDeliveryID(test.raw); got != test.want {
				t.Fatalf("sanitizeDeliveryID(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

// An event name outside the configured set must never reach a metric label, or
// one made-up header is one time series.
func TestEventLabel(t *testing.T) {
	t.Parallel()
	known := map[string]struct{}{EventIssueComment: {}}

	if got := eventLabel(EventIssueComment, known); got != EventIssueComment {
		t.Fatalf("a configured event = %q", got)
	}
	// Answered by the pipeline itself, so it is always its own label.
	if got := eventLabel(EventPing, known); got != EventPing {
		t.Fatalf("ping = %q", got)
	}
	if got := eventLabel("<script>alert(1)</script>", known); got != eventOther {
		t.Fatalf("an unknown event = %q, want %q", got, eventOther)
	}
}
