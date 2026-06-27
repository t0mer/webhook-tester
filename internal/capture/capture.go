// Package capture records inbound HTTP requests against a token and writes the
// token's configured default response. It is the public-facing sink, so it
// enforces body-size limits, the per-token rate limit, request_limit and expiry.
package capture

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/t0mer/raptor/internal/actions"
	"github.com/t0mer/raptor/internal/metrics"
	"github.com/t0mer/raptor/internal/models"
	"github.com/t0mer/raptor/internal/store"
)

// ErrExpired is returned by Record when the token's TTL has elapsed.
var ErrExpired = errors.New("token expired")

// DefaultMaxBodyBytes is the cap applied to captured request bodies.
const DefaultMaxBodyBytes int64 = 10 << 20 // 10 MiB

// Publisher receives newly captured requests for real-time fan-out (SSE). The
// no-op implementation is used until the SSE hub is wired in.
type Publisher interface {
	Publish(tokenID string, req *models.Request)
}

type nopPublisher struct{}

func (nopPublisher) Publish(string, *models.Request) {}

// Capturer records requests and renders default responses.
type Capturer struct {
	store        *store.Store
	baseURL      string
	maxBodyBytes int64
	globalLimit  int    // config --max-requests; fallback when token.RequestLimit == 0
	filesDir     string // directory for stored attachment/file blobs
	limiter      *rateLimiter
	pub          Publisher
	actions      *actions.Service // optional; runs the action chain on HTTP capture
	forwarder    *forwarder       // CLI `listen` response coordination
}

// Option configures a Capturer.
type Option func(*Capturer)

// WithPublisher sets the real-time publisher (SSE hub).
func WithPublisher(p Publisher) Option {
	return func(c *Capturer) {
		if p != nil {
			c.pub = p
		}
	}
}

// WithMaxBodyBytes overrides the captured-body size cap.
func WithMaxBodyBytes(n int64) Option {
	return func(c *Capturer) {
		if n > 0 {
			c.maxBodyBytes = n
		}
	}
}

// WithGlobalRequestLimit sets the fallback per-token stored-request cap.
func WithGlobalRequestLimit(n int) Option {
	return func(c *Capturer) { c.globalLimit = n }
}

// WithFilesDir sets the directory where attachment/file blobs are written.
func WithFilesDir(dir string) Option {
	return func(c *Capturer) { c.filesDir = dir }
}

// WithActions enables the Custom Actions engine for HTTP capture.
func WithActions(svc *actions.Service) Option {
	return func(c *Capturer) { c.actions = svc }
}

