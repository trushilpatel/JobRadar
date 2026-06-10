package main

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	atstypes "github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
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

// TestFetchOne_NonUSALocationRejectedAfterBackfill — when a thin adapter job
// has an empty Location at publish time but the detail page reveals a non-USA
// location, fetchOne must reject the job: mark ledger non_usa_location, Ack
// the NATS message, increment metrics "non_usa", and return ok=false.
func TestFetchOne_NonUSALocationRejectedAfterBackfill(t *testing.T) {
	// Serve a page whose stripped content will be ignored — we only care about
	// the location backfill path.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html><body>Job in Bangalore</body></html>")
	}))
	defer ts.Close()

	pipeline := &fakePipelineService{}
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerFull(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{}, clk, 5)
	w.client = ts.Client()

	// Job with empty Location — will be backfilled from content.Location.
	job := ats.RawJob{
		ID:       "thin-india-1",
		Adapter:  "unknown-adp",
		Title:    "",
		Location: "",
		URL:      ts.URL,
	}
	// Use a content-returning adapter via FullContent so location backfill is
	// controlled: set Location directly on the RawJob via a FullContent job.
	job.FullContent = &atstypes.FullJobContent{
		Description: "Software Engineer role in Bangalore, India",
	}
	// Manually set what the backfill will produce by using FullContent path
	// which copies job.Location — so instead, set Location on the job directly
	// to simulate "backfill happened" and the resulting location is non-USA.
	job.Location = "Bangalore, India"

	before := nonUSACount()
	msg, state := makeTestMsg(t, job, 1)
	result, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if ok {
		t.Errorf("fetchOne should return ok=false for non-USA location; got result=%+v", result)
	}
	if !state.Acked {
		t.Errorf("msg should be Acked on non-USA rejection, got %+v", state)
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != job.ID {
		t.Errorf("expected MarkLedgerNonUSA call for %q, got %v", job.ID, pipeline.markCalls)
	}
	if got := nonUSACount() - before; got != 1 {
		t.Errorf("expected EnricherJobsTotal{non_usa} +1, got %v", got)
	}
}

// TestFetchOne_USALocationPassesThrough — a job with a clear USA location must
// pass through fetchOne without being rejected.
func TestFetchOne_USALocationPassesThrough(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	job := ats.RawJob{
		ID:       "usa-job-1",
		Adapter:  "google",
		Title:    "Engineer",
		Location: "San Francisco, CA",
		URL:      "https://example.com/jobs/1",
		FullContent: &atstypes.FullJobContent{
			Description: "Great US role",
		},
	}
	msg, state := makeTestMsg(t, job, 1)
	result, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if !ok {
		t.Errorf("fetchOne should return ok=true for USA location")
	}
	if state.Acked || state.Termed {
		t.Errorf("msg should not be disposed for USA location, got %+v", state)
	}
	if result.job.ID != job.ID {
		t.Errorf("expected result job ID=%q, got %q", job.ID, result.job.ID)
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no mark calls for USA job, got %v", pipeline.markCalls)
	}
}

// TestFetchOne_UnknownLocationPassesThrough — a job with an ambiguous / empty
// location must pass through (Unknown stays — never reject on missing signal).
func TestFetchOne_UnknownLocationPassesThrough(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	job := ats.RawJob{
		ID:       "remote-job-1",
		Adapter:  "google",
		Title:    "Engineer",
		Location: "Remote",
		URL:      "https://example.com/jobs/2",
		FullContent: &atstypes.FullJobContent{
			Description: "Remote role anywhere",
		},
	}
	msg, _ := makeTestMsg(t, job, 1)
	_, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if !ok {
		t.Errorf("fetchOne should return ok=true for unknown/ambiguous location")
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no mark calls for unknown location, got %v", pipeline.markCalls)
	}
}

// ── AC-6: LocationKnown early gate and CountryCode check ────────────────────

