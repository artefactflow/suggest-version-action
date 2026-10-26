// Package afclient is a small typed client for the ArtefactFlow API, scoped to the single endpoint this action needs: the suggested next versions for an artefact.
//
// Authentication is centralised in a single bearer-token RoundTripper, so the token is applied in exactly one place and never reaches an error string or a log line. The base URL is always supplied by the caller (never hard-coded), per the ArtefactFlow integration rules.
package afclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to one ArtefactFlow API instance.
type Client struct {
	baseURL string
	http    *http.Client
	retry   retryPolicy
}

// retryPolicy bounds how the client rides out transient upstream failures. Only the idempotent GET is retried, and only on failures a retry could plausibly fix: connection errors, HTTP 429, and 5xx. Permanent failures (4xx other than 429) never retry. An exhausted retry still fails loudly: a workflow that cannot learn the next version must stop rather than tag something arbitrary.
type retryPolicy struct {
	maxAttempts int           // total tries including the first; <1 is clamped to 1
	baseDelay   time.Duration // backoff before the 2nd attempt; doubles each round
	maxDelay    time.Duration // ceiling on any single backoff sleep
}

func defaultRetry() retryPolicy {
	return retryPolicy{maxAttempts: 3, baseDelay: 200 * time.Millisecond, maxDelay: 5 * time.Second}
}

// delay computes the backoff before the retry following attempt index (0-based: 0 is the wait before the 2nd attempt). A server-supplied Retry-After wins, capped at maxDelay so one slow API cannot stall a whole job. Otherwise it is exponential with full jitter in [d/2, d], so a fleet of concurrent workflow runs does not synchronise its retry storm against a recovering API.
func (p retryPolicy) delay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return min(retryAfter, p.maxDelay)
	}
	if p.baseDelay <= 0 {
		return 0
	}
	d := p.baseDelay
	for i := 0; i < attempt && d < p.maxDelay; i++ {
		d *= 2
	}
	if d > p.maxDelay {
		d = p.maxDelay
	}
	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

// UserAgent identifies the tool and its version on every request, including the token exchange, so ArtefactFlow can tell which release a service account runs: that is how users of a vulnerable release are found and told. main sets it from the build version.
var UserAgent = "suggest-version-action/dev"

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the underlying HTTP client. The bearer transport is layered on top of the provided client's transport.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithRetry overrides the bounded-retry policy for transient upstream failures. maxAttempts is the total number of tries including the first; a value below 1 is clamped to 1, which disables retries. baseDelay is the backoff before the second attempt and doubles each round up to maxDelay.
func WithRetry(maxAttempts int, baseDelay, maxDelay time.Duration) Option {
	return func(c *Client) {
		if maxAttempts < 1 {
			maxAttempts = 1
		}
		c.retry = retryPolicy{maxAttempts: maxAttempts, baseDelay: baseDelay, maxDelay: maxDelay}
	}
}

// New builds a Client for baseURL that authenticates every request with token. token must not be empty. It is never logged and is redacted from errors.
func New(baseURL, token string, opts ...Option) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("afclient: bearer token is required")
	}
	c, err := newClient(baseURL, opts...)
	if err != nil {
		return nil, err
	}
	base := c.http.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	c.http.Transport = &bearerTransport{token: token, base: base}
	return c, nil
}

// newClient builds an unauthenticated client. Only the token endpoint uses one directly. Every other call goes through New and carries a bearer token.
func newClient(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("afclient: base URL is required")
	}
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
		retry:   defaultRetry(),
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// bearerTransport injects the Authorization header on every request. Keeping it as a RoundTripper means auth is defined once, and the token lives nowhere else in the call path.
type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone before mutating: RoundTrippers must not modify the caller's request.
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(r)
}

// getJSON performs a GET against path (relative to the base URL) and decodes a 200 body into dst. Non-200 responses become errors that name the status and a bounded snippet of the body, never the token.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, dst any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	body, err := c.getRaw(ctx, u)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// getRaw performs a GET and returns the raw 200 body.
func (c *Client) getRaw(ctx context.Context, fullURL string) ([]byte, error) {
	return c.doRaw(ctx, http.MethodGet, fullURL, nil)
}

