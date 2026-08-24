package apply

import (
	"context"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type noChangesStore struct{ Store }

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
