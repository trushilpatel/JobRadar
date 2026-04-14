package job

import (
	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/api/response"
	"github.com/trushilpatel/jobradar/model"
)

// Validator validates user-submitted input for the job domain.
type Validator interface {
	ValidateList(filter JobFilter, p request.Pagination) error
	ValidateDismiss(input DismissInput) error
	ValidateSetStatus(status string) error
	ValidateSetNotes(input SetNotesInput) error
}

type validator struct{}

// NewValidator constructs a Validator.
func NewValidator() Validator {
	return &validator{}
}

var validJobStatuses = map[string]bool{
	model.JobStatusNew:     true,
	model.JobStatusSaved:   true,
	model.JobStatusApplied: true,
}

var validDiscardReasons = map[string]bool{
	model.DiscardReasonDismissed:   true,
	model.DiscardReasonNotRelevant: true,
}

// ValidateList validates list filter and pagination parameters.
func (v *validator) ValidateList(filter JobFilter, p request.Pagination) error {
	if filter.Status != "" && !validJobStatuses[filter.Status] {
		return &response.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be 'new', 'saved', or 'applied'",
		}
	}
	if p.Page < 1 {
		return &response.ValidationError{
			Field:   "page",
			Code:    "INVALID_VALUE",
			Message: "page must be >= 1",
		}
	}
	if p.Limit < 1 || p.Limit > 100 {
		return &response.ValidationError{
			Field:   "limit",
			Code:    "INVALID_VALUE",
			Message: "limit must be between 1 and 100",
		}
	}
	return nil
}

// ValidateDismiss validates dismiss reason.
func (v *validator) ValidateDismiss(input DismissInput) error {
	if !validDiscardReasons[input.Reason] {
		return &response.ValidationError{
			Field:   "reason",
			Code:    "INVALID_VALUE",
			Message: "reason must be 'user_dismissed' or 'user_not_relevant'",
		}
	}
	return nil
}

// ValidateSetStatus validates that status is a valid user-settable value.
func (v *validator) ValidateSetStatus(status string) error {
	if !validJobStatuses[status] {
		return &response.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be 'new', 'saved', or 'applied'",
		}
	}
	return nil
}

// ValidateSetNotes validates notes input (no constraint — empty notes are allowed).
func (v *validator) ValidateSetNotes(input SetNotesInput) error {
	return nil
}
