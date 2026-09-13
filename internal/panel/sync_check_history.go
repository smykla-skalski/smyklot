package panel

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

type syncCheckCursor struct {
	CheckID string `json:"check_id"`
	After   int    `json:"after"`
}

func (s *Server) getSyncCheckObservations(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	checkID := r.PathValue("check")
	limit, after, err := parseSyncCheckPage(r.URL.Query(), checkID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_check_query", err.Error())
		return
	}
	page, err := s.store.ListSyncCheckObservations(r.Context(), target.ID, checkID, after, limit)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	result := pageResponse[orgsync.CheckObservation]{Items: page.Items, Total: page.Total}
	if page.Next != nil {
		encoded, err := json.Marshal(syncCheckCursor{CheckID: checkID, After: *page.Next})
		if err != nil {
			s.writeInternal(w, err)
			return
		}
		cursor := base64.RawURLEncoding.EncodeToString(encoded)
		result.NextCursor = &cursor
	}
	writeJSON(w, http.StatusOK, result)
}

func parseSyncCheckPage(values url.Values, checkID string) (int, int, error) {
	limit := DefaultPageSize
	for key, entries := range values {
		if (key != "limit" && key != "cursor") || len(entries) != 1 {
			return 0, 0, fmt.Errorf("unsupported check evidence query")
		}
	}
	if raw := values.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > MaxPageSize {
			return 0, 0, fmt.Errorf("invalid check evidence page size")
		}
		limit = parsed
	}
	raw := values.Get("cursor")
	if raw == "" {
		return limit, 0, nil
	}
	if len(raw) > 1024 {
		return 0, 0, fmt.Errorf("invalid check evidence cursor")
	}
	encoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid check evidence cursor")
	}
	var cursor syncCheckCursor
	if err := json.Unmarshal(encoded, &cursor); err != nil || cursor.CheckID != checkID || cursor.After <= 0 {
		return 0, 0, fmt.Errorf("invalid check evidence cursor")
	}
	return limit, cursor.After, nil
}
