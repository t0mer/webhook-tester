package sse

import (
	"testing"
	"time"

	"github.com/t0mer/raptor/internal/models"
)

func TestPublishDeliversToSubscriber(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe("tok")
	defer cancel()

	if h.Subscribers("tok") != 1 {
		t.Fatalf("Subscribers = %d, want 1", h.Subscribers("tok"))
	}

	req := &models.Request{UUID: "r1"}
	h.Publish("tok", req)

	select {
	case got := <-ch:
		if got.UUID != "r1" {
			t.Errorf("got %q, want r1", got.UUID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestPublishIsolatedByToken(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe("a")
	defer cancel()

	h.Publish("b", &models.Request{UUID: "x"})

	select {
	case <-ch:
		t.Fatal("received event for a different token")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCancelUnsubscribes(t *testing.T) {
	h := NewHub()
	_, cancel := h.Subscribe("tok")
	cancel()
	if h.Subscribers("tok") != 0 {
		t.Errorf("Subscribers after cancel = %d, want 0", h.Subscribers("tok"))
	}
	// Publishing after cancel must not panic.
	h.Publish("tok", &models.Request{UUID: "z"})
}

func TestPublishDropsWhenFull(t *testing.T) {
	h := NewHub()
	_, cancel := h.Subscribe("tok")
	defer cancel()
	// Overfill the buffer; must not block or panic.
	for i := 0; i < bufferSize*4; i++ {
		h.Publish("tok", &models.Request{UUID: "r"})
	}
}
