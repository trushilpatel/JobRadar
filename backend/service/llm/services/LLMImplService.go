package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/service/llm/dto"
)

// RateLimits holds the three quota dimensions for an LLM model.
// All fields are 0 = unlimited. Values come from config — never hardcoded.
type RateLimits struct {
	RPM   int
	RPD   int
	TPMIn int
}

type rateLimitedScorer struct {
	inner Scorer
	log   logger.Logger

	rpm *rate.Limiter

	tpm *rate.Limiter

	mu       sync.Mutex
	dayCount int
	dayMax   int
	dayReset time.Time
}

func newRateLimitedScorer(inner Scorer, limits RateLimits, log logger.Logger) Scorer {
	s := &rateLimitedScorer{
		inner:    inner,
		log:      log,
		dayMax:   limits.RPD,
		dayReset: nextMidnightUTC(),
	}

	if limits.RPM > 0 {
		s.rpm = rate.NewLimiter(rate.Limit(float64(limits.RPM)/60.0), limits.RPM)
	}
	if limits.TPMIn > 0 {
		s.tpm = rate.NewLimiter(rate.Limit(float64(limits.TPMIn)/60.0), limits.TPMIn)
	}

	return s
}

func (s *rateLimitedScorer) wait(ctx context.Context, estimatedTokens int) error {
	if s.dayMax > 0 {
		s.mu.Lock()
		for {
			if time.Now().UTC().After(s.dayReset) {
				s.dayCount = 0
				s.dayReset = nextMidnightUTC()
			}

			if s.dayCount < s.dayMax {
				s.dayCount++
				s.mu.Unlock()
				break
			}

			waitUntil := s.dayReset
			s.mu.Unlock()
			s.log.Warn("daily request limit reached, sleeping",
				logger.Int("rpd_limit", s.dayMax),
				logger.Str("reset_at", waitUntil.Format(time.RFC3339)),
			)

			select {
			case <-time.After(time.Until(waitUntil) + time.Second):
			case <-ctx.Done():
				return fmt.Errorf("rate limit wait cancelled: %w", ctx.Err())
			}
			s.mu.Lock()
		}
	}

	if s.rpm != nil {
		if err := s.rpm.Wait(ctx); err != nil {
			return fmt.Errorf("rpm limit wait: %w", err)
		}
	}

	if s.tpm != nil && estimatedTokens > 0 {
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

func (s *rateLimitedScorer) ScoreBatch(ctx context.Context, jobs []dto.JobsRelevanceFilterInput) ([]string, error) {
	if err := s.wait(ctx, estimateScoreBatchTokens(jobs)); err != nil {
		return nil, err
	}
	return s.inner.ScoreBatch(ctx, jobs)
}

func (s *rateLimitedScorer) Enrich(ctx context.Context, input dto.JobPostEnrichInput) (*dto.JobPostEnrichResult, error) {
	if err := s.wait(ctx, estimateEnrichTokens(input)); err != nil {
		return nil, err
	}
	return s.inner.Enrich(ctx, input)
}

func estimateScoreBatchTokens(jobs []dto.JobsRelevanceFilterInput) int {
	return 150 + len(jobs)*20
}

func estimateEnrichTokens(input dto.JobPostEnrichInput) int {
	return 200 + len(input.RawText)/4
}

func nextMidnightUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
}
