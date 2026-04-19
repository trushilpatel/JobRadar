package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/domain"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
)

type validationService struct {
	BaseService
}

// NewApplicationValidationService constructs the input-validation layer of the application chain.
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

// Create validates the stage then delegates.
func (v *validationService) Create(ctx context.Context, jobID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	if !validStages[input.Stage] {
		return nil, &domain.ValidationError{
			Field:   "stage",
			Code:    "INVALID_VALUE",
			Message: "stage must be a valid application stage",
		}
	}
	return v.next.Create(ctx, jobID, input)
}

// UpdateStage validates the stage then delegates.
func (v *validationService) UpdateStage(jobID, appID string, input dto.UpdateStageInput) error {
	if !validStages[input.Stage] {
		return &domain.ValidationError{
			Field:   "stage",
			Code:    "INVALID_VALUE",
			Message: "stage must be a valid application stage",
		}
	}
	return v.next.UpdateStage(jobID, appID, input)
}

// UpdateNote validates the note is non-empty then delegates.
func (v *validationService) UpdateNote(jobID, appID string, input dto.UpdateNoteInput) error {
	if input.Note == "" {
		return &domain.ValidationError{
			Field:   "note",
			Code:    "REQUIRED",
			Message: "note is required",
		}
	}
	return v.next.UpdateNote(jobID, appID, input)
}

// ListAll validates pagination parameters then delegates.
func (v *validationService) ListAll(page, limit int) ([]ApplicationWithJob, int, error) {
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
	return v.next.ListAll(page, limit)
}
