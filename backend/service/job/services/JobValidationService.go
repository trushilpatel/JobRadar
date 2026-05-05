package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
)

// validationService validates inputs before delegating to next. All other
// methods auto-delegate via BaseService.
type validationService struct {
	BaseService
}

// NewValidationService wraps next with input validation. Returns
// JobService so the chain composes uniformly.
func NewValidationService(next JobService) JobService {
	return &validationService{BaseService: NewBaseService(next)}
}

// validJobStatuses are the per-user status values clients may pass through
// SetStatus or as a List filter. "new" is allowed as a filter value (it
// means "no row"); SetStatus rejects "new" because it isn't a writable
// state.
var validJobStatuses = map[string]bool{
	jobcommon.JobStatusNew:          true,
	jobcommon.JobStatusSaved:        true,
	jobcommon.JobStatusApplied:      true,
	jobcommon.JobStatusDismissed:    true,
	jobcommon.JobStatusInterviewing: true,
	jobcommon.JobStatusOffer:        true,
	jobcommon.JobStatusRejected:     true,
}

var validDismissalReasons = map[string]bool{
	jobcommon.DismissalReasonUserDismissed: true,
	jobcommon.DismissalReasonNotRelevant:   true,
}

// List validates filter and pagination before delegating.
func (v *validationService) List(ctx context.Context, filter dto.JobFilter, cursor string, limit int) (dto.PageResult, error) {
	var errs errors.ValidationErrors
	if filter.Status != "" && !validJobStatuses[filter.Status] {
		errs = append(errs, errors.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be one of: new, saved, applied, interviewing, offer, dismissed, rejected",
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
	if limit < 0 || limit > 100 {
		errs = append(errs, errors.ValidationError{
			Field:   "limit",
			Code:    "INVALID_VALUE",
			Message: "limit must be between 0 and 100",
		})
	}
	if len(errs) > 0 {
		return dto.PageResult{}, errs
	}
	return v.next.List(ctx, filter, cursor, limit)
}

// Dismiss validates the reason before delegating.
func (v *validationService) Dismiss(ctx context.Context, id string, reason, note string) error {
	if !validDismissalReasons[reason] {
		return errors.ValidationErrors{{
			Field:   "reason",
			Code:    "INVALID_VALUE",
			Message: "reason must be 'user_dismissed' or 'user_not_relevant'",
		}}
	}
	return v.next.Dismiss(ctx, id, reason, note)
}

// SetStatus validates the status value before delegating. "new" is
// rejected because it is not a writable status (delete the row instead).
func (v *validationService) SetStatus(ctx context.Context, id, status string) error {
	if status == jobcommon.JobStatusNew || !validJobStatuses[status] {
		return errors.ValidationErrors{{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be one of: saved, applied, interviewing, offer, dismissed, rejected",
		}}
	}
	return v.next.SetStatus(ctx, id, status)
}

// BulkDismiss validates the IDs slice and reason before delegating.
func (v *validationService) BulkDismiss(ctx context.Context, ids []string, reason, note string) error {
	var errs errors.ValidationErrors
	if len(ids) == 0 {
		errs = append(errs, errors.ValidationError{
			Field:   "ids",
			Code:    "REQUIRED",
			Message: "ids must not be empty",
		})
	}
	if len(ids) > 100 {
		errs = append(errs, errors.ValidationError{
			Field:   "ids",
			Code:    "INVALID_VALUE",
			Message: "ids must contain 100 or fewer entries",
		})
	}
	if reason != "" && !validDismissalReasons[reason] {
		errs = append(errs, errors.ValidationError{
			Field:   "reason",
			Code:    "INVALID_VALUE",
			Message: "reason must be 'user_dismissed' or 'user_not_relevant'",
		})
	}
	if len(errs) > 0 {
		return errs
	}
	return v.next.BulkDismiss(ctx, ids, reason, note)
}
