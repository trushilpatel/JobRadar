package company

import (
	"github.com/trushilpatel/jobradar/api/response"
	"github.com/trushilpatel/jobradar/model"
)

// Validator validates user-submitted input for the company domain.
// It never makes DB calls.
type Validator interface {
	ValidateList(filter CompanyFilter) error
	ValidateCreate(input CreateCompanyInput) error
	ValidateUpdate(input UpdateCompanyInput) error
	ValidateSetAllActive(active *bool) error
}

type validator struct{}

// NewValidator constructs a Validator.
func NewValidator() Validator {
	return &validator{}
}

// fixedURLAdapters lists adapters whose target URL is hard-coded in the scraper.
// When creating a company with one of these adapters, the URL field is optional.
var fixedURLAdapters = map[string]bool{
	model.AdapterGoogle:    true,
	model.AdapterNetflix:   true,
	model.AdapterUber:      true,
	model.AdapterApple:     true,
	model.AdapterMeta:      true,
	model.AdapterMicrosoft: true,
	model.AdapterRippling:  true,
}

var validAdapters = map[string]bool{
	model.AdapterGreenhouse: true,
	model.AdapterLever:      true,
	model.AdapterAshby:      true,
	model.AdapterGoogle:     true,
	model.AdapterNetflix:    true,
	model.AdapterUber:       true,
	model.AdapterApple:      true,
	model.AdapterMeta:       true,
	model.AdapterMicrosoft:  true,
	model.AdapterRippling:   true,
}

// ValidateList validates list filter parameters.
func (v *validator) ValidateList(filter CompanyFilter) error {
	if filter.AdapterType != "" && !validAdapters[filter.AdapterType] {
		return &response.ValidationError{
			Field:   "adapter_type",
			Code:    "INVALID_VALUE",
			Message: "adapter_type is not a recognised adapter",
		}
	}
	return nil
}

// ValidateCreate validates all required fields for company creation.
func (v *validator) ValidateCreate(input CreateCompanyInput) error {
	if input.ID == "" {
		return &response.ValidationError{Field: "id", Code: "REQUIRED", Message: "id is required"}
	}
	if input.Name == "" {
		return &response.ValidationError{Field: "name", Code: "REQUIRED", Message: "name is required"}
	}
	if !fixedURLAdapters[input.AdapterType] && input.URL == "" {
		return &response.ValidationError{Field: "url", Code: "REQUIRED", Message: "url is required for this adapter type"}
	}
	if !validAdapters[input.AdapterType] {
		return &response.ValidationError{
			Field:   "adapter_type",
			Code:    "INVALID_VALUE",
			Message: "adapter_type is not a recognised adapter",
		}
	}
	return nil
}

// ValidateUpdate validates update input — at least one field must be provided,
// and adapter_type must be a valid constant if set.
func (v *validator) ValidateUpdate(input UpdateCompanyInput) error {
	if input.Name == nil && input.URL == nil && input.AdapterType == nil && input.Active == nil {
		return &response.ValidationError{
			Field:   "",
			Code:    "BAD_REQUEST",
			Message: "at least one field must be provided",
		}
	}
	if input.AdapterType != nil && !validAdapters[*input.AdapterType] {
		return &response.ValidationError{
			Field:   "adapter_type",
			Code:    "INVALID_VALUE",
			Message: "adapter_type is not a recognised adapter",
		}
	}
	return nil
}

// ValidateSetAllActive validates that the active pointer is non-nil.
func (v *validator) ValidateSetAllActive(active *bool) error {
	if active == nil {
		return &response.ValidationError{
			Field:   "active",
			Code:    "REQUIRED",
			Message: "active (bool) is required",
		}
	}
	return nil
}
