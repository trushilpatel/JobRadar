package services

import (
	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

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

// ValidateList validates list filter and pagination parameters.
func (v *validator) ValidateList(filter companydto.CompanyFilter, page, limit int) error {
	if filter.AdapterType != "" && !validAdapters[filter.AdapterType] {
		return &domain.ValidationError{
			Field:   "adapter_type",
			Code:    "INVALID_VALUE",
			Message: "adapter_type is not a recognised adapter",
		}
	}
	if len(filter.Search) > 100 {
		return &domain.ValidationError{
			Field:   "search",
			Code:    "INVALID_VALUE",
			Message: "search must be 100 characters or fewer",
		}
	}
	if page < 1 {
		return &domain.ValidationError{
			Field:   "page",
			Code:    "INVALID_VALUE",
			Message: "page must be >= 1",
		}
	}
	if limit < 1 || limit > 100 {
		return &domain.ValidationError{
			Field:   "limit",
			Code:    "INVALID_VALUE",
			Message: "limit must be between 1 and 100",
		}
	}
	return nil
}

// ValidateCreate validates all required fields for company creation.
func (v *validator) ValidateCreate(input companydto.CreateCompanyInput) error {
	if input.ID == "" {
		return &domain.ValidationError{Field: "id", Code: "REQUIRED", Message: "id is required"}
	}
	if input.Name == "" {
		return &domain.ValidationError{Field: "name", Code: "REQUIRED", Message: "name is required"}
	}
	if !fixedURLAdapters[input.AdapterType] && input.URL == "" {
		return &domain.ValidationError{Field: "url", Code: "REQUIRED", Message: "url is required for this adapter type"}
	}
	if !validAdapters[input.AdapterType] {
		return &domain.ValidationError{
			Field:   "adapter_type",
			Code:    "INVALID_VALUE",
			Message: "adapter_type is not a recognised adapter",
		}
	}
	return nil
}

// ValidateUpdate validates update input — at least one field must be provided,
// and adapter_type must be a valid constant if set.
func (v *validator) ValidateUpdate(input companydto.UpdateCompanyInput) error {
	if input.Name == nil && input.URL == nil && input.AdapterType == nil && input.Active == nil {
		return &domain.ValidationError{
			Field:   "",
			Code:    "BAD_REQUEST",
			Message: "at least one field must be provided",
		}
	}
	if input.AdapterType != nil && !validAdapters[*input.AdapterType] {
		return &domain.ValidationError{
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
		return &domain.ValidationError{
			Field:   "active",
			Code:    "REQUIRED",
			Message: "active (bool) is required",
		}
	}
	return nil
}
