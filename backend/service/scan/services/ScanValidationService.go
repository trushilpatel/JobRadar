package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

var validQueueStatuses = map[string]bool{
	scancommon.QueueStatusQueued:  true,
	scancommon.QueueStatusRunning: true,
}

var validQueueSources = map[string]bool{
	scancommon.QueueSourceScheduler: true,
	scancommon.QueueSourceManual:    true,
}

// validationService is the input-validation decorator.
// It embeds BaseService so only the methods it validates are overridden.
type validationService struct {
	BaseService
}

// NewValidator constructs the ValidationService decorator.
func NewValidator(next Service) Service {
	return &validationService{BaseService: NewBaseService(next)}
}

// TriggerScan validates that companyID is non-empty before delegating.
func (v *validationService) TriggerScan(ctx context.Context, companyID string) error {
	var errs errors.ValidationErrors
	if companyID == "" {
		errs = append(errs, errors.ValidationError{
			Field:   "company_id",
			Code:    "REQUIRED",
			Message: "company_id is required",
		})
	}
	if len(errs) > 0 {
		return errs
	}
	return v.next.TriggerScan(ctx, companyID)
}

// ListQueue validates filter parameters before delegating.
func (v *validationService) ListQueue(ctx context.Context, filter dto.QueueFilter) ([]dto.QueueEntryView, error) {
	var errs errors.ValidationErrors
	if filter.Status != "" && !validQueueStatuses[filter.Status] {
		errs = append(errs, errors.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be 'queued' or 'running'",
		})
	}
	if filter.Source != "" && !validQueueSources[filter.Source] {
		errs = append(errs, errors.ValidationError{
			Field:   "source",
			Code:    "INVALID_VALUE",
			Message: "source must be 'manual' or 'scheduler'",
		})
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return v.next.ListQueue(ctx, filter)
}