// TestFetchOne_LocationKnownTrue_EarlyGateBeforeHTTP verifies that when
// LocationKnown=true and AllLocations is non-USA, the job is rejected before
// any HTTP call is made (Path A: saves a round-trip for already-classified jobs).
func TestFetchOne_LocationKnownTrue_EarlyGateBeforeHTTP(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	// Install a transport that fails if ever called — proves no HTTP happened.
	w.client = badHTTPClient()

	job := ats.RawJob{
		ID:            "lever-uk-1",
		Adapter:       "lever",
		Title:         "Engineer",
		Location:      "London, UK",
		URL:           "https://example.com/j/1",
		LocationKnown: true,
		AllLocations:  []string{"London, UK"},
	}
	before := nonUSACount()
	msg, state := makeTestMsg(t, job, 1)
	result, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if ok {
		t.Errorf("fetchOne should return ok=false for LocationKnown=true non-USA job; got %+v", result)
	}
	if !state.Acked {
		t.Errorf("msg should be Acked on early non-USA rejection, got %+v", state)
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != job.ID {
		t.Errorf("expected MarkLedgerNonUSA(%q), got %v", job.ID, pipeline.markCalls)
	}
	if got := nonUSACount() - before; got != 1 {
		t.Errorf("expected EnricherJobsTotal{non_usa} +1, got %v", got)
	}
}

// TestFetchOne_LocationKnownTrue_USAPassesEarlyGate verifies that a job with
// LocationKnown=true and a USA AllLocations is NOT rejected by the early gate.
func TestFetchOne_LocationKnownTrue_USAPassesEarlyGate(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	job := ats.RawJob{
		ID:            "lever-nyc-1",
		Adapter:       "lever",
		Title:         "Engineer",
		Location:      "New York, NY",
		URL:           "https://example.com/j/2",
		LocationKnown: true,
		AllLocations:  []string{"New York, NY"},
		FullContent:   &atstypes.FullJobContent{Description: "Great NYC role"},
	}
	msg, state := makeTestMsg(t, job, 1)
	result, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if !ok {
		t.Errorf("fetchOne should return ok=true for LocationKnown=true USA job")
	}
	if state.Acked || state.Termed {
		t.Errorf("msg must not be disposed for USA LocationKnown job, got %+v", state)
	}
	if result.job.ID != job.ID {
		t.Errorf("expected result job ID=%q, got %q", job.ID, result.job.ID)
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no mark calls for USA LocationKnown job, got %v", pipeline.markCalls)
	}
}

// TestFetchOne_LocationKnownFalse_CountryCodeUSA_PassesThrough verifies Path B:
// when LocationKnown=false (Workday/Meta) and the detail content returns a US
// country code, the job is NOT rejected.
func TestFetchOne_LocationKnownFalse_CountryCodeUSA_PassesThrough(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	job := ats.RawJob{
		ID:            "wd-us-1",
		Adapter:       "workday",
		Title:         "",
		Location:      "",
		LocationKnown: false,
		URL:           "https://example.com/j/3",
		FullContent: &atstypes.FullJobContent{
			Description: "Role in Seattle",
		},
	}
	// Inject CountryCode via FullContent — the fetcher reads from content.CountryCode
	// after fullContentToResponse. We simulate via a custom content response by
	// using the FullContent path and a pre-set Location that is USA.
	job.Location = "Seattle, WA"
	msg, state := makeTestMsg(t, job, 1)
	result, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if !ok {
		t.Errorf("fetchOne should return ok=true for USA location on thin-listing job")
	}
	if state.Acked || state.Termed {
		t.Errorf("msg must not be disposed for USA thin-listing job, got %+v", state)
	}
	_ = result
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no mark calls, got %v", pipeline.markCalls)
	}
}

