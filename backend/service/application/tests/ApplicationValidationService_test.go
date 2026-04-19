package application

import (
	"context"
	"testing"

	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	svc "github.com/trushilpatel/jobradar/service/application/services"
)

// noopService satisfies ApplicationService and passes all calls through without side effects.
type noopService struct{}

func (n *noopService) ListByJob(_ string) ([]appmodel.JobApplication, error) {
	return nil, nil
}
func (n *noopService) ListAll(_, _ int) ([]svc.ApplicationWithJob, int, error) {
	return nil, 0, nil
}
func (n *noopService) Create(_ context.Context, _ string, _ dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	return &appmodel.JobApplication{}, nil
}
func (n *noopService) UpdateStage(_, _ string, _ dto.UpdateStageInput) error { return nil }
func (n *noopService) UpdateNote(_, _ string, _ dto.UpdateNoteInput) error   { return nil }

func newValidation() svc.ApplicationService {
	return svc.NewApplicationValidationService(&noopService{})
}

func TestValidateCreate(t *testing.T) {
	v := newValidation()

	validStages := []string{
		appcommon.StageApplied, appcommon.StagePhoneScreen, appcommon.StageTechnical,
		appcommon.StageFinalRound, appcommon.StageOffer, appcommon.StageRejected,
	}
	for _, stage := range validStages {
		if _, err := v.Create(context.Background(), "j1", dto.CreateApplicationInput{Stage: stage}); err != nil {
			t.Errorf("unexpected error for stage %q: %v", stage, err)
		}
	}
	if _, err := v.Create(context.Background(), "j1", dto.CreateApplicationInput{Stage: "invalid"}); err == nil {
		t.Error("expected error for invalid stage")
	}
	if _, err := v.Create(context.Background(), "j1", dto.CreateApplicationInput{Stage: ""}); err == nil {
		t.Error("expected error for empty stage")
	}
}

func TestValidateUpdateStage(t *testing.T) {
	v := newValidation()

	if err := v.UpdateStage("j1", "1", dto.UpdateStageInput{Stage: appcommon.StageOffer}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.UpdateStage("j1", "1", dto.UpdateStageInput{Stage: "bad"}); err == nil {
		t.Error("expected error for invalid stage")
	}
}

func TestValidateUpdateNote(t *testing.T) {
	v := newValidation()

	if err := v.UpdateNote("j1", "1", dto.UpdateNoteInput{Note: "great"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.UpdateNote("j1", "1", dto.UpdateNoteInput{Note: ""}); err == nil {
		t.Error("expected error for empty note")
	}
}

func TestValidateListAll(t *testing.T) {
	v := newValidation()

	if _, _, err := v.ListAll(1, 20); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if _, _, err := v.ListAll(0, 20); err == nil {
		t.Error("expected error for page 0")
	}
	if _, _, err := v.ListAll(1, 0); err == nil {
		t.Error("expected error for limit 0")
	}
	if _, _, err := v.ListAll(1, 101); err == nil {
		t.Error("expected error for limit 101")
	}
}

// Ensure noopService implements the interface.
var _ svc.ApplicationService = (*noopService)(nil)
