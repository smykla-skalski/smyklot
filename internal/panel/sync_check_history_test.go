package panel

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func createPanelCheckEvidence(t *testing.T, h *panelHarness, target string, count int, recorded bool) string {
	t.Helper()
	item, claimed, err := h.store.ClaimRecurringWork(t.Context(), workqueue.RecurringClaim{Kind: workqueue.KindSyncScan, TargetID: &target, Title: "Check repositories", Now: h.now, LeaseDuration: time.Minute})
	if err != nil || !claimed {
		t.Fatalf("claim: %#v %v", item, err)
	}
	if !recorded {
		return item.ID
	}
	result := orgsync.CheckResult{Outcome: orgsync.CheckOutcome{CompletedAt: h.now, Disposition: "checked", Summary: "Checked repositories", Counts: map[orgsync.Observation]int{orgsync.ObservationMatched: count}}}
	for index := range count {
		result.Observations = append(result.Observations, orgsync.CheckObservation{RepositoryID: fmt.Sprintf("repo-%d", index), Repository: fmt.Sprintf("owner/historical-%d", index), Kind: orgsync.KindLabels, Outcome: orgsync.ObservationMatched, ObservedAt: h.now, InputDigest: "original-input"})
	}
	if err := h.store.RecordSyncCheckResult(t.Context(), orgsync.CheckResultCreate{Check: orgsync.CheckReference{QueueID: item.ID, Attempt: item.Attempt}, TargetID: target, Result: result, Now: h.now}); err != nil {
		t.Fatal(err)
	}
	return item.ID
}

func TestSyncCheckEvidencePagesOriginalResults(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	id := createPanelCheckEvidence(t, h, panelSyncTarget, 3, true)
	path := "/panel/api/v1/targets/" + panelSyncTarget + "/sync/checks/" + url.PathEscape(id) + "/observations"
	response := h.request(t, http.MethodGet, path+"?limit=2", nil, session)
	requireResponse(t, response, "check evidence", http.StatusOK)
	var first pageResponse[orgsync.CheckObservation]
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Total != 3 || len(first.Items) != 2 || first.Items[0].Repository != "owner/historical-0" || first.NextCursor == nil {
		t.Fatalf("first page: %#v", first)
	}
	var wire any
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	assertWireNames(t, path, wire)
	response = h.request(t, http.MethodGet, path+"?limit=2&cursor="+url.QueryEscape(*first.NextCursor), nil, session)
	requireResponse(t, response, "next evidence", http.StatusOK)
	var second pageResponse[orgsync.CheckObservation]
	if err := json.Unmarshal(response.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].Repository != "owner/historical-2" || second.Items[0].InputDigest != "original-input" || second.NextCursor != nil {
		t.Fatalf("last page: %#v", second)
	}
	wrongCheck := base64.RawURLEncoding.EncodeToString([]byte(`{"check_id":"other","after":2}`))
	for _, query := range []string{"limit=0", "limit=-1", "limit=101", "limit=2&limit=3", "sort=name", "cursor=broken", "cursor=e30", "cursor=bnVsbA", "cursor=" + wrongCheck, "cursor=a&cursor=b"} {
		requireResponse(t, h.request(t, http.MethodGet, path+"?"+query, nil, session), query, http.StatusBadRequest)
	}
}

func TestSyncCheckEvidenceSeparatesMissingAndEmpty(t *testing.T) {
	for _, recorded := range []bool{false, true} {
		t.Run(fmt.Sprint(recorded), func(t *testing.T) {
			h := newPanelHarness(t, "owner")
			session := h.signIn(t)
			id := createPanelCheckEvidence(t, h, panelSyncTarget, 0, recorded)
			path := "/panel/api/v1/targets/" + panelSyncTarget + "/sync/checks/" + url.PathEscape(id) + "/observations"
			response := h.request(t, http.MethodGet, path, nil, session)
			if !recorded {
				requireResponse(t, response, "unrecorded check", http.StatusNotFound)
				return
			}
			requireResponse(t, response, "empty recorded result", http.StatusOK)
			var page pageResponse[orgsync.CheckObservation]
			if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if page.Items == nil || len(page.Items) != 0 || page.Total != 0 || page.NextCursor != nil {
				t.Fatalf("empty page: %#v", page)
			}
		})
	}
}

func TestSyncCheckEvidenceCannotCrossWorkspaceScope(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	_, workspace := seedNonOwnedWorkspace(t, h)
	id := createPanelCheckEvidence(t, h, workspace.TargetID, 1, true)
	for _, target := range []string{panelSyncTarget, workspace.TargetID} {
		path := "/panel/api/v1/targets/" + target + "/sync/checks/" + url.PathEscape(id) + "/observations"
		requireResponse(t, h.request(t, http.MethodGet, path, nil, session), "foreign evidence", http.StatusNotFound)
	}
}
