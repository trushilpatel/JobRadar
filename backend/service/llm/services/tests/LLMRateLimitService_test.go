package tests

import (
	"context"
	"errors"
	"testing"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/services"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

type fakeScorer struct {
	scoreErr  error
	scoreOut  []string
	enrichErr error
	enrichOut *dto.JobPostEnrichResult
}

func (f *fakeScorer) ScoreBatch(_ context.Context, _ []dto.JobsRelevanceFilterInput) ([]string, error) {
	return f.scoreOut, f.scoreErr
}

func (f *fakeScorer) Enrich(_ context.Context, _ dto.JobPostEnrichInput) (*dto.JobPostEnrichResult, error) {
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
