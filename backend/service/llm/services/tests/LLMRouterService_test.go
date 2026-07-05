package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/services"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

func TestRouter_FallsBackOnLocalQuota(t *testing.T) {
	quota := &fakeScorer{scoreErr: llmerr.ErrLLMLocalQuotaExceeded}
	ok := &fakeScorer{scoreOut: []string{"id-1"}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: quota},
		{SlotID: "s2", Model: "m1", Scorer: ok},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))

	out, err := r.ScoreBatch(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] != "id-1" {
		t.Fatalf("unexpected %v", out)
	}
}

func TestRouter_RecordsCooldownOnProvider429(t *testing.T) {
	limited := &fakeScorer{scoreErr: llmerr.ErrLLMProviderRateLimited}
	ok := &fakeScorer{scoreOut: []string{"id-1"}}
	cd := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: 10 * time.Second, Max: time.Minute})
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: limited},
		{SlotID: "s2", Model: "m1", Scorer: ok},
	}, cd)

	if _, err := r.ScoreBatch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	st, err := cd.Status(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if st.FailureCount != 1 {
		t.Fatalf("want cooldown recorded, got %+v", st)
	}
}

func TestRouter_BubblesOtherErrors(t *testing.T) {
	boom := &fakeScorer{scoreErr: errors.New("network boom")}
	ok := &fakeScorer{scoreOut: []string{"x"}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: boom},
		{SlotID: "s2", Model: "m1", Scorer: ok},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))

	_, err := r.ScoreBatch(context.Background(), nil)
	if err == nil || err.Error() != "network boom" {
		t.Fatalf("expected network boom to bubble, got %v", err)
	}
}

func TestRouter_ReturnsAllExhaustedError(t *testing.T) {
	q1 := &fakeScorer{scoreErr: llmerr.ErrLLMLocalQuotaExceeded}
	q2 := &fakeScorer{scoreErr: llmerr.ErrLLMLocalQuotaExceeded}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: q1},
		{SlotID: "s2", Model: "m1", Scorer: q2},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))

	_, err := r.ScoreBatch(context.Background(), nil)
	var ae *llmerr.LLMAllExhaustedError
	if !errors.As(err, &ae) {
		t.Fatalf("want LLMAllExhaustedError, got %v", err)
	}
	if len(ae.Slots) != 2 {
		t.Fatalf("want 2 slot statuses, got %d", len(ae.Slots))
	}
}

func TestRouter_SkipsCooledSlotAndUsesNext(t *testing.T) {
	// Pre-cool slot 1; router must skip and call slot 2.
	cd := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Hour, Max: 24 * time.Hour})
	_ = cd.RecordFailure(context.Background(), "s1")

	called1 := &fakeScorer{scoreErr: errors.New("should not be called")}
	ok := &fakeScorer{scoreOut: []string{"x"}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: called1},
		{SlotID: "s2", Model: "m2", Scorer: ok},
	}, cd)

	out, err := r.ScoreBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0] != "x" {
		t.Fatalf("expected fallback slot output, got %v", out)
	}
}

func TestRouter_SuccessClearsCooldown(t *testing.T) {
	cd := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: 10 * time.Second, Max: time.Minute})
	_ = cd.RecordFailure(context.Background(), "s1") // count -> 1, but cooldown still active

	// In production we don't trigger this path while still cooling, but if a
	// success ever lands the count must reset. Use a far-future cooldown so we
	// still call into the scorer.
	cd2 := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Microsecond, Max: time.Microsecond})
	_ = cd2.RecordFailure(context.Background(), "s1")
	// Sleep one ms so cooldown expires.
	time.Sleep(2 * time.Millisecond)

	ok := &fakeScorer{scoreOut: []string{"a"}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: ok},
	}, cd2)

	if _, err := r.ScoreBatch(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st, _ := cd2.Status(context.Background(), "s1")
	if st.FailureCount != 0 {
		t.Fatalf("expected failure count to reset to 0 after success, got %d", st.FailureCount)
	}
}

