package services

import (
	"context"
	"fmt"

	"github.com/trushilpatel/jobradar/common/logger"
	llmcommon "github.com/trushilpatel/jobradar/service/llm/common"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

// SlotOptions describes a single (API key, model) slot used by the router.
// All fields are required except the three limits, which are 0 = unlimited.
type SlotOptions struct {
	KeyID  string
	APIKey string
	Model  string
	RPM    int
	RPD    int
	TPMIn  int
}

// Options holds everything needed to build and configure a routed Scorer.
// Slots must be non-empty — configure via LLM_SLOTS_CONFIG (YAML) or
// FILTER_LLM_SLOTS / ENRICHER_LLM_SLOTS (JSON env vars).
type Options struct {
	Provider    string
	TargetRoles []string
	YearsExp    int
	Log         logger.Logger

	Slots          []SlotOptions
	RateLimitStore store.LLMRateLimitStore
	CooldownStore  store.LLMCooldownStore
}

// NewScorer builds one Gemini scorer per configured slot, wraps each in a
// rate-limit decorator, and collects all slots behind an LLMRouterService.
// The returned Scorer satisfies both JobsRelevanceFilter and JobPostEnricher.
//
// The supplied ctx is passed to provider-client initialization so callers can
// cancel startup on shutdown or apply a startup deadline.
func NewScorer(ctx context.Context, opts Options) (Scorer, error) {
	if opts.Provider != llmcommon.ProviderGemini {
		return nil, fmt.Errorf("llm: unsupported provider %q (supported: gemini)", opts.Provider)
	}
	if opts.RateLimitStore == nil {
		return nil, fmt.Errorf("llm: RateLimitStore is required")
	}
	if opts.CooldownStore == nil {
		return nil, fmt.Errorf("llm: CooldownStore is required")
	}

	if len(opts.Slots) == 0 {
		return nil, fmt.Errorf("llm: no slots configured — set LLM_SLOTS_CONFIG, FILTER_LLM_SLOTS, or ENRICHER_LLM_SLOTS")
	}

	seen := make(map[string]struct{}, len(opts.Slots))
	routerSlots := make([]RouterSlot, 0, len(opts.Slots))

	for i, s := range opts.Slots {
		if s.KeyID == "" {
			return nil, fmt.Errorf("llm: slot[%d] missing KeyID", i)
		}
		if s.APIKey == "" {
			return nil, fmt.Errorf("llm: slot[%d] (%s) missing APIKey", i, s.KeyID)
		}
		if s.Model == "" {
			return nil, fmt.Errorf("llm: slot[%d] (%s) missing Model", i, s.KeyID)
		}

		slotID := llmcommon.SlotID(opts.Provider, s.KeyID, s.Model)
		if _, dup := seen[slotID]; dup {
			return nil, fmt.Errorf("llm: duplicate slot id %q", slotID)
		}
		seen[slotID] = struct{}{}

		inner, err := newGeminiScorer(ctx, s.APIKey, s.Model, opts.TargetRoles, opts.YearsExp)
		if err != nil {
			return nil, fmt.Errorf("llm: init gemini slot %s: %w", slotID, err)
		}

		rlSlot := RateLimitSlot{
			SlotID:   slotID,
			Provider: opts.Provider,
			KeyID:    s.KeyID,
			Model:    s.Model,
			RPM:      s.RPM,
			RPD:      s.RPD,
			TPMIn:    s.TPMIn,
		}
		wrapped := NewLLMRateLimitService(rlSlot, inner, opts.RateLimitStore)
		if rls, ok := wrapped.(*rateLimitService); ok && opts.Log != nil {
			wrapped = rls.WithLogger(opts.Log)
		}

		routerSlots = append(routerSlots, RouterSlot{
			SlotID:   slotID,
			Provider: opts.Provider,
			KeyID:    s.KeyID,
			Model:    s.Model,
			Scorer:   wrapped,
		})
	}

	router := NewLLMRouterService(routerSlots, opts.CooldownStore)
	if rs, ok := router.(*routerService); ok && opts.Log != nil {
		router = rs.WithLogger(opts.Log)
	}
	return router, nil
}
