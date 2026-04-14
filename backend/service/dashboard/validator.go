package dashboard

import "github.com/trushilpatel/jobradar/api/response"

// Validator validates user-submitted input for the dashboard domain.
type Validator interface {
	ValidateActivity(days int) error
}

type validator struct{}

// NewValidator constructs a Validator.
func NewValidator() Validator {
	return &validator{}
}

// ValidateActivity validates that days >= 1.
func (v *validator) ValidateActivity(days int) error {
	if days < 1 {
		return &response.ValidationError{
			Field:   "days",
			Code:    "INVALID_VALUE",
			Message: "days must be >= 1",
		}
	}
	return nil
}