func TestRouter_EnrichBatch_FallsBackOnLocalQuota(t *testing.T) {
	quota := &fakeScorer{enrichErr: llmerr.ErrLLMLocalQuotaExceeded}
	ok := &fakeScorer{enrichOut: []*dto.JobPostEnrichResult{{JobID: "j1"}}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: quota},
		{SlotID: "s2", Model: "m2", Scorer: ok},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))

	out, err := r.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j1", RawText: "x"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].JobID != "j1" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestRouter_EnrichBatch_RecordsCooldownOnProvider429(t *testing.T) {
	limited := &fakeScorer{enrichErr: llmerr.ErrLLMProviderRateLimited}
	ok := &fakeScorer{enrichOut: []*dto.JobPostEnrichResult{{JobID: "j1"}}}
	cd := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: 10 * time.Second, Max: time.Minute})
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: limited},
		{SlotID: "s2", Model: "m2", Scorer: ok},
	}, cd)

	if _, err := r.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j1", RawText: "x"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st, _ := cd.Status(context.Background(), "s1")
	if st.FailureCount != 1 {
		t.Fatalf("want cooldown recorded on slot 1, got %+v", st)
	}
}

func TestRouter_EnrichBatch_BubblesOtherErrors(t *testing.T) {
	boom := &fakeScorer{enrichErr: errors.New("network boom")}
	ok := &fakeScorer{enrichOut: []*dto.JobPostEnrichResult{{JobID: "z"}}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: boom},
		{SlotID: "s2", Model: "m2", Scorer: ok},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))

	_, err := r.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}})
	if err == nil || err.Error() != "network boom" {
		t.Fatalf("expected network boom to bubble, got %v", err)
	}
}

func TestRouter_EnrichBatch_ReturnsAllExhaustedError(t *testing.T) {
	q1 := &fakeScorer{enrichErr: llmerr.ErrLLMLocalQuotaExceeded}
	q2 := &fakeScorer{enrichErr: llmerr.ErrLLMProviderRateLimited}
	cd := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute})
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: q1},
		{SlotID: "s2", Model: "m2", Scorer: q2},
	}, cd)

	_, err := r.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}})
	var ae *llmerr.LLMAllExhaustedError
	if !errors.As(err, &ae) {
		t.Fatalf("want LLMAllExhaustedError, got %v", err)
	}
	if len(ae.Slots) != 2 {
		t.Fatalf("want 2 slot statuses, got %d", len(ae.Slots))
	}
	// At least one slot should now be in cooldown with a non-zero RetryAfter.
	if ae.RetryAfter.IsZero() {
		t.Fatal("expected RetryAfter to reflect provider-429 slot's cooldown")
	}
}

func TestRouter_EnrichBatch_ZeroSlotsReturnsAllExhausted(t *testing.T) {
	r := services.NewLLMRouterService(nil,
		store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))
	_, err := r.EnrichBatch(context.Background(), nil)
	var ae *llmerr.LLMAllExhaustedError
	if !errors.As(err, &ae) {
		t.Fatalf("want LLMAllExhaustedError for empty slot list, got %v", err)
	}
	if len(ae.Slots) != 0 || !ae.RetryAfter.IsZero() {
		t.Fatalf("expected empty slots + zero RetryAfter, got %+v", ae)
	}
}

func TestRouter_ScoreBatch_ZeroSlotsReturnsAllExhausted(t *testing.T) {
	r := services.NewLLMRouterService(nil,
		store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))
	_, err := r.ScoreBatch(context.Background(), nil)
	var ae *llmerr.LLMAllExhaustedError
	if !errors.As(err, &ae) {
		t.Fatalf("want LLMAllExhaustedError for empty slot list, got %v", err)
	}
	if len(ae.Slots) != 0 {
		t.Fatalf("want 0 slot statuses, got %d", len(ae.Slots))
	}
}

