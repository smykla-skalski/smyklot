package main

import (
	"context"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

func TestPendingCIControlSerializesCancellationWithRepositoryWork(t *testing.T) {
	t.Parallel()
	coordinator := newPendingCICoordinator()
	store := &pendingCIControlStoreStub{
		request:  pendingci.Request{ID: 41, RepositoryID: "repository:7", Revision: 3},
		read:     make(chan struct{}),
		finished: make(chan struct{}),
	}
	control := newPendingCIControl(store, coordinator, func() {})
	held := make(chan struct{})
	release := make(chan struct{})
	ownerDone := make(chan error, 1)
	go func() {
		ownerDone <- coordinator.Exclusive(t.Context(), store.request.RepositoryID, func() error {
			close(held)
			<-release

			return nil
		})
	}()
	<-held

	cancelDone := make(chan error, 1)
	go func() {
		_, err := control.Cancel(t.Context(), pendingci.FinishRequest{
			ID: store.request.ID, ExpectedRevision: store.request.Revision,
			Lifecycle: pendingci.LifecycleCancelled, Reason: "operator cancelled",
			FinishedAt: time.Now().UTC(),
		})
		cancelDone <- err
	}()
	<-store.read

	select {
	case <-store.finished:
		t.Fatal("cancellation bypassed repository ownership")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-ownerDone; err != nil {
		t.Fatal(err)
	}
	if err := <-cancelDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.finished:
	default:
		t.Fatal("cancellation did not finish after repository ownership was released")
	}
}

func TestPendingCIControlSerializesTargetSettingsWithEveryRepository(t *testing.T) {
	t.Parallel()
	coordinator := newPendingCICoordinator()
	control := newPendingCIControl(&pendingCIControlStoreStub{}, coordinator, func() {})
	held := make(chan struct{})
	release := make(chan struct{})
	ownerDone := make(chan error, 1)
	go func() {
		ownerDone <- coordinator.Exclusive(t.Context(), "repository:2", func() error {
			close(held)
			<-release

			return nil
		})
	}()
	<-held

	settingsStarted := make(chan struct{})
	settingsDone := make(chan error, 1)
	go func() {
		settingsDone <- control.Exclusive(
			t.Context(),
			[]string{"repository:2", "repository:1", "repository:2"},
			func() error {
				close(settingsStarted)

				return nil
			},
		)
	}()

	select {
	case <-settingsStarted:
		t.Fatal("target settings bypassed repository gate work")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-ownerDone; err != nil {
		t.Fatal(err)
	}
	if err := <-settingsDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-settingsStarted:
	default:
		t.Fatal("target settings did not start after repository work finished")
	}
}

type pendingCIControlStoreStub struct {
	request  pendingci.Request
	read     chan struct{}
	finished chan struct{}
}

func (store *pendingCIControlStoreStub) Get(
	context.Context,
	int64,
) (pendingci.Request, error) {
	close(store.read)

	return store.request, nil
}

func (store *pendingCIControlStoreStub) CheckNow(
	context.Context,
	pendingci.CheckNowRequest,
) (pendingci.Request, error) {
	return store.request, nil
}

func (store *pendingCIControlStoreStub) Finish(
	context.Context,
	pendingci.FinishRequest,
) (pendingci.Request, error) {
	close(store.finished)

	return store.request, nil
}
