package application

import (
	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/api/response"
	"github.com/trushilpatel/jobradar/model"
)

// Validator validates user-submitted input for the application domain.
type Validator interface {
	ValidateCreate(input CreateApplicationInput) error
	ValidateUpdateStage(input UpdateStageInput) error
	ValidateUpdateNote(input UpdateNoteInput) error
	ValidateListAll(p request.Pagination) error
}

type validator struct{}

// NewValidator constructs a Validator.
func NewValidator() Validator {
	return &validator{}
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
func (v *validator) ValidateCreate(input CreateApplicationInput) error {
	if !validStages[input.Stage] {
		return &response.ValidationError{
			Field:   "stage",
			Code:    "INVALID_VALUE",
			Message: "stage must be a valid application stage",
		}
	}
	return nil
}

// ValidateUpdateStage validates that the stage is a valid value.
func (v *validator) ValidateUpdateStage(input UpdateStageInput) error {
	if !validStages[input.Stage] {
		return &response.ValidationError{
			Field:   "stage",
			Code:    "INVALID_VALUE",
			Message: "stage must be a valid application stage",
		}
	}
	return nil
}

// ValidateUpdateNote validates that the note is non-empty.
func (v *validator) ValidateUpdateNote(input UpdateNoteInput) error {
	if input.Note == "" {
		return &response.ValidationError{
			Field:   "note",
			Code:    "REQUIRED",
			Message: "note is required",
		}
	}
	return nil
}

// ValidateListAll validates pagination parameters.
func (v *validator) ValidateListAll(p request.Pagination) error {
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
