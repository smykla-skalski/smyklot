package apply

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestScanSummaryPreservesNoActionOutcomes(t *testing.T) {
	for _, test := range []struct {
		name           string
		kind           orgsync.Kind
		document, pull string
		fail           bool
		want           orgsync.Observation
		words          string
	}{
		{name: "matching", kind: orgsync.KindLabels, document: `{"labels":[]}`, want: orgsync.ObservationMatched, words: "1 check matched saved settings"},
		{name: "read failed", kind: orgsync.KindLabels, document: `{"labels":[]}`, fail: true, want: orgsync.ObservationFailed, words: "1 check failed"},
		{name: "invalid input", kind: orgsync.KindLabels, document: `{`, want: orgsync.ObservationBlocked, words: "1 check could not proceed"},
		{name: "open proposal", kind: orgsync.KindFiles, document: `{"files":[{"path":"test.txt","content":"hello"}]}`, pull: "open", want: orgsync.ObservationProposed, words: "1 check found an open proposal"},
		{name: "declined proposal", kind: orgsync.KindFiles, document: `{"files":[{"path":"test.txt","content":"hello"}]}`, pull: "closed", want: orgsync.ObservationDeclined, words: "1 check found a declined proposal"},
	} {
		t.Run(test.name, func(t *testing.T) {
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveScanOutcome(t, w, r, test.fail, test.pull)
			}))
			defer endpoint.Close()
			client, err := github.NewClient("test-token", endpoint.URL)
			if err != nil {
				t.Fatal(err)
			}
			store := &freshCheckStore{saved: orgsync.Config{Kind: test.kind, Enabled: true, Document: []byte(test.document), Digest: "saved"}, repository: storage.Repository{ID: "repo", FullName: "owner/repo", DefaultBranch: "main", Available: true}}
			summary, err := New(store, nil, "").PlanInstallationWithSummary(t.Context(), client, "target", orgsync.TriggerManual)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(summary, test.words) || strings.Contains(summary, "No changes") {
				t.Fatalf("misleading summary: %q", summary)
			}
			if len(store.learned) != 1 || store.learned[0].Observation != test.want {
				t.Fatalf("observations: %#v", store.learned)
			}
			if test.want == orgsync.ObservationBlocked && store.learned[0].Problem == "" {
				t.Fatal("invalid input has no recovery guidance")
			}
		})
	}
}

func TestMixedScanSummaryRetainsProblemsBesideDifferences(t *testing.T) {
	result := syncScanResult{cached: 2, unpermitted: 1}
	for _, state := range []orgsync.Observation{orgsync.ObservationDifferent, orgsync.ObservationFailed, orgsync.ObservationFailed, orgsync.ObservationMatched, "future"} {
		result.observations = append(result.observations, orgsync.RepositoryState{Observation: state})
	}
	summary := result.summary()
	for _, words := range []string{"1 check found differences", "2 checks failed", "1 check matched saved settings", "1 check has no confirmed result", "Recent checks reused: 2", "Sync categories missing GitHub permissions: 1"} {
		if !strings.Contains(summary, words) {
			t.Errorf("%q missing from %q", words, summary)
		}
	}
}

type mixedScanStore struct {
	freshCheckStore
	created  orgsync.PlanCreate
	conflict bool
}

func (*mixedScanStore) ListRepositories(context.Context, string) ([]storage.Repository, error) {
	return []storage.Repository{{ID: "ready", FullName: "owner/ready", Available: true}, {ID: "failed", FullName: "owner/failed", Available: true}}, nil
}

func (s *mixedScanStore) CreateSyncPlan(_ context.Context, create orgsync.PlanCreate) (orgsync.Plan, error) {
	s.created = create
	if s.conflict {
		return orgsync.Plan{}, storage.ErrConflict
	}
	return orgsync.Plan{ID: create.ID, ComputedAt: create.Now}, nil
}

func (*mixedScanStore) RecordSyncAudit(context.Context, orgsync.AuditEntry) error { return nil }

func TestQueuedChangesDoNotHideOtherFailedChecks(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/repos/owner/ready/labels" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if r.URL.Path != "/repos/owner/failed/labels" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer endpoint.Close()
	client, err := github.NewClient("test-token", endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	store := &mixedScanStore{freshCheckStore: freshCheckStore{saved: orgsync.Config{Kind: orgsync.KindLabels, Enabled: true, Digest: "saved", Document: []byte(`{"labels":[{"name":"bug","color":"ffffff"}]}`)}}}
	summary, err := New(store, nil, "").PlanInstallationWithSummary(t.Context(), client, "target", orgsync.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.created.Actions) != 1 || store.created.Actions[0].RepositoryID != "ready" {
		t.Fatalf("planned actions: %#v", store.created.Actions)
	}
	if !strings.Contains(summary, "queued for automatic sync") || !strings.Contains(summary, "1 check failed") || !strings.Contains(summary, "1 check found differences") {
		t.Fatalf("partial outcome lost: %q", summary)
	}
	store.conflict = true
	summary, err = New(store, nil, "").PlanInstallationWithSummary(t.Context(), client, "target", orgsync.TriggerManual)
	if err != nil || !strings.Contains(summary, "A live sync plan is already available") || !strings.Contains(summary, "1 check failed") {
		t.Fatalf("concurrent plan hid observed failure: %q, %v", summary, err)
	}
}

func serveScanOutcome(t *testing.T, w http.ResponseWriter, r *http.Request, fail bool, pull string) {
	t.Helper()
	if r.Method != http.MethodGet {
		t.Errorf("scan wrote to GitHub: %s", r.Method)
	}
	w.Header().Set("Content-Type", "application/json")
	if fail {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch r.URL.Path {
	case "/repos/owner/repo/labels":
		_, _ = w.Write([]byte(`[]`))
	case "/repos/owner/repo/git/trees/main":
		_, _ = w.Write([]byte(`{"sha":"tree","tree":[],"truncated":false}`))
	case "/repos/owner/repo/pulls":
		_, _ = w.Write([]byte(`[{"number":42,"state":"` + pull + `","html_url":"https://github.com/owner/repo/pull/42"}]`))
	default:
		t.Errorf("unexpected request: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}
