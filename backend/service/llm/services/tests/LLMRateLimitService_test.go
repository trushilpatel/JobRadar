package tests

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/services"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

// errStore lets tests inject failures at the rate-limit store boundary.
type errStore struct {
	reserveResult store.ReserveResult
	reserveErr    error
	releaseErr    error
	usage         store.Usage
}

func (e errStore) Reserve(_ context.Context, _ store.ReserveReq) (store.ReserveResult, error) {
	if e.reserveErr != nil {
		return 0, e.reserveErr
	}
	return e.reserveResult, nil
}

func (e errStore) Release(_ context.Context, _, _ string) error { return e.releaseErr }

func (e errStore) Usage(_ context.Context, _ string) (store.Usage, error) {
	return e.usage, nil
}

// recordingLogger captures Warn calls so tests can assert that
// non-fatal bookkeeping failures are surfaced through the logger seam.
type recordingLogger struct {
	warned atomic.Bool
}

func (r *recordingLogger) Warn(_ string, _ ...logger.Field)            { r.warned.Store(true) }
func (r *recordingLogger) Info(_ string, _ ...logger.Field)            {}
func (r *recordingLogger) Debug(_ string, _ ...logger.Field)           {}
func (r *recordingLogger) Error(_ string, _ error, _ ...logger.Field)  {}
func (r *recordingLogger) With(_ ...logger.Field) logger.Logger        { return r }
func (r *recordingLogger) WithContext(_ context.Context) logger.Logger { return r }
func (r *recordingLogger) Warned() bool                                { return r.warned.Load() }

type fakeScorer struct {
	scoreErr  error
	scoreOut  []string
	enrichErr error
	enrichOut []*dto.JobPostEnrichResult
}

func (f *fakeScorer) ScoreBatch(_ context.Context, _ []dto.JobsRelevanceFilterInput) ([]string, error) {
	return f.scoreOut, f.scoreErr
}

func (f *fakeScorer) EnrichBatch(_ context.Context, _ []dto.JobPostEnrichInput) ([]*dto.JobPostEnrichResult, error) {
	return f.enrichOut, f.enrichErr
}

func TestRateLimit_DenyReturnsLocalQuota(t *testing.T) {
	inner := &fakeScorer{}
	rs := store.NewLLMRateLimitMemoryStore()
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{
		SlotID: "s1", RPM: 1, RPD: 100,
	}, inner, rs)

	if _, err := rl.ScoreBatch(context.Background(), nil); err != nil {
		t.Fatalf("first call: %v", err)
	}

	_, err := rl.ScoreBatch(context.Background(), nil)
	if !errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
		t.Fatalf("want local quota, got %v", err)
	}
}

func TestRateLimit_ReleaseOnInnerError(t *testing.T) {
	inner := &fakeScorer{scoreErr: errors.New("network boom")}
	rs := store.NewLLMRateLimitMemoryStore()
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 1, RPD: 100}, inner, rs)

	if _, err := rl.ScoreBatch(context.Background(), nil); err == nil {
		t.Fatal("expected first call to fail with network boom")
	}

	// inner failed -> reservation released -> next call should succeed.
	inner.scoreErr = nil
	if _, err := rl.ScoreBatch(context.Background(), nil); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestRateLimit_KeepReservationOnProvider429(t *testing.T) {
	inner := &fakeScorer{scoreErr: llmerr.ErrLLMProviderRateLimited}
	rs := store.NewLLMRateLimitMemoryStore()
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 1, RPD: 100}, inner, rs)

	_, err := rl.ScoreBatch(context.Background(), nil)
	if !errors.Is(err, llmerr.ErrLLMProviderRateLimited) {
		t.Fatalf("want provider 429, got %v", err)
	}

	// Reservation kept -> next call should get local quota.
	inner.scoreErr = nil
	_, err = rl.ScoreBatch(context.Background(), nil)
	if !errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
		t.Fatalf("want local quota, got %v", err)
	}
}

func TestRateLimit_EnrichBatch_Success(t *testing.T) {
	want := []*dto.JobPostEnrichResult{{JobID: "j1", Summary: "s"}}
	inner := &fakeScorer{enrichOut: want}
	rs := store.NewLLMRateLimitMemoryStore()
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{
		SlotID: "s1", RPM: 10, RPD: 100, TPMIn: 1_000_000,
	}, inner, rs)

	got, err := rl.EnrichBatch(context.Background(),
		[]dto.JobPostEnrichInput{{ID: "j1", RawText: "raw"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].JobID != "j1" {
		t.Fatalf("unexpected output: %+v", got)
	}
}

func TestRateLimit_EnrichBatch_DenyLocalQuota(t *testing.T) {
	inner := &fakeScorer{}
	rs := store.NewLLMRateLimitMemoryStore()
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 1, RPD: 100}, inner, rs)

	if _, err := rl.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	_, err := rl.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}})
	if !errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
		t.Fatalf("want local quota, got %v", err)
	}
}

