// Package afclient is a small typed client for the ArtefactFlow API, scoped to the single endpoint this action needs: the suggested next versions for an artefact.
//
// Authentication is centralised in a single bearer-token RoundTripper, so the token is applied in exactly one place and never reaches an error string or a log line. The base URL is always supplied by the caller — never hard-coded — per the ArtefactFlow integration rules.
package afclient

import (
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

// retryPolicy bounds how the client rides out transient upstream failures. Only the idempotent GET is retried, and only on failures a retry could plausibly fix: connection errors, HTTP 429, and 5xx. Permanent failures (4xx other than 429) never retry. An exhausted retry still fails loudly — a workflow that cannot learn the next version must stop rather than tag something arbitrary.
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
	if baseURL == "" {
		return nil, fmt.Errorf("afclient: base URL is required")
	}
	if token == "" {
		return nil, fmt.Errorf("afclient: bearer token is required")
	}
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
		retry:   defaultRetry(),
	}
	for _, o := range opts {
		o(c)
	}
	base := c.http.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	c.http.Transport = &bearerTransport{token: token, base: base}
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

// getJSON performs a GET against path (relative to the base URL) and decodes a 200 body into dst. Non-200 responses become errors that name the status and a bounded snippet of the body — never the token.
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

// getRaw performs a GET and returns the raw 200 body, mapping non-200 statuses to descriptive errors. Transient failures (connection errors, HTTP 429, and 5xx) are retried with exponential backoff and jitter, bounded by the client's retry policy; permanent failures (4xx other than 429) return immediately.
func (c *Client) getRaw(ctx context.Context, fullURL string) ([]byte, error) {
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
		body, retryable, retryAfter, err := c.getOnce(ctx, fullURL)
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

// getOnce performs a single GET attempt. It reports whether the failure is worth retrying and any server-requested Retry-After delay.
func (c *Client) getOnce(ctx context.Context, fullURL string) (body []byte, retryable bool, retryAfter time.Duration, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, false, 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// A cancelled or timed-out context is deliberate — never retry it.
		if ctx.Err() != nil {
			return nil, false, 0, err
		}
		// Connection refused/reset, DNS failure, etc. — transient, worth a retry.
		return nil, true, 0, err
	}
	defer resp.Body.Close()
	// The response is a handful of version strings; a megabyte is already far more than any legitimate body.
	body, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusOK {
		return body, false, 0, nil
	}
	statusErr := &StatusError{Status: resp.StatusCode, Snippet: snippet(body)}
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
	Status  int
	Snippet string
}

func (e *StatusError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized:
		return fmt.Sprintf("HTTP 401: the token was rejected. Check that it is valid and not expired: %s", e.Snippet)
	case http.StatusForbidden:
		// Both authorization axes fail with 403, and the scope is the one people check first — name the team layer explicitly so a correctly-scoped service account is not mistaken for a scope problem.
		return fmt.Sprintf("HTTP 403: the token needs the artefact:read scope, and its principal must belong to a team with read access to the artefact's group: %s", e.Snippet)
	case http.StatusNotFound:
		return fmt.Sprintf("HTTP 404: no such artefact. artefact-id must be the artefact's 24-hex id, not its name: %s", e.Snippet)
	default:
		return fmt.Sprintf("artefactflow api returned HTTP %d: %s", e.Status, e.Snippet)
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
