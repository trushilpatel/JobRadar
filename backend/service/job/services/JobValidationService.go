package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/domain"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"github.com/trushilpatel/jobradar/service/job/dto"
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
	if filter.Status != "" && !validJobStatuses[filter.Status] {
		return nil, 0, &domain.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be 'new', 'saved', 'applied', 'interviewing', or 'offer'",
		}
	}
	for _, r := range filter.ExperienceRanges {
		if r.Min < 0 {
			return nil, 0, &domain.ValidationError{
				Field:   "experience_ranges",
				Code:    "INVALID_VALUE",
				Message: "experience range min must be >= 0",
			}
		}
		if r.Max != nil && *r.Max <= r.Min {
			return nil, 0, &domain.ValidationError{
				Field:   "experience_ranges",
				Code:    "INVALID_VALUE",
				Message: "experience range max must be > min",
			}
		}
	}
	if page < 1 {
		return nil, 0, &domain.ValidationError{
			Field:   "page",
			Code:    "INVALID_VALUE",
			Message: "page must be >= 1",
		}
	}
	if limit < 1 || limit > 100 {
		return nil, 0, &domain.ValidationError{
			Field:   "limit",
			Code:    "INVALID_VALUE",
			Message: "limit must be between 1 and 100",
		}
	}
	return v.next.List(ctx, filter, page, limit)
}

// Dismiss validates the dismiss input before delegating.
func (v *validationService) Dismiss(ctx context.Context, id string, input dto.DismissInput) error {
	if !validDiscardReasons[input.Reason] {
		return &domain.ValidationError{
			Field:   "reason",
			Code:    "INVALID_VALUE",
			Message: "reason must be 'user_dismissed' or 'user_not_relevant'",
		}
	}
	return v.next.Dismiss(ctx, id, input)
}

// SetStatus validates the status value before delegating.
func (v *validationService) SetStatus(ctx context.Context, id, status string) error {
	if !validJobStatuses[status] {
		return &domain.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be 'new', 'saved', 'applied', 'interviewing', or 'offer'",
		}
	}
	return v.next.SetStatus(ctx, id, status)
}

// SetNotes delegates directly — notes have no constraints.
// (BaseService delegation would also work, but being explicit avoids confusion.)
func (v *validationService) SetNotes(ctx context.Context, id, notes string) error {
	return v.next.SetNotes(ctx, id, notes)
}
