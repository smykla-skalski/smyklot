package main

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	adminpanel "github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/pendingci/gate"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/open"
)

func TestPendingCIRequestCancelsAfterInstallationDisappearsWithoutRefreshing(t *testing.T) {
	t.Parallel()
	store, err := open.Store(t.Context(), filepath.Join(t.TempDir(), "uninstalled.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()

	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	initial := catalogTransferSnapshot("installation:a", "1", "owner-a", now)
	if err := store.ReconcileCatalog(t.Context(), []storage.InstallationSnapshot{initial}); err != nil {
		t.Fatal(err)
	}
	coordinator := bot.NewCoordinator()
	srv := &server{
		store: store, panel: &adminpanel.Server{}, pendingCICoordinator: coordinator,
		gate: gate.New(gate.Dependencies{
			Store: store, Gates: store, Checks: store, Transitions: store,
			Leases: store, Handoffs: store, Current: store,
			Coordinator: coordinator, Panelled: true,
			Logger: slog.New(slog.DiscardHandler),
		}),
	}
	if err := srv.reconcileCatalogSnapshots(t.Context(), nil); err != nil {
		t.Fatal(err)
	}

	backend := srv.gate.Backend()
	done := make(chan error, 1)
	go func() {
		done <- coordinator.Exclusive(t.Context(), "repository:7", func() error {
			observation, observeErr := backend.Observe(t.Context(), pendingci.Request{
				TargetID: "installation:a", RepositoryID: "repository:7",
				HeadSHA: "head", BaseBranch: "main",
			})
			if observeErr != nil {
				return observeErr
			}
			if observation.CancelReason != gate.RepositoryDisabledReason {
				return fmt.Errorf("cancel reason = %q", observation.CancelReason)
			}

			return nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending CI request deadlocked while refreshing an unavailable installation")
	}
}

func TestCatalogTransferWaitsForRepositoryGateEffects(t *testing.T) {
	t.Parallel()
	store, err := open.Store(t.Context(), filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()

	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	initial := catalogTransferSnapshot("installation:a", "1", "owner-a", now)
	if err := store.ReconcileCatalog(t.Context(), []storage.InstallationSnapshot{initial}); err != nil {
		t.Fatal(err)
	}

	coordinator := bot.NewCoordinator()
	srv := &server{store: store, pendingCICoordinator: coordinator}
	held := make(chan struct{})
	release := make(chan struct{})
	ownerDone := make(chan error, 1)
	go func() {
		ownerDone <- coordinator.Exclusive(t.Context(), "repository:7", func() error {
			close(held)
			<-release

			return nil
		})
	}()
	<-held

	transferred := catalogTransferSnapshot("installation:b", "2", "owner-b", now.Add(time.Minute))
	transferDone := make(chan error, 1)
	go func() {
		transferDone <- srv.reconcileCatalogSnapshots(
			t.Context(), []storage.InstallationSnapshot{transferred},
		)
	}()
	select {
	case err := <-transferDone:
		t.Fatalf("catalog transfer bypassed repository gate work: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := store.GetRepository(t.Context(), initial.TargetID, "repository:7"); err != nil {
		t.Fatalf("repository moved while gate work owned it: %v", err)
	}

	close(release)
	if err := <-ownerDone; err != nil {
		t.Fatal(err)
	}
	if err := <-transferDone; err != nil {
		t.Fatal(err)
	}
	repository, err := store.GetRepository(t.Context(), transferred.TargetID, "repository:7")
	if err != nil {
		t.Fatal(err)
	}
	if repository.TargetID != transferred.TargetID {
		t.Fatalf("repository target = %q, want %q", repository.TargetID, transferred.TargetID)
	}
}

func catalogTransferSnapshot(
	targetID string,
	installationID string,
	owner string,
	syncedAt time.Time,
) storage.InstallationSnapshot {
	return storage.InstallationSnapshot{
		TargetID: targetID, InstallationID: installationID,
		Kind: storage.TargetOrganization,
		Account: storage.Account{
			ID: "account:" + installationID, Provider: "github",
			SubjectID: installationID, Login: owner, UpdatedAt: syncedAt,
		},
		Repositories: []storage.RepositorySnapshot{{
			ID: "repository:7", Name: "repo", FullName: owner + "/repo",
			DefaultBranch: "main",
		}},
		SyncedAt: syncedAt,
	}
}
