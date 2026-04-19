package services

import (
	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/service/application/dto"
)

type validationService struct{}

// NewApplicationValidationService constructs an ApplicationValidationService.
func NewApplicationValidationService() ApplicationValidationService {
	return &validationService{}
}

var validStages = map[string]bool{
	model.StageApplied:     true,
	model.StagePhoneScreen: true,
	model.StageTechnical:   true,
	model.StageFinalRound:  true,
	model.StageOffer:       true,
	model.StageRejected:    true,
}

// ValidateCreate validates the stage value for application creation.
func (v *validationService) ValidateCreate(input dto.CreateApplicationInput) error {
	if !validStages[input.Stage] {
		return &domain.ValidationError{
			Field:   "stage",
			Code:    "INVALID_VALUE",
			Message: "stage must be a valid application stage",
		}
	}
	return nil
}

// ValidateUpdateStage validates that the stage is a valid value.
func (v *validationService) ValidateUpdateStage(input dto.UpdateStageInput) error {
	if !validStages[input.Stage] {
		return &domain.ValidationError{
			Field:   "stage",
			Code:    "INVALID_VALUE",
			Message: "stage must be a valid application stage",
		}
	}
	return nil
}

// ValidateUpdateNote validates that the note is non-empty.
func (v *validationService) ValidateUpdateNote(input dto.UpdateNoteInput) error {
	if input.Note == "" {
		return &domain.ValidationError{
			Field:   "note",
			Code:    "REQUIRED",
			Message: "note is required",
		}
	}
	return nil
}

// ValidateListAll validates pagination parameters.
func (v *validationService) ValidateListAll(page, limit int) error {
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
