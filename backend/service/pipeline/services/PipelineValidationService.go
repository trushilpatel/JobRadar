package services

import (
	"context"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	pipelinedto "github.com/trushilpatel/jobradar/service/pipeline/dto"
)

// validRetryTargets maps worker → tier for TriggerRetry input validation.
var validRetryTargets = map[string]map[string]struct{}{
	"filter": {
		"1m": {}, "10m": {}, "1h": {}, "dlq": {},
	},
	"enricher": {
		"1m": {}, "10m": {}, "1h": {}, "dlq": {},
	},
}

// validationService is the input-validation layer of the pipeline chain.
// Only TriggerRetry needs validation; GetStats passes through unmodified.
type validationService struct {
	BasePipelineService
}

// NewValidationService constructs the validation layer.
func NewValidationService(next PipelineService) PipelineService {
	return &validationService{BasePipelineService: NewBasePipelineService(next)}
}

// TriggerRetry validates the (worker, tier) pair before delegating to next.
func (v *validationService) TriggerRetry(
	ctx context.Context,
	input pipelinedto.TriggerRetryInput,
) (pipelinedto.TriggerRetryResult, error) {
	tiers, ok := validRetryTargets[input.Worker]
	if !ok {
		return pipelinedto.TriggerRetryResult{}, domainerrs.ErrBadRequest.WithExternal("unknown worker")
	}
	if _, ok := tiers[input.Tier]; !ok {
		return pipelinedto.TriggerRetryResult{}, domainerrs.ErrBadRequest.WithExternal("unknown tier")
	}
	return v.next.TriggerRetry(ctx, input)
}

// Compile-time assertion: validationService implements PipelineService.
var _ PipelineService = (*validationService)(nil)
