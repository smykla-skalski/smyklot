package panel

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type syncRequestHistoryItem struct {
	Action           string    `json:"action"`
	RequestKey       string    `json:"request_key"`
	CheckID          string    `json:"check_id,omitempty"`
	QueueID          string    `json:"queue_id,omitempty"`
	PlanID           string    `json:"plan_id,omitempty"`
	ExpectedRevision int64     `json:"expected_revision,omitempty"`
	Reason           string    `json:"reason"`
	AcceptedAt       time.Time `json:"accepted_at"`
}

type syncRequestHistoryResponse struct {
	Items      []syncRequestHistoryItem `json:"items"`
	NextCursor *string                  `json:"next_cursor"`
}

type syncRequestCursor struct {
	Version    int       `json:"version"`
	ActorID    string    `json:"actor_id"`
	TargetID   string    `json:"target_id"`
	AcceptedAt time.Time `json:"accepted_at"`
	Action     string    `json:"action"`
	RequestKey string    `json:"request_key"`
}

func (s *Server) getSyncRequests(w http.ResponseWriter, r *http.Request) {
	account, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	query, err := parseSyncRequestHistory(r.URL.Query(), account.ID, target.ID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_history_query", err.Error())
		return
	}
	query.SessionTokenHash = syncRequestSessionHash(r)
	page, err := s.store.ListSyncRequests(r.Context(), query, s.now)
	if err != nil {
		if errors.Is(err, storage.ErrRevoked) || errors.Is(err, storage.ErrExpired) {
			s.writeError(w, http.StatusForbidden, "access_revoked", "Your access changed; refresh the workspace to read your accepted requests")
		} else {
			s.writeStorageError(w, err)
		}
		return
	}
	result := syncRequestHistoryResponse{Items: make([]syncRequestHistoryItem, 0, len(page.Items))}
	for _, item := range page.Items {
		dto := syncRequestHistoryItem{Action: item.Action, RequestKey: item.RequestKey, Reason: item.Reason, AcceptedAt: item.AcceptedAt}
		switch item.Action {
		case orgsync.RequestActionCheck:
			dto.CheckID = item.QueueID
		case orgsync.RequestActionDispatch:
			dto.QueueID, dto.PlanID, dto.ExpectedRevision = item.QueueID, item.PlanID, item.ExpectedRevision
		default:
			s.writeInternal(w, fmt.Errorf("unknown accepted request action %q", item.Action))
			return
		}
		result.Items = append(result.Items, dto)
	}
	if after := page.Next; after != nil {
		cursor := syncRequestCursor{Version: 1, ActorID: after.ActorID, TargetID: after.TargetID, AcceptedAt: after.AcceptedAt, Action: after.Action, RequestKey: after.RequestKey}
		encoded, err := json.Marshal(cursor)
		if err != nil {
			s.writeInternal(w, err)
			return
		}
		token := base64.RawURLEncoding.EncodeToString(encoded)
		result.NextCursor = &token
	}
	writeJSON(w, http.StatusOK, result)
}

func parseSyncRequestHistory(values url.Values, actorID, targetID string) (orgsync.RequestHistoryQuery, error) {
	query := orgsync.RequestHistoryQuery{ActorID: actorID, TargetID: targetID, Limit: DefaultPageSize}
	for key, entries := range values {
		if (key != historyLimitParameter && key != historyCursorParameter) || len(entries) != 1 {
			return query, fmt.Errorf("unsupported request history query")
		}
	}
	if raw := values.Get(historyLimitParameter); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 || limit > MaxPageSize {
			return query, fmt.Errorf("invalid request history page size")
		}
		query.Limit = limit
	}
	raw := values.Get(historyCursorParameter)
	if raw == "" {
		return query, nil
	}
	cursor, err := decodeSyncRequestCursor(raw, actorID, targetID)
	if err != nil {
		return query, err
	}
	query.After = &orgsync.RequestHistoryPosition{ActorID: actorID, TargetID: targetID, AcceptedAt: cursor.AcceptedAt, Action: cursor.Action, RequestKey: cursor.RequestKey}
	return query, nil
}

func decodeSyncRequestCursor(raw, actorID, targetID string) (syncRequestCursor, error) {
	var cursor syncRequestCursor
	invalid := fmt.Errorf("invalid request history cursor")
	if len(raw) > 4096 || strings.ContainsAny(raw, "\r\n") {
		return cursor, invalid
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil {
		return cursor, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return cursor, invalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return cursor, invalid
	}
	if cursor.Version != 1 || cursor.ActorID != actorID || cursor.TargetID != targetID || cursor.AcceptedAt.IsZero() || cursor.RequestKey == "" || len(cursor.RequestKey) > 200 || strings.TrimSpace(cursor.RequestKey) != cursor.RequestKey || (cursor.Action != orgsync.RequestActionCheck && cursor.Action != orgsync.RequestActionDispatch) {
		return cursor, invalid
	}
	return cursor, nil
}
