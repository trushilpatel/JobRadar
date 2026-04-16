package scan

import (
	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
)

// Validator validates user-submitted input for the scan domain.
type Validator interface {
	ValidateListQueue(filter QueueFilter) error
	ValidateTriggerScan(companyID string) error
}

type validator struct{}

// NewValidator constructs a Validator.
func NewValidator() Validator {
	return &validator{}
}

var validQueueStatuses = map[string]bool{
	model.QueueStatusQueued:     true,
	model.QueueStatusProcessing: true,
}

var validQueueSources = map[string]bool{
	model.QueueSourceScheduler: true,
	model.QueueSourceManual:    true,
}

// ValidateListQueue validates filter parameters for listing the queue.
func (v *validator) ValidateListQueue(filter QueueFilter) error {
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
