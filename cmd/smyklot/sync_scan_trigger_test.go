package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

type scanOccurrenceStore struct {
	storage.Store
	finished         string
	completedAttempt int
	readID           string
	readErr          error
}

func (s *scanOccurrenceStore) FinishRecurringWork(_ context.Context, id string, completion workqueue.RecurringCompletion, _ time.Time) (workqueue.Item, error) {
	s.finished = id
	s.completedAttempt = completion.Attempt
	return workqueue.Item{}, nil
}

func (s *scanOccurrenceStore) QueueRunWasRequested(_ context.Context, id string) (bool, error) {
	s.readID = id
	return false, s.readErr
}

func TestMaintenanceSummaryReceivesClaimedOccurrence(t *testing.T) {
	store := &scanOccurrenceStore{}
	service := &server{store: store}
	item := workqueue.Item{ID: "claimed-scan", Kind: workqueue.KindSyncScan, State: workqueue.StateRunning, Attempt: 2}
	var received workqueue.Item
	err := service.runClaimedMaintenanceJob(t.Context(), item, maintenanceJob{work: recurringWork{kind: workqueue.KindSyncScan}, runWithSummary: func(got workqueue.Item) (string, error) { received = got; return "checked", nil }})
	if err != nil {
		t.Fatal(err)
	}
	if received.ID != item.ID || received.Attempt != 2 || received.State != workqueue.StateRunning || store.finished != item.ID || store.completedAttempt != item.Attempt {
		t.Fatalf("received=%#v finished=%q", received, store.finished)
	}
}

func TestSyncScanDoesNotGuessIntentWhenHistoryReadFails(t *testing.T) {
	unavailable := errors.New("history unavailable")
	store := &scanOccurrenceStore{readErr: unavailable}
	service := &server{store: store}
	_, err := service.runSyncScan(t.Context(), nil, "target", workqueue.Item{ID: "claimed-scan"})
	if !errors.Is(err, unavailable) || store.readID != "claimed-scan" {
		t.Fatalf("err=%v occurrence=%q", err, store.readID)
	}
}

func TestSyncScanRetryUsesItsRetainedResult(t *testing.T) {
	service := &server{}
	summary, err := service.runSyncScan(t.Context(), nil, "target", workqueue.Item{
		ID: "check", Kind: workqueue.KindSyncScan, Attempt: 2,
		Details: []byte(`{"result_plan_id":"original-plan"}`),
	})
	if err != nil || summary == "" {
		t.Fatalf("retained result was not reused: %q, %v", summary, err)
	}
}

func TestSyncScanRejectsMalformedRetainedResult(t *testing.T) {
	service := &server{}
	_, err := service.runSyncScan(t.Context(), nil, "target", workqueue.Item{Details: []byte(`{"result_plan_id":123}`)})
	if err == nil {
		t.Fatal("malformed result allowed a new scan")
	}
}

func TestSyncScanRetryPreservesNoPlanOutcome(t *testing.T) {
	service := &server{}
	summary, err := service.runSyncScan(t.Context(), nil, "target", workqueue.Item{Details: []byte(`{"outcome":{"summary":"1 check failed. 2 checks matched saved settings."}}`)})
	if err != nil || summary != "1 check failed. 2 checks matched saved settings." {
		t.Fatalf("retained no-plan result changed: %q, %v", summary, err)
	}
	_, err = service.runSyncScan(t.Context(), nil, "target", workqueue.Item{Details: []byte(`{"outcome":{}}`)})
	if err == nil {
		t.Fatal("incomplete retained outcome allowed a new scan")
	}
}
