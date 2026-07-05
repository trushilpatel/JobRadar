package services

import (
	"context"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/ats/adaptermeta"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
)

// validScanStatuses is the admin-list scan_status filter allow-list. Keyed by
// the owning scan-domain constants so the validator and the repo switch share
// one source of truth. An empty/unset scan_status means "no filter".
var validScanStatuses = map[string]bool{
	scancommon.QueueStatusQueued:  true,
	scancommon.QueueStatusRunning: true,
	scancommon.ScanStatusError:    true,
	scancommon.ScanStatusOK:       true,
}

type adminValidationService struct {
	BaseAdminService
	atsSvc atssvc.AtsService
}

// NewAdminValidationService wraps next with the admin validator.
// atsSvc is used to validate adapter_type values against the live ats table.
func NewAdminValidationService(next CompanyAdminService, atsSvc atssvc.AtsService) CompanyAdminService {
	return &adminValidationService{BaseAdminService: NewBaseAdminService(next), atsSvc: atsSvc}
}

// List validates filter and pagination then delegates.
func (v *adminValidationService) List(ctx context.Context, filter companydto.CompanyFilter, limit int) (companydto.CompanyAdminListResult, error) {
	var errs apperr.ValidationErrors
	for _, at := range filter.AdapterTypes {
		if _, err := v.atsSvc.GetATS(ctx, at); err != nil {
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
	if filter.ScanStatus != "" && !validScanStatuses[filter.ScanStatus] {
		errs = append(errs, apperr.ValidationError{
			Field:   "scan_status",
			Code:    "INVALID_VALUE",
			Message: "scan_status must be one of queued, running, error, ok",
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
	settingsErrs, err := v.validateScanSettings(ctx, settings, true)
	if err != nil {
		return nil, err
	}
	allErrs = append(allErrs, settingsErrs...)
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
	settingsErrs, err := v.validateScanSettings(ctx, p, false)
	if err != nil {
		return err
	}
	errs = append(errs, settingsErrs...)
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

// validateScanSettings checks adapter_type when present via a live AtsService
// lookup. When requireAdapter is true AND the input is non-empty, adapter_type
// must be supplied — Create passes true (a scan_settings row makes no sense
// without an adapter), Update passes false (partial-patch: adapter_type is
// only validated when present).
//
// For the Create path it additionally checks that the adapter's
// IsAdminCreatable flag is true — adapters that require extra configuration
// (e.g. eightfold) are blocked until the admin UI supports their config form.
//
// On both Create and Update it also checks adaptermeta.RequiresConfig:
// adapters that only work via FetchJobListingWithConfig (workday, eightfold)
// must be saved with a non-empty adapter_config, or crawling always fails
// generically at run time.
//
// Returns (ValidationErrors, nil) on validation failure, (nil, ServiceError)
// on an unexpected AtsService error, or (nil, nil) when all checks pass.
func (v *adminValidationService) validateScanSettings(ctx context.Context, s companydto.CompanyScanSettingsInput, requireAdapter bool) (apperr.ValidationErrors, error) {
	var errs apperr.ValidationErrors
	if requireAdapter && !s.IsEmpty() && s.AdapterType == nil {
		errs = append(errs, apperr.ValidationError{
			Field:   "adapter_type",
			Code:    "INVALID_VALUE",
			Message: "adapter_type is required when scan settings are provided",
		})
	}
	if s.AdapterType != nil {
		ats, err := v.atsSvc.GetATS(ctx, *s.AdapterType)
		if err != nil {
			if apperr.IsNotFound(err) {
				errs = append(errs, apperr.ValidationError{
					Field:   "adapter_type",
					Code:    "INVALID_VALUE",
					Message: "adapter_type is not a recognised adapter",
				})
			} else {
				// Unexpected service error — propagate as-is so the handler can map it.
				return nil, err
			}
		} else if requireAdapter && !ats.IsAdminCreatable {
			errs = append(errs, apperr.ValidationError{
				Field:   "adapter_type",
				Code:    "INVALID_VALUE",
				Message: "adapter_type is not available for admin creation",
			})
		}
		// Some adapters (workday, eightfold) always error on a plain crawl —
		// they only work via FetchJobListingWithConfig, which requires a
		// non-empty adapter_config. Catch that misconfiguration at write time
		// instead of leaving it to fail generically at crawl time. Applies on
		// both Create and Update (unlike the Create-only is_admin_creatable
		// gate above) since either path can set adapter_type. This is a
		// presence-only check — adapter_config content validity (e.g. `{}`
		// missing required sub-fields) remains the adapter's own validate()
		// responsibility at crawl time.
		if adaptermeta.RequiresConfig(*s.AdapterType) && (s.AdapterConfig == nil || len(*s.AdapterConfig) == 0) {
			errs = append(errs, apperr.ValidationError{
				Field:   "adapter_config",
				Code:    "INVALID_VALUE",
				Message: "adapter_config is required (non-empty) for adapter_type " + *s.AdapterType,
			})
		}
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
	return errs, nil
}

var _ CompanyAdminService = (*adminValidationService)(nil)
