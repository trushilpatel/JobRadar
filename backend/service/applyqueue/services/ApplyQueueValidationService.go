package services

import (
	"context"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	applyqueuecommon "github.com/trushilpatel/jobradar/service/applyqueue/common"
	"github.com/trushilpatel/jobradar/service/applyqueue/dto"
)

// validFilters whitelists the ?filter= query param — same defense-in-depth
// rationale as JobRestService's sort_by whitelist: prevents an unrecognised
// value from ever reaching the repo's SQL branch.
var validFilters = map[string]bool{
	applyqueuecommon.FilterAll:   true,
	applyqueuecommon.FilterSaved: true,
}

// validationService is the input-validation layer of the apply-queue chain.
// It depends on the two narrow checker interfaces (not the full Service) so
// its DB-backed existence/ownership checks are visible in its constructor
// signature — the same pattern CompanyAdminValidationService uses for its
// atsSvc dependency.
type validationService struct {
	BaseService
	engagement EngagementChecker
	resumes    ResumeOwnershipChecker
}

// NewValidationService constructs the validation layer.
func NewValidationService(next Service, engagement EngagementChecker, resumes ResumeOwnershipChecker) Service {
	return &validationService{
		BaseService: NewBaseService(next),
		engagement:  engagement,
		resumes:     resumes,
	}
}

// List defaults an empty filter to FilterAll and rejects any other
// unrecognised value.
func (v *validationService) List(ctx context.Context, userID, filter, cursor string, limit int) (dto.QueueListResult, error) {
	if filter == "" {
		filter = applyqueuecommon.FilterAll
	}
	if !validFilters[filter] {
		return dto.QueueListResult{}, domainerrs.ValidationErrors{{
			Field:   "filter",
			Code:    "INVALID_VALUE",
			Message: "filter must be \"all\" or \"saved\"",
		}}
	}
	return v.next.List(ctx, userID, filter, cursor, limit)
}

// SetResume rejects:
//   - an empty jobID,
//   - a jobID with no user_job_state row for this user (can't attach a
//     resume to a job you haven't engaged with) — a validation error, not a 500,
//   - a resumeID that doesn't belong to userID — ErrNotFound (404), matching
//     ResumeRepositoryService.Get's precedent of returning NotFound for both
//     "doesn't exist" and "belongs to someone else" so ownership is never
//     leaked via a different status code.
func (v *validationService) SetResume(ctx context.Context, userID, jobID string, resumeID *string) (*dto.AttachmentView, error) {
	if jobID == "" {
		return nil, domainerrs.ValidationErrors{{Field: "job_id", Code: "REQUIRED", Message: "job id is required"}}
	}

	has, err := v.engagement.HasEngagement(ctx, userID, jobID)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, domainerrs.ValidationErrors{{
			Field:   "job_id",
			Code:    "NO_ENGAGEMENT",
			Message: "resume cannot be attached to a job you have not saved or applied to",
		}}
	}

	if resumeID != nil {
		if *resumeID == "" {
			return nil, domainerrs.ValidationErrors{{Field: "resume_id", Code: "INVALID_VALUE", Message: "resume_id must not be empty"}}
		}
		owned, err := v.resumes.ResumeOwnedByUser(ctx, userID, *resumeID)
		if err != nil {
			return nil, err
		}
		if !owned {
			return nil, domainerrs.ErrNotFound.WithMessage("applyqueue validation: resume not found for user")
		}
	}

	return v.next.SetResume(ctx, userID, jobID, resumeID)
}

var _ Service = (*validationService)(nil)
