package api

import (
	"encoding/base64"
	"net/http"
)

type setResponseRequest struct {
	Status      int               `json:"status"`
	Content     string            `json:"content"`      // base64-encoded body
	ContentText string            `json:"content_text"` // alternative: raw text body
	Headers     map[string]string `json:"headers"`
}

// setResponse supplies the response for a pending captured request (the CLI
// `listen` flow). The content may be base64 (`content`) or raw (`content_text`).
func (a *API) setResponse(w http.ResponseWriter, r *http.Request) {
	req, ok := a.loadRequest(w, r)
	if !ok {
		return
	}
	var body setResponseRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	content := body.ContentText
	if body.Content != "" {
		decoded, err := base64.StdEncoding.DecodeString(body.Content)
		if err != nil {
			writeError(w, http.StatusBadRequest, "content must be base64")
			return
		}
		content = string(decoded)
	}

	if a.forwarder == nil || !a.forwarder.SetResponse(req.UUID, body.Status, content, body.Headers) {
		writeError(w, http.StatusNotFound, "no request is waiting for a response (not listening, or already responded)")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
