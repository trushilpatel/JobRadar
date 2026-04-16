package insights

import "github.com/trushilpatel/jobradar/domain"

// Validator validates user-submitted input for the insights domain.
type Validator interface {
	ValidateFilter(filter InsightsFilter) error
}

type validator struct{}

// NewValidator constructs a Validator.
func NewValidator() Validator {
	return &validator{}
}

// ValidateFilter validates that days >= 1.
func (v *validator) ValidateFilter(filter InsightsFilter) error {
	if filter.Days < 1 {
		return &domain.ValidationError{
			Field:   "days",
			Code:    "INVALID_VALUE",
			Message: "days must be >= 1",
		}
	}
	return nil
}
