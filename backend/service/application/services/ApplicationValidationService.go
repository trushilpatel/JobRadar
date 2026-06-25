package services

import (
	"context"

	"github.com/google/uuid"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
)

var _ ApplicationService = (*validationService)(nil)

// maxNoteLength caps the persisted application note. Larger values are
// rejected up front rather than relying on DB-level truncation.
const maxNoteLength = 4000

type validationService struct {
	BaseService
}

// NewApplicationValidationService constructs the input-validation layer of
// the application chain. It overrides only methods that have validation
// rules; everything else passes through BaseService.
func NewApplicationValidationService(next ApplicationService) ApplicationService {
	return &validationService{BaseService: NewBaseService(next)}
}

var validStages = map[string]bool{
	appcommon.StageApplied:     true,
	appcommon.StagePhoneScreen: true,
	appcommon.StageTechnical:   true,
	appcommon.StageFinalRound:  true,
	appcommon.StageOffer:       true,
	appcommon.StageRejected:    true,
}

// ListByJob validates the jobID is a UUID then delegates.
func (v *validationService) ListByJob(ctx context.Context, jobID string) ([]appmodel.JobApplication, error) {
	if errs := validateJobID(jobID); len(errs) > 0 {
		return nil, errs
	}
	return v.next.ListByJob(ctx, jobID)
}

// Create validates jobID, stage and note length then delegates.
func (v *validationService) Create(ctx context.Context, jobID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	var errs apperr.ValidationErrors
	errs = append(errs, validateJobID(jobID)...)
	errs = append(errs, validateStage(input.Stage)...)
	errs = append(errs, validateNote(input.Note)...)
	if len(errs) > 0 {
		return nil, errs
	}
	return v.next.Create(ctx, jobID, input)
}

// UpdateStage validates jobID and stage then delegates.
func (v *validationService) UpdateStage(ctx context.Context, jobID, appID string, input dto.UpdateStageInput) error {
	var errs apperr.ValidationErrors
	errs = append(errs, validateJobID(jobID)...)
	errs = append(errs, validateStage(input.Stage)...)
	if len(errs) > 0 {
		return errs
	}
	return v.next.UpdateStage(ctx, jobID, appID, input)
}

// UpdateStageByID validates the stage then delegates.
func (v *validationService) UpdateStageByID(ctx context.Context, appID string, input dto.UpdateStageInput) error {
	if errs := validateStage(input.Stage); len(errs) > 0 {
		return errs
	}
	return v.next.UpdateStageByID(ctx, appID, input)
}

// UpdateNote validates the note is non-empty + within length, then delegates.
func (v *validationService) UpdateNote(ctx context.Context, jobID, appID string, input dto.UpdateNoteInput) error {
	var errs apperr.ValidationErrors
	errs = append(errs, validateJobID(jobID)...)
	if input.Note == "" {
		errs = append(errs, apperr.ValidationError{
			Field:   "note",
			Code:    "REQUIRED",
			Message: "note is required",
		})
	} else {
		errs = append(errs, validateNote(input.Note)...)
	}
	if len(errs) > 0 {
		return errs
	}
	return v.next.UpdateNote(ctx, jobID, appID, input)
}

// ListAll validates pagination parameters then delegates.
func (v *validationService) ListAll(ctx context.Context, page, limit int) ([]ApplicationWithJob, int, error) {
	var errs apperr.ValidationErrors
	if page < 1 {
		errs = append(errs, apperr.ValidationError{
			Field:   "page",
			Code:    "INVALID_VALUE",
			Message: "page must be >= 1",
		})
	}
	if limit < 1 || limit > 100 {
		errs = append(errs, apperr.ValidationError{
			Field:   "limit",
			Code:    "INVALID_VALUE",
			Message: "limit must be between 1 and 100",
		})
	}
	if len(errs) > 0 {
		return nil, 0, errs
	}
	return v.next.ListAll(ctx, page, limit)
}

func validateJobID(jobID string) apperr.ValidationErrors {
	if _, err := uuid.Parse(jobID); err != nil {
		return apperr.ValidationErrors{{
			Field:   "job_id",
			Code:    "INVALID_VALUE",
			Message: "job_id must be a valid UUID",
		}}
	}
	return nil
}

func validateStage(stage string) apperr.ValidationErrors {
	if !validStages[stage] {
		return apperr.ValidationErrors{{
			Field:   "stage",
			Code:    "INVALID_VALUE",
			Message: "stage must be a valid application stage",
		}}
	}
	return nil
}

func validateNote(note string) apperr.ValidationErrors {
	if len(note) > maxNoteLength {
		return apperr.ValidationErrors{{
			Field:   "note",
			Code:    "INVALID_VALUE",
			Message: "note exceeds maximum length",
		}}
	}
	return nil
}
