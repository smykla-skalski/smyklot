package apply

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

type freshCheckStore struct {
	Store
	saved      orgsync.Config
	repository storage.Repository
	before     orgsync.RepositoryState
	learned    []orgsync.RepositoryState
	live       bool
}

func (s *freshCheckStore) ListSyncConfigs(context.Context, string) ([]orgsync.Config, error) {
	return []orgsync.Config{s.saved}, nil
}

func (s *freshCheckStore) GetTarget(context.Context, string) (storage.Target, error) {
	return storage.Target{ID: "target", Available: true, RepositoryDefaultEnabled: true, Permissions: map[string]string{"issues": "write"}}, nil
}

func (s *freshCheckStore) ListRepositories(context.Context, string) ([]storage.Repository, error) {
	return []storage.Repository{s.repository}, nil
}

func (*freshCheckStore) ListSyncRepositoryOverrides(context.Context, string) ([]orgsync.RepositoryOverride, error) {
	return nil, nil
}

func (s *freshCheckStore) ListSyncRepositoryState(context.Context, string) ([]orgsync.RepositoryState, error) {
	return []orgsync.RepositoryState{s.before}, nil
}

func (s *freshCheckStore) RecordSyncRepositoryState(_ context.Context, states []orgsync.RepositoryState) error {
	s.learned = states
	return nil
}

func (s *freshCheckStore) GetLiveSyncPlan(context.Context, string) (orgsync.Plan, []orgsync.Action, error) {
	if s.live {
		return orgsync.Plan{ID: "existing"}, nil, nil
	}
	return orgsync.Plan{}, nil, storage.ErrNotFound
}

func TestExplicitCheckRefreshesCachedRepositoryEvidence(t *testing.T) {
	for _, test := range []struct {
		name            string
		trigger         orgsync.Trigger
		live, available bool
		wantReads       int
	}{
		{"scheduled", orgsync.TriggerReconcile, false, true, 0},
		{"explicit", orgsync.TriggerManual, false, true, 1},
		{"active plan", orgsync.TriggerManual, true, true, 0},
		{"unavailable repository", orgsync.TriggerManual, false, false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			reads := 0
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads++
				if r.Method != http.MethodGet || r.URL.Path != "/repos/owner/repo/labels" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
			}))
			defer endpoint.Close()
			client, err := github.NewClient("test-token", endpoint.URL)
			if err != nil {
				t.Fatal(err)
			}
			saved := orgsync.Config{Kind: orgsync.KindLabels, Enabled: true, Document: []byte(`{"labels":[]}`), Digest: "saved"}
			repository := storage.Repository{ID: "repo", FullName: "owner/repo", Available: test.available}
			scope := newSyncScope(saved, nil, nil, time.Now().UTC(), config.DefaultFormattingPolicy(), config.Patch{})
			before := orgsync.RepositoryState{RepositoryID: repository.ID, Kind: saved.Kind, AppliedDigest: scope.digestFor(repository), ObservedDigest: scope.digestFor(repository), Observation: orgsync.ObservationMatched, AppliedAt: time.Now().Add(-time.Minute)}
			store := &freshCheckStore{saved: saved, repository: repository, before: before, live: test.live}
			_, err = New(store, nil, "").PlanInstallationWithSummary(t.Context(), client, "target", test.trigger)
			if err != nil {
				t.Fatal(err)
			}
			if reads != test.wantReads {
				t.Fatalf("GitHub reads=%d, want %d", reads, test.wantReads)
			}
			if test.wantReads > 0 && (len(store.learned) != 1 || !store.learned[0].AppliedAt.After(before.AppliedAt) || store.learned[0].Observation != orgsync.ObservationMatched) {
				t.Fatalf("fresh evidence not recorded: %#v", store.learned)
			}
		})
	}
}
