package panel

import (
	"context"
	"encoding/json"
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
	panelEventReady          = "ready"
	panelEventSessionRevoked = "session.revoked"
	panelEventAccessChanged  = "access.changed"
	panelEventResync         = "resync"
	panelEventPrefsChanged   = "prefs.changed"
	panelEventPrefsRejected  = "prefs.rejected"

	panelInboundPrefsPatch = "prefs.patch"
	// panelInboundReadLimit bounds one client frame; a full preference patch
	// is far below this.
	panelInboundReadLimit = 8 << 10
	// panelInboundBurst and panelInboundRefillInterval shape the per-connection
	// token bucket: room for a burst of coalesced patches, then two per second
	// sustained — well above what the client's trailing debounce can produce.
	panelInboundBurst          = 20
	panelInboundRefillInterval = 500 * time.Millisecond
	// panelInboundMaxPatchKeys bounds keys in one patch; a coalescing client
	// never sends more than the registry holds.
	panelInboundMaxPatchKeys = 32
)

type panelEvent struct {
	Version      int                        `json:"version"`
	Type         string                     `json:"type"`
	TargetID     string                     `json:"target_id,omitempty"`
	RepositoryID string                     `json:"repository_id,omitempty"`
	Code         string                     `json:"code,omitempty"`
	Reason       string                     `json:"reason,omitempty"`
	Rev          int64                      `json:"rev,omitempty"`
	Changes      map[string]json.RawMessage `json:"changes,omitempty"`
	Keys         []string                   `json:"keys,omitempty"`
	Prefs        *panelPrefsInfo            `json:"prefs,omitempty"`
}

// panelPrefsInfo rides the ready frame: revision and checksum always, the
// canonical document only when the client's handshake did not match. Values
// serializes as "{}" for an empty snapshot, so presence alone signals it.
type panelPrefsInfo struct {
	Rev    int64           `json:"rev"`
	Sum    string          `json:"sum"`
	Values json.RawMessage `json:"values,omitempty"`
}

// panelInboundFrame is the one client-to-server message shape. Unknown types
// are ignored so older servers tolerate newer clients.
type panelInboundFrame struct {
	Version int                        `json:"version"`
	Type    string                     `json:"type"`
	Changes map[string]json.RawMessage `json:"changes"`
}

type eventSubscriber struct {
	accountID    string
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

func (h *eventHub) subscribe(accountID, sessionHash string) (*eventSubscriber, func()) {
	subscriber := &eventSubscriber{
		accountID:   accountID,
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

// announceAccount fans an event out to every connection of one account,
// with the same overflow eviction as announce.
func (h *eventHub) announceAccount(accountID string, event panelEvent) {
	event.Version = panelEventVersion
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscriber := range h.subscribers {
		if subscriber.accountID != accountID {
			continue
		}
		select {
		case subscriber.events <- event:
		default:
			delete(h.subscribers, subscriber)
			subscriber.disconnectSlowConsumer()
		}
	}
}

// deliver queues an event for one subscriber. Routing through the subscriber
// channel keeps the serve loop the only socket writer.
func (h *eventHub) deliver(subscriber *eventSubscriber, event panelEvent) {
	event.Version = panelEventVersion
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, subscribed := h.subscribers[subscriber]; !subscribed {
		return
	}
	select {
	case subscriber.events <- event:
	default:
		delete(h.subscribers, subscriber)
		subscriber.disconnectSlowConsumer()
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
	account, sessionHash, ok := s.eventViewer(w, r)
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
	connection.SetReadLimit(panelInboundReadLimit)

	// Background instead of the request context: the connection is hijacked,
	// and this cancel — shared with the read loop — is what ends both loops.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe before reading preferences: a patch landing in between is
	// then either in the snapshot (its later event is ignored by revision)
	// or queued behind it — never lost.
	subscriber, unsubscribe := s.events.subscribe(account.ID, sessionHash)
	defer unsubscribe()

	prefs, err := s.store.GetPreferences(ctx, account.ID)
	if err != nil {
		return
	}
	if err := writePanelEvent(ctx, connection, panelEvent{
		Version: panelEventVersion,
		Type:    panelEventReady,
		Prefs:   prefsReadyInfo(r.URL.Query(), prefs),
	}); err != nil {
		return
	}

	go s.readPanelFrames(ctx, cancel, connection, account.ID, subscriber)
	s.servePanelEvents(ctx, connection, subscriber, sessionHash)
}

// readPanelFrames owns the inbound half of the stream. Any read error —
// close, oversized frame, malformed JSON — cancels the shared context and
// with it the serve loop.
func (s *Server) readPanelFrames(
	ctx context.Context,
	cancel context.CancelFunc,
	connection *websocket.Conn,
	accountID string,
	subscriber *eventSubscriber,
) {
	defer cancel()
	tokens := panelInboundBurst
	refilled := s.now()
	for {
		var frame panelInboundFrame
		if err := wsjson.Read(ctx, connection, &frame); err != nil {
			return
		}

		now := s.now()
		if refill := int(now.Sub(refilled) / panelInboundRefillInterval); refill > 0 {
			tokens = min(panelInboundBurst, tokens+refill)
			refilled = refilled.Add(time.Duration(refill) * panelInboundRefillInterval)
		}
		if tokens == 0 {
			_ = connection.Close(websocket.StatusPolicyViolation, "too many messages")
			return
		}
		tokens--

		if frame.Version != panelEventVersion || frame.Type != panelInboundPrefsPatch {
			continue
		}
		if !s.handlePrefsPatch(ctx, connection, accountID, subscriber, frame) {
			return
		}
	}
}

// handlePrefsPatch validates and applies one inbound patch, reporting
// whether the connection should stay open.
func (s *Server) handlePrefsPatch(
	ctx context.Context,
	connection *websocket.Conn,
	accountID string,
	subscriber *eventSubscriber,
	frame panelInboundFrame,
) bool {
	if len(frame.Changes) == 0 {
		return true
	}
	if len(frame.Changes) > panelInboundMaxPatchKeys {
		_ = connection.Close(websocket.StatusPolicyViolation, "preference patch too large")
		return false
	}

	accepted, rejected := validatePrefChanges(frame.Changes)
	if len(accepted) > 0 {
		if err := s.applyPrefsPatch(ctx, accountID, accepted); err != nil {
			_ = connection.Close(websocket.StatusInternalError, "preferences update failed")
			return false
		}
	}
	if len(rejected) > 0 {
		s.events.deliver(subscriber, panelEvent{
			Type: panelEventPrefsRejected,
			Keys: rejected,
		})
	}

	return true
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
			if !s.panelEventVisible(ctx, subscriber.accountID, event) {
				continue
			}
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

// panelEventVisible keeps workspace-scoped queue changes inside the same
// authorization boundary as the queue API. Root sees global and workspace
// work; an workspace user sees only targets they can still access. Access
// is resolved when the event is delivered so a long-lived connection cannot
// retain a scope after ownership or an explicit role changes.
func (s *Server) panelEventVisible(
	ctx context.Context,
	accountID string,
	event panelEvent,
) bool {
	if event.Type != panelEventQueueChanged {
		return true
	}
	user, err := s.store.GetPanelUser(ctx, accountID)
	if err != nil || user.Status != storage.PanelUserActive {
		return false
	}
	if user.SystemRole.IsRoot() {
		return true
	}
	if event.TargetID == "" {
		return false
	}
	access, err := s.store.ResolveTargetAccess(ctx, accountID, event.TargetID, s.now().UTC())

	return err == nil && access.Role != storage.InstallationRoleNone
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
