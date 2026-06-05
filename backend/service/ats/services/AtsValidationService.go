package services

import (
	"context"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
)

// validationService is the input-validation layer of the ats chain. Only
// UpdateATS needs validation; ListATS and GetATS pass through unmodified.
type validationService struct {
	BaseAtsService
}

// NewAtsValidationService constructs the validation layer.
func NewAtsValidationService(next AtsService) AtsService {
	return &validationService{BaseAtsService: NewBaseAtsService(next)}
}

const (
	scanIntervalMinLow  = 1
	scanIntervalMinHigh = 10080 // 7 days in minutes
	delayMinLow         = 0
	delayMaxHigh        = 3600
)

// UpdateATS validates the input before delegating to next.
func (v *validationService) UpdateATS(ctx context.Context, name string, input atsdto.UpdateATSInput) (atsdto.ATSDTO, error) {
	var errs domainerrs.ValidationErrors

	if name == "" {
		errs = append(errs, domainerrs.ValidationError{
			Field:   "name",
			Code:    "REQUIRED",
			Message: "name is required",
		})
	}

	if input.ScanIntervalMin != nil {
		v := *input.ScanIntervalMin
		if v < scanIntervalMinLow || v > scanIntervalMinHigh {
			errs = append(errs, domainerrs.ValidationError{
				Field:   "scan_interval_min",
				Code:    "INVALID_VALUE",
				Message: "scan_interval_min must be between 1 and 10080",
			})
		}
	}

	if input.InterCompanyDelayS != nil {
		d := *input.InterCompanyDelayS
		if d < delayMinLow || d > delayMaxHigh {
			errs = append(errs, domainerrs.ValidationError{
				Field:   "inter_company_delay_s",
				Code:    "INVALID_VALUE",
				Message: "inter_company_delay_s must be between 0 and 3600",
			})
		}
	}

	if len(errs) > 0 {
		return atsdto.ATSDTO{}, errs
	}

	// At least one field must be provided — all nil means no-op update.
	if input.ScanIntervalMin == nil && input.InterCompanyDelayS == nil &&
		input.IsActive == nil && input.IsAdminCreatable == nil {
		return atsdto.ATSDTO{}, domainerrs.ErrBadRequest.WithExternal("at least one field required")
	}

	return v.next.UpdateATS(ctx, name, input)
}

// Compile-time assertion: validationService implements AtsService.
var _ AtsService = (*validationService)(nil)