// doRaw performs a request and returns the raw 200 body, mapping non-200 statuses to descriptive errors. Transient failures (connection errors, HTTP 429, and 5xx) are retried with exponential backoff and jitter, bounded by the client's retry policy; permanent failures (4xx other than 429) return immediately.
//
// reqBody is held as bytes rather than a reader so each attempt can build a fresh request. Only two calls use this: an idempotent GET, and the POST that mints a token. Replaying that one mints another token and has no other effect, so it is equally safe to retry.
func (c *Client) doRaw(ctx context.Context, method, fullURL string, reqBody []byte) ([]byte, error) {
	var (
		lastErr        error
		lastRetryAfter time.Duration
	)
	for attempt := 0; attempt < c.retry.maxAttempts; attempt++ {
		if attempt > 0 {
			if err := waitBackoff(ctx, c.retry.delay(attempt-1, lastRetryAfter)); err != nil {
				return nil, err
			}
		}
		body, retryable, retryAfter, err := c.doOnce(ctx, method, fullURL, reqBody)
		if err == nil {
			return body, nil
		}
		lastErr, lastRetryAfter = err, retryAfter
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

// doOnce performs a single attempt. It reports whether the failure is worth retrying and any server-requested Retry-After delay.
func (c *Client) doOnce(ctx context.Context, method, fullURL string, reqBody []byte) (body []byte, retryable bool, retryAfter time.Duration, err error) {
	var payload io.Reader
	if reqBody != nil {
		payload = bytes.NewReader(reqBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, payload)
	if err != nil {
		return nil, false, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// A cancelled or timed-out context is deliberate: never retry it.
		if ctx.Err() != nil {
			return nil, false, 0, err
		}
		// Connection refused/reset, DNS failure, and the like are transient, worth a retry.
		return nil, true, 0, err
	}
	defer resp.Body.Close()
	// The response is a handful of version strings; a megabyte is already far more than any legitimate body.
	body, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusOK {
		return body, false, 0, nil
	}
	statusErr := newStatusError(resp.StatusCode, body)
	if isRetryableStatus(resp.StatusCode) {
		return nil, true, parseRetryAfter(resp.Header.Get("Retry-After")), statusErr
	}
	return nil, false, 0, statusErr
}

// isRetryableStatus reports whether an HTTP status is a transient upstream failure worth retrying: rate limiting (429) or any server-side 5xx.
func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// parseRetryAfter reads the delta-seconds form of a Retry-After header. The HTTP-date form is ignored (returns 0), falling back to normal backoff.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

// waitBackoff sleeps for d, returning early with the context error if ctx is cancelled first so a retry loop never outlives its deadline.
func waitBackoff(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// StatusError is a non-200 response from the ArtefactFlow API.
type StatusError struct {
	Status int
	// Code, Message, and LogID are the API's own error envelope, when the body carried one. LogID is what support needs to find the request in the API's logs, so it is always surfaced.
	Code    string
	Message string
	LogID   string
	// Snippet is a bounded excerpt of the raw body, used when the response was not the documented error envelope (an HTML error page from a proxy, say).
	Snippet string
}

// Error codes the action can say something more useful about than the API's own wording. Anything else falls through to the generic message plus whatever the API said.
const (
	codeInvalidCredentials     = "AUTH00001"
	codeServiceAccountUnusable = "SERVICEACCOUNT01455"
	codeServiceAccountExpired  = "SERVICEACCOUNT01456"
)

// apiError is the API's error envelope. The log id is `log_id` on the wire; some older deployments send it as `logid`, so both are read.
type apiError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	LogID       string `json:"log_id"`
	LegacyLogID string `json:"logid"`
}

// newStatusError builds a StatusError, preferring the API's structured error body over a raw excerpt. A body that is not the documented envelope (a proxy's HTML error page, for instance) falls back to the excerpt.
func newStatusError(status int, body []byte) *StatusError {
	e := &StatusError{Status: status}
	var parsed apiError
	// The gateway's own rejections (a missing Authorization header, say) carry a message but no code, and still count as the envelope.
	if err := json.Unmarshal(body, &parsed); err == nil && (parsed.Code != "" || parsed.Message != "") {
		e.Code, e.Message, e.LogID = parsed.Code, parsed.Message, parsed.LogID
		if e.LogID == "" {
			e.LogID = parsed.LegacyLogID
		}
		return e
	}
	e.Snippet = snippet(body)
	return e
}

// detail renders whatever the API said about the failure, in one line.
func (e *StatusError) detail() string {
	parts := make([]string, 0, 3)
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	if e.Code != "" {
		parts = append(parts, "code "+e.Code)
	}
	if e.LogID != "" {
		parts = append(parts, "log id "+e.LogID)
	}
	if len(parts) == 0 {
		return e.Snippet
	}
	return strings.Join(parts, ", ")
}

func (e *StatusError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized:
		// The token endpoint returns 401 for several distinct causes, and the codes are the only way to tell them apart: "invalid credentials" is deliberately returned for both an unknown client id and a wrong secret, so that one cannot be narrowed further.
		switch e.Code {
		case codeInvalidCredentials:
			return fmt.Sprintf("HTTP 401: the client credentials were rejected. Check `client-id` and `client-secret`; a rotated secret invalidates the previous one immediately (%s)", e.detail())
		case codeServiceAccountUnusable:
			return fmt.Sprintf("HTTP 401: the service account cannot mint tokens because it is deactivated, or its organisation is inactive (%s)", e.detail())
		case codeServiceAccountExpired:
			return fmt.Sprintf("HTTP 401: the service account has passed its expiry date and can no longer mint tokens. Extend it before it lapses (%s)", e.detail())
		}
		return fmt.Sprintf("HTTP 401: the token was rejected. Check that it is valid and not expired (%s)", e.detail())
	case http.StatusForbidden:
		// Both authorization axes fail with 403, and the scope is the one people check first. Name the team layer explicitly so a correctly-scoped service account is not mistaken for a scope problem. The two calls need different scopes, so both are named.
		return fmt.Sprintf("HTTP 403: the token needs the artefact:read scope to look an artefact up by name and version:read to read its versions, and its principal must belong to a team with read access to the artefact's group (%s)", e.detail())
	case http.StatusNotFound:
		return fmt.Sprintf("HTTP 404: no such artefact on this instance (%s)", e.detail())
	default:
		return fmt.Sprintf("artefactflow api returned HTTP %d (%s)", e.Status, e.detail())
	}
}

func snippet(b []byte) string {
	const max = 512
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