func TestRouter_ExtractResumeKeywords_FallsBackOnLocalQuota(t *testing.T) {
	quota := &fakeScorer{keywordsErr: llmerr.ErrLLMLocalQuotaExceeded}
	ok := &fakeScorer{keywordsOut: &dto.ResumeKeywordResult{Keywords: []string{"Go"}}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: quota},
		{SlotID: "s2", Model: "m2", Scorer: ok},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))

	out, err := r.ExtractResumeKeywords(context.Background(), dto.ResumeKeywordInput{Text: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Keywords) != 1 || out.Keywords[0] != "Go" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestRouter_ExtractResumeKeywords_RecordsCooldownOnProvider429(t *testing.T) {
	limited := &fakeScorer{keywordsErr: llmerr.ErrLLMProviderRateLimited}
	ok := &fakeScorer{keywordsOut: &dto.ResumeKeywordResult{Keywords: []string{"Go"}}}
	cd := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: 10 * time.Second, Max: time.Minute})
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: limited},
		{SlotID: "s2", Model: "m2", Scorer: ok},
	}, cd)

	if _, err := r.ExtractResumeKeywords(context.Background(), dto.ResumeKeywordInput{Text: "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st, _ := cd.Status(context.Background(), "s1")
	if st.FailureCount != 1 {
		t.Fatalf("want cooldown recorded on slot 1, got %+v", st)
	}
}

func TestRouter_ExtractResumeKeywords_BubblesOtherErrors(t *testing.T) {
	boom := &fakeScorer{keywordsErr: errors.New("network boom")}
	ok := &fakeScorer{keywordsOut: &dto.ResumeKeywordResult{Keywords: []string{"Go"}}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: boom},
		{SlotID: "s2", Model: "m2", Scorer: ok},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))

	_, err := r.ExtractResumeKeywords(context.Background(), dto.ResumeKeywordInput{Text: "x"})
	if err == nil || err.Error() != "network boom" {
		t.Fatalf("expected network boom to bubble, got %v", err)
	}
}

// TestRouter_ExtractResumeKeywords_ValidationErrorFailsFast verifies the one
// deliberate divergence from the ScoreBatch/EnrichBatch loop shape: a caller
// bug (validation failure surfaced by the first slot) short-circuits instead
// of cycling every remaining slot and burning their RPM/RPD budget on input
// that will fail identically at every slot.
func TestRouter_ExtractResumeKeywords_ValidationErrorFailsFast(t *testing.T) {
	invalid := &fakeScorer{keywordsErr: llmerr.ErrLLMValidation}
	neverCalled := &fakeScorer{keywordsErr: errors.New("must not be reached")}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: invalid},
		{SlotID: "s2", Model: "m2", Scorer: neverCalled},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))

	_, err := r.ExtractResumeKeywords(context.Background(), dto.ResumeKeywordInput{Text: ""})
	if !errors.Is(err, llmerr.ErrLLMValidation) {
		t.Fatalf("want ErrLLMValidation to fail fast, got %v", err)
	}
}

func TestRouter_ExtractResumeKeywords_ReturnsAllExhaustedError(t *testing.T) {
	q1 := &fakeScorer{keywordsErr: llmerr.ErrLLMLocalQuotaExceeded}
	q2 := &fakeScorer{keywordsErr: llmerr.ErrLLMProviderRateLimited}
	cd := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute})
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: q1},
		{SlotID: "s2", Model: "m2", Scorer: q2},
	}, cd)

	_, err := r.ExtractResumeKeywords(context.Background(), dto.ResumeKeywordInput{Text: "x"})
	var ae *llmerr.LLMAllExhaustedError
	if !errors.As(err, &ae) {
		t.Fatalf("want LLMAllExhaustedError, got %v", err)
	}
	if len(ae.Slots) != 2 {
		t.Fatalf("want 2 slot statuses, got %d", len(ae.Slots))
	}
}

func TestRouter_ExtractResumeKeywords_ZeroSlotsReturnsAllExhausted(t *testing.T) {
	r := services.NewLLMRouterService(nil,
		store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))
	_, err := r.ExtractResumeKeywords(context.Background(), dto.ResumeKeywordInput{Text: "x"})
	var ae *llmerr.LLMAllExhaustedError
	if !errors.As(err, &ae) {
		t.Fatalf("want LLMAllExhaustedError for empty slot list, got %v", err)
	}
	if len(ae.Slots) != 0 {
		t.Fatalf("want 0 slot statuses, got %d", len(ae.Slots))
	}
}

