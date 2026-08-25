package panel

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// pathIndexAnswer is what GET .../sync/paths writes.
type pathIndexAnswer struct {
	Paths []struct {
		Path         string `json:"path"`
		Repositories int    `json:"repositories"`
	} `json:"paths"`
	Repositories int        `json:"repositories"`
	Partial      bool       `json:"partial"`
	ObservedAt   *time.Time `json:"observed_at"`
}

// seedPathIndex puts a second repository in the catalog and gives both a list.
//
// Two, because everything this endpoint does is aggregation: one repository
// proves the shape and none of the arithmetic.
func seedPathIndex(t *testing.T, harness *panelHarness, first, second orgsync.RepositoryPaths) {
	t.Helper()

	snapshot := storage.InstallationSnapshot{
		TargetID:       "github:installation:10",
		InstallationID: "10",
		Kind:           storage.TargetOrganization,
		Account: storage.Account{
			ID:          "github:test:account:2",
			Provider:    "github:test",
			SubjectID:   "2",
			Login:       "smykla-skalski",
			DisplayName: "Smykla Skalski",
			UpdatedAt:   harness.now,
		},
		Repositories: []storage.RepositorySnapshot{
			{ID: "repository-20", Name: "smyklot", FullName: "smykla-skalski/smyklot"},
			{ID: "repository-21", Name: "docs", FullName: "smykla-skalski/docs"},
		},
		Ownership: storage.OwnershipSnapshot{
			Source: storage.OwnershipSourceOrganizationAdmin,
			Status: storage.OwnershipStatusFresh,
			Owners: []storage.Account{{
				ID: "github:test:user:1", Provider: "github:test", SubjectID: "1",
				Login: "owner", DisplayName: "Panel Owner", UpdatedAt: harness.now,
			}},
			SyncedAt: harness.now,
		},
		SyncedAt: harness.now,
	}
	if err := harness.store.ReconcileInstallation(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	harness.server.catalog = fakeCatalog{store: harness.store, snapshot: snapshot}

	enabled := true
	if _, err := harness.store.SaveInstallationSettings(
		t.Context(), storage.SaveInstallationSettingsRequest{
			TargetID: "github:installation:10", ActorAccountID: "github:test:user:1",
			ChangedAt: harness.now,
			Repositories: []storage.InstallationRepositorySettingsChange{
				{RepositoryID: "repository-20", EnabledOverride: &enabled, ExpectedRevision: 1},
				{RepositoryID: "repository-21", EnabledOverride: &enabled, ExpectedRevision: 1},
			},
		},
	); err != nil {
		t.Fatal(err)
	}

	for _, row := range []orgsync.RepositoryPaths{first, second} {
		if err := harness.store.SetSyncRepositoryPaths(t.Context(), row); err != nil {
			t.Fatal(err)
		}
	}
}

func readPathIndex(t *testing.T, harness *panelHarness) pathIndexAnswer {
	t.Helper()

	session := harness.signIn(t)
	response := harness.request(t, http.MethodGet,
		"/panel/api/v1/targets/github:installation:10/sync/paths", nil, session)
	if response.Code != http.StatusOK {
		t.Fatalf("reading the path index = %d %s", response.Code, response.Body.String())
	}

	var answer pathIndexAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}

	return answer
}

// TestPathIndexCountsRepositoriesPerPath covers the whole reason this endpoint
// aggregates: the same file across twenty-five repositories is one thing being
// configured rather than twenty-five facts, and the count is what says so.
func TestPathIndexCountsRepositoriesPerPath(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	seedPathIndex(t, harness,
		orgsync.RepositoryPaths{
			RepositoryID: "repository-20",
			TargetID:     "github:installation:10",
			Paths:        []string{"renovate.json", "README.md"},
			ObservedAt:   harness.now,
		},
		orgsync.RepositoryPaths{
			RepositoryID: "repository-21",
			TargetID:     "github:installation:10",
			Paths:        []string{"renovate.json"},
			ObservedAt:   harness.now,
		})

	answer := readPathIndex(t, harness)

	if answer.Repositories != 2 {
		t.Errorf("repositories = %d, wanted 2", answer.Repositories)
	}
	// Held by most first, so the path a reader most likely means is offered
	// first among equally good matches.
	if len(answer.Paths) != 2 {
		t.Fatalf("paths = %+v, wanted two", answer.Paths)
	}
	if answer.Paths[0].Path != "renovate.json" || answer.Paths[0].Repositories != 2 {
		t.Errorf("first = %+v, wanted renovate.json held by 2", answer.Paths[0])
	}
	if answer.Paths[1].Path != "README.md" || answer.Paths[1].Repositories != 1 {
		t.Errorf("second = %+v, wanted README.md held by 1", answer.Paths[1])
	}
}

