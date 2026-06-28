package server

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
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

// newAuthServer builds a require-auth server for isolation tests.
func newAuthServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "iso.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	cfg := config.Defaults()
	cfg.BaseURL = "http://example.test"
	cfg.RequireAuth = true
	hub := sse.NewHub()
	capturer := capture.New(st, cfg.BaseURL, capture.WithPublisher(hub))
	svc := actions.NewService(actions.New(), st)
	runner := schedules.New(st, svc)
	srv := New(cfg, st, capturer, hub, svc, runner, netguard.New(nil, nil, true), auth.NewService(st))

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, st
}

// client returns an http.Client with its own cookie jar (one logical user).
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func postJSON(t *testing.T, c *http.Client, url, body string) *http.Response {
	t.Helper()
	res, err := c.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return res
}

func createToken(t *testing.T, c *http.Client, base string) string {
	t.Helper()
	res := postJSON(t, c, base+"/api/v1/tokens", `{}`)
	defer res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("create token status = %d", res.StatusCode)
	}
	var tok struct {
		UUID string `json:"uuid"`
	}
	json.NewDecoder(res.Body).Decode(&tok)
	return tok.UUID
}

func TestPerUserURLIsolation(t *testing.T) {
	ts, _ := newAuthServer(t)
	base := ts.URL

	// Bootstrap the admin and create two regular users.
	admin := newClient(t)
	res := postJSON(t, admin, base+"/api/v1/auth/bootstrap", `{"email":"admin@x.com","password":"supersecret"}`)
	res.Body.Close()
	for _, email := range []string{"alice@x.com", "bob@x.com"} {
		r := postJSON(t, admin, base+"/api/v1/users", `{"email":"`+email+`","password":"supersecret","role":"user"}`)
		if r.StatusCode != 201 {
			t.Fatalf("create %s status = %d", email, r.StatusCode)
		}
		r.Body.Close()
	}

	login := func(c *http.Client, email string) {
		r := postJSON(t, c, base+"/api/v1/auth/login", `{"email":"`+email+`","password":"supersecret"}`)
		if r.StatusCode != 200 {
			t.Fatalf("login %s status = %d", email, r.StatusCode)
		}
		r.Body.Close()
	}

	alice, bob := newClient(t), newClient(t)
	login(alice, "alice@x.com")
	login(bob, "bob@x.com")

	tokA := createToken(t, alice, base)
	tokB := createToken(t, bob, base)

	// Alice lists tokens → only her own.
	lr, _ := alice.Get(base + "/api/v1/tokens")
	var page struct {
		Data []struct {
			UUID string `json:"uuid"`
		} `json:"data"`
	}
	json.NewDecoder(lr.Body).Decode(&page)
	lr.Body.Close()
	if len(page.Data) != 1 || page.Data[0].UUID != tokA {
		t.Fatalf("alice sees %d tokens, want only her own", len(page.Data))
	}

	// Cross-user access is denied (reported as not found).
	checks := []struct {
		c    *http.Client
		path string
	}{
		{alice, "/api/v1/tokens/" + tokB},
		{alice, "/api/v1/tokens/" + tokB + "/requests"},
		{bob, "/api/v1/tokens/" + tokA},
		{bob, "/api/v1/tokens/" + tokA + "/requests"},
	}
	for _, ch := range checks {
		r, _ := ch.c.Get(base + ch.path)
		if r.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 (cross-user denied)", ch.path, r.StatusCode)
		}
		r.Body.Close()
	}

	// Cross-user delete is denied.
	req, _ := http.NewRequest(http.MethodDelete, base+"/api/v1/tokens/"+tokB, nil)
	dr, _ := alice.Do(req)
	dr.Body.Close()
	if dr.StatusCode != http.StatusNotFound {
		t.Errorf("alice DELETE bob's token = %d, want 404", dr.StatusCode)
	}

	// The capture endpoint stays public — anyone can deliver to the URL.
	cap, err := http.Post(base+"/"+tokB+"/hook", "text/plain", strings.NewReader("hi"))
	if err != nil {
		t.Fatal(err)
	}
	cap.Body.Close()
	if cap.StatusCode != 200 {
		t.Errorf("public capture to bob's URL = %d, want 200", cap.StatusCode)
	}

	// Admin sees all tokens.
	ar, _ := admin.Get(base + "/api/v1/tokens")
	var apage struct {
		Data []json.RawMessage `json:"data"`
	}
	json.NewDecoder(ar.Body).Decode(&apage)
	ar.Body.Close()
	if len(apage.Data) != 2 {
		t.Errorf("admin sees %d tokens, want 2", len(apage.Data))
	}
}
