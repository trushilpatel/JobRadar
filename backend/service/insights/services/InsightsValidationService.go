package services

import (
	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/service/insights/dto"
)

type validationService struct{}

// NewValidationService constructs a Validator implementation.
func NewValidationService() Validator {
	return &validationService{}
}

// ValidateFilter validates that days >= 1.
func (v *validationService) ValidateFilter(filter dto.InsightsFilter) error {
	if filter.Days < 1 {
		return &domain.ValidationError{
			Field:   "days",
			Code:    "INVALID_VALUE",
			Message: "days must be >= 1",
		}
	}
	return nil
}
