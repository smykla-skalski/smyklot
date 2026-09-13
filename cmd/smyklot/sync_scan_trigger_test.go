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
	finished string
	readID   string
	readErr  error
}

func (s *scanOccurrenceStore) FinishRecurringWork(_ context.Context, id string, _ workqueue.RecurringCompletion, _ time.Time) (workqueue.Item, error) {
	s.finished = id
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
	if received.ID != item.ID || received.Attempt != 2 || received.State != workqueue.StateRunning || store.finished != item.ID {
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
