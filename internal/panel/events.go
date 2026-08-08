package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type panelEvent struct {
	Type         string `json:"type"`
	TargetID     string `json:"target_id,omitempty"`
	RepositoryID string `json:"repository_id,omitempty"`
}

type eventHub struct {
	mu          sync.Mutex
	subscribers map[chan panelEvent]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{subscribers: make(map[chan panelEvent]struct{})}
}

func (h *eventHub) subscribe() (<-chan panelEvent, func()) {
	channel := make(chan panelEvent, 16)
	h.mu.Lock()
	h.subscribers[channel] = struct{}{}
	h.mu.Unlock()

	return channel, func() {
		h.mu.Lock()
		delete(h.subscribers, channel)
		h.mu.Unlock()
	}
}

func (h *eventHub) announce(event panelEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscriber := range h.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireViewer(w, r); !ok {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")

	events, unsubscribe := s.events.subscribe()
	defer unsubscribe()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	controller := http.NewResponseController(w)
	_, _ = fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	_ = controller.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			body, err := json.Marshal(event)
			if err != nil {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
			if err := controller.Flush(); err != nil {
				return
			}
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}
