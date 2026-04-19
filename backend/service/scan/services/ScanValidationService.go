package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/domain"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

var validQueueStatuses = map[string]bool{
	scancommon.QueueStatusQueued:     true,
	scancommon.QueueStatusProcessing: true,
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
	if companyID == "" {
		return &domain.ValidationError{
			Field:   "company_id",
			Code:    "REQUIRED",
			Message: "company_id is required",
		}
	}
	return v.next.TriggerScan(ctx, companyID)
}

// ListQueue validates filter parameters before delegating.
func (v *validationService) ListQueue(ctx context.Context, filter dto.QueueFilter) ([]dto.QueueEntryView, error) {
	if filter.Status != "" && !validQueueStatuses[filter.Status] {
		return nil, &domain.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be 'queued' or 'processing'",
		}
	}
	if filter.Source != "" && !validQueueSources[filter.Source] {
		return nil, &domain.ValidationError{
			Field:   "source",
			Code:    "INVALID_VALUE",
			Message: "source must be 'manual' or 'scheduler'",
		}
	}
	return v.next.ListQueue(ctx, filter)
}
