package services

import (
	"context"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// validAdapters mirrors the legacy validator: any adapter referenced in a
// scan_settings input must appear here. Keep in sync with
// common/CompanyServiceConstants.go.
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
	companycommon.AdapterGem:        true,
	companycommon.AdapterDeel:       true,
	companycommon.AdapterWorkday:    true,
	// NOTE: eightfold is registered in adapter_configs (migration 056) and
	// wired into the crawler, but intentionally NOT exposed via the admin
	// validation surface — the AddCompany modal does not yet collect the
	// host+domain adapter_config that the eightfold adapter requires.
	// Re-enable here when the modal gains an eightfold config form.
}

type adminValidationService struct {
	BaseAdminService
}

// NewAdminValidationService wraps next with the admin validator.
func NewAdminValidationService(next CompanyAdminService) CompanyAdminService {
	return &adminValidationService{BaseAdminService: NewBaseAdminService(next)}
}

// List validates filter and pagination then delegates.
func (v *adminValidationService) List(ctx context.Context, filter companydto.CompanyFilter, limit int) (companydto.CompanyAdminListResult, error) {
	var errs apperr.ValidationErrors
	for _, at := range filter.AdapterTypes {
		if !validAdapters[at] {
			errs = append(errs, apperr.ValidationError{
				Field:   "adapter_types",
				Code:    "INVALID_VALUE",
				Message: "adapter_types contains unrecognised adapter: " + at,
			})
			break
		}
	}
	if len(filter.Search) > 100 {
		errs = append(errs, apperr.ValidationError{
			Field:   "search",
			Code:    "INVALID_VALUE",
			Message: "search must be 100 characters or fewer",
		})
	}
	if limit < 1 || limit > 100 {
		errs = append(errs, apperr.ValidationError{
			Field:   "limit",
			Code:    "INVALID_VALUE",
			Message: "limit must be between 1 and 100",
		})
	}
	if len(errs) > 0 {
		return companydto.CompanyAdminListResult{}, errs
	}
	return v.next.List(ctx, filter, limit)
}

// Create validates required public fields and the scan_settings adapter
// (when supplied) then delegates. All validation errors from both halves are
// always combined before returning so the caller sees the full error set
// in one response.
func (v *adminValidationService) Create(ctx context.Context, public companydto.CompanyPublicInput, settings companydto.CompanyScanSettingsInput) (*companydto.CompanyAdminView, error) {
	var allErrs apperr.ValidationErrors
	allErrs = append(allErrs, validatePublicCreate(public)...)
	allErrs = append(allErrs, validateScanSettings(settings, true)...)
	if len(allErrs) > 0 {
		return nil, allErrs
	}
	return v.next.Create(ctx, public, settings)
}

// UpdatePublic validates the partial-patch — at least one field must be
// supplied; supplied fields are non-empty and within max-length bounds.
func (v *adminValidationService) UpdatePublic(ctx context.Context, id string, p companydto.CompanyPublicInput) error {
	var errs apperr.ValidationErrors
	if p.Name == "" && p.Slug == "" && p.Website == nil && p.CareersURL == nil && p.LogoURL == nil {
		errs = append(errs, apperr.ValidationError{
			Field:   "",
			Code:    "BAD_REQUEST",
			Message: "at least one field must be provided",
		})
	}
	if p.Name != "" && len(p.Name) > 200 {
		errs = append(errs, apperr.ValidationError{Field: "name", Code: "INVALID_VALUE", Message: "name must be 200 characters or fewer"})
	}
	if p.Slug != "" && len(p.Slug) > 100 {
		errs = append(errs, apperr.ValidationError{Field: "slug", Code: "INVALID_VALUE", Message: "slug must be 100 characters or fewer"})
	}
	if p.Website != nil && len(*p.Website) > 2048 {
		errs = append(errs, apperr.ValidationError{Field: "website", Code: "INVALID_VALUE", Message: "website must be 2048 characters or fewer"})
	}
	if p.CareersURL != nil && len(*p.CareersURL) > 2048 {
		errs = append(errs, apperr.ValidationError{Field: "careers_url", Code: "INVALID_VALUE", Message: "careers_url must be 2048 characters or fewer"})
	}
	if p.LogoURL != nil && len(*p.LogoURL) > 2048 {
		errs = append(errs, apperr.ValidationError{Field: "logo_url", Code: "INVALID_VALUE", Message: "logo_url must be 2048 characters or fewer"})
	}
	if len(errs) > 0 {
		return errs
	}
	return v.next.UpdatePublic(ctx, id, p)
}

