// Package sse implements a minimal Server-Sent Events hub for real-time
// fan-out of captured requests. Each token has its own set of subscribers;
// the browser is receive-only, so SSE is preferred over a WebSocket dependency.
package sse

import (
	"sync"

	"github.com/t0mer/raptor/internal/models"
)

// bufferSize is the per-subscriber channel buffer. A slow client that fills its
// buffer drops events rather than blocking publishers.
const bufferSize = 16

// Hub fans captured requests out to per-token subscribers.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[*subscriber]struct{}
}

type subscriber struct {
	ch chan *models.Request
}

// NewHub creates an empty Hub.
func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[*subscriber]struct{})}
}

// Subscribe registers a subscriber for a token and returns its event channel
// plus a cancel func that must be called to release resources.
func (h *Hub) Subscribe(tokenID string) (<-chan *models.Request, func()) {
	sub := &subscriber{ch: make(chan *models.Request, bufferSize)}

	h.mu.Lock()
	if h.subs[tokenID] == nil {
		h.subs[tokenID] = make(map[*subscriber]struct{})
	}
	h.subs[tokenID][sub] = struct{}{}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		if set, ok := h.subs[tokenID]; ok {
			if _, ok := set[sub]; ok {
				delete(set, sub)
				close(sub.ch)
			}
			if len(set) == 0 {
				delete(h.subs, tokenID)
			}
		}
		h.mu.Unlock()
	}
	return sub.ch, cancel
}

// Publish delivers a request to all current subscribers of its token. Delivery
// is best-effort and non-blocking: a full subscriber buffer drops the event.
func (h *Hub) Publish(tokenID string, req *models.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.subs[tokenID] {
		select {
		case sub.ch <- req:
		default:
		}
	}
}

// Subscribers returns the number of active subscribers for a token (for tests
// and metrics).
func (h *Hub) Subscribers(tokenID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[tokenID])
}
