package main

import (
	"context"
	"errors"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestPendingCICommandRecognizesExistingArtifactOwnership(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		request pendingci.Request
		err     error
		label   string
		comment int
		owned   pendingCIArtifactOwnership
	}{
		{
			name: "same label",
			request: pendingci.Request{
				Label: "smyklot:pending:ci:squash", SourceCommentID: 101,
			},
			label: "smyklot:pending:ci:squash", comment: 101,
			owned: pendingCIArtifactOwnership{
				label: true, reaction: true, serviceMarker: true,
			},
		},
		{
			name: "same label with different comment",
			request: pendingci.Request{
				Label: "smyklot:pending:ci:squash", SourceCommentID: 202,
			},
			label: "smyklot:pending:ci:squash", comment: 101,
			owned: pendingCIArtifactOwnership{label: true, serviceMarker: true},
		},
		{
			name: "different label with same comment",
			request: pendingci.Request{
				Label: "smyklot:pending:ci:rebase", SourceCommentID: 101,
			},
			label: "smyklot:pending:ci:squash", comment: 101,
			owned: pendingCIArtifactOwnership{reaction: true, serviceMarker: true},
		},
		{
			name: "no armed request", err: storage.ErrNotFound,
			label: "smyklot:pending:ci:squash", comment: 101,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			command := pendingCICommand{
				store:        pendingCICommandStoreStub{request: test.request, getErr: test.err},
				repositoryID: "repository:7",
			}
			owned, err := command.armedArtifactOwnership(
				t.Context(), 198, test.label, test.comment,
			)
			if err != nil {
				t.Fatal(err)
			}
			if owned != test.owned {
				t.Fatalf("owned = %+v, want %+v", owned, test.owned)
			}
		})
	}
}

func TestPendingCICommandReportsLabelOwnershipReadFailure(t *testing.T) {
	t.Parallel()
	readErr := errors.New("database unavailable")
	command := pendingCICommand{
		store:        pendingCICommandStoreStub{getErr: readErr},
		repositoryID: "repository:7",
	}
	_, err := command.armedArtifactOwnership(
		t.Context(), 198, "smyklot:pending:ci:squash", 101,
	)
	if !errors.Is(err, readErr) {
		t.Fatalf("ownership error = %v, want database error", err)
	}
}

type pendingCICommandStoreStub struct {
	request      pendingci.Request
	getErr       error
	armErr       error
	finishResult *pendingci.Request
	finishErr    error
}

func (store pendingCICommandStoreStub) GetArmed(
	context.Context,
	string,
	int,
) (pendingci.Request, error) {
	return store.request, store.getErr
}

func (store pendingCICommandStoreStub) Arm(
	context.Context,
	pendingci.ArmRequest,
) (pendingci.ArmResult, error) {
	return pendingci.ArmResult{}, store.armErr
}

func (pendingCICommandStoreStub) CancelBySource(
	context.Context,
	pendingci.CancelRequest,
) (*pendingci.Request, error) {
	return nil, nil
}

func (store pendingCICommandStoreStub) FinishPR(
	context.Context,
	pendingci.FinishPRRequest,
) (*pendingci.Request, error) {
	return store.finishResult, store.finishErr
}
