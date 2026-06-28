package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/t0mer/raptor/internal/actions"
	"github.com/t0mer/raptor/internal/auth"
	"github.com/t0mer/raptor/internal/capture"
	"github.com/t0mer/raptor/internal/config"
	"github.com/t0mer/raptor/internal/netguard"
	"github.com/t0mer/raptor/internal/schedules"
	"github.com/t0mer/raptor/internal/sse"
	"github.com/t0mer/raptor/internal/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "srv.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	cfg := config.Defaults()
	cfg.BaseURL = "http://example.test"
	hub := sse.NewHub()
	capturer := capture.New(st, cfg.BaseURL, capture.WithPublisher(hub))
	svc := actions.NewService(actions.New(), st)
	runner := schedules.New(st, svc)
	srv := New(cfg, st, capturer, hub, svc, runner, netguard.New(nil, nil, true), auth.NewService(st))

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestHealth(t *testing.T) {
	ts := newTestServer(t)
	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(res.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("status field = %v", body["status"])
	}
}

func TestEndToEndCaptureFlow(t *testing.T) {
	ts := newTestServer(t)
	client := ts.Client()

	// Create a token.
	res, err := client.Post(ts.URL+"/api/v1/tokens", "application/json",
		strings.NewReader(`{"default_content":"pong","cors":true}`))
	if err != nil {
		t.Fatal(err)
	}
	var tok struct {
		UUID string `json:"uuid"`
		URL  string `json:"url"`
	}
	json.NewDecoder(res.Body).Decode(&tok)
	res.Body.Close()
	if tok.UUID == "" {
		t.Fatal("no token uuid returned")
	}

	// Fire a capture request against the token.
	cap, err := client.Post(ts.URL+"/"+tok.UUID+"/path?q=1", "text/plain", strings.NewReader("hi"))
	if err != nil {
		t.Fatal(err)
	}
	capBody, _ := io.ReadAll(cap.Body)
	cap.Body.Close()
	if cap.StatusCode != 200 || string(capBody) != "pong" {
		t.Fatalf("capture response = %d %q", cap.StatusCode, capBody)
	}
	if cap.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header on capture response")
	}

	// List requests and verify capture was recorded.
	lr, err := client.Get(ts.URL + "/api/v1/tokens/" + tok.UUID + "/requests")
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	json.NewDecoder(lr.Body).Decode(&page)
	lr.Body.Close()
	if page.Total != 1 || len(page.Data) != 1 {
		t.Fatalf("expected 1 request, got total=%d len=%d", page.Total, len(page.Data))
	}
	if page.Data[0]["method"] != "POST" || page.Data[0]["content"] != "hi" {
		t.Errorf("recorded request mismatch: %+v", page.Data[0])
	}
}

func TestDocsAndMetricsAndSPA(t *testing.T) {
	ts := newTestServer(t)
	cases := map[string]int{
		"/api/openapi.yaml": 200,
		"/api/docs/":        200,
		"/metrics":          200,
		"/":                 200, // SPA placeholder (no build embedded in tests)
	}
	for path, want := range cases {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("GET %s = %d, want %d", path, res.StatusCode, want)
		}
	}
}

func TestCaptureUnknownTokenIsNotFound(t *testing.T) {
	ts := newTestServer(t)
	// Non-HTML POST to an unknown token → 404 (not SPA fallback).
	res, err := http.Post(ts.URL+"/does-not-exist", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}
