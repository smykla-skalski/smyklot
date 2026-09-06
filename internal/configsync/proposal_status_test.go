package configsync

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func mergedProposalCalls() []remoteCall {
	return append([]remoteCall{{method: "GET", path: "/repos/acme/web/pulls", answer: `[]`}}, settlementSourceCalls()...)
}

func settlementSourceCalls() []remoteCall {
	return []remoteCall{repositoryIdentityCall(), {method: "GET", path: "/repos/acme/web/git/ref/heads/main", answer: `{"object":{"sha":"` + remoteHead + `"}}`}}
}

func revertedProposalEngine(t *testing.T) (Engine, []remoteCall) {
	t.Helper()
	engine := Engine{Store: engineStore(t)}
	snapshot, err := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
	if err != nil {
		t.Fatal(err)
	}
	semantic, _ := snapshot.JSON()
	document, _ := snapshot.Document()
	content, err := config.RenderFileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	proposal := readyProposal()
	proposal.Number, proposal.URL = 42, "https://github.com/acme/web/pull/42"
	storeConnection(t, engine, "github:repository:11", Connection{
		Version: 1, Status: StatusProposed, Base: Snapshot{Exists: true, Document: semantic}, Proposal: &proposal,
	})
	return engine, engineFileCalls(".smyklot.toml", string(content))
}

func openProposalCall(number int) remoteCall {
	return remoteCall{method: "GET", path: "/repos/acme/web/pulls", answer: fmt.Sprintf(`[{"number":%d,"state":"open","html_url":"https://github.com/acme/web/pull/%d","base":{"ref":"release"}}]`, number, number), check: func(t *testing.T, request *http.Request) {
		query := request.URL.Query()
		if query.Get("state") != "open" || query.Get("head") != "acme:smyklot/repository-configuration" || query.Has("base") {
			t.Errorf("outstanding proposal lookup must cover every base and only open PRs: %s", request.URL)
		}
	}}
}

