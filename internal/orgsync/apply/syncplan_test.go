package apply

import (
	"context"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type noChangesStore struct{ Store }

type repositoryFilterStore struct {
	Store
	target       storage.Target
	repositories []storage.Repository
	pathWrites   int
}

func (store *repositoryFilterStore) ListSyncConfigs(
	context.Context,
	string,
) ([]orgsync.Config, error) {
	return []orgsync.Config{{Kind: orgsync.KindFiles}}, nil
}

func (store *repositoryFilterStore) GetTarget(
	context.Context,
	string,
) (storage.Target, error) {
	return store.target, nil
}

func (store *repositoryFilterStore) ListRepositories(
	context.Context,
	string,
) ([]storage.Repository, error) {
	return store.repositories, nil
}

func (*repositoryFilterStore) ListSyncRepositoryOverrides(
	context.Context,
	string,
) ([]orgsync.RepositoryOverride, error) {
	return nil, nil
}

func (*repositoryFilterStore) ListSyncRepositoryPathScans(
	context.Context,
	string,
) ([]orgsync.RepositoryPathScan, error) {
	return nil, nil
}

func (store *repositoryFilterStore) SetSyncRepositoryPaths(
	context.Context,
	orgsync.RepositoryPaths,
) error {
	store.pathWrites++

	return nil
}

func (*repositoryFilterStore) PruneSyncRepositoryPaths(context.Context, string) (int64, error) {
	return 0, nil
}

func (noChangesStore) ListSyncConfigs(context.Context, string) ([]orgsync.Config, error) {
	return nil, nil
}

func (noChangesStore) GetTarget(context.Context, string) (storage.Target, error) {
	return storage.Target{}, nil
}

func (noChangesStore) ListSyncRepositoryState(
	context.Context,
	string,
) ([]orgsync.RepositoryState, error) {
	return nil, nil
}

func TestPlanInstallationNamesAnEmptyDriftScan(t *testing.T) {
	engine := New(noChangesStore{}, nil, "")
	summary, err := engine.PlanInstallationWithSummary(
		t.Context(), nil, "github:installation:10", orgsync.TriggerManual,
	)
	if err != nil {
		t.Fatal(err)
	}
	if summary != "No changes" {
		t.Errorf("summary = %q, want No changes", summary)
	}
}

func TestSyncInventoryAndPathIndexExcludeDisabledRepositories(t *testing.T) {
	enabled := true
	target := storage.Target{
		ID: "github:installation:10", Available: true,
		RepositoryDefaultEnabled: false,
	}
	store := &repositoryFilterStore{
		target: target,
		repositories: []storage.Repository{
			{ID: "disabled", Available: true},
			{ID: "enabled", Available: true, EnabledOverride: &enabled},
		},
	}
	engine := New(store, nil, "")
	inventory, err := engine.syncInventoryFor(t.Context(), target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.repositories) != 1 || inventory.repositories[0].ID != "enabled" {
		t.Fatalf("sync inventory = %#v", inventory.repositories)
	}

	store.repositories = store.repositories[:1]
	engine.RefreshPaths(t.Context(), nil, target.ID, 0)
	if store.pathWrites != 0 {
		t.Fatalf("disabled path index writes = %d", store.pathWrites)
	}
}
