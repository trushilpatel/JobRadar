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
//
// The new multi-slot path is Slots + RateLimitStore + CooldownStore. The
// legacy single-slot fields (APIKey / Model / Limits) stay for backwards
// compatibility — when Slots is empty, the factory synthesizes a single
// "default" slot from the legacy fields.
type Options struct {
	Provider    string
	TargetRoles []string
	YearsExp    int
	Log         logger.Logger

	// Multi-slot routing.
	Slots          []SlotOptions
	RateLimitStore store.LLMRateLimitStore
	CooldownStore  store.LLMCooldownStore

	// Legacy single-slot fields — used only when Slots is empty.
	APIKey string
	Model  string
	Limits RateLimits
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

	slots := opts.Slots
	if len(slots) == 0 {
		slots = []SlotOptions{{
			KeyID:  "default",
			APIKey: opts.APIKey,
			Model:  opts.Model,
			RPM:    opts.Limits.RPM,
			RPD:    opts.Limits.RPD,
			TPMIn:  opts.Limits.TPMIn,
		}}
	}

	seen := make(map[string]struct{}, len(slots))
	routerSlots := make([]RouterSlot, 0, len(slots))

	for i, s := range slots {
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