// TestPathIndexReportsItsStalestReading pins the reading this answer takes of
// itself.
//
// The list is the union of every repository's, so how far it can be trusted is
// decided by its oldest row rather than by its freshest - the same reading
// `partial` takes, one repository GitHub would not finish listing making the
// whole answer incomplete. Reported as the newest, it said "checked a minute
// ago" for a list holding a repository nothing had looked at in a week.
func TestPathIndexReportsItsStalestReading(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	stale := harness.now.Add(-7 * 24 * time.Hour)
	seedPathIndex(t, harness,
		orgsync.RepositoryPaths{
			RepositoryID: "repository-20",
			TargetID:     "github:installation:10",
			Paths:        []string{"renovate.json"},
			ObservedAt:   harness.now,
		},
		orgsync.RepositoryPaths{
			RepositoryID: "repository-21",
			TargetID:     "github:installation:10",
			Paths:        []string{"README.md"},
			ObservedAt:   stale,
			Partial:      true,
		})

	answer := readPathIndex(t, harness)

	if answer.ObservedAt == nil {
		t.Fatal("observed_at is absent, wanted the stalest reading")
	}
	if !answer.ObservedAt.Equal(stale) {
		t.Errorf("observed_at = %s, wanted the stalest reading %s", answer.ObservedAt, stale)
	}
	// One repository GitHub would not list whole makes the whole answer some of
	// what the installation holds.
	if !answer.Partial {
		t.Error("partial = false, wanted true: one of these lists is incomplete")
	}
}

// TestPathIndexIsAggregatedOncePerVersionOfTheRows covers both halves of the
// cache: that a second reader is answered from the first reader's work, and
// that a sweep writing a row ends that.
//
// Aggregating is the expensive part - the union of two hundred repositories is
// millions of map operations over lists the finder opens against - and the rows
// behind it change about once a day. What decides is the scan read, which
// carries no paths at all.
func TestPathIndexIsAggregatedOncePerVersionOfTheRows(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	seedPathIndex(t, harness,
		orgsync.RepositoryPaths{
			RepositoryID: "repository-20",
			TargetID:     "github:installation:10",
			Paths:        []string{"renovate.json"},
			ObservedAt:   harness.now,
			HeadSHA:      "abc123",
		},
		orgsync.RepositoryPaths{
			RepositoryID: "repository-21",
			TargetID:     "github:installation:10",
			Paths:        []string{"README.md"},
			ObservedAt:   harness.now,
			HeadSHA:      "def456",
		})

	first := readPathIndex(t, harness)
	if len(first.Paths) != 2 {
		t.Fatalf("paths = %+v, wanted two", first.Paths)
	}

	// Written behind the endpoint's back. A reader answered from the held copy
	// cannot see it; one that rebuilds can - which is what makes this a test of
	// the cache rather than of the query.
	if err := harness.store.SetSyncRepositoryPaths(t.Context(), orgsync.RepositoryPaths{
		RepositoryID: "repository-21",
		TargetID:     "github:installation:10",
		Paths:        []string{"README.md", "CONTRIBUTING.md"},
		ObservedAt:   harness.now,
		// The same commit and the same moment, so ONLY the paths differ: the
		// stamp must not notice, because the refresh never rewrites a list
		// without the commit moving.
		HeadSHA: "def456",
	}); err != nil {
		t.Fatal(err)
	}

	held := readPathIndex(t, harness)
	if len(held.Paths) != 2 {
		t.Errorf("paths = %+v, wanted the two the held answer was built from", held.Paths)
	}

	// And a row read at a new commit, which is what a refresh really writes.
	if err := harness.store.SetSyncRepositoryPaths(t.Context(), orgsync.RepositoryPaths{
		RepositoryID: "repository-21",
		TargetID:     "github:installation:10",
		Paths:        []string{"README.md", "CONTRIBUTING.md"},
		ObservedAt:   harness.now,
		HeadSHA:      "moved789",
	}); err != nil {
		t.Fatal(err)
	}

	rebuilt := readPathIndex(t, harness)
	if len(rebuilt.Paths) != 3 {
		t.Errorf("paths = %+v, wanted three: the rows moved and the answer should have", rebuilt.Paths)
	}
}

func TestPathIndexDropsDisabledRepositoriesWithoutARefresh(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	seedPathIndex(t, harness,
		orgsync.RepositoryPaths{
			RepositoryID: "repository-20", TargetID: "github:installation:10",
			Paths: []string{"renovate.json"}, ObservedAt: harness.now,
		},
		orgsync.RepositoryPaths{
			RepositoryID: "repository-21", TargetID: "github:installation:10",
			Paths: []string{"private/config.yml"}, ObservedAt: harness.now,
		})

	first := readPathIndex(t, harness)
	if first.Repositories != 2 {
		t.Fatalf("repositories = %d, wanted 2", first.Repositories)
	}

	disabled := false
	if _, err := harness.store.SaveInstallationSettings(
		t.Context(), storage.SaveInstallationSettingsRequest{
			TargetID: "github:installation:10", ActorAccountID: "github:test:user:1",
			ChangedAt: harness.now.Add(time.Minute),
			Repositories: []storage.InstallationRepositorySettingsChange{{
				RepositoryID: "repository-21", EnabledOverride: &disabled, ExpectedRevision: 2,
			}},
		},
	); err != nil {
		t.Fatal(err)
	}

	rebuilt := readPathIndex(t, harness)
	if rebuilt.Repositories != 1 || len(rebuilt.Paths) != 1 ||
		rebuilt.Paths[0].Path != "renovate.json" {
		t.Fatalf("path index after disable = %+v", rebuilt)
	}
}
