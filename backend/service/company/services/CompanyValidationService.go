package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
)

// fixedURLAdapters lists adapters whose target URL is hard-coded in the scraper.
// When creating a company with one of these adapters, the URL field is optional.
var fixedURLAdapters = map[string]bool{
	companycommon.AdapterGoogle:    true,
	companycommon.AdapterNetflix:   true,
	companycommon.AdapterUber:      true,
	companycommon.AdapterApple:     true,
	companycommon.AdapterMeta:      true,
	companycommon.AdapterMicrosoft: true,
	companycommon.AdapterRippling:  true,
}

var validAdapters = map[string]bool{
	companycommon.AdapterGreenhouse: true,
	companycommon.AdapterLever:      true,
	companycommon.AdapterAshby:      true,
	companycommon.AdapterGoogle:     true,
	companycommon.AdapterNetflix:    true,
	companycommon.AdapterUber:       true,
	companycommon.AdapterApple:      true,
	companycommon.AdapterMeta:       true,
	companycommon.AdapterMicrosoft:  true,
	companycommon.AdapterRippling:   true,
}

type validationService struct {
	BaseService
}

// NewValidator constructs the validation decorator wrapping next.
func NewValidator(next Service) Service {
	return &validationService{BaseService: NewBaseService(next)}
}

// List validates filter and pagination then delegates.
func (v *validationService) List(ctx context.Context, filter companydto.CompanyFilter, page, limit int) (companydto.CompanyListResult, error) {
	var errs errors.ValidationErrors
	for _, at := range filter.AdapterTypes {
		if !validAdapters[at] {
			errs = append(errs, errors.ValidationError{
				Field:   "adapter_types",
				Code:    "INVALID_VALUE",
				Message: "adapter_types contains unrecognised adapter: " + at,
			})
			break
		}
	}
	if len(filter.Search) > 100 {
		errs = append(errs, errors.ValidationError{
			Field:   "search",
			Code:    "INVALID_VALUE",
			Message: "search must be 100 characters or fewer",
		})
	}
	if page < 1 {
		errs = append(errs, errors.ValidationError{
			Field:   "page",
			Code:    "INVALID_VALUE",
			Message: "page must be >= 1",
		})
	}
	if limit < 1 || limit > 100 {
		errs = append(errs, errors.ValidationError{
			Field:   "limit",
			Code:    "INVALID_VALUE",
			Message: "limit must be between 1 and 100",
		})
	}
	if len(errs) > 0 {
		return companydto.CompanyListResult{}, errs
	}
	return v.next.List(ctx, filter, page, limit)
}

// Create validates required fields then delegates.
func (v *validationService) Create(ctx context.Context, input companydto.CreateCompanyInput) (*companymodel.Company, error) {
	var errs errors.ValidationErrors
	if input.ID == "" {
		errs = append(errs, errors.ValidationError{Field: "id", Code: "REQUIRED", Message: "id is required"})
	}
	if input.Name == "" {
		errs = append(errs, errors.ValidationError{Field: "name", Code: "REQUIRED", Message: "name is required"})
	}
	if !fixedURLAdapters[input.AdapterType] && input.URL == "" {
		errs = append(errs, errors.ValidationError{Field: "url", Code: "REQUIRED", Message: "url is required for this adapter type"})
	}
	if !validAdapters[input.AdapterType] {
		errs = append(errs, errors.ValidationError{
			Field:   "adapter_type",
			Code:    "INVALID_VALUE",
			Message: "adapter_type is not a recognised adapter",
		})
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return v.next.Create(ctx, input)
}

// Update validates that at least one field is provided then delegates.
func (v *validationService) Update(ctx context.Context, id string, input companydto.UpdateCompanyInput) error {
	var errs errors.ValidationErrors
	if input.Name == nil && input.URL == nil && input.AdapterType == nil && input.Active == nil {
		errs = append(errs, errors.ValidationError{
			Field:   "",
			Code:    "BAD_REQUEST",
			Message: "at least one field must be provided",
		})
	}
	if input.AdapterType != nil && !validAdapters[*input.AdapterType] {
		errs = append(errs, errors.ValidationError{
			Field:   "adapter_type",
			Code:    "INVALID_VALUE",
			Message: "adapter_type is not a recognised adapter",
		})
	}
	if len(errs) > 0 {
		return errs
	}
	return v.next.Update(ctx, id, input)
}

// SetAllActive validates that the active value is present (always is — Go bool — but
// the HTTP layer passes the pointer through the body, so we validate the body struct
// upstream; this override handles the boolean-already-unwrapped path).
func (v *validationService) SetAllActive(ctx context.Context, active bool) error {
	return v.next.SetAllActive(ctx, active)
}
