package application

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
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

// TestValidateListByJob_EmptyID — empty jobID is not a valid UUID and must
// be rejected before the impl/repo layer is invoked.
func TestValidateListByJob_EmptyID(t *testing.T) {
	v := newValidation()
	if _, err := v.ListByJob(context.Background(), ""); err == nil {
		t.Error("expected error for empty jobID")
	}
}

// TestValidateCreate_ValidationErrorShape — validation failures must surface
// as apperr.ValidationErrors so HandleError can render the 400 list response.
func TestValidateCreate_ValidationErrorShape(t *testing.T) {
	v := newValidation()
	_, err := v.Create(context.Background(), "not-a-uuid", dto.CreateApplicationInput{Stage: "bogus"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var verrs apperr.ValidationErrors
	if !errorsAs(err, &verrs) {
		t.Fatalf("expected apperr.ValidationErrors, got %T (%v)", err, err)
	}
	// Should report BOTH the bad jobID and the bad stage.
	if len(verrs) < 2 {
		t.Errorf("expected at least 2 validation errors, got %d (%v)", len(verrs), verrs)
	}
}

// TestValidateUpdateNote_AggregatesErrors — bad jobID + empty note must both
// be reported. An empty note is "REQUIRED", not "INVALID_VALUE".
func TestValidateUpdateNote_AggregatesErrors(t *testing.T) {
	v := newValidation()
	err := v.UpdateNote(context.Background(), "not-a-uuid", "1", dto.UpdateNoteInput{Note: ""})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var verrs apperr.ValidationErrors
	if !errorsAs(err, &verrs) {
		t.Fatalf("expected apperr.ValidationErrors, got %T", err)
	}
	if len(verrs) < 2 {
		t.Fatalf("expected at least 2 validation errors, got %d", len(verrs))
	}
	foundRequired := false
	for _, ve := range verrs {
		if ve.Field == "note" && ve.Code == "REQUIRED" {
			foundRequired = true
		}
	}
	if !foundRequired {
		t.Errorf("expected note=REQUIRED error in %v", verrs)
	}
}

// TestValidateListAll_NegativeAndOverLimit ensures the validator catches
// both halves of the limit range and the page lower bound.
func TestValidateListAll_NegativeAndOverLimit(t *testing.T) {
	v := newValidation()
	if _, _, err := v.ListAll(context.Background(), -1, 20); err == nil {
		t.Error("expected error for negative page")
	}
	if _, _, err := v.ListAll(context.Background(), 1, -5); err == nil {
		t.Error("expected error for negative limit")
	}
	if _, _, err := v.ListAll(context.Background(), 1, 100); err != nil {
		t.Errorf("limit=100 must be allowed (inclusive upper bound), got %v", err)
	}
}

// TestValidateListActive_Passthrough — ListActive has no validation rules,
// so BaseService delegation must forward the call through unchanged.
func TestValidateListActive_Passthrough(t *testing.T) {
	v := newValidation()
	if _, err := v.ListActive(context.Background()); err != nil {
		t.Errorf("ListActive must pass through; got %v", err)
	}
}

// TestValidateCreate_NoteAtMaxLength — note of exactly 4000 chars is allowed.
func TestValidateCreate_NoteAtMaxLength(t *testing.T) {
	v := newValidation()
	noteAtMax := strings.Repeat("y", 4000)
	if _, err := v.Create(context.Background(), validJobUUID, dto.CreateApplicationInput{Stage: appcommon.StageApplied, Note: noteAtMax}); err != nil {
		t.Errorf("note at exactly 4000 chars must be allowed, got %v", err)
	}
}

// TestValidateUpdateStageByID_AllStages — every constant stage is accepted.
func TestValidateUpdateStageByID_AllStages(t *testing.T) {
	v := newValidation()
	for _, stage := range []string{
		appcommon.StageApplied, appcommon.StagePhoneScreen, appcommon.StageTechnical,
		appcommon.StageFinalRound, appcommon.StageOffer, appcommon.StageRejected,
	} {
		if err := v.UpdateStageByID(context.Background(), "1", dto.UpdateStageInput{Stage: stage}); err != nil {
			t.Errorf("stage %q: unexpected error %v", stage, err)
		}
	}
}

// errorsAs is a tiny helper so the test file does not need a fresh
// errors import block.
func errorsAs(err error, target interface{}) bool {
	return stderrors.As(err, target)
}

// Ensure noopService implements the interface.
var _ svc.ApplicationService = (*noopService)(nil)
