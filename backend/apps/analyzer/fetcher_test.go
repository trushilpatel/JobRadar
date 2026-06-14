package main

import (
	"context"
	stderrors "errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters"
	"github.com/trushilpatel/jobradar/common/ats/core"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
)

// ── resolveJobPost: fat vs thin vs failure dispatch ──────────────────────────

// TestResolveJobPost_FatAdapter_BuildsInlineNoFetch verifies that a summary
// carrying inline JobContent (fat adapter) is converted to a JobPost WITHOUT
// resolving the bundle or calling FetchJobPost.
func TestResolveJobPost_FatAdapter_BuildsInlineNoFetch(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	pf := &fakePostFetcher{}
	w.bundleFn = func(_ string, _ logger.Logger, _ *http.Client) (adapters.AdapterBundle, error) {
		t.Error("bundleFn must not be called for a fat adapter")
		return adapters.AdapterBundle{Post: pf}, nil
	}

	summary := fatSummary("gem-1", "desc body")
	summary.Title = "Senior Engineer"
	post, err := w.resolveJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("resolveJobPost: %v", err)
	}
	if post.Description != "desc body" {
		t.Errorf("description mismatch: %q", post.Description)
	}
	if post.Title != "Senior Engineer" {
		t.Errorf("title should come from summary: %q", post.Title)
	}
	if pf.calls != 0 {
		t.Errorf("FetchJobPost must not be called for fat adapter; calls=%d", pf.calls)
	}
}

// TestResolveJobPost_ThinAdapter_CallsFetchJobPostOnce verifies that a thin
// summary (JobContent==nil) resolves the bundle and calls FetchJobPost exactly
// once, with the URL/identity preserved, and the resulting JobPost flows back.
func TestResolveJobPost_ThinAdapter_CallsFetchJobPostOnce(t *testing.T) {
	pf := &fakePostFetcher{post: core.JobPost{Title: "Senior Engineer", Location: "Austin, TX", Description: "thin detail"}}
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = bundleFnReturning(pf, nil)

	summary := core.JobSummary{ID: "lever-1", Adapter: "lever", URL: "https://jobs.lever.co/co/1"}
	post, err := w.resolveJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("resolveJobPost: %v", err)
	}
	if pf.calls != 1 {
		t.Fatalf("FetchJobPost should be called exactly once; calls=%d", pf.calls)
	}
	if pf.gotSumms[0].URL != summary.URL || pf.gotSumms[0].ID != summary.ID {
		t.Errorf("FetchJobPost summary identity not preserved: %+v", pf.gotSumms[0])
	}
	if post.Title != "Senior Engineer" || post.Description != "thin detail" {
		t.Errorf("fetched post not returned: %+v", post)
	}
}

// TestResolveJobPost_ThinAdapter_RateLimiterWaited verifies the per-adapter rate
// limiter is consulted before the FetchJobPost call on the thin path.
func TestResolveJobPost_ThinAdapter_RateLimiterWaited(t *testing.T) {
	pf := &fakePostFetcher{post: core.JobPost{Title: "T", Location: "NYC"}}
	rl := &countingRateLimiter{}
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = bundleFnReturning(pf, nil)
	w.rateLimiter = rl

	summary := core.JobSummary{ID: "lever-2", Adapter: "lever", URL: "https://jobs.lever.co/co/2"}
	if _, err := w.resolveJobPost(context.Background(), summary); err != nil {
		t.Fatalf("resolveJobPost: %v", err)
	}
	if rl.waits != 1 || rl.lastAdapter != "lever" {
		t.Errorf("expected one rate-limiter Wait for adapter 'lever'; waits=%d adapter=%q", rl.waits, rl.lastAdapter)
	}
	if pf.calls != 1 {
		t.Errorf("expected FetchJobPost after rate-limit wait; calls=%d", pf.calls)
	}
}

// TestResolveJobPost_RateLimiterErrorSkipsFetch verifies a rate-limiter error
// is wrapped and FetchJobPost is never called.
func TestResolveJobPost_RateLimiterErrorSkipsFetch(t *testing.T) {
	pf := &fakePostFetcher{}
	rl := &countingRateLimiter{err: stderrors.New("ctx cancelled")}
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = bundleFnReturning(pf, nil)
	w.rateLimiter = rl

	summary := core.JobSummary{ID: "lever-3", Adapter: "lever", URL: "https://jobs.lever.co/co/3"}
	_, err := w.resolveJobPost(context.Background(), summary)
	if err == nil {
		t.Fatalf("expected rate-limiter error")
	}
	if pf.calls != 0 {
		t.Errorf("FetchJobPost must not run when rate limiter errors; calls=%d", pf.calls)
	}
}

