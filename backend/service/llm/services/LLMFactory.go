package services

import (
	"context"
	"fmt"

	llmcommon "github.com/trushilpatel/jobradar/service/llm/common"
	"github.com/trushilpatel/jobradar/infra/logger"
)

// Options holds everything needed to build and configure a Scorer.
type Options struct {
	Provider    string
	APIKey      string
	Model       string
	TargetRoles []string
	YearsExp    int
	Limits      RateLimits
	Log         logger.Logger
}

// NewScorer builds the provider scorer and wraps it with the rate limiter if configured.
// Returns a Scorer satisfying both JobsRelevanceFilter and JobPostEnricher.
func NewScorer(opts Options) (Scorer, error) {
	var (
		inner Scorer
		err   error
	)

	switch opts.Provider {
	case llmcommon.ProviderGemini:
		inner, err = newGeminiScorer(context.Background(), opts.APIKey, opts.Model, opts.TargetRoles, opts.YearsExp)
	default:
		return nil, fmt.Errorf("llm: unknown provider %q (supported: gemini)", opts.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("llm: init %s: %w", opts.Provider, err)
	}

	if opts.Limits.RPM > 0 || opts.Limits.RPD > 0 || opts.Limits.TPMIn > 0 {
		return newRateLimitedScorer(inner, opts.Limits, opts.Log), nil
	}
	return inner, nil
}
