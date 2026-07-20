package afclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const artefactID = "0123456789abcdef01234567"

// newTestClient points a client at a test server with retries disabled, so a test asserting on an error does not wait out three attempts.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := New(srv.URL, "secret-token", WithHTTPClient(srv.Client()), WithRetry(1, 0, 0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestSuggestedVersions(t *testing.T) {
	var gotPath, gotAuth, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotAccept = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"suggested_version":"1.2.4","increments":{"major":"2.0.0","minor":"1.3.0","patch":"1.2.4"},"latest":"1.2.3","pre_releases":{"alpha":"1.2.4-alpha.1","beta":"1.2.4-beta.1","rc":"1.2.4-rc.1"}}`))
	}))
	defer srv.Close()

	s, err := newTestClient(t, srv).SuggestedVersions(context.Background(), artefactID)
	if err != nil {
		t.Fatalf("SuggestedVersions: %v", err)
	}
	if want := "/v1/artefacts/" + artefactID + "/suggested-versions"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if s.SuggestedVersion != "1.2.4" || s.Latest != "1.2.3" || s.Increments.Major != "2.0.0" || s.PreReleases.RC != "1.2.4-rc.1" {
		t.Errorf("unexpected decode: %+v", s)
	}
}

// A trailing slash on the operator-supplied base URL must not produce a double slash in the path.
func TestBaseURLTrailingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"suggested_version":"1.0.0"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL+"/", "secret-token", WithHTTPClient(srv.Client()), WithRetry(1, 0, 0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.SuggestedVersions(context.Background(), artefactID); err != nil {
		t.Fatalf("SuggestedVersions: %v", err)
	}
	if strings.Contains(gotPath, "//") {
		t.Errorf("path contains a double slash: %q", gotPath)
	}
}

func TestNewRequiresBaseURLAndToken(t *testing.T) {
	if _, err := New("", "token"); err == nil {
		t.Error("expected an error for an empty base URL")
	}
	if _, err := New("https://api.example.com", ""); err == nil {
		t.Error("expected an error for an empty token")
	}
}

func TestStatusErrors(t *testing.T) {
	tests := []struct {
		status  int
		wantSub string
	}{
		{http.StatusUnauthorized, "token was rejected"},
		{http.StatusForbidden, "artefact:read"},
		{http.StatusForbidden, "team"},
		{http.StatusNotFound, "24-hex id"},
		{http.StatusBadRequest, "HTTP 400"},
	}
	for _, tc := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(`{"error":"denied"}`))
		}))
		_, err := newTestClient(t, srv).SuggestedVersions(context.Background(), artefactID)
		srv.Close()
		if err == nil {
			t.Errorf("status %d: expected an error", tc.status)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantSub) {
			t.Errorf("status %d: error %q does not mention %q", tc.status, err, tc.wantSub)
		}
		if strings.Contains(err.Error(), "secret-token") {
			t.Errorf("status %d: error leaked the token: %v", tc.status, err)
		}
	}
}

// A 200 that omits suggested_version is not a usable answer — it must not silently become an empty output.
func TestEmptySuggestionIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"latest":"1.2.3"}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(t, srv).SuggestedVersions(context.Background(), artefactID); err == nil {
		t.Fatal("expected an error when suggested_version is missing")
	}
}

func TestMalformedJSONIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>not json</html>`))
	}))
	defer srv.Close()

	if _, err := newTestClient(t, srv).SuggestedVersions(context.Background(), artefactID); err == nil {
		t.Fatal("expected an error for a non-JSON body")
	}
}

func TestRetriesTransientFailures(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"suggested_version":"1.2.4"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "secret-token", WithHTTPClient(srv.Client()), WithRetry(3, time.Millisecond, 5*time.Millisecond))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s, err := c.SuggestedVersions(context.Background(), artefactID)
	if err != nil {
		t.Fatalf("SuggestedVersions: %v", err)
	}
	if s.SuggestedVersion != "1.2.4" {
		t.Errorf("SuggestedVersion = %q", s.SuggestedVersion)
	}
	if calls != 3 {
		t.Errorf("server saw %d calls, want 3", calls)
	}
}

// A permanent failure must not be retried — a 404 will still be a 404 on the third try.
func TestDoesNotRetryPermanentFailures(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := New(srv.URL, "secret-token", WithHTTPClient(srv.Client()), WithRetry(3, time.Millisecond, 5*time.Millisecond))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.SuggestedVersions(context.Background(), artefactID); err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Errorf("server saw %d calls, want 1", calls)
	}
}

func TestContextCancellationStopsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c, err := New(srv.URL, "secret-token", WithHTTPClient(srv.Client()), WithRetry(5, time.Second, time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.SuggestedVersions(ctx, artefactID)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("retry loop outlived its context")
	}
}

func TestRetryAfterIsHonoured(t *testing.T) {
	p := retryPolicy{maxAttempts: 3, baseDelay: time.Second, maxDelay: 10 * time.Second}
	if got := p.delay(0, 2*time.Second); got != 2*time.Second {
		t.Errorf("Retry-After ignored: got %v", got)
	}
	// A hostile or mistaken Retry-After must not stall the job past the ceiling.
	if got := p.delay(0, time.Hour); got != 10*time.Second {
		t.Errorf("Retry-After not capped at maxDelay: got %v", got)
	}
	// Without a Retry-After the backoff is jittered into [d/2, d].
	if got := p.delay(0, 0); got < 500*time.Millisecond || got > time.Second {
		t.Errorf("jittered delay %v outside [500ms, 1s]", got)
	}
}
