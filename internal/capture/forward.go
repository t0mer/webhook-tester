package capture

import "sync"

// forwardResponse is a response supplied out-of-band by a CLI client for a
// pending captured request (the webhook.site `listen` flow).
type forwardResponse struct {
	status  int
	content string
	headers map[string]string
}

// forwarder coordinates the listen flow: a capture handler registers a pending
// request and blocks; a CLI client delivers the response via the set-response
// endpoint, unblocking the handler.
type forwarder struct {
	mu      sync.Mutex
	waiting map[string]chan forwardResponse
}

func newForwarder() *forwarder {
	return &forwarder{waiting: make(map[string]chan forwardResponse)}
}

// register creates a buffered channel for a request and returns it.
func (f *forwarder) register(id string) chan forwardResponse {
	ch := make(chan forwardResponse, 1)
	f.mu.Lock()
	f.waiting[id] = ch
	f.mu.Unlock()
	return ch
}

// cancel drops a pending registration (timeout or client disconnect).
func (f *forwarder) cancel(id string) {
	f.mu.Lock()
	delete(f.waiting, id)
	f.mu.Unlock()
}

// deliver hands a response to a waiting handler. Returns false if no handler is
// waiting (unknown request, already responded, or timed out).
func (f *forwarder) deliver(id string, r forwardResponse) bool {
	f.mu.Lock()
	ch, ok := f.waiting[id]
	if ok {
		delete(f.waiting, id)
	}
	f.mu.Unlock()
	if !ok {
		return false
	}
	ch <- r
	return true
}
