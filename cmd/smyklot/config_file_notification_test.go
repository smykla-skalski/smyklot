package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

type configNotificationCatalogStore struct {
	maintenanceCatalogStore
	stop      error
	published bool
	retained  bool
}

func (store *configNotificationCatalogStore) DispatchConfigFileNotifications(context.Context, time.Time) (int, error) {
	// An opt-in save can complete just before notifications are dispatched.
	store.repositories[0].ConfigFileSyncEnabled = true
	store.published = true
	return 1, nil
}

func (store *configNotificationCatalogStore) SupersedeMissingRecurringWork(
	_ context.Context, claims []workqueue.RecurringClaim, _ time.Time,
) ([]workqueue.Item, error) {
	for _, claim := range claims {
		if claim.Kind == workqueue.KindConfigFileSync && claim.RepositoryID != nil &&
			*claim.RepositoryID == store.repositories[0].ID {
			store.retained = true
		}
	}
	return nil, store.stop
}

func TestConfigFileNotificationsUseUpdatedCatalog(t *testing.T) {
	t.Parallel()
	store := &configNotificationCatalogStore{
		maintenanceCatalogStore: maintenanceCatalogStore{
			target: storage.Target{ID: storage.InstallationID(1), InstallationID: "1", Available: true},
			repositories: []storage.Repository{{
				ID: storage.RepositoryID(11), FullName: "owner/connected", Available: true,
			}},
		},
		stop: errors.New("stop before external work"),
	}
	service := &server{store: store}
	if err := service.dispatchDurableMaintenance(t.Context()); !errors.Is(err, store.stop) {
		t.Fatalf("maintenance dispatch returned %v", err)
	}
	if !store.published || !store.retained {
		t.Fatalf("new configuration check published = %t, retained in job snapshot = %t", store.published, store.retained)
	}
}