func TestRateLimit_EnrichBatch_ReleaseOnInnerError(t *testing.T) {
	inner := &fakeScorer{enrichErr: errors.New("network boom")}
	rs := store.NewLLMRateLimitMemoryStore()
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 1, RPD: 100}, inner, rs)

	if _, err := rl.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}}); err == nil {
		t.Fatal("expected first call to error")
	}
	inner.enrichErr = nil
	if _, err := rl.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}}); err != nil {
		t.Fatalf("expected success after release, got %v", err)
	}
}

func TestRateLimit_EnrichBatch_KeepReservationOnProvider429(t *testing.T) {
	inner := &fakeScorer{enrichErr: llmerr.ErrLLMProviderRateLimited}
	rs := store.NewLLMRateLimitMemoryStore()
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 1, RPD: 100}, inner, rs)

	_, err := rl.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}})
	if !errors.Is(err, llmerr.ErrLLMProviderRateLimited) {
		t.Fatalf("want provider 429, got %v", err)
	}
	inner.enrichErr = nil
	_, err = rl.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}})
	if !errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
		t.Fatalf("want local quota after kept reservation, got %v", err)
	}
}

func TestRateLimit_DenyTPMReturnsLocalQuotaTPM(t *testing.T) {
	inner := &fakeScorer{}
	rs := store.NewLLMRateLimitMemoryStore()
	// TPM cap is so small the first batch alone exceeds it.
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{
		SlotID: "s1", RPM: 100, RPD: 100, TPMIn: 100,
	}, inner, rs)

	// estimateEnrichBatchTokens base = 500 -> first call must trip TPM.
	_, err := rl.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}})
	if !errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
		t.Fatalf("want ErrLLMLocalQuotaExceeded for TPM, got %v", err)
	}
	if !strings.Contains(err.Error(), "local_quota_tpm") {
		t.Fatalf("expected TPM outcome string in error, got %v", err)
	}
}

func TestRateLimit_DenyRPDReturnsLocalQuotaRPD(t *testing.T) {
	inner := &fakeScorer{}
	rs := store.NewLLMRateLimitMemoryStore()
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 100, RPD: 1}, inner, rs)

	if _, err := rl.ScoreBatch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	_, err := rl.ScoreBatch(context.Background(), nil)
	if !errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
		t.Fatalf("want local quota RPD, got %v", err)
	}
	if !strings.Contains(err.Error(), "local_quota_rpd") {
		t.Fatalf("expected RPD outcome string in error, got %v", err)
	}
}

// TestRateLimit_ReserveStoreError_TreatedAsLocalQuota verifies AC-6 (RC-6):
// when store.Reserve returns an infra error (Redis unavailable), the error is
// treated as a local-quota denial (ErrLLMLocalQuotaExceeded) so the router
// continues to the next slot rather than propagating the infra error. This
// prevents a Redis outage from blocking all LLM calls.
func TestRateLimit_ReserveStoreError_TreatedAsLocalQuota(t *testing.T) {
	inner := &fakeScorer{}
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 1, RPD: 100},
		inner, errStore{reserveErr: errors.New("redis down")})

	_, err := rl.ScoreBatch(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
		t.Fatalf("Redis infra error must surface as ErrLLMLocalQuotaExceeded (AC-6), got: %v", err)
	}
}

// TestRateLimit_ReserveStoreError_EnrichBatch_TreatedAsLocalQuota verifies AC-6
// for the EnrichBatch path: same Redis-as-local-quota behavior.
func TestRateLimit_ReserveStoreError_EnrichBatch_TreatedAsLocalQuota(t *testing.T) {
	inner := &fakeScorer{}
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 1, RPD: 100},
		inner, errStore{reserveErr: errors.New("redis conn refused")})

	_, err := rl.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}})
	if !errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
		t.Fatalf("Redis infra error must surface as ErrLLMLocalQuotaExceeded (AC-6), got: %v", err)
	}
}

func TestRateLimit_UnexpectedReserveCodeBubblesAsError(t *testing.T) {
	inner := &fakeScorer{}
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 1, RPD: 100},
		inner, errStore{reserveResult: store.ReserveResult(99)})

	_, err := rl.ScoreBatch(context.Background(), nil)
	if err == nil {
		t.Fatal("expected unexpected-code error")
	}
	if errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
		t.Fatal("unexpected-code error must not surface as local quota")
	}
	if !strings.Contains(err.Error(), "unexpected code 99") {
		t.Fatalf("expected 'unexpected code 99' in error, got %v", err)
	}
}

func TestRateLimit_WithLoggerLogsReleaseFailure(t *testing.T) {
	rec := &recordingLogger{}
	inner := &fakeScorer{scoreErr: errors.New("inner")}
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 1, RPD: 100},
		inner, errStore{releaseErr: errors.New("release boom")})
	// Apply logger via wrapping (the constructor returns the interface).
	if hooked, ok := rl.(interface {
		WithLogger(logger.Logger) services.Scorer
	}); ok {
		rl = hooked.WithLogger(rec)
	}

	_, _ = rl.ScoreBatch(context.Background(), nil)
	if !rec.Warned() {
		t.Fatal("expected logger to capture release failure warning")
	}
}
