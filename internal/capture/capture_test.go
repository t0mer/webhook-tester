package capture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/t0mer/raptor/internal/models"
	"github.com/t0mer/raptor/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mkToken(t *testing.T, s *store.Store, mut func(*models.Token)) *models.Token {
	t.Helper()
	tok := &models.Token{
		UUID:               uuid.NewString(),
		DefaultStatus:      200,
		DefaultContent:     "hello world",
		DefaultContentType: "text/plain",
		Premium:            true,
	}
	if mut != nil {
		mut(tok)
	}
	if err := s.CreateToken(context.Background(), tok); err != nil {
		t.Fatalf("create token: %v", err)
	}
	return tok
}

func TestHandleRecordsAndResponds(t *testing.T) {
	s := newStore(t)
	tok := mkToken(t, s, func(tk *models.Token) { tk.CORS = true })
	c := New(s, "http://localhost:8084")

	body := strings.NewReader(`{"k":"v"}`)
	r := httptest.NewRequest(http.MethodPost, "/"+tok.UUID+"/path?a=1&a=2", body)
	r.Header.Set("X-Custom", "yes")
	r.Header.Set("User-Agent", "test-agent")
	w := httptest.NewRecorder()

	c.Handle(w, r, tok, nil)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "hello world" {
		t.Errorf("body = %q, want default content", got)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}

	reqs, err := s.ListRequests(context.Background(), tok.UUID, 10, 0)
	if err != nil || len(reqs) != 1 {
		t.Fatalf("ListRequests: %v len=%d", err, len(reqs))
	}
	got := reqs[0]
	if got.Method != "POST" || got.Content != `{"k":"v"}` {
		t.Errorf("recorded request mismatch: %+v", got)
	}
	if got.Query["a"][0] != "1" || got.Query["a"][1] != "2" {
		t.Errorf("query not captured: %+v", got.Query)
	}
	if got.Headers["X-Custom"][0] != "yes" {
		t.Errorf("header not captured: %+v", got.Headers)
	}
	if got.UserAgent != "test-agent" {
		t.Errorf("user agent = %q", got.UserAgent)
	}
}

func TestStatusOverride(t *testing.T) {
	s := newStore(t)
	tok := mkToken(t, s, nil)
	c := New(s, "http://x")

	override := 418
	r := httptest.NewRequest(http.MethodGet, "/"+tok.UUID+"/418", nil)
	w := httptest.NewRecorder()
	c.Handle(w, r, tok, &override)

	if w.Code != 418 {
		t.Errorf("status = %d, want 418 override", w.Code)
	}
}

func TestRedirect(t *testing.T) {
	s := newStore(t)
	tok := mkToken(t, s, func(tk *models.Token) { tk.Redirect = "https://example.com" })
	c := New(s, "http://x")

	r := httptest.NewRequest(http.MethodGet, "/"+tok.UUID, nil)
	w := httptest.NewRecorder()
	c.Handle(w, r, tok, nil)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://example.com" {
		t.Errorf("Location = %q", loc)
	}
}

func TestExpired(t *testing.T) {
	s := newStore(t)
	tok := mkToken(t, s, func(tk *models.Token) {
		tk.Expiry = 1
		tk.CreatedAt = time.Now().Add(-time.Hour)
	})
	c := New(s, "http://x")

	r := httptest.NewRequest(http.MethodGet, "/"+tok.UUID, nil)
	w := httptest.NewRecorder()
	c.Handle(w, r, tok, nil)

	if w.Code != http.StatusGone {
		t.Errorf("status = %d, want 410 Gone", w.Code)
	}
}

func TestRateLimit(t *testing.T) {
	s := newStore(t)
	// timeout=100 => limit = 100/100 = 1 request/min
	tok := mkToken(t, s, func(tk *models.Token) { tk.Timeout = 100 })
	c := New(s, "http://x")

	first := httptest.NewRecorder()
	c.Handle(first, httptest.NewRequest(http.MethodGet, "/"+tok.UUID, nil), tok, nil)
	if first.Code != 200 {
		t.Fatalf("first request status = %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	c.Handle(second, httptest.NewRequest(http.MethodGet, "/"+tok.UUID, nil), tok, nil)
	if second.Code != http.StatusTooManyRequests {
		t.Errorf("second request status = %d, want 429", second.Code)
	}
}

func TestRecordNonHTTP(t *testing.T) {
	s := newStore(t)
	tok := mkToken(t, s, nil)
	c := New(s, "http://x")

	req := &models.Request{
		UUID:    uuid.NewString(),
		TokenID: tok.UUID,
		Type:    models.RequestTypeEmail,
		Sender:  "a@b.com",
		Subject: "hi",
	}
	if err := c.Record(context.Background(), tok, req); err != nil {
		t.Fatalf("Record: %v", err)
	}
	got, err := s.GetRequest(context.Background(), req.UUID)
	if err != nil || got.Type != models.RequestTypeEmail {
		t.Fatalf("recorded email not found: %v / %+v", err, got)
	}
}

func TestRecordExpired(t *testing.T) {
	s := newStore(t)
	tok := mkToken(t, s, func(tk *models.Token) {
		tk.Expiry = 1
		tk.CreatedAt = time.Now().Add(-time.Hour)
	})
	c := New(s, "http://x")
	req := &models.Request{UUID: uuid.NewString(), TokenID: tok.UUID, Type: models.RequestTypeDNS}
	if err := c.Record(context.Background(), tok, req); err != ErrExpired {
		t.Errorf("Record on expired = %v, want ErrExpired", err)
	}
}

func TestSaveFile(t *testing.T) {
	s := newStore(t)
	tok := mkToken(t, s, nil)
	dir := t.TempDir()
	c := New(s, "http://x", WithFilesDir(dir))

	req := &models.Request{UUID: uuid.NewString(), TokenID: tok.UUID, Type: models.RequestTypeEmail}
	if err := c.Record(context.Background(), tok, req); err != nil {
		t.Fatal(err)
	}
	f, err := c.SaveFile(context.Background(), req.UUID, "doc.txt", "text/plain", []byte("data"))
	if err != nil {
		t.Fatalf("SaveFile: %v", err)
	}
	if f.Size != 4 {
		t.Errorf("size = %d, want 4", f.Size)
	}
	files, err := s.ListFilesByRequest(context.Background(), req.UUID)
	if err != nil || len(files) != 1 || files[0].Filename != "doc.txt" {
		t.Fatalf("file not recorded: %v / %+v", err, files)
	}
}

func TestResolveByAlias(t *testing.T) {
	s := newStore(t)
	tok := mkToken(t, s, func(tk *models.Token) { tk.Alias = "myalias" })
	c := New(s, "http://x")

	got, err := c.Resolve(context.Background(), "myalias")
	if err != nil || got.UUID != tok.UUID {
		t.Fatalf("Resolve by alias: %v / %v", err, got)
	}
	if _, err := c.Resolve(context.Background(), "nonexistent"); err != store.ErrNotFound {
		t.Errorf("Resolve unknown = %v, want ErrNotFound", err)
	}
}
