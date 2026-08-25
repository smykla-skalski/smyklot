package main

import (
	"context"
	"testing"

	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestMaintenanceQueueCoversEveryRecurringWorkload(t *testing.T) {
	t.Parallel()
	targetID := "github:installation:1"
	server := &server{panel: &adminpanel.Server{}}
	jobs := server.panelMaintenanceJobs(context.Background())
	jobs = append(jobs, server.targetMaintenanceJobs(context.Background(), targetID, 1)...)
	jobs = append(jobs, server.repositoryMaintenanceJobs(
		context.Background(), targetID, 1,
		github.Repository{ID: 1, Owner: "smykla-skalski", Name: "smyklot"},
	)...)

	queued := make(map[workqueue.Kind]int, len(jobs))
	for _, job := range jobs {
		queued[job.work.kind]++
	}
	for _, kind := range workqueue.Kinds() {
		if !kind.Recurring() {
			continue
		}
		if queued[kind] != 1 {
			t.Errorf("recurring workload %q has %d maintenance queue producers", kind, queued[kind])
		}
	}
	if len(queued) != 8 {
		t.Errorf("maintenance queue has %d workload producers, want 8", len(queued))
	}
}

func TestStoredSweepRepositoryReadsScopedIdentity(t *testing.T) {
	t.Parallel()

	repository, err := storedSweepRepository(storage.Repository{
		ID:            storage.RepositoryID(31),
		FullName:      "smykla-skalski/smyklot",
		Private:       true,
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.ID != 31 || repository.Owner != "smykla-skalski" ||
		repository.Name != "smyklot" || !repository.Private || repository.DefaultBranch != "main" {
		t.Fatalf("converted repository = %#v", repository)
	}
}
