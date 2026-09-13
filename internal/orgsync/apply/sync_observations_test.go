package apply

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestSyncObservationReplacesStaleAgreement(t *testing.T) {
	now := time.Now().UTC()
	repository := storage.Repository{ID: "repo", Available: true}
	scope := newSyncScope(orgsync.Config{Kind: orgsync.KindFiles, Digest: "config"},
		nil, nil, now, config.DefaultFormattingPolicy(), config.Patch{})
	for _, test := range []struct {
		name        string
		answer      repositoryAnswer
		err         error
		wantDigest  bool
		wantProblem bool
	}{
		{name: "matching", answer: compared(nil), wantDigest: true},
		{name: "proposed", answer: repositoryAnswer{observation: orgsync.ObservationProposed}, wantDigest: true},
		{name: "declined", answer: repositoryAnswer{observation: orgsync.ObservationDeclined}, wantDigest: true},
		{name: "new drift", answer: compared([]orgsync.Action{{Subject: "README.md"}})},
		{name: "read failed", err: errors.New("transport failed"), wantProblem: true},
		{name: "refused", answer: repositoryAnswer{problem: "cannot compose"}, wantProblem: true},
		{name: "no evidence", answer: repositoryAnswer{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			scope.applied[repository.ID] = orgsync.RepositoryState{
				RepositoryID: repository.ID, Kind: orgsync.KindFiles,
				AppliedDigest: scope.digestFor(repository), AppliedAt: now.Add(-RecheckInterval),
				Observation: orgsync.ObservationMatched,
			}
			actions, states := scope.ask(t.Context(), func(context.Context, storage.Repository) (repositoryAnswer, error) {
				return test.answer, test.err
			}, repository)
			if len(states) != 1 {
				t.Fatalf("states = %#v", states)
			}
			state := states[0]
			if (state.AppliedDigest != "") != test.wantDigest || (state.Problem != "") != test.wantProblem {
				t.Fatalf("state = %#v", state)
			}
			if state.Observation != test.answer.observation || !state.AppliedAt.Equal(now) {
				t.Fatalf("evidence = %#v", state)
			}
			if len(actions) != len(test.answer.actions) {
				t.Fatalf("actions = %#v", actions)
			}
			scope.applied[repository.ID] = state
			if scope.covers(repository) == test.wantDigest {
				t.Fatalf("cache reuse disagrees with evidence: %#v", state)
			}
		})
	}
}

func TestCarriedSyncActionsDoNotInventFreshObservations(t *testing.T) {
	engine := New(&syncExecutionStore{}, nil, "")
	for _, kind := range orgsync.Kinds() {
		t.Run(string(kind), func(t *testing.T) {
			work := orgsync.KindWork{Kind: kind, Actions: []orgsync.Action{
				{ID: 1, Kind: kind, State: orgsync.ActionApplied},
			}}
			var outcome orgsync.Outcome
			observation, succeeded := engine.applyKind(t.Context(), nil, syncTarget{}, work, &outcome)
			if !succeeded || observation != "" {
				t.Fatalf("carried result = %q, %t", observation, succeeded)
			}
			work.Actions[0].State = orgsync.ActionFailed
			observation, succeeded = engine.applyKind(t.Context(), nil, syncTarget{}, work, &outcome)
			if succeeded || observation != "" {
				t.Fatalf("failed result = %q, %t", observation, succeeded)
			}
		})
	}
}