// TestResolveJobPost_ThinAdapter_NilPostFetcher_Errors verifies that when the
// bundle has no PostFetcher (Post==nil) the thin path is a per-job failure —
// there is no generic content fetch fallback anymore.
func TestResolveJobPost_ThinAdapter_NilPostFetcher_Errors(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = bundleFnReturning(nil, nil) // bundle with Post == nil

	summary := core.JobSummary{ID: "deel-1", Adapter: "deel", URL: "https://deel.com/1"}
	if _, err := w.resolveJobPost(context.Background(), summary); err == nil {
		t.Fatalf("expected error when bundle.Post is nil")
	}
}

// TestResolveJobPost_UnknownAdapter_Errors verifies an unknown adapter (bundle
// lookup error) routes to a per-job failure.
func TestResolveJobPost_UnknownAdapter_Errors(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = bundleFnReturning(nil, adapters.ErrInvalidAdapter)

	summary := core.JobSummary{ID: "x-1", Adapter: "totally-unknown", URL: "https://x/1"}
	if _, err := w.resolveJobPost(context.Background(), summary); err == nil {
		t.Fatalf("expected error for unknown adapter")
	}
}

// TestResolveJobPost_FetchJobPostError_Propagates verifies a FetchJobPost error
// is wrapped and returned.
func TestResolveJobPost_FetchJobPostError_Propagates(t *testing.T) {
	pf := &fakePostFetcher{err: stderrors.New("upstream 500")}
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = bundleFnReturning(pf, nil)

	summary := core.JobSummary{ID: "lever-err", Adapter: "lever", URL: "https://jobs.lever.co/co/err"}
	if _, err := w.resolveJobPost(context.Background(), summary); err == nil {
		t.Fatalf("expected FetchJobPost error to propagate")
	}
}

// ── fetchOne: per-job dispatch & isolation ───────────────────────────────────

// TestFetchOne_ThinAdapter_NilPostFetcher_RoutesToFailure verifies that a thin
// summary whose bundle has no PostFetcher is routed to handleFailure (Nak within
// budget) — NOT enriched, no jobs row written.
func TestFetchOne_ThinAdapter_NilPostFetcher_RoutesToFailure(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = bundleFnReturning(nil, nil)
	w.clock = &fakeClock{now: time.Now()}

	summary := core.JobSummary{ID: "deel-2", Adapter: "deel", URL: "https://deel.com/2"}
	msg, state := makeTestMsg(t, summary, 1) // deliveryCount=1 → retryCount=0 → Nak
	result, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if ok {
		t.Fatalf("fetchOne should return ok=false when no post fetcher; got %+v", result)
	}
	if !state.Naked {
		t.Errorf("expected msg Nak'd (within retry budget), got %+v", state)
	}
	if len(pipeline.savedInputs) != 0 {
		t.Errorf("no job should be saved on resolve failure, got %d", len(pipeline.savedInputs))
	}
}

// TestFetchOne_FetchJobPostError_RoutesToFailure verifies a FetchJobPost error
// routes the job to handleFailure and returns ok=false.
func TestFetchOne_FetchJobPostError_RoutesToFailure(t *testing.T) {
	pipeline := &fakePipelineService{}
	pf := &fakePostFetcher{err: stderrors.New("boom")}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = bundleFnReturning(pf, nil)
	w.clock = &fakeClock{now: time.Now()}

	summary := core.JobSummary{ID: "lever-fail", Adapter: "lever", URL: "https://jobs.lever.co/co/f"}
	msg, state := makeTestMsg(t, summary, 1)
	_, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if ok {
		t.Fatalf("fetchOne should return ok=false on FetchJobPost error")
	}
	if !state.Naked {
		t.Errorf("expected msg Nak'd on fetch error, got %+v", state)
	}
}

// TestFetchAll_PerJobIsolation verifies one job's FetchJobPost failure does not
// stop the others in the batch from being fetched successfully.
func TestFetchAll_PerJobIsolation(t *testing.T) {
	// Good fetcher for the success jobs; the failing job uses a different adapter
	// whose bundle returns an error.
	good := &fakePostFetcher{post: core.JobPost{Title: "T", Location: "Denver, CO"}}
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	w.clock = &fakeClock{now: time.Now()}
	w.bundleFn = func(adapter string, _ logger.Logger, _ *http.Client) (adapters.AdapterBundle, error) {
		if adapter == "bad" {
			return adapters.AdapterBundle{}, stderrors.New("lookup boom")
		}
		return adapters.AdapterBundle{Post: good}, nil
	}

	okMsg1, _ := makeTestMsg(t, core.JobSummary{ID: "ok-1", Adapter: "lever", URL: "u1"}, 1)
	badMsg, badState := makeTestMsg(t, core.JobSummary{ID: "bad-1", Adapter: "bad", URL: "u2"}, 1)
	okMsg2, _ := makeTestMsg(t, core.JobSummary{ID: "ok-2", Adapter: "lever", URL: "u3"}, 1)

	results := w.fetchAll(context.Background(), logger.NewNoop(), []*js.Msg{okMsg1, badMsg, okMsg2})

	if len(results) != 2 {
		t.Fatalf("expected 2 successful fetches (per-job isolation), got %d", len(results))
	}
	if !badState.Naked {
		t.Errorf("the failing job should be Nak'd, got %+v", badState)
	}
}

