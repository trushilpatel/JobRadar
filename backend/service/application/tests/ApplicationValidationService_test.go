package application

import (
	"context"
	"strings"
	"testing"

	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	svc "github.com/trushilpatel/jobradar/service/application/services"
)

const validJobUUID = "11111111-1111-1111-1111-111111111111"

// noopService satisfies ApplicationService with passthrough no-ops so the
// validation layer can be exercised without a backing impl.
type noopService struct{}

func (n *noopService) ListByJob(_ context.Context, _ string) ([]appmodel.JobApplication, error) {
	return nil, nil
}
func (n *noopService) ListAll(_ context.Context, _, _ int) ([]svc.ApplicationWithJob, int, error) {
	return nil, 0, nil
}
func (n *noopService) ListActive(_ context.Context) ([]svc.ApplicationWithJob, error) {
	return nil, nil
}
func (n *noopService) Create(_ context.Context, _ string, _ dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	return &appmodel.JobApplication{}, nil
}
func (n *noopService) UpdateStage(_ context.Context, _, _ string, _ dto.UpdateStageInput) error {
	return nil
}
func (n *noopService) UpdateStageByID(_ context.Context, _ string, _ dto.UpdateStageInput) error {
	return nil
}
func (n *noopService) UpdateNote(_ context.Context, _, _ string, _ dto.UpdateNoteInput) error {
	return nil
}

func newValidation() svc.ApplicationService {
	return svc.NewApplicationValidationService(&noopService{})
}

func TestValidateCreate(t *testing.T) {
	v := newValidation()
	ctx := context.Background()

	validStages := []string{
		appcommon.StageApplied, appcommon.StagePhoneScreen, appcommon.StageTechnical,
		appcommon.StageFinalRound, appcommon.StageOffer, appcommon.StageRejected,
	}
	for _, stage := range validStages {
		if _, err := v.Create(ctx, validJobUUID, dto.CreateApplicationInput{Stage: stage}); err != nil {
			t.Errorf("unexpected error for stage %q: %v", stage, err)
		}
	}
	if _, err := v.Create(ctx, validJobUUID, dto.CreateApplicationInput{Stage: "invalid"}); err == nil {
		t.Error("expected error for invalid stage")
	}
	if _, err := v.Create(ctx, validJobUUID, dto.CreateApplicationInput{Stage: ""}); err == nil {
		t.Error("expected error for empty stage")
	}
	if _, err := v.Create(ctx, "not-a-uuid", dto.CreateApplicationInput{Stage: appcommon.StageApplied}); err == nil {
		t.Error("expected error for non-UUID jobID")
	}
	bigNote := strings.Repeat("x", 4001)
	if _, err := v.Create(ctx, validJobUUID, dto.CreateApplicationInput{Stage: appcommon.StageApplied, Note: bigNote}); err == nil {
		t.Error("expected error for over-length note")
	}
}

func TestValidateUpdateStage(t *testing.T) {
	v := newValidation()
	ctx := context.Background()

	if err := v.UpdateStage(ctx, validJobUUID, "1", dto.UpdateStageInput{Stage: appcommon.StageOffer}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.UpdateStage(ctx, validJobUUID, "1", dto.UpdateStageInput{Stage: "bad"}); err == nil {
		t.Error("expected error for invalid stage")
	}
	if err := v.UpdateStage(ctx, "not-a-uuid", "1", dto.UpdateStageInput{Stage: appcommon.StageOffer}); err == nil {
		t.Error("expected error for non-UUID jobID")
	}
}

func TestValidateUpdateStageByID(t *testing.T) {
	v := newValidation()
	ctx := context.Background()

	if err := v.UpdateStageByID(ctx, "1", dto.UpdateStageInput{Stage: appcommon.StageOffer}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.UpdateStageByID(ctx, "1", dto.UpdateStageInput{Stage: "bad"}); err == nil {
		t.Error("expected error for invalid stage")
	}
}

func TestValidateUpdateNote(t *testing.T) {
	v := newValidation()
	ctx := context.Background()

	if err := v.UpdateNote(ctx, validJobUUID, "1", dto.UpdateNoteInput{Note: "great"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.UpdateNote(ctx, validJobUUID, "1", dto.UpdateNoteInput{Note: ""}); err == nil {
		t.Error("expected error for empty note")
	}
	if err := v.UpdateNote(ctx, "not-a-uuid", "1", dto.UpdateNoteInput{Note: "x"}); err == nil {
		t.Error("expected error for non-UUID jobID")
	}
	bigNote := strings.Repeat("x", 4001)
	if err := v.UpdateNote(ctx, validJobUUID, "1", dto.UpdateNoteInput{Note: bigNote}); err == nil {
		t.Error("expected error for over-length note")
	}
}

func TestValidateListAll(t *testing.T) {
	v := newValidation()
	ctx := context.Background()

	if _, _, err := v.ListAll(ctx, 1, 20); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if _, _, err := v.ListAll(ctx, 0, 20); err == nil {
		t.Error("expected error for page 0")
	}
	if _, _, err := v.ListAll(ctx, 1, 0); err == nil {
		t.Error("expected error for limit 0")
	}
	if _, _, err := v.ListAll(ctx, 1, 101); err == nil {
		t.Error("expected error for limit 101")
	}
}

func TestValidateListByJob(t *testing.T) {
	v := newValidation()
	ctx := context.Background()

	if _, err := v.ListByJob(ctx, validJobUUID); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if _, err := v.ListByJob(ctx, "not-a-uuid"); err == nil {
		t.Error("expected error for non-UUID jobID")
	}
}

// Ensure noopService implements the interface.
var _ svc.ApplicationService = (*noopService)(nil)
