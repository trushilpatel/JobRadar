package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
)

type validationService struct {
	BaseService
}

// NewValidator constructs a validation Service that validates inputs before
// delegating to next.
func NewValidator(next Service) Service {
	return &validationService{BaseService: NewBaseService(next)}
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

// List validates filter and pagination before delegating.
func (v *validationService) List(ctx context.Context, filter dto.JobFilter, page, limit int) ([]jobmodel.Job, int, error) {
	var errs errors.ValidationErrors
	if filter.Status != "" && !validJobStatuses[filter.Status] {
		errs = append(errs, errors.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be 'new', 'saved', 'applied', 'interviewing', or 'offer'",
		})
	}
	for _, r := range filter.ExperienceRanges {
		if r.Min < 0 {
			errs = append(errs, errors.ValidationError{
				Field:   "experience_ranges",
				Code:    "INVALID_VALUE",
				Message: "experience range min must be >= 0",
			})
		}
		if r.Max != nil && *r.Max <= r.Min {
			errs = append(errs, errors.ValidationError{
				Field:   "experience_ranges",
				Code:    "INVALID_VALUE",
				Message: "experience range max must be > min",
			})
		}
	}
	if len(filter.Search) > 200 {
		errs = append(errs, errors.ValidationError{
			Field:   "search",
			Code:    "INVALID_VALUE",
			Message: "search must be 200 characters or fewer",
		})
	}
	if page < 1 {
		errs = append(errs, errors.ValidationError{
			Field:   "page",
			Code:    "INVALID_VALUE",
			Message: "page must be >= 1",
		})
	}
	if limit < 1 || limit > 100 {
		errs = append(errs, errors.ValidationError{
			Field:   "limit",
			Code:    "INVALID_VALUE",
			Message: "limit must be between 1 and 100",
		})
	}
	if len(errs) > 0 {
		return nil, 0, errs
	}
	return v.next.List(ctx, filter, page, limit)
}

// Dismiss validates the dismiss input before delegating.
func (v *validationService) Dismiss(ctx context.Context, id string, input dto.DismissInput) error {
	var errs errors.ValidationErrors
	if !validDiscardReasons[input.Reason] {
		errs = append(errs, errors.ValidationError{
			Field:   "reason",
			Code:    "INVALID_VALUE",
			Message: "reason must be 'user_dismissed' or 'user_not_relevant'",
		})
	}
	if len(errs) > 0 {
		return errs
	}
	return v.next.Dismiss(ctx, id, input)
}

// SetStatus validates the status value before delegating.
func (v *validationService) SetStatus(ctx context.Context, id, status string) error {
	var errs errors.ValidationErrors
	if !validJobStatuses[status] {
		errs = append(errs, errors.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be 'new', 'saved', 'applied', 'interviewing', or 'offer'",
		})
	}
	if len(errs) > 0 {
		return errs
	}
	return v.next.SetStatus(ctx, id, status)
}

// SetNotes delegates directly — notes have no constraints.
// (BaseService delegation would also work, but being explicit avoids confusion.)
func (v *validationService) SetNotes(ctx context.Context, id, notes string) error {
	return v.next.SetNotes(ctx, id, notes)
}

// BulkDismiss validates the bulk dismiss input before delegating.
func (v *validationService) BulkDismiss(ctx context.Context, input dto.BulkDismissInput) (int, error) {
	var errs errors.ValidationErrors
	if len(input.IDs) == 0 {
		errs = append(errs, errors.ValidationError{
			Field:   "ids",
			Code:    "REQUIRED",
			Message: "ids must not be empty",
		})
	}
	if len(input.IDs) > 100 {
		errs = append(errs, errors.ValidationError{
			Field:   "ids",
			Code:    "INVALID_VALUE",
			Message: "ids must contain 100 or fewer entries",
		})
	}
	if len(errs) > 0 {
		return 0, errs
	}
	return v.next.BulkDismiss(ctx, input)
}