// UpdateScanSettings validates the partial-patch and the adapter_type when
// supplied.
func (v *adminValidationService) UpdateScanSettings(ctx context.Context, id string, p companydto.CompanyScanSettingsInput) error {
	var errs apperr.ValidationErrors
	if p.IsEmpty() {
		errs = append(errs, apperr.ValidationError{
			Field:   "",
			Code:    "BAD_REQUEST",
			Message: "at least one field must be provided",
		})
	}
	errs = append(errs, validateScanSettings(p, false)...)
	if len(errs) > 0 {
		return errs
	}
	return v.next.UpdateScanSettings(ctx, id, p)
}

// validatePublicCreate enforces the required-public-field invariants used
// only on Create. Also enforces max-length bounds to prevent unbounded
// DB column overflows and storage abuse.
func validatePublicCreate(p companydto.CompanyPublicInput) apperr.ValidationErrors {
	var errs apperr.ValidationErrors
	if p.ID == "" {
		errs = append(errs, apperr.ValidationError{Field: "id", Code: "REQUIRED", Message: "id is required"})
	} else if len(p.ID) > 100 {
		errs = append(errs, apperr.ValidationError{Field: "id", Code: "INVALID_VALUE", Message: "id must be 100 characters or fewer"})
	}
	if p.Name == "" {
		errs = append(errs, apperr.ValidationError{Field: "name", Code: "REQUIRED", Message: "name is required"})
	} else if len(p.Name) > 200 {
		errs = append(errs, apperr.ValidationError{Field: "name", Code: "INVALID_VALUE", Message: "name must be 200 characters or fewer"})
	}
	if p.Slug == "" {
		errs = append(errs, apperr.ValidationError{Field: "slug", Code: "REQUIRED", Message: "slug is required"})
	} else if len(p.Slug) > 100 {
		errs = append(errs, apperr.ValidationError{Field: "slug", Code: "INVALID_VALUE", Message: "slug must be 100 characters or fewer"})
	}
	if p.Website != nil && len(*p.Website) > 2048 {
		errs = append(errs, apperr.ValidationError{Field: "website", Code: "INVALID_VALUE", Message: "website must be 2048 characters or fewer"})
	}
	if p.CareersURL != nil && len(*p.CareersURL) > 2048 {
		errs = append(errs, apperr.ValidationError{Field: "careers_url", Code: "INVALID_VALUE", Message: "careers_url must be 2048 characters or fewer"})
	}
	if p.LogoURL != nil && len(*p.LogoURL) > 2048 {
		errs = append(errs, apperr.ValidationError{Field: "logo_url", Code: "INVALID_VALUE", Message: "logo_url must be 2048 characters or fewer"})
	}
	return errs
}

// validateScanSettings checks adapter_type when present. Set requireAdapter
// when the input is non-empty AND supplied via Create — Update only checks
// adapter_type when explicitly patched.
func validateScanSettings(s companydto.CompanyScanSettingsInput, _ bool) apperr.ValidationErrors {
	var errs apperr.ValidationErrors
	if s.AdapterType != nil && !validAdapters[*s.AdapterType] {
		errs = append(errs, apperr.ValidationError{
			Field:   "adapter_type",
			Code:    "INVALID_VALUE",
			Message: "adapter_type is not a recognised adapter",
		})
	}
	if s.ScanIntervalOverride != nil && *s.ScanIntervalOverride <= 0 {
		errs = append(errs, apperr.ValidationError{
			Field:   "scan_interval_override",
			Code:    "INVALID_VALUE",
			Message: "scan_interval_override must be > 0",
		})
	}
	if s.AdapterConfig != nil && len(*s.AdapterConfig) > 10240 {
		errs = append(errs, apperr.ValidationError{
			Field:   "adapter_config",
			Code:    "INVALID_VALUE",
			Message: "adapter_config must be 10240 bytes (10 KB) or fewer",
		})
	}
	return errs
}

var _ CompanyAdminService = (*adminValidationService)(nil)