// TestFetchOne_FatAdapter_NonUSAStringMatch_Rejected verifies the post-fetch
// non-USA gate: a fat job whose location string is foreign is rejected.
func TestFetchOne_FatAdapter_NonUSAStringMatch_Rejected(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	summary := fatSummary("gem-india", "Role in Bangalore")
	summary.Location = "Bangalore, India"
	before := nonUSACount()
	msg, state := makeTestMsg(t, summary, 1)
	_, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if ok {
		t.Fatalf("fetchOne should reject non-USA fat job")
	}
	if !state.Acked {
		t.Errorf("expected msg Acked on non-USA rejection, got %+v", state)
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != summary.ID {
		t.Errorf("expected MarkLedgerNonUSA(%q), got %v", summary.ID, pipeline.markCalls)
	}
	if got := nonUSACount() - before; got != 1 {
		t.Errorf("expected non_usa metric +1, got %v", got)
	}
}

// TestFetchOne_ThinAdapter_CountryCodeNonUS_Rejected verifies the post-fetch
// CountryCode gate rejects a thin job whose fetched post has a non-US code.
func TestFetchOne_ThinAdapter_CountryCodeNonUS_Rejected(t *testing.T) {
	pipeline := &fakePipelineService{}
	pf := &fakePostFetcher{post: core.JobPost{Title: "Eng", Location: "Bengaluru", CountryCode: "IN"}}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = bundleFnReturning(pf, nil)

	summary := core.JobSummary{ID: "wd-in", Adapter: "workday", URL: "https://wd/1"}
	before := nonUSACount()
	msg, state := makeTestMsg(t, summary, 1)
	_, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if ok {
		t.Fatalf("fetchOne should reject CountryCode=IN job")
	}
	if !state.Acked {
		t.Errorf("expected msg Acked on country_code rejection, got %+v", state)
	}
	if len(pipeline.markCalls) != 1 || pipeline.markCalls[0] != summary.ID {
		t.Errorf("expected MarkLedgerNonUSA(%q), got %v", summary.ID, pipeline.markCalls)
	}
	if got := nonUSACount() - before; got != 1 {
		t.Errorf("expected non_usa metric +1, got %v", got)
	}
}

// TestFetchOne_ThinAdapter_CountryCodeUS_PassesThrough verifies a US country
// code keeps the job (returns ok=true, not rejected).
func TestFetchOne_ThinAdapter_CountryCodeUS_PassesThrough(t *testing.T) {
	pipeline := &fakePipelineService{}
	pf := &fakePostFetcher{post: core.JobPost{Title: "Eng", Location: "Seattle, WA", CountryCode: "US", Description: "d"}}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = bundleFnReturning(pf, nil)

	summary := core.JobSummary{ID: "wd-us", Adapter: "workday", URL: "https://wd/2"}
	msg, state := makeTestMsg(t, summary, 1)
	result, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if !ok {
		t.Fatalf("fetchOne should pass through CountryCode=US job")
	}
	if state.Acked || state.Termed {
		t.Errorf("msg must not be disposed for US job, got %+v", state)
	}
	if result.summary.ID != summary.ID || result.post.Title != "Eng" {
		t.Errorf("unexpected fetched job: %+v", result)
	}
}

// TestFetchOne_LocationKnownTrue_NonUSA_EarlyGateNoFetch verifies the early gate:
// LocationKnown=true non-USA jobs are rejected before any bundle resolution.
func TestFetchOne_LocationKnownTrue_NonUSA_EarlyGateNoFetch(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})
	w.bundleFn = func(_ string, _ logger.Logger, _ *http.Client) (adapters.AdapterBundle, error) {
		t.Error("bundleFn must not be called when the early gate rejects")
		return adapters.AdapterBundle{}, stderrors.New("must not resolve")
	}

	summary := core.JobSummary{
		ID: "lever-uk", Adapter: "lever", Location: "London, UK",
		LocationKnown: true, AllLocations: []string{"London, UK"},
	}
	before := nonUSACount()
	msg, state := makeTestMsg(t, summary, 1)
	_, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)

	if ok {
		t.Fatalf("fetchOne should reject LocationKnown=true non-USA job at the early gate")
	}
	if !state.Acked {
		t.Errorf("expected msg Acked on early non-USA rejection, got %+v", state)
	}
	if got := nonUSACount() - before; got != 1 {
		t.Errorf("expected non_usa metric +1, got %v", got)
	}
}

