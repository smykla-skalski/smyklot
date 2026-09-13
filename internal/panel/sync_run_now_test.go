package panel

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestSyncCheckNeverDispatchesApprovedChanges(t *testing.T) {
	h := newPanelHarness(t, "owner")
	session := h.signIn(t)
	createPanelSyncPlan(t, h, "approved", h.now.Add(time.Hour))
	if _, err := h.store.ApproveSyncPlan(t.Context(), orgsync.PlanApproval{
		TargetID: panelSyncTarget, PlanID: "approved", Digest: "sha256:approved",
		ActorID: "github:test:user:1", Now: h.now,
	}); err != nil {
		t.Fatal(err)
	}
	before, err := h.store.GetQueueItem(t.Context(), "sync-plan:approved")
	if err != nil {
		t.Fatal(err)
	}
	response := postPanelSyncRunNow(t, h, session, 0)
	requireResponse(t, response, "check beside approved changes", http.StatusOK,
		`"status":"changes_pending"`, `"id":"approved"`)
	after, err := h.store.GetQueueItem(t.Context(), before.ID)
	if err != nil {
		t.Fatal(err)
	}
	// EstimatedStartAt is projected from the read clock, not a persisted mutation.
	before.EstimatedStartAt, after.EstimatedStartAt = nil, nil
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("check mutated approved work: before=%#v after=%#v", before, after)
	}
}

func TestSyncDispatchNeverFallsBackToAnotherPlanOrCheck(t *testing.T) {
	for _, tc := range []struct {
		requested string
		newer     bool
	}{
		{"missing", false}, {"missing", true}, {"expired", false}, {"expired", true},
	} {
		t.Run(fmt.Sprintf("%s/newer=%t", tc.requested, tc.newer), func(t *testing.T) {
			h := newPanelHarness(t, "owner")
			session := h.signIn(t)
			createPanelSyncPlan(t, h, "expired", h.now.Add(time.Minute))
			later := h.now.Add(2 * time.Minute)
			if err := h.store.ExpireSyncPlans(t.Context(), later); err != nil {
				t.Fatal(err)
			}
			*h.clock = later
			if tc.newer {
				createPanelSyncPlan(t, h, "newer", later.Add(time.Hour))
			}
			response := h.request(t, http.MethodPost,
				"/panel/api/v1/targets/"+panelSyncTarget+"/sync/run-now",
				strings.NewReader(fmt.Sprintf(`{"action":"dispatch","plan_id":%q,"expected_revision":1,"reason":"run reviewed changes"}`, tc.requested)), session)
			expected := http.StatusConflict
			if tc.requested == "missing" {
				expected = http.StatusNotFound
			}
			if response.Code != expected {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if tc.newer {
				assertSyncPlanNotDispatched(t, h, "newer")
			}
		})
	}
}

func assertSyncPlanNotDispatched(t *testing.T, h *panelHarness, id string) {
	t.Helper()
	item, err := h.store.GetQueueItem(t.Context(), "sync-plan:"+id)
	if err != nil {
		t.Fatal(err)
	}
	if item.State != workqueue.StateAwaitingApproval || item.Immediate {
		t.Fatalf("changed newer work: %#v", item)
	}
}

func TestSyncRunNowRequiresUnambiguousIntent(t *testing.T) {
	for _, body := range []string{
		`{"reason":"old ambiguous request"}`,
		`{"action":"other","reason":"unknown action"}`,
		`{"action":"check","plan_id":"some-plan","reason":"mixed intent"}`,
		`{"action":"check","expected_revision":2,"reason":"mixed intent"}`,
		`{"action":"dispatch","expected_revision":2,"reason":"missing identity"}`,
		`{"action":"dispatch","plan_id":" p ","expected_revision":2,"reason":"invalid identity"}`,
		`{"action":"dispatch","plan_id":"p","reason":"missing revision"}`,
	} {
		t.Run(body, func(t *testing.T) {
			h := newPanelHarness(t, "owner")
			response := h.request(t, http.MethodPost,
				"/panel/api/v1/targets/"+panelSyncTarget+"/sync/run-now", strings.NewReader(body), h.signIn(t))
			requireResponse(t, response, "ambiguous intent", http.StatusBadRequest, `"code":"invalid_request"`)
		})
	}
}
