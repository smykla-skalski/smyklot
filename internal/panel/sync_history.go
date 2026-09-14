package panel

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

const (
	historyCursorParameter = "cursor"
	historyLimitParameter  = "limit"
)

type syncHistoryCursor struct {
	ComputedAt time.Time `json:"computed_at"`
	ID         string    `json:"id"`
}

type syncPlanSummaryDTO struct {
	ID         string            `json:"id"`
	Trigger    orgsync.Trigger   `json:"trigger"`
	State      orgsync.PlanState `json:"state"`
	Counts     syncCountsDTO     `json:"counts"`
	ComputedAt time.Time         `json:"computed_at"`
	FinishedAt *time.Time        `json:"finished_at,omitempty"`
}

func (s *Server) getSyncHistory(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	request, err := parseSyncHistoryPage(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_history_query", err.Error())
		return
	}
	page, err := s.store.ListSyncPlans(r.Context(), target.ID, request)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	result := pageResponse[syncPlanSummaryDTO]{Items: make([]syncPlanSummaryDTO, 0, len(page.Items)), Total: page.Total}
	for _, plan := range page.Items {
		result.Items = append(result.Items, syncPlanSummary(plan))
	}
	if page.Next != nil {
		encoded, err := json.Marshal(syncHistoryCursor{ComputedAt: page.Next.ComputedAt, ID: page.Next.ID})
		if err != nil {
			s.writeInternal(w, err)
			return
		}
		cursor := base64.RawURLEncoding.EncodeToString(encoded)
		result.NextCursor = &cursor
	}
	writeJSON(w, http.StatusOK, result)
}

func parseSyncHistoryPage(values url.Values) (orgsync.PlanPageRequest, error) {
	page := orgsync.PlanPageRequest{Limit: DefaultPageSize}
	for key, entries := range values {
		if (key != historyLimitParameter && key != historyCursorParameter) || len(entries) != 1 {
			return page, fmt.Errorf("unsupported sync history query")
		}
	}
	if raw := values.Get(historyLimitParameter); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 || limit > MaxPageSize {
			return page, fmt.Errorf("invalid history page size")
		}
		page.Limit = limit
	}
	if raw := values.Get(historyCursorParameter); raw != "" {
		if len(raw) > 1024 {
			return page, fmt.Errorf("invalid history cursor")
		}
		encoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			return page, fmt.Errorf("invalid history cursor")
		}
		var cursor syncHistoryCursor
		if err := json.Unmarshal(encoded, &cursor); err != nil || cursor.ComputedAt.IsZero() || strings.TrimSpace(cursor.ID) == "" {
			return page, fmt.Errorf("invalid history cursor")
		}
		page.Before = &orgsync.PlanCursor{ComputedAt: cursor.ComputedAt, ID: cursor.ID}
	}
	return page, nil
}

func syncPlanSummary(plan orgsync.Plan) syncPlanSummaryDTO {
	return syncPlanSummaryDTO{ID: plan.ID, Trigger: plan.Trigger, State: plan.State, Counts: syncCountsDTO{Create: plan.Counts.Create, Update: plan.Counts.Update, Delete: plan.Counts.Delete}, ComputedAt: plan.ComputedAt, FinishedAt: plan.FinishedAt}
}
