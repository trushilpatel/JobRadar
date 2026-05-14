package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	atstypes "github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
)

func newFullContentRawJob(adapter, id, title, url, description string) ats.RawJob {
	return ats.RawJob{
		ID:      id,
		Adapter: adapter,
		Title:   title,
		URL:     url,
		FullContent: &atstypes.FullJobContent{
			Description: description,
		},
	}
}

// TestFetchPage_OkReturnsBody — happy path: GET 200 returns the body string.
func TestFetchPage_OkReturnsBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); !strings.Contains(got, "JobRadar") {
			t.Errorf("expected User-Agent to identify JobRadar, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "<html><body>hi</body></html>")
	}))
	defer ts.Close()

	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.client = ts.Client()

	body, err := w.fetchPage(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	if !strings.Contains(body, "hi") {
		t.Errorf("body should contain 'hi'; got %q", body)
	}
}

// TestFetchPage_NonOKStatusReturnsError — any non-200 status is an error.
func TestFetchPage_NonOKStatusReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.client = ts.Client()

	if _, err := w.fetchPage(context.Background(), ts.URL); err == nil {
		t.Errorf("expected error on 500, got nil")
	}
}

// TestFetchPage_InvalidURLReturnsError — NewRequest fails on a malformed URL
// like a control-character; the wrapped error contains "build request".
func TestFetchPage_InvalidURLReturnsError(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	_, err := w.fetchPage(context.Background(), "http://\x7f.invalid/")
	if err == nil {
		t.Fatalf("expected error from malformed URL")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Errorf("expected 'build request' in error, got %v", err)
	}
}

// TestFetchPage_HTTPClientErrorWrapped — transport-level errors return a
// wrapped "get: ..." error.
func TestFetchPage_HTTPClientErrorWrapped(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.client = &http.Client{Transport: errTransport{}}
	if _, err := w.fetchPage(context.Background(), "http://example.invalid/"); err == nil {
		t.Errorf("expected transport error")
	}
}

// TestFetchPage_LargeBodyTruncatedAt50KB — body reader is wrapped in a
// LimitReader(50KB). A response far larger than 50KB is truncated.
func TestFetchPage_LargeBodyTruncatedAt50KB(t *testing.T) {
	huge := strings.Repeat("A", 200*1024)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, huge)
	}))
	defer ts.Close()

	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.client = ts.Client()

	body, err := w.fetchPage(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	if len(body) != 50*1024 {
		t.Errorf("expected truncation to 50KB, got %d bytes", len(body))
	}
}

// TestFetchContent_FullContentSkipsHTTPAndRateLimiter — when FullContent is
// set, fetchContent must NOT consult the rate limiter or HTTP client.
func TestFetchContent_FullContentSkipsHTTPAndRateLimiter(t *testing.T) {
	rl := NewAdapterRateLimiter(1.0, 1, 0, logger.NewNoop())
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.rateLimiter = rl
	w.client = &http.Client{Transport: errTransport{}}

	fc := newFullContentRawJob("uber", "uber-1", "Senior", "https://example.com/u1", "desc body")
	resp, err := w.fetchContent(context.Background(), fc)
	if err != nil {
		t.Fatalf("fetchContent: %v", err)
	}
	if resp.Description != "desc body" {
		t.Errorf("description mismatch: %q", resp.Description)
	}
}

// TestFetchContent_UnknownAdapterFallsBackToHTTP — when the adapter is unknown
// (Registry returns error), fetchContent falls back to fetchPage and returns
// the stripped HTML as Description.
func TestFetchContent_UnknownAdapterFallsBackToHTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html><p>Hello <b>World</b></p></html>")
	}))
	defer ts.Close()

	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.client = ts.Client()

	job := ats.RawJob{
		ID:       "x-1",
		Adapter:  "totally-unknown-adapter",
		URL:      ts.URL,
		Title:    "T",
		Location: "L",
	}
	resp, err := w.fetchContent(context.Background(), job)
	if err != nil {
		t.Fatalf("fetchContent: %v", err)
	}
	if !strings.Contains(resp.Description, "Hello World") {
		t.Errorf("expected stripped HTML 'Hello World' in description, got %q", resp.Description)
	}
	if resp.ID != "x-1" || resp.Title != "T" || resp.Location != "L" || resp.URL != ts.URL {
		t.Errorf("expected job fields copied through, got %+v", resp)
	}
}

