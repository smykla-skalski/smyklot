package panel

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestParseQueueFilterCoversOperationalDimensions(t *testing.T) {
	request := httptest.NewRequest("GET", "/queue?workspace=github%3Ainstallation%3A42"+
		"&repository=repository%3A7&profile=weekday&state=ready,running"+
		"&workload=sync_scan&priority=high&created_after=2026-08-24T10%3A00%3A00Z"+
		"&created_before=2026-08-25T10%3A00%3A00Z&order=dispatch&summary=true"+
		"&limit=50&offset=100", nil)
	filter, err := parseQueueFilter(request)
	if err != nil {
		t.Fatalf("parse queue filter: %v", err)
	}
	if filter.TargetID == nil || *filter.TargetID != "github:installation:42" ||
		filter.RepositoryID == nil || *filter.RepositoryID != "repository:7" ||
		filter.ProfileID == nil || *filter.ProfileID != "weekday" {
		t.Fatalf("scope filters were not parsed: %#v", filter)
	}
	if len(filter.States) != 2 || filter.States[0] != workqueue.StateReady ||
		filter.States[1] != workqueue.StateRunning ||
		len(filter.Kinds) != 1 || filter.Kinds[0] != workqueue.KindSyncScan ||
		len(filter.Priorities) != 1 || filter.Priorities[0] != workqueue.PriorityHigh {
		t.Fatalf("typed filters were not parsed: %#v", filter)
	}
	wantAfter := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	wantBefore := wantAfter.Add(24 * time.Hour)
	if filter.CreatedAfter == nil || !filter.CreatedAfter.Equal(wantAfter) ||
		filter.CreatedBefore == nil || !filter.CreatedBefore.Equal(wantBefore) ||
		!filter.DispatchOrder || !filter.Summary || filter.Limit != 50 || filter.Offset != 100 {
		t.Fatalf("time or page filters were not parsed: %#v", filter)
	}
}

func TestParseQueueFilterRestrictsDispatchOrderToActiveWork(t *testing.T) {
	for _, query := range []string{
		"/queue?order=dispatch",
		"/queue?order=dispatch&state=succeeded",
		"/queue?order=unknown&state=ready",
	} {
		request := httptest.NewRequest("GET", query, nil)
		if _, err := parseQueueFilter(request); err == nil {
			t.Fatalf("expected %q to be rejected", query)
		}
	}
}

func TestParseQueueFilterRejectsInvalidSummary(t *testing.T) {
	request := httptest.NewRequest("GET", "/queue?summary=false", nil)
	if _, err := parseQueueFilter(request); err == nil {
		t.Fatal("expected invalid summary query to be rejected")
	}
}

func TestParseQueueFilterRejectsInvertedTimeRange(t *testing.T) {
	request := httptest.NewRequest("GET", "/queue?created_after=2026-08-25T10%3A00%3A00Z"+
		"&created_before=2026-08-24T10%3A00%3A00Z", nil)
	if _, err := parseQueueFilter(request); err == nil {
		t.Fatal("expected inverted time range to be rejected")
	}
}

func TestWorkspaceQueueRedactsSensitiveFailureData(t *testing.T) {
	actor := "account:17"
	item := workqueue.Item{
		Kind: workqueue.KindWebhookDelivery, State: workqueue.StateFailed,
		RequestedBy: &actor, Reason: "manual retry", BlockedReason: "provider token expired",
		Details: json.RawMessage(`{"payload":"secret"}`),
	}
	redactWorkspaceQueueItem(&item)
	if item.Details != nil || item.BlockedReason == "provider token expired" {
		t.Fatalf("sensitive item data was not redacted: %#v", item)
	}
	if item.RequestedBy == nil || *item.RequestedBy != actor || item.Reason != "manual retry" {
		t.Fatalf("audited action provenance was redacted: %#v", item)
	}

	retrying := workqueue.Item{
		Kind: workqueue.KindReactionScan, State: workqueue.StateRetrying,
		BlockedReason: "GitHub 429 response: token scope read:org",
	}
	redactWorkspaceQueueItem(&retrying)
	if retrying.BlockedReason == "GitHub 429 response: token scope read:org" {
		t.Fatalf("sensitive retry data was not redacted: %#v", retrying)
	}

	events := []workqueue.Event{
		{State: workqueue.StateFailed, Summary: "provider token expired", Details: json.RawMessage(`{"secret":true}`)},
		{State: workqueue.StateRetrying, Summary: "rate limit response body"},
		{State: workqueue.StateSucceeded, Summary: "Webhook delivered"},
	}
	redactWorkspaceQueueEvents(events)
	if events[0].Details != nil || events[0].Summary == "provider token expired" {
		t.Fatalf("failed event was not redacted: %#v", events[0])
	}
	if events[1].Summary == "rate limit response body" {
		t.Fatalf("retry event was not redacted: %#v", events[1])
	}
	if events[2].Summary != "Webhook delivered" {
		t.Fatalf("non-sensitive event changed: %#v", events[2])
	}
}
