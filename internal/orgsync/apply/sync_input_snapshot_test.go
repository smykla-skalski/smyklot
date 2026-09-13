package apply

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type observationSnapshotStore struct {
	Store
	states []orgsync.RepositoryState
}

func (s *observationSnapshotStore) RecordSyncRepositoryState(_ context.Context, states []orgsync.RepositoryState) error {
	s.states = states
	return nil
}

func TestPlanningUsesTheInputsRecordedInItsObservation(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/owner/repo/git/trees/main":
			_, _ = w.Write([]byte(`{"sha":"tree","tree":[],"truncated":false}`))
		case "/repos/owner/repo/pulls":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer endpoint.Close()
	client, err := github.NewClient("test-token", endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	store := &observationSnapshotStore{}
	engine := New(store, nil, "")
	before := config.DefaultFormattingPolicy()
	before.Common.FinalNewline = "insert"
	after := before
	after.Common.FinalNewline = "remove"
	repository := storage.Repository{ID: "repo", FullName: "owner/repo", DefaultBranch: "main", Available: true}
	document, err := json.Marshal(orgsync.FileConfig{Files: []orgsync.File{{Path: "config.json", Content: "{}"}}})
	if err != nil {
		t.Fatal(err)
	}
	saved := orgsync.Config{
		Kind: orgsync.KindFiles, Enabled: true, Document: document,
		Digest: orgsync.DigestConfig(true, document),
	}
	scope := newSyncScope(saved, nil, nil, time.Now().UTC(), before, config.Patch{})
	// The process setting changes after the planning scope was captured.
	engine.SetFormattingPolicy(after)
	actions, err := engine.planSyncActions(t.Context(), client, []orgsync.Config{saved},
		map[orgsync.Kind]syncScope{orgsync.KindFiles: scope}, syncInventory{repositories: []storage.Repository{repository}})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || len(store.states) != 1 {
		t.Fatalf("actions=%d states=%d", len(actions), len(store.states))
	}
	file, err := orgsync.DecodeFile(actions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(file.Content), "\n") {
		t.Fatalf("planned bytes used newer inputs: %q", file.Content)
	}
	if store.states[0].ObservedDigest != scope.digestFor(repository) || store.states[0].Observation != orgsync.ObservationDifferent {
		t.Fatalf("observation does not describe planned inputs: %#v", store.states[0])
	}
}