// TestFetchOne_Malformed_Termed verifies a malformed message is Term'd.
func TestFetchOne_Malformed_Termed(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakePipelineService{}, &fakePublisher{}, &fakeLedger{})
	msg, state := js.NewTestMsg([]byte("not json"), 1)
	_, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)
	if ok {
		t.Fatalf("fetchOne should return ok=false for malformed message")
	}
	if !state.Termed {
		t.Errorf("expected malformed message Term'd, got %+v", state)
	}
}

// TestFetchOne_FatAdapter_USA_PassesThrough verifies a USA fat job passes through.
func TestFetchOne_FatAdapter_USA_PassesThrough(t *testing.T) {
	pipeline := &fakePipelineService{}
	w := makeWorker(&fakeScorer{}, pipeline, &fakePublisher{}, &fakeLedger{})

	summary := fatSummary("gem-usa", "Great US role")
	summary.Location = "San Francisco, CA"
	msg, state := makeTestMsg(t, summary, 1)
	result, ok := w.fetchOne(context.Background(), logger.NewNoop(), msg)
	if !ok {
		t.Fatalf("fetchOne should pass through a USA fat job")
	}
	if state.Acked || state.Termed {
		t.Errorf("msg must not be disposed for USA fat job, got %+v", state)
	}
	if result.summary.ID != summary.ID {
		t.Errorf("unexpected job: %+v", result)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// countingRateLimiter records Wait calls and can return a programmed error.
type countingRateLimiter struct {
	mu          sync.Mutex
	waits       int
	lastAdapter string
	err         error
}

func (c *countingRateLimiter) Wait(_ context.Context, adapter string) error {
	c.mu.Lock()
	c.waits++
	c.lastAdapter = adapter
	c.mu.Unlock()
	return c.err
}

var _ RateLimiter = (*countingRateLimiter)(nil)

func nonUSACount() float64 {
	return counterValue(metrics.EnricherJobsTotal.WithLabelValues("non_usa"))
}

// ── AdapterRateLimiter unit tests ─────────────────────────────────────────────

// TestAdapterRateLimiter_GetOrCreate_CachedReturnsSameLimiter verifies that
// calling getOrCreate twice for the same adapter returns the same limiter.
func TestAdapterRateLimiter_GetOrCreate_CachedReturnsSameLimiter(t *testing.T) {
	rl := NewAdapterRateLimiter(100.0, 10, 0, logger.NewNoop())
	l1 := rl.getOrCreate("greenhouse")
	l2 := rl.getOrCreate("greenhouse")
	if l1 != l2 {
		t.Error("expected the same limiter instance on second call (cached)")
	}
}

// TestAdapterRateLimiter_Wait_Success verifies Wait completes without error and
// the cached path is reused.
func TestAdapterRateLimiter_Wait_Success(t *testing.T) {
	rl := NewAdapterRateLimiter(1000.0, 100, 0, logger.NewNoop())
	if err := rl.Wait(context.Background(), "lever"); err != nil {
		t.Errorf("Wait: unexpected error: %v", err)
	}
	if err := rl.Wait(context.Background(), "lever"); err != nil {
		t.Errorf("Wait (cached): unexpected error: %v", err)
	}
}

// TestAdapterRateLimiter_Wait_CtxCancelWhileWaiting verifies Wait returns the
// context error when cancelled while the token-bucket is exhausted.
func TestAdapterRateLimiter_Wait_CtxCancelWhileWaiting(t *testing.T) {
	rl := NewAdapterRateLimiter(0.001, 1, 0, logger.NewNoop())
	_ = rl.Wait(context.Background(), "ashby")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rl.Wait(ctx, "ashby"); err == nil {
		t.Error("expected context error, got nil")
	}
}

// TestAdapterRateLimiter_Wait_JitterCtxCancel verifies a jitter-phase context
// cancellation returns ctx.Err().
func TestAdapterRateLimiter_Wait_JitterCtxCancel(t *testing.T) {
	rl := NewAdapterRateLimiter(1000.0, 100, 10*time.Second, logger.NewNoop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rl.Wait(ctx, "ashby"); err == nil {
		t.Error("expected context error from jitter phase, got nil")
	}
}

// TestAdapterRateLimiter_GetOrCreate_ConcurrentSameAdapter verifies concurrent
// getOrCreate calls for the same adapter return one shared limiter.
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