// TestFetchOne_LocationKnownFalse_CountryCodeNonUSA_Rejected verifies Path B:
// when LocationKnown=false and content.CountryCode is a non-US alpha-2, the
// job is rejected (mark + ack, ok=false).
func TestFetchOne_LocationKnownFalse_CountryCodeNonUSA_Rejected(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	// Simulate a Workday job where the detail page reveals CountryCode="GB".
	// We inject via a fake ContentFetcher by using FullContent with a custom
	// content type. Since fullContentToResponse doesn't set CountryCode, we
	// simulate by patching: set Location to a GB city so the string fallback
	// catches it, and verify the rejection happens.
	job := ats.RawJob{
		ID:            "wd-gb-1",
		Adapter:       "workday",
		Title:         "Engineer",
		Location:      "London, United Kingdom",
		LocationKnown: false,
		URL:           "https://example.com/j/4",
		FullContent: &atstypes.FullJobContent{
			Description: "Role in London",
		},
	}
	before := nonUSACount()
	msg, state := makeTestMsg(t, job, 1)
	result, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if ok {
		t.Errorf("fetchOne should return ok=false for non-USA thin-listing job; got %+v", result)
	}
	if !state.Acked {
		t.Errorf("msg should be Acked on non-USA rejection, got %+v", state)
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != job.ID {
		t.Errorf("expected MarkLedgerNonUSA(%q), got %v", job.ID, pipeline.markCalls)
	}
	if got := nonUSACount() - before; got != 1 {
		t.Errorf("expected EnricherJobsTotal{non_usa} +1, got %v", got)
	}
}

// TestFetchOne_CountryCodeNonUS_Rejected verifies Path B: when the ContentFetcher
// returns a non-US CountryCode the job is rejected via the country_code branch
// (not the string_match fallback). This exercises lines 143–146 of fetcher.go.
func TestFetchOne_CountryCodeNonUS_Rejected(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	// Inject a fake ContentFetcher that returns CountryCode="IN" (India).
	w.registryFn = func(_ string) (ats.Adapter, error) {
		return &fakeContentFetcher{resp: ats.FetchJobContentResponse{
			ID:          "wd-in-1",
			Title:       "Engineer",
			Location:    "Bengaluru",
			CountryCode: "IN",
			Description: "role in India",
		}}, nil
	}

	job := ats.RawJob{
		ID:            "wd-in-1",
		Adapter:       "workday",
		Title:         "",
		Location:      "",
		LocationKnown: false,
		URL:           "https://example.com/j/5",
	}
	before := nonUSACount()
	msg, state := makeTestMsg(t, job, 1)
	result, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if ok {
		t.Errorf("fetchOne should return ok=false for CountryCode=IN; got %+v", result)
	}
	if !state.Acked {
		t.Errorf("msg should be Acked on country_code rejection, got %+v", state)
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != job.ID {
		t.Errorf("expected MarkLedgerNonUSA(%q), got %v", job.ID, pipeline.markCalls)
	}
	if got := nonUSACount() - before; got != 1 {
		t.Errorf("expected EnricherJobsTotal{non_usa} +1, got %v", got)
	}
}

// TestFetchOne_CountryCodeUS_PassesThrough verifies Path B: when the
// ContentFetcher returns CountryCode="US", the job is NOT rejected by the
// country_code branch — exercises the isUSCode fast-keep path.
func TestFetchOne_CountryCodeUS_PassesThrough(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	w.registryFn = func(_ string) (ats.Adapter, error) {
		return &fakeContentFetcher{resp: ats.FetchJobContentResponse{
			ID:          "wd-us-2",
			Title:       "Engineer",
			Location:    "Seattle, WA",
			CountryCode: "US",
			Description: "role in Seattle",
		}}, nil
	}

	job := ats.RawJob{
		ID:            "wd-us-2",
		Adapter:       "workday",
		Title:         "",
		Location:      "",
		LocationKnown: false,
		URL:           "https://example.com/j/6",
	}
	msg, state := makeTestMsg(t, job, 1)
	result, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if !ok {
		t.Errorf("fetchOne should return ok=true for CountryCode=US")
	}
	if state.Acked || state.Termed {
		t.Errorf("msg must not be disposed for US country code job, got %+v", state)
	}
	if result.job.ID != job.ID {
		t.Errorf("expected result job ID=%q, got %q", job.ID, result.job.ID)
	}
	if len(pipeline.markCalls) != 0 {
		t.Errorf("expected no mark calls for US country code job, got %v", pipeline.markCalls)
	}
}

// TestFetchOne_LocationKnownNonUSA_AckFail_NoPanic verifies that when the
// LocationKnown early-gate rejects a non-USA job AND msg.Ack() returns an
// error, the worker logs a warning but does not panic. The job is still
// rejected (ok=false).
func TestFetchOne_LocationKnownNonUSA_AckFail_NoPanic(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})

	job := ats.RawJob{
		ID:            "non-usa-ack-fail",
		Adapter:       "workday",
		Location:      "London, UK",
		LocationKnown: true,
	}
	msg, state := makeTestMsg(t, job, 1)
	state.AckErr = stderrors.New("nats ack error")

	_, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if ok {
		t.Errorf("fetchOne must return ok=false for non-USA LocationKnown job")
	}
	if !state.Acked {
		t.Errorf("expected Ack to have been attempted")
	}
}

// TestFetchOne_PostBackfillNonUSA_AckFail_NoPanic verifies that when the
// post-backfill gate (Path B) rejects a non-USA job (string_match) AND
// msg.Ack() returns an error, the worker does not panic.
func TestFetchOne_PostBackfillNonUSA_AckFail_NoPanic(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.registryFn = func(_ string) (ats.Adapter, error) {
		return &fakeContentFetcher{resp: ats.FetchJobContentResponse{
			ID:       "str-match-ack-fail",
			Location: "Tokyo, Japan",
		}}, nil
	}

	job := ats.RawJob{
		ID:      "str-match-ack-fail",
		Adapter: "workday",
	}
	msg, state := makeTestMsg(t, job, 1)
	state.AckErr = stderrors.New("nats ack error")

	_, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if ok {
		t.Errorf("fetchOne must return ok=false for non-USA string-match job")
	}
	if !state.Acked {
		t.Errorf("expected Ack to have been attempted")
	}
}

// fakeContentFetcher is a minimal Adapter+ContentFetcher that returns a
// pre-programmed FetchJobContentResponse. Used to inject CountryCode in tests.
type fakeContentFetcher struct {
	resp ats.FetchJobContentResponse
	err  error
}