// errCooldown lets tests force errors out of every cooldown-store method.
type errCooldown struct {
	availableErr error
	available    bool
	successErr   error
	failureErr   error
	statusErr    error
	status       store.CooldownStatus
}

func (e errCooldown) IsAvailable(_ context.Context, _ string) (bool, error) {
	return e.available, e.availableErr
}
func (e errCooldown) RecordSuccess(_ context.Context, _ string) error { return e.successErr }
func (e errCooldown) RecordFailure(_ context.Context, _ string) error { return e.failureErr }
func (e errCooldown) Status(_ context.Context, _ string) (store.CooldownStatus, error) {
	return e.status, e.statusErr
}

// withLogger captures the WithLogger seam exposed by both *rateLimitService
// and *routerService. Both types implement it on top of Scorer.
type withLogger interface {
	services.Scorer
	WithLogger(l logger.Logger) services.Scorer
}

func TestRouter_WithLoggerWarnsOnCooldownAvailabilityError(t *testing.T) {
	// IsAvailable returns an error but reports the slot available — router
	// must surface the error via Warn and still proceed with the call.
	cd := errCooldown{available: true, availableErr: errors.New("avail down")}
	ok := &fakeScorer{scoreOut: []string{"x"}}

	rec := &recordingLogger{}
	r := services.NewLLMRouterService([]services.RouterSlot{{SlotID: "s1", Model: "m1", Scorer: ok}}, cd)
	wl, hooked := r.(withLogger)
	if !hooked {
		t.Fatal("router must expose WithLogger seam")
	}
	r = wl.WithLogger(rec)

	if _, err := r.ScoreBatch(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.Warned() {
		t.Fatal("expected warn on cooldown availability error")
	}
}

func TestRouter_WithLoggerWarnsOnRecordSuccessError(t *testing.T) {
	cd := errCooldown{available: true, successErr: errors.New("succ down")}
	ok := &fakeScorer{scoreOut: []string{"x"}}

	rec := &recordingLogger{}
	r := services.NewLLMRouterService([]services.RouterSlot{{SlotID: "s1", Model: "m1", Scorer: ok}}, cd)
	wl, _ := r.(withLogger)
	r = wl.WithLogger(rec)

	if _, err := r.ScoreBatch(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.Warned() {
		t.Fatal("expected warn on RecordSuccess error")
	}
}

func TestRouter_WithLoggerWarnsOnRecordFailureError(t *testing.T) {
	cd := errCooldown{available: true, failureErr: errors.New("fail down")}
	limited := &fakeScorer{scoreErr: llmerr.ErrLLMProviderRateLimited}

	rec := &recordingLogger{}
	r := services.NewLLMRouterService([]services.RouterSlot{{SlotID: "s1", Model: "m1", Scorer: limited}}, cd)
	wl, _ := r.(withLogger)
	r = wl.WithLogger(rec)

	_, _ = r.ScoreBatch(context.Background(), nil)
	if !rec.Warned() {
		t.Fatal("expected warn on RecordFailure error")
	}
}

func TestRouter_WithLoggerWarnsOnStatusError(t *testing.T) {
	cd := errCooldown{available: false, statusErr: errors.New("status down")}
	r := services.NewLLMRouterService([]services.RouterSlot{{SlotID: "s1", Model: "m1", Scorer: &fakeScorer{}}}, cd)

	rec := &recordingLogger{}
	wl, _ := r.(withLogger)
	r = wl.WithLogger(rec)

	_, _ = r.ScoreBatch(context.Background(), nil)
	if !rec.Warned() {
		t.Fatal("expected warn on Status error")
	}
}

func TestRouter_EnrichBatch_WithLoggerWarnsOnAvailabilityError(t *testing.T) {
	cd := errCooldown{available: true, availableErr: errors.New("avail down")}
	ok := &fakeScorer{enrichOut: []*dto.JobPostEnrichResult{{JobID: "j"}}}

	rec := &recordingLogger{}
	r := services.NewLLMRouterService([]services.RouterSlot{{SlotID: "s1", Model: "m1", Scorer: ok}}, cd)
	wl, _ := r.(withLogger)
	r = wl.WithLogger(rec)

	if _, err := r.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.Warned() {
		t.Fatal("expected warn on availability error from enrich path")
	}
}

func TestRouter_EnrichBatch_WithLoggerWarnsOnSuccessError(t *testing.T) {
	cd := errCooldown{available: true, successErr: errors.New("succ down")}
	ok := &fakeScorer{enrichOut: []*dto.JobPostEnrichResult{{JobID: "j"}}}

	rec := &recordingLogger{}
	r := services.NewLLMRouterService([]services.RouterSlot{{SlotID: "s1", Model: "m1", Scorer: ok}}, cd)
	wl, _ := r.(withLogger)
	r = wl.WithLogger(rec)

	if _, err := r.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.Warned() {
		t.Fatal("expected warn on RecordSuccess error from enrich path")
	}
}

func TestRouter_EnrichBatch_WithLoggerWarnsOnFailureError(t *testing.T) {
	cd := errCooldown{available: true, failureErr: errors.New("fail down")}
	limited := &fakeScorer{enrichErr: llmerr.ErrLLMProviderRateLimited}

	rec := &recordingLogger{}
	r := services.NewLLMRouterService([]services.RouterSlot{{SlotID: "s1", Model: "m1", Scorer: limited}}, cd)
	wl, _ := r.(withLogger)
	r = wl.WithLogger(rec)

	_, _ = r.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}})
	if !rec.Warned() {
		t.Fatal("expected warn on RecordFailure error from enrich path")
	}
}

