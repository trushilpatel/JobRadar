package llm

import (
	"context"
	"fmt"
)

// ScorerOptions holds everything needed to build and configure a Scorer.
// Using a struct keeps the factory signature stable as new fields are added.
type ScorerOptions struct {
	Provider    string     // "gemini" | "anthropic"
	APIKey      string
	Model       string     // provider-specific model name, e.g. "gemini-1.5-flash-8b"
	TargetRoles []string
	YearsExp    int
	Limits      RateLimits // zero value = no rate limiting
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
	case "anthropic":
		inner = newAnthropicScorer(opts.APIKey, opts.Model, opts.TargetRoles, opts.YearsExp)
	default:
		return nil, fmt.Errorf("llm: unknown provider %q (supported: gemini, anthropic)", opts.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("llm: init %s: %w", opts.Provider, err)
	}

	// Wrap with rate limiter only when at least one limit is set.
	if opts.Limits.RPM > 0 || opts.Limits.RPD > 0 || opts.Limits.TPMIn > 0 {
		return newRateLimitedScorer(inner, opts.Limits), nil
	}
	return inner, nil
}
