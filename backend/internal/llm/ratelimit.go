package llm

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimits holds the three quota dimensions for an LLM model.
// All fields are 0 = unlimited. Values come from config — never hardcoded.
type RateLimits struct {
	RPM   int // requests per minute       (token bucket, sliding window)
	RPD   int // requests per day          (hard counter, resets at UTC midnight)
	TPMIn int // input tokens per minute   (token bucket, sliding window)
}

// rateLimitedScorer wraps any Scorer and enforces RPM, RPD, and TPM limits
// before forwarding each call to the underlying provider.
// Workers depend on the Scorer interface — this wrapper is invisible to them.
type rateLimitedScorer struct {
	inner Scorer

	// RPM: token bucket — 1 token consumed per request
	rpm *rate.Limiter // nil when RPM=0

	// TPMIn: token bucket — N tokens consumed per request (estimated input size)
	tpm *rate.Limiter // nil when TPMIn=0

	// RPD: hard daily counter, resets at UTC midnight
	mu       sync.Mutex
	dayCount int
	dayMax   int // 0 = unlimited
	dayReset time.Time
}

func newRateLimitedScorer(inner Scorer, limits RateLimits) Scorer {
	s := &rateLimitedScorer{
		inner:    inner,
		dayMax:   limits.RPD,
		dayReset: nextMidnightUTC(),
	}

	if limits.RPM > 0 {
		// Refill rate: RPM tokens per minute. Burst = RPM (can use full minute quota instantly).
		s.rpm = rate.NewLimiter(rate.Limit(float64(limits.RPM)/60.0), limits.RPM)
	}
	if limits.TPMIn > 0 {
		// Refill rate: TPMIn tokens per minute.
		s.tpm = rate.NewLimiter(rate.Limit(float64(limits.TPMIn)/60.0), limits.TPMIn)
	}

	return s
}

// wait blocks until all three quotas have capacity for this request.
// estimatedTokens is a pre-call estimate of input token usage.
func (s *rateLimitedScorer) wait(ctx context.Context, estimatedTokens int) error {
	// ── 1. RPD: hard daily cap ──────────────────────────────────────────────
	if s.dayMax > 0 {
		s.mu.Lock()
		for {
			// Reset counter if we've crossed midnight
			if time.Now().UTC().After(s.dayReset) {
				s.dayCount = 0
				s.dayReset = nextMidnightUTC()
			}

			if s.dayCount < s.dayMax {
				s.dayCount++
				s.mu.Unlock()
				break
			}

			// Quota exhausted — wait until UTC midnight then re-check
			waitUntil := s.dayReset
			s.mu.Unlock()
			log.Printf("[llm] daily request limit (%d RPD) reached — sleeping until %s",
				s.dayMax, waitUntil.Format(time.RFC3339))

			select {
			case <-time.After(time.Until(waitUntil) + time.Second): // +1s buffer past midnight
			case <-ctx.Done():
				return fmt.Errorf("rate limit wait cancelled: %w", ctx.Err())
			}
			s.mu.Lock()
		}
	}

	// ── 2. RPM: sliding-window request rate ─────────────────────────────────
	if s.rpm != nil {
		if err := s.rpm.Wait(ctx); err != nil {
			return fmt.Errorf("rpm limit wait: %w", err)
		}
	}

	// ── 3. TPMIn: sliding-window token rate ──────────────────────────────────
	if s.tpm != nil && estimatedTokens > 0 {
		// Clamp to burst to avoid a WaitN panic if estimate exceeds bucket capacity.
		n := estimatedTokens
		if n > s.tpm.Burst() {
			n = s.tpm.Burst()
		}
		if err := s.tpm.WaitN(ctx, n); err != nil {
			return fmt.Errorf("tpm limit wait: %w", err)
		}
	}

	return nil
}

func (s *rateLimitedScorer) ScoreBatch(ctx context.Context, jobs []ScorerInput) ([]string, error) {
	if err := s.wait(ctx, estimateScoreBatchTokens(jobs)); err != nil {
		return nil, err
	}
	return s.inner.ScoreBatch(ctx, jobs)
}

func (s *rateLimitedScorer) Enrich(ctx context.Context, input EnrichInput) (*EnrichResult, error) {
	if err := s.wait(ctx, estimateEnrichTokens(input)); err != nil {
		return nil, err
	}
	return s.inner.Enrich(ctx, input)
}

// ── Token estimators ────────────────────────────────────────────────────────
// These are intentionally pessimistic (over-estimate) so we throttle slightly
// more than needed rather than risk hitting the API's hard limits.

func estimateScoreBatchTokens(jobs []ScorerInput) int {
	// ~150 tokens of prompt overhead + ~20 tokens per job title
	return 150 + len(jobs)*20
}

func estimateEnrichTokens(input EnrichInput) int {
	// ~200 tokens of prompt overhead + 1 token per 4 chars of raw job text
	return 200 + len(input.RawText)/4
}

func nextMidnightUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
}