func TestRouter_EnrichBatch_WithLoggerWarnsOnStatusError(t *testing.T) {
	cd := errCooldown{available: false, statusErr: errors.New("status down")}
	r := services.NewLLMRouterService([]services.RouterSlot{{SlotID: "s1", Model: "m1", Scorer: &fakeScorer{}}}, cd)

	rec := &recordingLogger{}
	wl, _ := r.(withLogger)
	r = wl.WithLogger(rec)

	_, _ = r.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}})
	if !rec.Warned() {
		t.Fatal("expected warn on Status error from enrich path")
	}
}

func TestRouter_NoLoggerDoesNotPanicOnCooldownErrors(t *testing.T) {
	// Router default has no logger — error from cooldown must not cause
	// a panic or stop the call.
	cd := errCooldown{available: true, availableErr: errors.New("avail down")}
	ok := &fakeScorer{scoreOut: []string{"x"}}
	r := services.NewLLMRouterService([]services.RouterSlot{{SlotID: "s1", Model: "m1", Scorer: ok}}, cd)
	if _, err := r.ScoreBatch(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestScoreBatch_CooldownMissIncrementsCounter verifies that a cooldown-store
// IsAvailable error increments LLMCooldownMissTotal by exactly 1. The slot is
// skipped (fail-closed, available=false) so the overall call returns
// LLMAllExhaustedError — the error return is intentionally ignored here because
// the assertion under test is the counter.
func TestScoreBatch_CooldownMissIncrementsCounter(t *testing.T) {
	before := testutil.ToFloat64(metrics.LLMCooldownMissTotal)

	cd := errCooldown{available: false, availableErr: errors.New("redis: i/o timeout")}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "slot1", Model: "gemini", Scorer: &fakeScorer{}},
	}, cd)

	_, _ = r.ScoreBatch(context.Background(), nil)

	after := testutil.ToFloat64(metrics.LLMCooldownMissTotal)
	if after-before != 1 {
		t.Errorf("LLMCooldownMissTotal: want +1, got +%g", after-before)
	}
}

// TestEnrichBatch_CooldownMissIncrementsCounter verifies the same counter
// increments on the EnrichBatch code path when IsAvailable returns an error.
func TestEnrichBatch_CooldownMissIncrementsCounter(t *testing.T) {
	before := testutil.ToFloat64(metrics.LLMCooldownMissTotal)

	cd := errCooldown{available: false, availableErr: errors.New("redis: connection refused")}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "slot1", Model: "gemini", Scorer: &fakeScorer{}},
	}, cd)

	_, _ = r.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j1", RawText: "x"}})

	after := testutil.ToFloat64(metrics.LLMCooldownMissTotal)
	if after-before != 1 {
		t.Errorf("LLMCooldownMissTotal: want +1, got +%g", after-before)
	}
}
