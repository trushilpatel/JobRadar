package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/service/insights/dto"
)

type validationService struct {
	BaseService
}

// NewValidationService constructs an InsightsService that validates inputs before delegating.
func NewValidationService(next InsightsService) InsightsService {
	return &validationService{BaseService: NewBaseService(next)}
}

// Sponsorship validates that filter.Days >= 1 before delegating.
func (v *validationService) Sponsorship(ctx context.Context, filter dto.InsightsFilter) (interface{}, error) {
	if filter.Days < 1 {
		return nil, &domain.ValidationError{
			Field:   "days",
			Code:    "INVALID_VALUE",
			Message: "days must be >= 1",
		}
	}
	return v.next.Sponsorship(ctx, filter)
}
