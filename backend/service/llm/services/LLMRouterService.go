package services

import (
	"context"
	"errors"
	"time"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	llmcommon "github.com/trushilpatel/jobradar/service/llm/common"
	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

// RouterSlot pairs one per-slot Scorer (typically a rateLimitService wrapping
// a geminiScorer) with the cooldown-store identifier used to record failures.
type RouterSlot struct {
	SlotID   string
	Provider string
	KeyID    string
	Model    string
	Scorer   Scorer
}

// routerService fans a single request across a list of RouterSlots, respecting
// per-slot cooldowns and local-quota denials. It implements Scorer so callers
// receive the same contract as a single-slot scorer.
type routerService struct {
	slots    []RouterSlot
	cooldown store.LLMCooldownStore
	log      logger.Logger
}

// NewLLMRouterService returns a Scorer that iterates through slots in order,
// falling back on local-quota denials and provider 429s, and returns
// *LLMAllExhaustedError once every slot is unavailable.
func NewLLMRouterService(slots []RouterSlot, cd store.LLMCooldownStore) Scorer {
	return &routerService{slots: slots, cooldown: cd}
}

// WithLogger attaches a logger used for non-fatal bookkeeping failures.
func (r *routerService) WithLogger(l logger.Logger) Scorer {
	r.log = l
	return r
}

func (r *routerService) ScoreBatch(ctx context.Context, jobs []dto.JobsRelevanceFilterInput) ([]string, error) {
	var statuses []llmerr.LLMSlotStatus
	for _, s := range r.slots {
		ok, cdErr := r.cooldown.IsAvailable(ctx, s.SlotID)
		if cdErr != nil {
			r.warnCooldown("cooldown availability check failed", s.SlotID, cdErr)
		}
		if !ok {
			statuses = append(statuses, r.statusOf(ctx, s, llmcommon.OutcomeCooldownSkipped))
			continue
		}

		out, err := s.Scorer.ScoreBatch(ctx, jobs)
		if err == nil {
			if sErr := r.cooldown.RecordSuccess(ctx, s.SlotID); sErr != nil {
				r.warnCooldown("record success failed", s.SlotID, sErr)
			}
			return out, nil
		}
		if errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
			statuses = append(statuses, r.statusOf(ctx, s, llmcommon.OutcomeLocalQuotaRPM))
			continue
		}
		if errors.Is(err, llmerr.ErrLLMProviderRateLimited) {
			if fErr := r.cooldown.RecordFailure(ctx, s.SlotID); fErr != nil {
				r.warnCooldown("record failure failed", s.SlotID, fErr)
			}
			statuses = append(statuses, r.statusOf(ctx, s, llmcommon.OutcomeProvider429))
			continue
		}
		return nil, err
	}

	return nil, &llmerr.LLMAllExhaustedError{Slots: statuses, RetryAfter: earliestCooldown(statuses)}
}

func (r *routerService) EnrichBatch(ctx context.Context, inputs []dto.JobPostEnrichInput) ([]*dto.JobPostEnrichResult, error) {
	var statuses []llmerr.LLMSlotStatus
	for _, s := range r.slots {
		ok, cdErr := r.cooldown.IsAvailable(ctx, s.SlotID)
		if cdErr != nil {
			r.warnCooldown("cooldown availability check failed", s.SlotID, cdErr)
		}
		if !ok {
			statuses = append(statuses, r.statusOf(ctx, s, llmcommon.OutcomeCooldownSkipped))
			continue
		}

		out, err := s.Scorer.EnrichBatch(ctx, inputs)
		if err == nil {
			if sErr := r.cooldown.RecordSuccess(ctx, s.SlotID); sErr != nil {
				r.warnCooldown("record success failed", s.SlotID, sErr)
			}
			return out, nil
		}
		if errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
			statuses = append(statuses, r.statusOf(ctx, s, llmcommon.OutcomeLocalQuotaRPM))
			continue
		}
		if errors.Is(err, llmerr.ErrLLMProviderRateLimited) {
			if fErr := r.cooldown.RecordFailure(ctx, s.SlotID); fErr != nil {
				r.warnCooldown("record failure failed", s.SlotID, fErr)
			}
			statuses = append(statuses, r.statusOf(ctx, s, llmcommon.OutcomeProvider429))
			continue
		}
		return nil, err
	}

	return nil, &llmerr.LLMAllExhaustedError{Slots: statuses, RetryAfter: earliestCooldown(statuses)}
}

func (r *routerService) statusOf(ctx context.Context, s RouterSlot, reason string) llmerr.LLMSlotStatus {
	cst, err := r.cooldown.Status(ctx, s.SlotID)
	if err != nil {
		r.warnCooldown("cooldown status failed", s.SlotID, err)
	}
	return llmerr.LLMSlotStatus{
		SlotID:        s.SlotID,
		Model:         s.Model,
		FailureCount:  cst.FailureCount,
		CooldownUntil: cst.CooldownUntil,
		LastDenial:    reason,
	}
}

func (r *routerService) warnCooldown(msg, slotID string, err error) {
	if r.log == nil {
		return
	}
	r.log.Warn(msg, logger.Str("slot", slotID), logger.Err(err))
}

// earliestCooldown returns the soonest non-zero CooldownUntil across the
// per-slot statuses, or the zero time when no slot is on cooldown.
func earliestCooldown(ss []llmerr.LLMSlotStatus) time.Time {
	var out time.Time
	for _, s := range ss {
		if s.CooldownUntil.IsZero() {
			continue
		}
		if out.IsZero() || s.CooldownUntil.Before(out) {
			out = s.CooldownUntil
		}
	}
	return out
}
