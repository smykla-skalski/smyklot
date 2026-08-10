package panel

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	panelEventVersion        = 1
	panelEventQueueSize      = 16
	panelEventWriteTimeout   = 5 * time.Second
	panelEventHeartbeat      = 25 * time.Second
	panelSessionRevokedCode  = websocket.StatusCode(4001)
	panelEventSessionRevoked = "session.revoked"
	panelEventAccessChanged  = "access.changed"
	panelEventResync         = "resync"
)

type panelEvent struct {
	Version      int    `json:"version"`
	Type         string `json:"type"`
	TargetID     string `json:"target_id,omitempty"`
	RepositoryID string `json:"repository_id,omitempty"`
	Code         string `json:"code,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type eventSubscriber struct {
	sessionHash  string
	events       chan panelEvent
	terminal     chan panelEvent
	overflow     chan struct{}
	overflowOnce sync.Once
}

func (s *eventSubscriber) disconnectSlowConsumer() {
	s.overflowOnce.Do(func() { close(s.overflow) })
}

type eventHub struct {
	mu          sync.Mutex
	subscribers map[*eventSubscriber]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{subscribers: make(map[*eventSubscriber]struct{})}
}

func (h *eventHub) subscribe(sessionHash string) (*eventSubscriber, func()) {
	subscriber := &eventSubscriber{
		sessionHash: sessionHash,
		events:      make(chan panelEvent, panelEventQueueSize),
		terminal:    make(chan panelEvent, 1),
		overflow:    make(chan struct{}),
	}
	h.mu.Lock()
	h.subscribers[subscriber] = struct{}{}
	h.mu.Unlock()

	return subscriber, func() {
		h.mu.Lock()
		delete(h.subscribers, subscriber)
		h.mu.Unlock()
	}
}

func (h *eventHub) announce(event panelEvent) {
	event.Version = panelEventVersion
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscriber := range h.subscribers {
		select {
		case subscriber.events <- event:
		default:
			delete(h.subscribers, subscriber)
			subscriber.disconnectSlowConsumer()
		}
	}
}

func (h *eventHub) revokeSession(sessionHash, code, reason string) {
	h.revokeWhere(func(subscriber *eventSubscriber) bool {
		return subscriber.sessionHash == sessionHash
	}, code, reason)
}

func (h *eventHub) revokeWhere(
	matches func(*eventSubscriber) bool,
	code, reason string,
) {
	event := panelEvent{
		Version: panelEventVersion,
		Type:    panelEventSessionRevoked,
		Code:    code,
		Reason:  reason,
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscriber := range h.subscribers {
		if !matches(subscriber) {
			continue
		}
		delete(h.subscribers, subscriber)
		subscriber.terminal <- event
	}
}

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	_, sessionHash, ok := s.eventViewer(w, r)
	if !ok {
		return
	}
	if r.Header.Get("Origin") != s.cfg.PublicOrigin {
		s.writeError(w, http.StatusForbidden, "forbidden", "request origin is not allowed")
		return
	}

	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // The exact configured origin was checked above.
		CompressionMode:    websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	defer func() { _ = connection.CloseNow() }()

	connectionContext := connection.CloseRead(context.Background())
	subscriber, unsubscribe := s.events.subscribe(sessionHash)
	defer unsubscribe()
	if err := writePanelEvent(connectionContext, connection, panelEvent{
		Version: panelEventVersion,
		Type:    "ready",
	}); err != nil {
		return
	}
	s.servePanelEvents(connectionContext, connection, subscriber, sessionHash)
}

func (s *Server) eventViewer(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Account, string, bool) {
	account, sessionHash, err := s.viewer(r)
	if err == nil {
		return account, sessionHash, true
	}
	if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrExpired) {
		s.writeError(w, http.StatusUnauthorized, "unauthenticated", "sign in to use the panel")
	} else if errors.Is(err, storage.ErrRevoked) {
		s.writeError(w, http.StatusUnauthorized, "session_revoked", err.Error())
	} else {
		s.writeInternal(w, err)
	}

	return storage.Account{}, "", false
}

func (s *Server) servePanelEvents(
	ctx context.Context,
	connection *websocket.Conn,
	subscriber *eventSubscriber,
	sessionHash string,
) {
	heartbeat := time.NewTicker(panelEventHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-subscriber.events:
			if err := writePanelEvent(ctx, connection, event); err != nil {
				return
			}
		case event := <-subscriber.terminal:
			if err := writePanelEvent(ctx, connection, event); err != nil {
				return
			}
			_ = connection.Close(panelSessionRevokedCode, "session revoked")
			return
		case <-subscriber.overflow:
			_ = connection.Close(websocket.StatusTryAgainLater, "event queue overflow")
			return
		case <-heartbeat.C:
			if !s.heartbeatPanelEvents(ctx, connection, sessionHash) {
				return
			}
		}
	}
}

func (s *Server) heartbeatPanelEvents(
	ctx context.Context,
	connection *websocket.Conn,
	sessionHash string,
) bool {
	if _, err := s.store.GetSession(ctx, sessionHash, s.now()); err != nil {
		revoked := panelEvent{
			Version: panelEventVersion,
			Type:    panelEventSessionRevoked,
			Code:    "session_expired",
			Reason:  "Your session expired",
		}
		_ = writePanelEvent(ctx, connection, revoked)
		_ = connection.Close(panelSessionRevokedCode, "session revoked")

		return false
	}
	pingContext, cancel := context.WithTimeout(ctx, panelEventWriteTimeout)
	err := connection.Ping(pingContext)
	cancel()

	return err == nil
}

func writePanelEvent(ctx context.Context, connection *websocket.Conn, event panelEvent) error {
	writeContext, cancel := context.WithTimeout(ctx, panelEventWriteTimeout)
	defer cancel()

	return wsjson.Write(writeContext, connection, event)
}