func TestPanelRevertKeepsOutstandingProposalVisible(t *testing.T) {
	for _, numberKnown := range []bool{true, false} {
		t.Run(fmt.Sprint(numberKnown), func(t *testing.T) {
			engine, calls := revertedProposalEngine(t)
			if !numberKnown {
				stored, _ := engine.Store.GetConfigFileState(t.Context(), "workspace", "github:repository:11")
				connection, _ := DecodeConnection(stored, config.PanelFileRepository)
				connection.Proposal.Number, connection.Proposal.URL = 0, ""
				storeConnection(t, engine, "github:repository:11", connection)
			}
			calls = append(calls, openProposalCall(42))
			calls = append(calls, settlementSourceCalls()...)
			connection, err := engine.Run(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
			if err != nil || connection.Status != StatusBlocked || connection.Problem != "proposal_outstanding" {
				t.Fatalf("revert concealed outstanding proposal: %+v (%v)", connection, err)
			}
			status, err := engine.ReadStatus(t.Context(), "workspace", "github:repository:11")
			if err != nil || status.LastCheck.Proposal == nil || status.LastCheck.Proposal.Number != 42 || status.LastCheck.Proposal.URL != "https://github.com/acme/web/pull/42" {
				t.Fatalf("outstanding proposal link disappeared: %+v (%v)", status, err)
			}
			snapshot, _ := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
			if snapshot.Repository.Revision != 2 || snapshot.Repository.ConfigPatch.CommandPrefix != nil {
				t.Fatal("observing the old proposal changed panel settings")
			}
		})
	}
}

func TestClosingOneProposalCannotHideAnotherFromTheSameBranch(t *testing.T) {
	engine, read := revertedProposalEngine(t)
	for _, number := range []int{43, 42, 0} {
		calls := append([]remoteCall(nil), read...)
		if number > 0 {
			calls = append(calls, openProposalCall(number))
			calls = append(calls, settlementSourceCalls()...)
		} else {
			calls = append(calls, mergedProposalCalls()...)
		}
		connection, err := engine.Run(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
		if err != nil {
			t.Fatal(err)
		}
		if number > 0 {
			if connection.Status != StatusBlocked || connection.Proposal.Number != number {
				t.Fatalf("remaining PR was hidden: %+v", connection)
			}
		} else if connection.Status != StatusReady || connection.Problem != "" {
			t.Fatalf("closed proposals prevented convergence: %+v", connection)
		}
	}
}

func TestProposalObservationCannotCrossAConcurrentSettingsSave(t *testing.T) {
	engine, calls := revertedProposalEngine(t)
	snapshot, _ := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
	call := openProposalCall(42)
	call.check = func(t *testing.T, _ *http.Request) { saveStatusSettingsChange(t, engine, snapshot, true) }
	calls = append(calls, call)
	calls = append(calls, settlementSourceCalls()...)
	_, err := engine.Run(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("concurrent save did not reject stale observation: %v", err)
	}
}

func TestProposalMergeDuringObservationRequiresFreshRead(t *testing.T) {
	engine, calls := revertedProposalEngine(t)
	calls = append(calls, mergedProposalCalls()...)
	calls[len(calls)-1].answer = `{"object":{"sha":"` + strings.Repeat("b", 40) + `"}}`
	_, again, err := engine.runOnce(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if err != nil || !again {
		t.Fatalf("concurrent merge accepted an old file observation: again=%t (%v)", again, err)
	}
	stored, _ := engine.Store.GetConfigFileState(t.Context(), "workspace", "github:repository:11")
	connection, _ := DecodeConnection(stored, config.PanelFileRepository)
	if connection.Status != StatusProposed {
		t.Fatalf("stale observation replaced the proposal state: %+v", connection)
	}
}

func TestFreshPreviewKeepsOutstandingProposalVisibleWithoutWriting(t *testing.T) {
	engine, calls := revertedProposalEngine(t)
	before, _ := engine.Store.GetConfigFileState(t.Context(), "workspace", "github:repository:11")
	calls = append(calls, openProposalCall(42))
	calls = append(calls, settlementSourceCalls()...)
	calls = append(calls, repositoryIdentityCall())
	preview, err := engine.Preview(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if err != nil || preview.Status != StatusBlocked || preview.Problem != "proposal_outstanding" ||
		preview.Proposal == nil || preview.Proposal.Number != 42 || preview.ReviewToken != "" || len(preview.Choices) != 0 {
		t.Fatalf("fresh preview concealed the outstanding proposal: %+v (%v)", preview, err)
	}
	after, _ := engine.Store.GetConfigFileState(t.Context(), "workspace", "github:repository:11")
	if after.Revision != before.Revision || string(after.Document) != string(before.Document) {
		t.Fatal("preview persisted proposal state")
	}
}

func TestFreshProposalPreviewRejectsSettingsChangedDuringLookup(t *testing.T) {
	engine, calls := revertedProposalEngine(t)
	snapshot, _ := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
	call := openProposalCall(42)
	call.check = func(t *testing.T, _ *http.Request) { saveStatusSettingsChange(t, engine, snapshot, true) }
	calls = append(calls, call)
	calls = append(calls, settlementSourceCalls()...)
	_, err := engine.Preview(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("preview accepted a stale proposal observation: %v", err)
	}
}

func TestOutstandingProposalStillAdvancesTheCommonBaseline(t *testing.T) {
	engine, _ := revertedProposalEngine(t)
	initial, _ := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
	saveStatusSettingsChange(t, engine, initial, false)
	agreed, _ := engine.Snapshot(t.Context(), "workspace", "github:repository:11")
	document, _ := agreed.Document()
	content, _ := config.RenderFileDocument(document)
	calls := append(engineFileCalls(".smyklot.toml", string(content)), openProposalCall(42))
	calls = append(calls, settlementSourceCalls()...)
	connection, err := engine.Run(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if err != nil || connection.Status != StatusBlocked {
		t.Fatalf("common settings with open proposal = %+v (%v)", connection, err)
	}
	// Restore the original panel value after both sides were observed at /new.
	// The file is unchanged, so this is a new panel edit to publish, not import.
	_, err = engine.Store.SaveInstallationSettings(t.Context(), storage.SaveInstallationSettingsRequest{
		TargetID: "workspace", ActorAccountID: "owner", ChangedAt: time.Now().UTC(),
		Repositories: []storage.InstallationRepositorySettingsChange{{
			RepositoryID: "github:repository:11", ExpectedRevision: agreed.OwnerRevision(), ConfigFileSyncEnabled: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls = append(engineFileCalls(".smyklot.toml", string(content)), publishAfterRetargetCalls()...)
	connection, err = engine.Run(t.Context(), scriptedRemote(t, calls...), "workspace", "github:repository:11")
	if err != nil || connection.Status != StatusProposed {
		t.Fatalf("new panel edit did not produce a proposal: %+v (%v)", connection, err)
	}
	repository, _ := engine.Store.GetRepository(t.Context(), "workspace", "github:repository:11")
	if repository.Revision != agreed.OwnerRevision()+1 || repository.ConfigPatch.CommandPrefix != nil {
		t.Fatalf("the unchanged file overwrote the new panel edit: %+v", repository)
	}
}

func publishAfterRetargetCalls() []remoteCall {
	oldHead, nextHead := strings.Repeat("e", 40), strings.Repeat("f", 40)
	calls := prepareEngineCalls()
	calls[0].status, calls[0].answer = 200, `{"object":{"sha":"`+oldHead+`"}}`
	calls[len(calls)-1].answer = `{"sha":"` + nextHead + `"}`
	calls = append(calls, settlementSourceCalls()...)
	return append(calls,
		remoteCall{method: "GET", path: proposalRefPath(), answer: `{"object":{"sha":"` + oldHead + `"}}`},
		remoteCall{method: "GET", path: "/repos/acme/web/pulls", answer: `[]`},
		remoteCall{method: "PATCH", path: "/repos/acme/web/git/refs/heads/smyklot/repository-configuration", answer: `{}`},
		remoteCall{method: "POST", path: "/repos/acme/web/pulls", answer: `{"number":43,"state":"open"}`},
	)
}