// TestFetchContent_RateLimiterErrorWrapped — when the limiter Wait fails (ctx
// cancel before granted), fetchContent returns a wrapped "rate limit wait"
// error and never hits the HTTP client.
func TestFetchContent_RateLimiterErrorWrapped(t *testing.T) {
	// Burst=1 + slow rps; after one grant the next wait will block.
	rl := NewAdapterRateLimiter(0.5, 1, 0, logger.NewNoop())
	// Drain the bucket.
	_ = rl.Wait(context.Background(), "ashby")

	transport := &countingTransport{}
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.rateLimiter = rl
	w.client = &http.Client{Transport: transport}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	_, err := w.fetchContent(ctx, ats.RawJob{ID: "x", Adapter: "ashby", URL: "http://nope.invalid/"})
	if err == nil {
		t.Fatalf("expected error from rate-limiter cancel")
	}
	if !strings.Contains(err.Error(), "rate limit wait") {
		t.Errorf("expected wrapped 'rate limit wait' error, got %v", err)
	}
	if transport.calls != 0 {
		t.Errorf("HTTP client should NOT be called when rate limiter fails; calls=%d", transport.calls)
	}
}

// TestFetchContent_NilRateLimiterSkipsLimiter — fetchContent must tolerate
// rateLimiter==nil and proceed straight to adapter/http fallback.
func TestFetchContent_NilRateLimiterSkipsLimiter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "body")
	}))
	defer ts.Close()

	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.rateLimiter = nil
	w.client = ts.Client()

	_, err := w.fetchContent(context.Background(), ats.RawJob{ID: "x", Adapter: "unknown-adp", URL: ts.URL})
	if err != nil {
		t.Errorf("expected success with nil rate limiter, got %v", err)
	}
}

// TestFetchContent_EmptyAdapterSkipsRateLimiter — adapter=="" must NOT
// consult the rate limiter (it would create a key under the empty string).
func TestFetchContent_EmptyAdapterSkipsRateLimiter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "body")
	}))
	defer ts.Close()

	// burst=0 → any rate-limiter call would error.
	// (We can't actually construct rps=0 burst=0; instead drain a small bucket.)
	rl := NewAdapterRateLimiter(1.0, 1, 0, logger.NewNoop())
	_ = rl.Wait(context.Background(), "")
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.rateLimiter = rl
	w.client = ts.Client()

	// Adapter=="" skips the limiter entirely.
	_, err := w.fetchContent(context.Background(), ats.RawJob{ID: "x", Adapter: "", URL: ts.URL})
	if err != nil {
		t.Errorf("expected success with empty adapter, got %v", err)
	}
}

// TestFetchContent_HTTPFallbackError — fallback HTTP fetch errors propagate.
func TestFetchContent_HTTPFallbackError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.client = ts.Client()
	_, err := w.fetchContent(context.Background(), ats.RawJob{ID: "x", Adapter: "unknown", URL: ts.URL})
	if err == nil {
		t.Errorf("expected error on 502 fallback")
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

type errTransport struct{}

func (errTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, errors.New("transport boom")
}

// badHTTPClient returns an *http.Client whose transport always fails. Used by
// tests that need fetchContent's fallback HTTP path to error.
func badHTTPClient() *http.Client { return &http.Client{Transport: errTransport{}} }

type countingTransport struct {
	calls int
}

func (c *countingTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	c.calls++
	return nil, errors.New("transport boom")
}
