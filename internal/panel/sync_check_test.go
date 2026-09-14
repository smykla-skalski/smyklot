package panel

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func readPanelCheck(t *testing.T, h *panelHarness, session *http.Cookie, id string) syncCheckResponse {
	t.Helper()
	path := "/panel/api/v1/targets/" + panelSyncTarget + "/sync/checks/" + url.PathEscape(id)
	response := h.request(t, http.MethodGet, path, nil, session)
	requireResponse(t, response, "retained check", http.StatusOK)
	var body syncCheckResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.CheckID != id || body.TargetID != panelSyncTarget || body.ObservedAt.IsZero() || body.Check.Action != "check" || body.Check.TargetID != panelSyncTarget {
		t.Fatalf("lost identity or current capability: %#v", body)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("check response is cacheable: %v", response.Header())
	}
	var wire any
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	assertWireNames(t, path, wire)
	return body
}

func TestSyncCheckReadPreservesComparisonAfterWorkerPruning(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	id := createPanelCheckEvidence(t, h, panelSyncTarget, 2, true)
	before := readPanelCheck(t, h, session, id)
	if before.Result == nil || before.Result.Outcome == nil || before.Execution == nil || before.Execution.State != workqueue.StateRunning {
		t.Fatalf("comparison was conflated with worker completion: %#v", before)
	}
	_, err := h.store.FinishRecurringWork(t.Context(), id, workqueue.RecurringCompletion{Attempt: before.Execution.Attempt, Failure: "Worker completion failed", Retryable: false}, h.now)
	if err != nil {
		t.Fatal(err)
	}
	failed := readPanelCheck(t, h, session, id)
	if failed.Execution == nil || failed.Execution.State != workqueue.StateFailed || !reflect.DeepEqual(failed.Result, before.Result) {
		t.Fatalf("worker failure rewrote comparison: %#v", failed)
	}
	if _, err := h.store.PruneWorkQueue(t.Context(), h.now.AddDate(2, 0, 0)); err != nil {
		t.Fatal(err)
	}
	after := readPanelCheck(t, h, session, id)
	if after.Execution != nil || !reflect.DeepEqual(after.Result, before.Result) {
		t.Fatalf("pruning lost result or invented execution: %#v", after)
	}
	createPanelSyncPlan(t, h, "newer-blocker", h.now.Add(time.Hour))
	current := readPanelCheck(t, h, session, id)
	if !reflect.DeepEqual(current.Result, before.Result) || current.Check.BlockingPlanID != "newer-blocker" || current.Check.Available {
		t.Fatalf("historical result replaced current workspace eligibility: %#v", current)
	}
	path := "/panel/api/v1/targets/" + panelSyncTarget + "/sync/checks/" + url.PathEscape(id) + "/observations"
	requireResponse(t, h.request(t, http.MethodGet, path, nil, session), "retained evidence", http.StatusOK)
}

func TestSyncCheckReadSeparatesUnrecordedAndEmptyComparison(t *testing.T) {
	for _, recorded := range []bool{false, true} {
		t.Run(map[bool]string{false: "unrecorded", true: "empty"}[recorded], func(t *testing.T) {
			h := newPanelHarness(t, "owner")
			session := h.signIn(t)
			id := createPanelCheckEvidence(t, h, panelSyncTarget, 0, recorded)
			body := readPanelCheck(t, h, session, id)
			if (body.Result != nil) != recorded || body.Execution == nil {
				t.Fatalf("missing and empty comparison conflated: %#v", body)
			}
		})
	}
}

func TestSyncCheckReadCannotCrossWorkspaceScope(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	_, workspace := seedNonOwnedWorkspace(t, h)
	id := createPanelCheckEvidence(t, h, workspace.TargetID, 1, true)
	for _, target := range []string{panelSyncTarget, workspace.TargetID} {
		path := "/panel/api/v1/targets/" + target + "/sync/checks/" + url.PathEscape(id)
		requireResponse(t, h.request(t, http.MethodGet, path, nil, session), "foreign check", http.StatusNotFound)
	}
	path := "/panel/api/v1/targets/" + panelSyncTarget + "/sync/checks/missing"
	requireResponse(t, h.request(t, http.MethodGet, path, nil, session), "missing check", http.StatusNotFound)
}
