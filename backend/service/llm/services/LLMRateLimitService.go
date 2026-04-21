package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	llmcommon "github.com/trushilpatel/jobradar/service/llm/common"
	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

// RateLimitSlot describes a single provider/key/model slot's local quota limits.
type RateLimitSlot struct {
	SlotID   string
	Provider string
	KeyID    string
	Model    string
	RPM      int
	RPD      int
	TPMIn    int
}

// rateLimitService is a per-slot decorator over an inner Scorer that reserves
// local quota before each call and releases it when the inner call fails for
// reasons other than provider rate limiting.
type rateLimitService struct {
	slot  RateLimitSlot
	inner Scorer
	store store.LLMRateLimitStore
	log   logger.Logger
}

// NewLLMRateLimitService returns a Scorer that enforces per-slot RPM/RPD/TPM
// quotas via the supplied LLMRateLimitStore. Callers compose many of these
// behind an LLMRouterService.
func NewLLMRateLimitService(slot RateLimitSlot, inner Scorer, s store.LLMRateLimitStore) Scorer {
	return &rateLimitService{slot: slot, inner: inner, store: s}
}

// WithLogger attaches a logger used to report non-fatal issues like release
// failures. Returns the same Scorer to allow fluent composition in wire.go.
func (s *rateLimitService) WithLogger(l logger.Logger) Scorer {
	s.log = l
	return s
}

func (s *rateLimitService) ScoreBatch(ctx context.Context, jobs []dto.JobsRelevanceFilterInput) ([]string, error) {
	reqID := uuid.NewString()
	tokens := estimateScoreBatchTokens(jobs)
	if err := s.reserve(ctx, reqID, tokens); err != nil {
		return nil, err
	}

	out, innerErr := s.inner.ScoreBatch(ctx, jobs)
	if err := s.aftercall(ctx, reqID, innerErr); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *rateLimitService) Enrich(ctx context.Context, in dto.JobPostEnrichInput) (*dto.JobPostEnrichResult, error) {
	reqID := uuid.NewString()
	tokens := estimateEnrichTokens(in)
	if err := s.reserve(ctx, reqID, tokens); err != nil {
		return nil, err
	}

	out, innerErr := s.inner.Enrich(ctx, in)
	if err := s.aftercall(ctx, reqID, innerErr); err != nil {
		return nil, err
	}
	return out, nil
}

// reserve asks the store for quota and returns a single error. Local-quota
// denials are translated to errors wrapping llmerr.ErrLLMLocalQuotaExceeded so
// callers (e.g. the router) can discriminate via errors.Is. Transport/infra
// failures are returned wrapped with context but without that sentinel.
func (s *rateLimitService) reserve(ctx context.Context, reqID string, tokens int) error {
	r, err := s.store.Reserve(ctx, store.ReserveReq{
		SlotID: s.slot.SlotID,
		ReqID:  reqID,
		RPM:    s.slot.RPM,
		RPD:    s.slot.RPD,
		TPMIn:  s.slot.TPMIn,
		Tokens: tokens,
	})
	if err != nil {
		return fmt.Errorf("ratelimit reserve slot=%s: %w", s.slot.SlotID, err)
	}

	switch r {
	case store.ReserveOK:
		return nil
	case store.ReserveDeniedRPM:
		return fmt.Errorf("%s: %w", llmcommon.OutcomeLocalQuotaRPM, llmerr.ErrLLMLocalQuotaExceeded)
	case store.ReserveDeniedRPD:
		return fmt.Errorf("%s: %w", llmcommon.OutcomeLocalQuotaRPD, llmerr.ErrLLMLocalQuotaExceeded)
	case store.ReserveDeniedTPM:
		return fmt.Errorf("%s: %w", llmcommon.OutcomeLocalQuotaTPM, llmerr.ErrLLMLocalQuotaExceeded)
	default:
		return fmt.Errorf("ratelimit reserve unexpected code %d", r)
	}
}

// aftercall releases the reservation on non-provider errors so the slot stays
// callable. Provider 429 errors keep the reservation — the router records a
// cooldown against the slot instead.
func (s *rateLimitService) aftercall(ctx context.Context, reqID string, innerErr error) error {
	if innerErr == nil {
		return nil
	}
	if errors.Is(innerErr, llmerr.ErrLLMProviderRateLimited) {
		return innerErr
	}
	if relErr := s.store.Release(ctx, s.slot.SlotID, reqID); relErr != nil && s.log != nil {
		s.log.Warn("ratelimit release failed",
			logger.Str("slot", s.slot.SlotID),
			logger.Err(relErr),
		)
	}
	return innerErr
}

// estimateScoreBatchTokens returns a coarse input-token estimate for a score
// batch. It intentionally over-counts slightly so the TPM limiter trends
// toward conservative behaviour under load.
func estimateScoreBatchTokens(jobs []dto.JobsRelevanceFilterInput) int {
	return 150 + len(jobs)*20
}

// estimateEnrichTokens returns a coarse input-token estimate for an enrich
// call, dominated by the full raw job text.
func estimateEnrichTokens(input dto.JobPostEnrichInput) int {
	return 200 + len(input.RawText)/4
}
