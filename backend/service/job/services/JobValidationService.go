package services

import (
	"github.com/trushilpatel/jobradar/domain"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
)

type validator struct{}

// NewValidator constructs a Validator.
func NewValidator() Validator {
	return &validator{}
}

var validJobStatuses = map[string]bool{
	jobcommon.JobStatusNew:          true,
	jobcommon.JobStatusSaved:        true,
	jobcommon.JobStatusApplied:      true,
	jobcommon.JobStatusInterviewing: true,
	jobcommon.JobStatusOffer:        true,
}

var validDiscardReasons = map[string]bool{
	jobcommon.DiscardReasonDismissed:   true,
	jobcommon.DiscardReasonNotRelevant: true,
}

// ValidateList validates list filter and pagination parameters.
func (v *validator) ValidateList(filter dto.JobFilter, page, limit int) error {
	if filter.Status != "" && !validJobStatuses[filter.Status] {
		return &domain.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be 'new', 'saved', 'applied', 'interviewing', or 'offer'",
		}
	}
	for _, r := range filter.ExperienceRanges {
		if r.Min < 0 {
			return &domain.ValidationError{
				Field:   "experience_ranges",
				Code:    "INVALID_VALUE",
				Message: "experience range min must be >= 0",
			}
		}
		if r.Max != nil && *r.Max <= r.Min {
			return &domain.ValidationError{
				Field:   "experience_ranges",
				Code:    "INVALID_VALUE",
				Message: "experience range max must be > min",
			}
		}
	}
	if page < 1 {
		return &domain.ValidationError{
			Field:   "page",
			Code:    "INVALID_VALUE",
			Message: "page must be >= 1",
		}
	}
	if limit < 1 || limit > 100 {
		return &domain.ValidationError{
			Field:   "limit",
			Code:    "INVALID_VALUE",
			Message: "limit must be between 1 and 100",
		}
	}
	return nil
}

// ValidateDismiss validates dismiss reason.
func (v *validator) ValidateDismiss(input dto.DismissInput) error {
	if !validDiscardReasons[input.Reason] {
		return &domain.ValidationError{
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
		return &domain.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be 'new', 'saved', 'applied', 'interviewing', or 'offer'",
		}
	}
	return nil
}

// ValidateSetNotes validates notes input (no constraint — empty notes are allowed).
func (v *validator) ValidateSetNotes(input dto.SetNotesInput) error {
	return nil
}
