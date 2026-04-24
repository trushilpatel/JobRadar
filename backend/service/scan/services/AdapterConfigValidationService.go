package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// adapterConfigValidationService is the input-validation decorator for AdapterConfigService.
// It embeds BaseAdapterConfigService so only the methods it validates are overridden.
type adapterConfigValidationService struct {
	BaseAdapterConfigService
}

// NewAdapterConfigValidator constructs the validation decorator wrapping next.
func NewAdapterConfigValidator(next AdapterConfigService) AdapterConfigService {
	return &adapterConfigValidationService{
		BaseAdapterConfigService: NewBaseAdapterConfigService(next),
	}
}

// UpdateAdapterConfig validates scan_interval_minutes (1–10080) and
// inter_company_delay_s (0–3600) before delegating.
func (v *adapterConfigValidationService) UpdateAdapterConfig(ctx context.Context, adapterType string, input dto.UpdateAdapterConfigInput) (dto.AdapterConfigDTO, error) {
	var errs errors.ValidationErrors

	if adapterType == "" {
		errs = append(errs, errors.ValidationError{
			Field:   "adapter_type",
			Code:    "REQUIRED",
			Message: "adapter_type is required",
		})
	}

	if input.ScanIntervalMinutes != nil {
		if *input.ScanIntervalMinutes < 1 || *input.ScanIntervalMinutes > 10080 {
			errs = append(errs, errors.ValidationError{
				Field:   "scan_interval_minutes",
				Code:    "INVALID_VALUE",
				Message: "scan_interval_minutes must be between 1 and 10080",
			})
		}
	}

	if input.InterCompanyDelayS != nil {
		if *input.InterCompanyDelayS < 0 || *input.InterCompanyDelayS > 3600 {
			errs = append(errs, errors.ValidationError{
				Field:   "inter_company_delay_s",
				Code:    "INVALID_VALUE",
				Message: "inter_company_delay_s must be between 0 and 3600",
			})
		}
	}

	if input.ScanIntervalMinutes == nil && input.InterCompanyDelayS == nil {
		errs = append(errs, errors.ValidationError{
			Field:   "",
			Code:    "BAD_REQUEST",
			Message: "at least one field must be provided",
		})
	}

	if len(errs) > 0 {
		return dto.AdapterConfigDTO{}, errs
	}
	return v.next.UpdateAdapterConfig(ctx, adapterType, input)
}

// UpdateScanIntervalOverride validates that override is nil or in range 1–10080.
func (v *adapterConfigValidationService) UpdateScanIntervalOverride(ctx context.Context, companyID string, override *int) error {
	var errs errors.ValidationErrors

	if companyID == "" {
		errs = append(errs, errors.ValidationError{
			Field:   "company_id",
			Code:    "REQUIRED",
			Message: "company_id is required",
		})
	}

	if override != nil && (*override < 1 || *override > 10080) {
		errs = append(errs, errors.ValidationError{
			Field:   "scan_interval_minutes_override",
			Code:    "INVALID_VALUE",
			Message: "scan_interval_minutes_override must be between 1 and 10080, or null to clear",
		})
	}

	if len(errs) > 0 {
		return errs
	}
	return v.next.UpdateScanIntervalOverride(ctx, companyID, override)
}
