package llm

import (
	"context"
	"fmt"

	"github.com/trushilpatel/jobradar/infra/logger"
)

// ScorerOptions holds everything needed to build and configure a Scorer.
// Using a struct keeps the factory signature stable as new fields are added.
type ScorerOptions struct {
	Provider    string // "gemini"
	APIKey      string
	Model       string // provider-specific model name, e.g. "Gemini-3.1-Flash-Lite"
	TargetRoles []string
	YearsExp    int
	Limits      RateLimits    // zero value = no rate limiting
	Log         logger.Logger // used by the rate limiter; noop if nil
}

// NewScorer builds the concrete provider scorer and wraps it with the rate
// limiter if any limit is non-zero. Workers always receive the Scorer interface
// — they never know whether rate limiting is active.
func NewScorer(opts ScorerOptions) (Scorer, error) {
	var (
		inner Scorer
		err   error
	)

	switch opts.Provider {
	case "gemini":
		inner, err = newGeminiScorer(context.Background(), opts.APIKey, opts.Model, opts.TargetRoles, opts.YearsExp)
	default:
		return nil, fmt.Errorf("llm: unknown provider %q (supported: gemini)", opts.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("llm: init %s: %w", opts.Provider, err)
	}

	// Wrap with rate limiter only when at least one limit is set.
	if opts.Limits.RPM > 0 || opts.Limits.RPD > 0 || opts.Limits.TPMIn > 0 {
		return newRateLimitedScorer(inner, opts.Limits, opts.Log), nil
	}
	return inner, nil
}
