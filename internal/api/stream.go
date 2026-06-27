package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// streamRequests serves a Server-Sent Events stream of newly captured requests
// for a token. The connection stays open until the client disconnects.
func (a *API) streamRequests(w http.ResponseWriter, r *http.Request) {
	tok, ok := a.loadToken(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)

	events, cancel := a.hub.Subscribe(tok.UUID)
	defer cancel()

	// Initial comment so the client knows the stream is live.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case req, open := <-events:
			if !open {
				return
			}
			payload, err := json.Marshal(req)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: request\ndata: %s\n\n", payload)
			flusher.Flush()
		}
	}
}
