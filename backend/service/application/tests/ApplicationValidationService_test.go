package application

import (
	"testing"

	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	svc "github.com/trushilpatel/jobradar/service/application/services"
)

func TestValidateCreate(t *testing.T) {
	v := svc.NewApplicationValidationService()

	validStages := []string{
		appcommon.StageApplied, appcommon.StagePhoneScreen, appcommon.StageTechnical,
		appcommon.StageFinalRound, appcommon.StageOffer, appcommon.StageRejected,
	}
	for _, stage := range validStages {
		if err := v.ValidateCreate(dto.CreateApplicationInput{Stage: stage}); err != nil {
			t.Errorf("unexpected error for stage %q: %v", stage, err)
		}
	}
	if err := v.ValidateCreate(dto.CreateApplicationInput{Stage: "invalid"}); err == nil {
		t.Error("expected error for invalid stage")
	}
	if err := v.ValidateCreate(dto.CreateApplicationInput{Stage: ""}); err == nil {
		t.Error("expected error for empty stage")
	}
}

func TestValidateUpdateStage(t *testing.T) {
	v := svc.NewApplicationValidationService()

	if err := v.ValidateUpdateStage(dto.UpdateStageInput{Stage: appcommon.StageOffer}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.ValidateUpdateStage(dto.UpdateStageInput{Stage: "bad"}); err == nil {
		t.Error("expected error for invalid stage")
	}
}

func TestValidateUpdateNote(t *testing.T) {
	v := svc.NewApplicationValidationService()

	if err := v.ValidateUpdateNote(dto.UpdateNoteInput{Note: "great"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.ValidateUpdateNote(dto.UpdateNoteInput{Note: ""}); err == nil {
		t.Error("expected error for empty note")
	}
}

func TestValidateListAll(t *testing.T) {
	v := svc.NewApplicationValidationService()

	if err := v.ValidateListAll(1, 20); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.ValidateListAll(0, 20); err == nil {
		t.Error("expected error for page 0")
	}
	if err := v.ValidateListAll(1, 0); err == nil {
		t.Error("expected error for limit 0")
	}
	if err := v.ValidateListAll(1, 101); err == nil {
		t.Error("expected error for limit 101")
	}
}
