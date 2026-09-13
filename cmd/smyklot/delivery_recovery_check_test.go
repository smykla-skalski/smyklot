package main

import (
	"context"
	"errors"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type recoveryCheckStore struct {
	storage.Store
	target         storage.Target
	repository     storage.Repository
	err            error
	sourceAccepted bool
}

func (s recoveryCheckStore) GetTarget(context.Context, string) (storage.Target, error) {
	return s.target, s.err
}

func (s recoveryCheckStore) GetRepository(context.Context, string, string) (storage.Repository, error) {
	return s.repository, s.err
}

func (s recoveryCheckStore) CheckSourceRevision(context.Context, pendingci.SourceRevisionRequest) (pendingci.SourceRevisionResult, error) {
	return pendingci.SourceRevisionResult{Accepted: s.sourceAccepted}, s.err
}

func TestDeliveryRecoveryWorkloadControls(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		change  func(*storage.DeliveryRecoveryInput, *recoveryCheckStore)
		reason  panel.DeliveryRecoveryReason
		wantErr bool
	}{
		{name: "configuration wake", reason: panel.RecoveryAvailable},
		{name: "CI event wakes current state", change: func(i *storage.DeliveryRecoveryInput, _ *recoveryCheckStore) {
			i.Event = "status"
			i.Payload = []byte(`{"installation":{"id":9},"repository":{"id":7,"owner":{"login":"owner"},"name":"repo"},"sha":"head","context":"tests","state":"success"}`)
		}, reason: panel.RecoveryAvailable},
		{name: "malformed CI event", change: func(i *storage.DeliveryRecoveryInput, _ *recoveryCheckStore) { i.Event = "status" }, reason: panel.RecoveryPayloadInvalid},
		{name: "configuration still works when commands disabled", change: func(_ *storage.DeliveryRecoveryInput, s *recoveryCheckStore) {
			s.target.RepositoryDefaultEnabled = false
		}, reason: panel.RecoveryAvailable},
		{name: "removed repository", change: func(_ *storage.DeliveryRecoveryInput, s *recoveryCheckStore) { s.repository.Available = false }, reason: panel.RecoveryRepositoryUnavailable},
		{name: "unknown target", change: func(_ *storage.DeliveryRecoveryInput, s *recoveryCheckStore) { s.err = storage.ErrNotFound }, reason: panel.RecoveryRepositoryUnavailable},
		{name: "database outage", change: func(_ *storage.DeliveryRecoveryInput, s *recoveryCheckStore) {
			s.err = errors.New("database unavailable")
		}, wantErr: true},
		{name: "wrong installation", change: func(i *storage.DeliveryRecoveryInput, _ *recoveryCheckStore) { i.TargetID = "installation:2" }, reason: panel.RecoveryPayloadInvalid},
		{name: "wrong repository", change: func(i *storage.DeliveryRecoveryInput, _ *recoveryCheckStore) {
			v := "repository:2"
			i.RepositoryID = &v
		}, reason: panel.RecoveryPayloadInvalid},
		{name: "expired payload", change: func(i *storage.DeliveryRecoveryInput, _ *recoveryCheckStore) { i.Payload = nil }, reason: panel.RecoveryPayloadInvalid},
		{name: "unsupported event", change: func(i *storage.DeliveryRecoveryInput, _ *recoveryCheckStore) { i.Event = "unrecognized" }, reason: panel.RecoveryPayloadInvalid},
		{name: "old default branch", change: func(_ *storage.DeliveryRecoveryInput, s *recoveryCheckStore) { s.repository.DefaultBranch = "trunk" }, reason: panel.RecoveryNoAction},
		{name: "disabled command processing", change: func(i *storage.DeliveryRecoveryInput, s *recoveryCheckStore) {
			i.Event = "issue_comment"
			s.target.RepositoryDefaultEnabled = false
		}, reason: panel.RecoveryRepositoryDisabled},
	} {
		t.Run(test.name, func(t *testing.T) {
			repoID := storage.RepositoryID(7)
			input := storage.DeliveryRecoveryInput{TargetID: storage.InstallationID(9), RepositoryID: &repoID, Event: "push", Payload: []byte(`{"installation":{"id":9},"repository":{"id":7,"owner":{"login":"owner"},"name":"repo","default_branch":"main"},"ref":"refs/heads/main"}`)}
			store := recoveryCheckStore{target: storage.Target{Available: true, RepositoryDefaultEnabled: true}, repository: storage.Repository{Available: true, DefaultBranch: "main"}}
			if test.change != nil {
				test.change(&input, &store)
			}
			check, err := (&server{store: store}).CheckDeliveryRecovery(t.Context(), input)
			if (err != nil) != test.wantErr || check.Reason != test.reason {
				t.Fatalf("check = %+v, %v", check, err)
			}
			if check.Reason == panel.RecoveryAvailable && check.Effect == "" {
				t.Fatal("available recovery has no effect explanation")
			}
		})
	}
}

type recoveryReauthorizationStub struct {
	allowed bool
	err     error
}

func (s recoveryReauthorizationStub) CheckReauthorization(context.Context, string, pendingci.Signal) (bool, error) {
	return s.allowed, s.err
}

func TestCIRecoveryExplainsApprovalAndUnavailableRequirements(t *testing.T) {
	t.Parallel()
	signal := []pendingci.Signal{{Kind: pendingci.SignalReauthorize}}
	check, err := checkNotificationRecovery(t.Context(), recoveryReauthorizationStub{allowed: true}, "repo", signal)
	if err != nil || check.Reason != panel.RecoveryAvailable || check.Effect != "Recheck CI authorization and restore the required approval using the original request." {
		t.Fatalf("available CI recovery = %+v, %v", check, err)
	}
	check, err = checkNotificationRecovery(t.Context(), recoveryReauthorizationStub{}, "repo", signal)
	if err != nil || check.Reason != panel.RecoveryReauthorizationUnavailable {
		t.Fatalf("unavailable CI recovery = %+v, %v", check, err)
	}
	cause := errors.New("GitHub unavailable")
	_, err = checkNotificationRecovery(t.Context(), recoveryReauthorizationStub{err: cause}, "repo", signal)
	if !errors.Is(err, cause) {
		t.Fatalf("verification failure = %v", err)
	}
}
