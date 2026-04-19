package services

import (
	"github.com/trushilpatel/jobradar/common/domain"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// Validator validates user-submitted input for the scan domain.
type Validator interface {
	ValidateListQueue(filter dto.QueueFilter) error
	ValidateTriggerScan(companyID string) error
}

type validator struct{}

// NewValidator constructs a Validator.
func NewValidator() Validator {
	return &validator{}
}

var validQueueStatuses = map[string]bool{
	scancommon.QueueStatusQueued:     true,
	scancommon.QueueStatusProcessing: true,
}

var validQueueSources = map[string]bool{
	scancommon.QueueSourceScheduler: true,
	scancommon.QueueSourceManual:    true,
}

// ValidateListQueue validates filter parameters for listing the queue.
func (v *validator) ValidateListQueue(filter dto.QueueFilter) error {
	if filter.Status != "" && !validQueueStatuses[filter.Status] {
		return &domain.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be 'queued' or 'processing'",
		}
	}
	if filter.Source != "" && !validQueueSources[filter.Source] {
		return &domain.ValidationError{
			Field:   "source",
			Code:    "INVALID_VALUE",
			Message: "source must be 'manual' or 'scheduler'",
		}
	}
	return nil
}

// ValidateTriggerScan validates that companyID is non-empty.
func (v *validator) ValidateTriggerScan(companyID string) error {
	if companyID == "" {
		return &domain.ValidationError{
			Field:   "company_id",
			Code:    "REQUIRED",
			Message: "company_id is required",
		}
	}
	return nil
}
