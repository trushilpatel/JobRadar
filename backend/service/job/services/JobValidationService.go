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
// state. "dismissed" is intentionally excluded — the dismiss feature has
// been removed; existing dismissed rows remain in the DB (see migration
// 091) but are no longer reachable via any API surface.
var validJobStatuses = map[string]bool{
	jobcommon.JobStatusNew:          true,
	jobcommon.JobStatusSaved:        true,
	jobcommon.JobStatusApplied:      true,
	jobcommon.JobStatusInterviewing: true,
	jobcommon.JobStatusOffer:        true,
	jobcommon.JobStatusRejected:     true,
}

// List validates filter and pagination before delegating.
func (v *validationService) List(ctx context.Context, filter dto.JobFilter, cursor string, limit int) (dto.PageResult, error) {
	var errs errors.ValidationErrors
	if filter.Status != "" && !validJobStatuses[filter.Status] {
		errs = append(errs, errors.ValidationError{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be one of: new, saved, applied, interviewing, offer, rejected",
		})
	}
	for _, st := range filter.Statuses {
		if !validJobStatuses[st] {
			errs = append(errs, errors.ValidationError{
				Field:   "statuses",
				Code:    "INVALID_VALUE",
				Message: "each status must be one of: new, saved, applied, interviewing, offer, rejected",
			})
			break
		}
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

// SetNotes validates the notes length before delegating.
func (v *validationService) SetNotes(ctx context.Context, id, notes string) error {
	if len(notes) > 5000 {
		return errors.ValidationErrors{{
			Field:   "notes",
			Code:    "INVALID_VALUE",
			Message: "notes must be 5000 characters or fewer",
		}}
	}
	return v.next.SetNotes(ctx, id, notes)
}

// SetStatus validates the status value before delegating. "new" is allowed —
// UserJobStateRepositoryService.Upsert interprets it as clearing the
// workflow status (status = NULL), leaving saved_at untouched; it is the
// documented revert-to-new path, not merely a list-filter synonym. "saved"
// is rejected because saved is orthogonal to workflow status and must go
// through the dedicated SetSaved path.
func (v *validationService) SetStatus(ctx context.Context, id, status string) error {
	if status == jobcommon.JobStatusSaved {
		return errors.ValidationErrors{{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "'saved' is not a workflow status; use the save endpoint",
		}}
	}
	if !validJobStatuses[status] {
		return errors.ValidationErrors{{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "status must be one of: new, applied, interviewing, offer, rejected",
		}}
	}
	return v.next.SetStatus(ctx, id, status)
}