// New constructs a Capturer.
func New(st *store.Store, baseURL string, opts ...Option) *Capturer {
	c := &Capturer{
		store:        st,
		baseURL:      baseURL,
		maxBodyBytes: DefaultMaxBodyBytes,
		limiter:      newRateLimiter(),
		pub:          nopPublisher{},
		forwarder:    newForwarder(),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Resolve looks up a token by its UUID, falling back to its alias. It returns
// store.ErrNotFound when neither matches.
func (c *Capturer) Resolve(ctx context.Context, identifier string) (*models.Token, error) {
	if identifier == "" {
		return nil, store.ErrNotFound
	}
	tok, err := c.store.GetToken(ctx, identifier)
	if err == nil {
		return tok, nil
	}
	if err != store.ErrNotFound {
		return nil, err
	}
	return c.store.GetTokenByAlias(ctx, identifier)
}

// Handle records the request against token and writes its default response.
// statusOverride, when non-nil, replaces the token's default response status
// (the /{tokenId}/{statusCode} form).
func (c *Capturer) Handle(w http.ResponseWriter, r *http.Request, token *models.Token, statusOverride *int) {
	now := time.Now().UTC()

	if IsExpired(token, now) {
		metrics.RequestsRejected.WithLabelValues("expired").Inc()
		http.Error(w, "this URL has expired", http.StatusGone)
		return
	}

	if !c.limiter.allow(token.UUID, token.Timeout) {
		metrics.RequestsRejected.WithLabelValues("rate_limited").Inc()
		w.Header().Set("Retry-After", "60")
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	req := c.buildRequest(r, token, now)

	// Run the Custom Actions chain (if enabled) before persisting/responding, so
	// actions can override the response, extract data, or drop the request.
	var engineResp *actions.Response
	dontSave := false
	var results []actions.RunResult
	runActions := c.actions != nil && token.Actions
	if runActions {
		ec, res, err := c.actions.Execute(r.Context(), req, token)
		if err != nil {
			slog.Error("run actions", "token", token.UUID, "error", err)
			runActions = false
		} else {
			results = res
			req.CustomActionOutput, req.CustomActionErrors = actions.Summarise(res)
			if ec.Response.Set {
				engineResp = ec.Response
			}
			dontSave = ec.DontSave
		}
	}

	if !dontSave {
		if err := c.persist(r.Context(), token, req); err != nil {
			http.Error(w, "failed to store request", http.StatusInternalServerError)
			return
		}
		if runActions && len(results) > 0 {
			if err := c.actions.SaveRuns(r.Context(), req.UUID, results); err != nil {
				slog.Warn("save action runs", "request", req.UUID, "error", err)
			}
		}
	}

	// CLI forwarding: when listen > 0 and no action produced a response, hold the
	// request open until a CLI client sets a response (or the window elapses).
	if token.Listen > 0 && engineResp == nil && !dontSave {
		ch := c.forwarder.register(req.UUID)
		select {
		case fr := <-ch:
			c.writeForwarded(w, token, fr)
			return
		case <-time.After(time.Duration(token.Listen) * time.Second):
			c.forwarder.cancel(req.UUID)
		case <-r.Context().Done():
			c.forwarder.cancel(req.UUID)
			return
		}
	}

	c.writeResponse(w, token, statusOverride, engineResp)
}

// SetResponse delivers a CLI-supplied response to a pending captured request
// (the listen flow). Returns false if no request is waiting for that id.
func (c *Capturer) SetResponse(requestID string, status int, content string, headers map[string]string) bool {
	if status == 0 {
		status = http.StatusOK
	}
	return c.forwarder.deliver(requestID, forwardResponse{status: status, content: content, headers: headers})
}

func (c *Capturer) writeForwarded(w http.ResponseWriter, token *models.Token, fr forwardResponse) {
	h := w.Header()
	if token.CORS {
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "*")
	}
	for k, v := range fr.headers {
		h.Set(k, v)
	}
	if h.Get("Content-Type") == "" {
		h.Set("Content-Type", "text/plain")
	}
	w.WriteHeader(fr.status)
	_, _ = io.WriteString(w, fr.content)
}

// Record stores a non-HTTP captured request (email, DNS) against a token,
// enforcing expiry and the stored-request limit, then publishes it. Returns
// ErrExpired if the token's TTL has elapsed.
func (c *Capturer) Record(ctx context.Context, token *models.Token, req *models.Request) error {
	if IsExpired(token, time.Now().UTC()) {
		metrics.RequestsRejected.WithLabelValues("expired").Inc()
		return ErrExpired
	}
	return c.persist(ctx, token, req)
}

// persist stores a request, increments metrics, and fans it out to subscribers.
func (c *Capturer) persist(ctx context.Context, token *models.Token, req *models.Request) error {
	limit := token.RequestLimit
	if limit == 0 {
		limit = c.globalLimit
	}
	if err := c.store.CreateRequest(ctx, req, limit); err != nil {
		return err
	}
	metrics.RequestsCaptured.WithLabelValues(req.Type).Inc()
	c.pub.Publish(token.UUID, req)
	return nil
}

// SaveFile writes a captured file blob under the configured files directory and
// records it against a request. Used for email attachments.
func (c *Capturer) SaveFile(ctx context.Context, requestID, filename, contentType string, data []byte) (*models.File, error) {
	if c.filesDir == "" {
		return nil, errors.New("files directory not configured")
	}
	if err := os.MkdirAll(c.filesDir, 0o750); err != nil {
		return nil, err
	}
	id := uuid.NewString()
	path := filepath.Join(c.filesDir, id)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return nil, err
	}
	f := &models.File{
		ID:          id,
		RequestID:   requestID,
		Filename:    filename,
		ContentType: contentType,
		Size:        len(data),
		Path:        path,
	}
	if err := c.store.CreateFile(ctx, f); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return f, nil
}

func (c *Capturer) buildRequest(r *http.Request, token *models.Token, now time.Time) *models.Request {
	body, _ := io.ReadAll(io.LimitReader(r.Body, c.maxBodyBytes))

	headers := map[string][]string(r.Header.Clone())
	query := map[string][]string(r.URL.Query())

	return &models.Request{
		UUID:      uuid.NewString(),
		TokenID:   token.UUID,
		Type:      models.RequestTypeWeb,
		Method:    r.Method,
		IP:        clientIP(r),
		Hostname:  r.Host,
		UserAgent: r.UserAgent(),
		Content:   string(body),
		Query:     query,
		Headers:   headers,
		URL:       c.baseURL + r.URL.RequestURI(),
		Size:      len(body),
		Sorting:   now.UnixMilli(),
		CreatedAt: now,
	}
}

// writeResponse renders the response. Precedence: an action-set response
// overrides everything; otherwise a status override (the /{id}/{code} form)
// overrides the token default; a token redirect applies only when no action
// produced a response.
func (c *Capturer) writeResponse(w http.ResponseWriter, token *models.Token, statusOverride *int, eng *actions.Response) {
	h := w.Header()
	if token.CORS {
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "*")
	}
	if eng != nil {
		for k, v := range eng.Headers {
			h.Set(k, v)
		}
	}

	if token.Redirect != "" && eng == nil {
		// Set Location directly rather than via http.Redirect, which would
		// require a *http.Request to resolve relative targets (and panic on a
		// nil URL). The configured value is used verbatim.
		h.Set("Location", token.Redirect)
		w.WriteHeader(http.StatusFound)
		return
	}

	ct := token.DefaultContentType
	content := token.DefaultContent
	status := token.DefaultStatus
	if status == 0 {
		status = http.StatusOK
	}
	if statusOverride != nil {
		status = *statusOverride
	}
	if eng != nil {
		content = eng.Content
		if eng.ContentType != "" {
			ct = eng.ContentType
		}
		if eng.Status != 0 {
			status = eng.Status
		}
	}
	if ct == "" {
		ct = "text/plain"
	}
	h.Set("Content-Type", ct)
	w.WriteHeader(status)
	_, _ = io.WriteString(w, content)
}

// IsExpired reports whether a token's TTL (expiry seconds from creation) has
// elapsed. A zero expiry means the token never expires.
func IsExpired(token *models.Token, now time.Time) bool {
	if token.Expiry <= 0 {
		return false
	}
	return now.After(token.CreatedAt.Add(time.Duration(token.Expiry) * time.Second))
}

// clientIP returns the best-effort client IP, honouring a single
// X-Forwarded-For hop (Raptor is expected to run behind a reverse proxy).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, ok := strings.Cut(xff, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return strings.TrimSpace(xr)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