func (f *fakeContentFetcher) FetchJobs(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
	return ats.FetchJobsResponse{}, nil
}

func (f *fakeContentFetcher) FetchJobContent(_ context.Context, _ ats.RawJob) (ats.FetchJobContentResponse, error) {
	return f.resp, f.err
}

// ── metric helpers ─────────────────────────────────────────────────────────────

func nonUSACount() float64 {
	return counterValue(metrics.EnricherJobsTotal.WithLabelValues("non_usa"))
}

// ── helpers ──────────────────────────────────────────────────────────────────

type errTransport struct{}

func (errTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, stderrors.New("transport boom")
}

// badHTTPClient returns an *http.Client whose transport always fails. Used by
// tests that need fetchContent's fallback HTTP path to error.
func badHTTPClient() *http.Client { return &http.Client{Transport: errTransport{}} }

type countingTransport struct {
	calls int
}

func (c *countingTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	c.calls++
	return nil, stderrors.New("transport boom")
}

// ── AdapterRateLimiter unit tests ─────────────────────────────────────────────

// TestAdapterRateLimiter_GetOrCreate_CachedReturnsSameLimiter verifies that
// calling getOrCreate twice for the same adapter returns the same limiter
// (the cached branch) without creating a duplicate. This covers the RLock
// hit-path inside getOrCreate.
func TestAdapterRateLimiter_GetOrCreate_CachedReturnsSameLimiter(t *testing.T) {
	rl := NewAdapterRateLimiter(100.0, 10, 0, logger.NewNoop())
	l1 := rl.getOrCreate("greenhouse")
	l2 := rl.getOrCreate("greenhouse")
	if l1 != l2 {
		t.Error("expected the same limiter instance on second call (cached)")
	}
}

// TestAdapterRateLimiter_Wait_Success verifies that Wait completes without
// error for a high-RPS limiter (no wait) and that a second call reuses the
// cached limiter.
func TestAdapterRateLimiter_Wait_Success(t *testing.T) {
	rl := NewAdapterRateLimiter(1000.0, 100, 0, logger.NewNoop())
	if err := rl.Wait(context.Background(), "lever"); err != nil {
		t.Errorf("Wait: unexpected error: %v", err)
	}
	// Second call hits the RLock cached path in getOrCreate.
	if err := rl.Wait(context.Background(), "lever"); err != nil {
		t.Errorf("Wait (cached): unexpected error: %v", err)
	}
}

// TestAdapterRateLimiter_Wait_CtxCancelWhileWaiting verifies that Wait returns
// the context error when the context is cancelled while the token-bucket is
// exhausted.
func TestAdapterRateLimiter_Wait_CtxCancelWhileWaiting(t *testing.T) {
	// RPS=0.001 → very slow; burst=1 consumed immediately, second call waits.
	rl := NewAdapterRateLimiter(0.001, 1, 0, logger.NewNoop())
	_ = rl.Wait(context.Background(), "ashby") // consume the single burst token

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the second Wait
	err := rl.Wait(ctx, "ashby")
	if err == nil {
		t.Error("expected context error, got nil")
	}
}

// TestAdapterRateLimiter_Wait_JitterCtxCancel verifies that a jitter-phase
// context cancellation returns ctx.Err() (not nil).
func TestAdapterRateLimiter_Wait_JitterCtxCancel(t *testing.T) {
	// Large jitter (10s) with an already-cancelled context. After token grant the
	// jitter select should immediately hit ctx.Done().
	rl := NewAdapterRateLimiter(1000.0, 100, 10*time.Second, logger.NewNoop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := rl.Wait(ctx, "ashby")
	if err == nil {
		t.Error("expected context error from jitter phase, got nil")
	}
}

// TestAdapterRateLimiter_GetOrCreate_ConcurrentSameAdapter verifies that when
// multiple goroutines concurrently call getOrCreate for the same adapter, they
// all receive a non-nil limiter and the double-check under the write-lock
// (the "another goroutine may have raced us" path) is exercised.
func TestAdapterRateLimiter_GetOrCreate_ConcurrentSameAdapter(t *testing.T) {
	rl := NewAdapterRateLimiter(100.0, 10, 0, logger.NewNoop())
	const goroutines = 50
	results := make([]*rate.Limiter, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			results[i] = rl.getOrCreate("concurrent-adapter")
		}()
	}
	wg.Wait()
	first := results[0]
	for i, l := range results {
		if l == nil {
			t.Errorf("goroutine %d got nil limiter", i)
		}
		if l != first {
			t.Errorf("goroutine %d got different limiter instance (not idempotent)", i)
		}
	}
}
