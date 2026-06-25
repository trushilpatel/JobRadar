package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/insights/dto"
)

var _ InsightsService = (*validationService)(nil)

type validationService struct {
	BaseService
}

// NewValidationService constructs an InsightsService that validates inputs before delegating.
func NewValidationService(next InsightsService) InsightsService {
	return &validationService{BaseService: NewBaseService(next)}
}

// Sponsorship validates that filter.Days >= 1 and <= 365 before delegating.
func (v *validationService) Sponsorship(ctx context.Context, filter dto.InsightsFilter) (_ []dto.SponsorshipDay, err error) {
	ctx, span := tracing.Start(ctx, "insights.validation.Sponsorship")
	defer tracing.End(span, &err)
	var errs errors.ValidationErrors
	if filter.Days < 1 {
		errs = append(errs, errors.ValidationError{
			Field:   "days",
			Code:    "INVALID_VALUE",
			Message: "days must be >= 1",
		})
	}
	if filter.Days > 365 {
		errs = append(errs, errors.ValidationError{
			Field:   "days",
			Code:    "INVALID_VALUE",
			Message: "days must be <= 365",
		})
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return v.next.Sponsorship(ctx, filter)
}
