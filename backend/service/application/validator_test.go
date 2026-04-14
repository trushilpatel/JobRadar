package application

import (
	"testing"

	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/model"
)

func TestValidateCreate(t *testing.T) {
	v := NewValidator()

	validStages := []string{
		model.StageApplied, model.StagePhoneScreen, model.StageTechnical,
		model.StageFinalRound, model.StageOffer, model.StageRejected,
	}
	for _, stage := range validStages {
		if err := v.ValidateCreate(CreateApplicationInput{Stage: stage}); err != nil {
			t.Errorf("unexpected error for stage %q: %v", stage, err)
		}
	}
	if err := v.ValidateCreate(CreateApplicationInput{Stage: "invalid"}); err == nil {
		t.Error("expected error for invalid stage")
	}
	if err := v.ValidateCreate(CreateApplicationInput{Stage: ""}); err == nil {
		t.Error("expected error for empty stage")
	}
}

func TestValidateUpdateStage(t *testing.T) {
	v := NewValidator()

	if err := v.ValidateUpdateStage(UpdateStageInput{Stage: model.StageOffer}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.ValidateUpdateStage(UpdateStageInput{Stage: "bad"}); err == nil {
		t.Error("expected error for invalid stage")
	}
}

func TestValidateUpdateNote(t *testing.T) {
	v := NewValidator()

	if err := v.ValidateUpdateNote(UpdateNoteInput{Note: "great"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.ValidateUpdateNote(UpdateNoteInput{Note: ""}); err == nil {
		t.Error("expected error for empty note")
	}
}

func TestValidateListAll(t *testing.T) {
	v := NewValidator()

	if err := v.ValidateListAll(request.Pagination{Page: 1, Limit: 20}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.ValidateListAll(request.Pagination{Page: 0, Limit: 20}); err == nil {
		t.Error("expected error for page 0")
	}
	if err := v.ValidateListAll(request.Pagination{Page: 1, Limit: 0}); err == nil {
		t.Error("expected error for limit 0")
	}
	if err := v.ValidateListAll(request.Pagination{Page: 1, Limit: 101}); err == nil {
		t.Error("expected error for limit 101")
	}
}
